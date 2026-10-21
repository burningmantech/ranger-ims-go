//
// See the file COPYRIGHT for copyright information.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

//! Binds values to a query parsed from a sqlc query file, and runs it.

use std::collections::HashMap;

use sqlx::mysql::{MySqlArguments, MySqlQueryResult, MySqlRow};
use sqlx::{Arguments, FromRow, MySqlExecutor};

use super::sqlfile::{ParsedQuery, Segment};

/// A value bound to a query parameter.
#[derive(Debug, Clone, PartialEq)]
pub enum Val {
    I64(Option<i64>),
    F64(Option<f64>),
    Str(Option<String>),
    Bool(Option<bool>),
}

impl Val {
    fn add_to(self, args: &mut MySqlArguments) -> Result<(), sqlx::Error> {
        let result = match self {
            Val::I64(v) => args.add(v),
            Val::F64(v) => args.add(v),
            Val::Str(v) => args.add(v),
            Val::Bool(v) => args.add(v),
        };
        result.map_err(sqlx::Error::Encode)
    }
}

macro_rules! val_from {
    ($variant:ident, $inner:ty, $($t:ty),+) => {
        $(
            impl From<$t> for Val {
                fn from(v: $t) -> Self {
                    Val::$variant(Some(<$inner>::from(v)))
                }
            }
            impl From<Option<$t>> for Val {
                fn from(v: Option<$t>) -> Self {
                    Val::$variant(v.map(<$inner>::from))
                }
            }
        )+
    };
}

val_from!(I64, i64, i8, i16, i32, i64, u8, u16, u32);
val_from!(F64, f64, f32, f64);
val_from!(Bool, bool, bool);
val_from!(Str, String, String, &str);

impl From<&String> for Val {
    fn from(v: &String) -> Self {
        Val::Str(Some(v.clone()))
    }
}

impl From<Option<&String>> for Val {
    fn from(v: Option<&String>) -> Self {
        Val::Str(v.cloned())
    }
}

/// A query awaiting its parameters. Positional (`?`) values are given with
/// `arg`, in order; `sqlc.arg`/`sqlc.narg` values with `named`; and
/// `sqlc.slice` values with `slice`.
#[must_use]
pub struct Query {
    parsed: &'static ParsedQuery,
    positional: Vec<Val>,
    named: HashMap<&'static str, Val>,
    slices: HashMap<&'static str, Vec<Val>>,
}

impl Query {
    pub fn new(parsed: &'static ParsedQuery) -> Self {
        Query {
            parsed,
            positional: Vec::new(),
            named: HashMap::new(),
            slices: HashMap::new(),
        }
    }

    pub fn arg(mut self, v: impl Into<Val>) -> Self {
        self.positional.push(v.into());
        self
    }

    pub fn named(mut self, name: &'static str, v: impl Into<Val>) -> Self {
        self.named.insert(name, v.into());
        self
    }

    pub fn slice<V: Into<Val>>(
        mut self,
        name: &'static str,
        vs: impl IntoIterator<Item = V>,
    ) -> Self {
        self.slices
            .insert(name, vs.into_iter().map(Into::into).collect());
        self
    }

    /// Renders the final SQL and its arguments. A mismatch between the values
    /// given and the query's parameters is a programming error, so it panics.
    pub fn build(self) -> Result<(String, MySqlArguments), sqlx::Error> {
        let name = &self.parsed.name;
        let mut sql = String::new();
        let mut args = MySqlArguments::default();
        let mut positional = self.positional.into_iter();
        for seg in &self.parsed.segments {
            match seg {
                Segment::Text(t) => sql.push_str(t),
                Segment::Positional => {
                    let v = positional
                        .next()
                        .unwrap_or_else(|| panic!("query {name}: too few positional args"));
                    sql.push('?');
                    v.add_to(&mut args)?;
                }
                Segment::Named(n) => {
                    let v = self
                        .named
                        .get(n.as_str())
                        .unwrap_or_else(|| panic!("query {name}: missing named arg {n}"));
                    sql.push('?');
                    v.clone().add_to(&mut args)?;
                }
                Segment::Slice(n) => {
                    let vs = self
                        .slices
                        .get(n.as_str())
                        .unwrap_or_else(|| panic!("query {name}: missing slice arg {n}"));
                    if vs.is_empty() {
                        // What sqlc's generated Go does for an empty slice.
                        sql.push_str("NULL");
                    }
                    for (i, v) in vs.iter().enumerate() {
                        if i > 0 {
                            sql.push(',');
                        }
                        sql.push('?');
                        v.clone().add_to(&mut args)?;
                    }
                }
            }
        }
        assert!(
            positional.next().is_none(),
            "query {name}: too many positional args"
        );
        Ok((sql, args))
    }

    pub async fn fetch_all<'e, T, E>(self, db: E) -> Result<Vec<T>, sqlx::Error>
    where
        T: for<'r> FromRow<'r, MySqlRow> + Send + Unpin,
        E: MySqlExecutor<'e>,
    {
        let (sql, args) = self.build()?;
        sqlx::query_as_with::<_, T, _>(&sql, args)
            .fetch_all(db)
            .await
    }

    pub async fn fetch_optional<'e, T, E>(self, db: E) -> Result<Option<T>, sqlx::Error>
    where
        T: for<'r> FromRow<'r, MySqlRow> + Send + Unpin,
        E: MySqlExecutor<'e>,
    {
        let (sql, args) = self.build()?;
        sqlx::query_as_with::<_, T, _>(&sql, args)
            .fetch_optional(db)
            .await
    }

    /// For a sqlc `:one` query: no row is an error, as with sql.ErrNoRows.
    pub async fn fetch_one<'e, T, E>(self, db: E) -> Result<T, sqlx::Error>
    where
        T: for<'r> FromRow<'r, MySqlRow> + Send + Unpin,
        E: MySqlExecutor<'e>,
    {
        let (sql, args) = self.build()?;
        sqlx::query_as_with::<_, T, _>(&sql, args)
            .fetch_one(db)
            .await
    }

    pub async fn fetch_all_scalar<'e, T, E>(self, db: E) -> Result<Vec<T>, sqlx::Error>
    where
        (T,): for<'r> FromRow<'r, MySqlRow>,
        T: Send + Unpin,
        E: MySqlExecutor<'e>,
    {
        let (sql, args) = self.build()?;
        sqlx::query_scalar_with::<_, T, _>(&sql, args)
            .fetch_all(db)
            .await
    }

    pub async fn fetch_one_scalar<'e, T, E>(self, db: E) -> Result<T, sqlx::Error>
    where
        (T,): for<'r> FromRow<'r, MySqlRow>,
        T: Send + Unpin,
        E: MySqlExecutor<'e>,
    {
        let (sql, args) = self.build()?;
        sqlx::query_scalar_with::<_, T, _>(&sql, args)
            .fetch_one(db)
            .await
    }

    pub async fn execute<'e, E>(self, db: E) -> Result<MySqlQueryResult, sqlx::Error>
    where
        E: MySqlExecutor<'e>,
    {
        let (sql, args) = self.build()?;
        sqlx::query_with(&sql, args).execute(db).await
    }
}

#[cfg(test)]
mod tests {
    use std::sync::LazyLock;

    use super::*;
    use crate::store::sqlfile::parse_file;

    static QUERIES: LazyLock<HashMap<String, ParsedQuery>> = LazyLock::new(|| {
        parse_file(
            "-- name: Pos :one\nselect A from T where B = ? and C = ?;\n\
             -- name: Mixed :many\nselect A from T where B in (sqlc.slice(ids)) and C = ? and D = sqlc.arg(d) and E = sqlc.arg(d);\n",
        )
    });

    #[test]
    fn builds_positional() {
        let (sql, _) = Query::new(&QUERIES["Pos"]).arg(1).arg("x").build().unwrap();
        assert_eq!(sql, "select A from T where B = ? and C = ?");
    }

    #[test]
    fn builds_mixed_and_repeated_named() {
        let (sql, _) = Query::new(&QUERIES["Mixed"])
            .slice("ids", [1, 2, 3])
            .arg(true)
            .named("d", 4.5)
            .build()
            .unwrap();
        assert_eq!(
            sql,
            "select A from T where B in (?,?,?) and C = ? and D = ? and E = ?"
        );
    }

    #[test]
    fn builds_empty_slice_as_null() {
        let (sql, _) = Query::new(&QUERIES["Mixed"])
            .slice("ids", Vec::<i32>::new())
            .arg(true)
            .named("d", 4.5)
            .build()
            .unwrap();
        assert_eq!(
            sql,
            "select A from T where B in (NULL) and C = ? and D = ? and E = ?"
        );
    }

    #[test]
    #[should_panic(expected = "too few positional args")]
    fn panics_on_missing_positional() {
        let _ = Query::new(&QUERIES["Pos"]).arg(1).build();
    }

    #[test]
    #[should_panic(expected = "too many positional args")]
    fn panics_on_extra_positional() {
        let _ = Query::new(&QUERIES["Pos"]).arg(1).arg(2).arg(3).build();
    }
}
