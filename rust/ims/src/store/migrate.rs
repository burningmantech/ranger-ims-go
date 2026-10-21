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

//! Schema migrations, sharing the Go server's SQL files in `store/schema/`.

use include_dir::{Dir, include_dir};
use sqlx::MySqlPool;

use super::queries;

pub const CURRENT_SCHEMA: &str = include_str!("../../../../store/schema/current.sql");

static MIGRATIONS: Dir<'_> = include_dir!("$CARGO_MANIFEST_DIR/../../store/schema");

/// The schema version that `current.sql` sets up.
pub fn repo_schema_version() -> Result<i16, String> {
    let insert = "insert into SCHEMA_INFO (VERSION) values (";
    let (_, after_insert) = CURRENT_SCHEMA
        .split_once(insert)
        .ok_or("couldn't find SCHEMA_INFO insert in current.sql")?;
    let (version, _) = after_insert
        .split_once(')')
        .ok_or("couldn't find `)` after SCHEMA_INFO insert in current.sql")?;
    version
        .trim()
        .parse()
        .map_err(|e| format!("bad schema version: {e}"))
}

/// The schema version of the database, or 0 for a database with no schema.
pub async fn db_schema_version(pool: &MySqlPool) -> Result<i16, sqlx::Error> {
    const TABLE_UNKNOWN_ERROR: u16 = 1146;
    match queries::schema_version(pool).await {
        Ok(v) => Ok(v),
        Err(sqlx::Error::Database(e))
            if e.try_downcast_ref::<sqlx::mysql::MySqlDatabaseError>()
                .is_some_and(|e| e.number() == TABLE_UNKNOWN_ERROR) =>
        {
            tracing::info!("No SCHEMA_INFO table found. This must be a new database.");
            Ok(0)
        }
        Err(e) => Err(e),
    }
}

fn migration_script(step: i16) -> Result<&'static str, String> {
    let name = format!("{step:02}-from-{:02}.sql", step - 1);
    MIGRATIONS
        .get_file(&name)
        .and_then(|f| f.contents_utf8())
        .ok_or_else(|| format!("missing migration file {name}"))
}

async fn migrate(pool: &MySqlPool, to: i16, from: i16) -> Result<(), String> {
    if from == 0 {
        sqlx::raw_sql(CURRENT_SCHEMA)
            .execute(pool)
            .await
            .map_err(|e| format!("[runScript]: {e}"))?;
        tracing::info!(to, from, "Migrated schema version");
        return Ok(());
    }
    for step in from + 1..=to {
        sqlx::raw_sql(migration_script(step)?)
            .execute(pool)
            .await
            .map_err(|e| format!("[runScript] {step}: {e}"))?;
        tracing::info!(to = step, from = step - 1, "Migrated schema version");
    }
    Ok(())
}

pub async fn migrate_db(pool: &MySqlPool) -> Result<(), String> {
    let db_version = db_schema_version(pool)
        .await
        .map_err(|e| format!("[dbSchemaVersion]: {e}"))?;
    let repo_version = repo_schema_version()?;
    tracing::info!(repo_version, db_version, "Read schema versions");
    if db_version == repo_version {
        return Ok(());
    }
    if db_version > repo_version {
        return Err(format!(
            "the DB schema is ahead of the schema in the code ({db_version} > {repo_version}). Something is wrong"
        ));
    }
    migrate(pool, repo_version, db_version).await?;

    let db_version = db_schema_version(pool)
        .await
        .map_err(|e| format!("[dbSchemaVersion]: {e}"))?;
    if db_version != repo_version {
        return Err(format!(
            "failed to migrate to schema version {repo_version}. Database reports a version of {db_version}"
        ));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn repo_version_parses() {
        assert!(repo_schema_version().unwrap() >= 41);
    }

    #[test]
    fn every_step_has_a_script() {
        let repo_version = repo_schema_version().unwrap();
        // The oldest migration in the repo goes from 4 to 5.
        for step in 5..=repo_version {
            assert!(migration_script(step).is_ok(), "no script for step {step}");
        }
    }
}
