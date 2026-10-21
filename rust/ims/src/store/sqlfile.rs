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

//! Reads the sqlc query files (`store/queries.sql`, `directory/queries.sql`)
//! directly, so that the Rust port runs the very same SQL as the Go server
//! instead of a copy that could drift.
//!
//! sqlc's macros are expanded here:
//! - `sqlc.embed(t)` becomes `t.*`
//! - `sqlc.arg(n)` and `sqlc.narg(n)` become a `?` bound to the named value,
//!   which may be used several times in one query
//! - `sqlc.slice(n)` becomes a `?, ?, ...` list, one per element
//!
//! Plain `?` placeholders are bound positionally, in order of appearance,
//! just as sqlc orders its generated params.

use std::collections::HashMap;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Segment {
    Text(String),
    Positional,
    Named(String),
    Slice(String),
}

#[derive(Debug, Clone)]
pub struct ParsedQuery {
    pub name: String,
    pub segments: Vec<Segment>,
}

/// Splits a sqlc query file into its named queries.
pub fn parse_file(contents: &str) -> HashMap<String, ParsedQuery> {
    let mut queries = HashMap::new();
    let mut current: Option<(String, String)> = None;
    for line in contents.lines() {
        let trimmed = line.trim_start();
        if let Some(rest) = trimmed.strip_prefix("-- name:") {
            if let Some((name, body)) = current.take() {
                queries.insert(name.clone(), parse_query(name, &body));
            }
            let name = rest
                .split_whitespace()
                .next()
                .unwrap_or_default()
                .to_string();
            current = Some((name, String::new()));
            continue;
        }
        if let Some((_, body)) = current.as_mut() {
            body.push_str(strip_comment(line));
            body.push('\n');
        }
    }
    if let Some((name, body)) = current.take() {
        queries.insert(name.clone(), parse_query(name, &body));
    }
    queries
}

/// Drops a trailing `-- comment`, leaving any `--` inside a string literal be.
fn strip_comment(line: &str) -> &str {
    let mut quote: Option<char> = None;
    let mut prev = '\0';
    for (i, c) in line.char_indices() {
        match quote {
            Some(q) if c == q => quote = None,
            Some(_) => {}
            None if c == '\'' || c == '"' || c == '`' => quote = Some(c),
            None if c == '-' && prev == '-' => return &line[..i - 1],
            None => {}
        }
        prev = c;
    }
    line
}

fn parse_query(name: String, body: &str) -> ParsedQuery {
    let body = body.trim().trim_end_matches(';').trim_end();
    let mut segments = Vec::new();
    let mut text = String::new();
    let mut rest = body;
    let mut quote: Option<char> = None;
    while let Some(c) = rest.chars().next() {
        if let Some(q) = quote {
            text.push(c);
            rest = &rest[c.len_utf8()..];
            if c == q {
                quote = None;
            }
            continue;
        }
        if c == '\'' || c == '"' || c == '`' {
            quote = Some(c);
            text.push(c);
            rest = &rest[1..];
            continue;
        }
        if c == '?' {
            flush(&mut segments, &mut text);
            segments.push(Segment::Positional);
            rest = &rest[1..];
            continue;
        }
        if let Some((macro_name, arg, after)) = parse_macro(rest) {
            match macro_name {
                "embed" => text.push_str(&format!("{arg}.*")),
                "arg" | "narg" => {
                    flush(&mut segments, &mut text);
                    segments.push(Segment::Named(arg.to_string()));
                }
                "slice" => {
                    flush(&mut segments, &mut text);
                    segments.push(Segment::Slice(arg.to_string()));
                }
                other => panic!("query {name}: unsupported sqlc macro sqlc.{other}"),
            }
            rest = after;
            continue;
        }
        text.push(c);
        rest = &rest[c.len_utf8()..];
    }
    flush(&mut segments, &mut text);
    ParsedQuery { name, segments }
}

fn flush(segments: &mut Vec<Segment>, text: &mut String) {
    if !text.is_empty() {
        segments.push(Segment::Text(std::mem::take(text)));
    }
}

/// Matches `sqlc.<macro>(<arg>)` at the start of `s`.
fn parse_macro(s: &str) -> Option<(&str, &str, &str)> {
    let after_prefix = s.strip_prefix("sqlc.")?;
    let open = after_prefix.find('(')?;
    let macro_name = &after_prefix[..open];
    if macro_name.is_empty() || !macro_name.chars().all(|c| c.is_ascii_alphabetic()) {
        return None;
    }
    let after_open = &after_prefix[open + 1..];
    let close = after_open.find(')')?;
    let arg = after_open[..close].trim();
    Some((macro_name, arg, &after_open[close + 1..]))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_names_and_positional_params() {
        let q = parse_file("-- name: Foo :one\nselect X from T where A = ? and B = ?;\n");
        let foo = &q["Foo"];
        assert_eq!(
            foo.segments,
            vec![
                Segment::Text("select X from T where A = ".to_string()),
                Segment::Positional,
                Segment::Text(" and B = ".to_string()),
                Segment::Positional,
            ]
        );
    }

    #[test]
    fn expands_sqlc_macros() {
        let q = parse_file(
            "-- name: Bar :many\nselect sqlc.embed(e) from E e where ID in (sqlc.slice(ids)) or P = sqlc.arg(p) or Q = sqlc.narg(p);\n",
        );
        assert_eq!(
            q["Bar"].segments,
            vec![
                Segment::Text("select e.* from E e where ID in (".to_string()),
                Segment::Slice("ids".to_string()),
                Segment::Text(") or P = ".to_string()),
                Segment::Named("p".to_string()),
                Segment::Text(" or Q = ".to_string()),
                Segment::Named("p".to_string()),
            ]
        );
    }

    #[test]
    fn ignores_comments_and_string_literals() {
        let q = parse_file(
            "-- leading comment? not a query\n-- name: Baz :one\nselect '?' as A, \"--\" as B -- trailing? comment\nfrom T;\n-- name: Next :one\nselect 1;\n",
        );
        assert_eq!(
            q["Baz"].segments,
            vec![Segment::Text(
                "select '?' as A, \"--\" as B \nfrom T".to_string()
            )]
        );
        assert_eq!(
            q["Next"].segments,
            vec![Segment::Text("select 1".to_string())]
        );
        assert_eq!(q.len(), 2);
    }

    #[test]
    fn parses_the_real_query_files() {
        let ims = parse_file(include_str!("../../../../store/queries.sql"));
        assert!(ims.contains_key("SchemaVersion"));
        assert!(ims.contains_key("SearchVisits"));
        let search = &ims["SearchIncidents"];
        assert!(
            search
                .segments
                .contains(&Segment::Slice("event_ids".to_string()))
        );
        assert!(
            search
                .segments
                .contains(&Segment::Named("text_like".to_string()))
        );

        let clubhouse = parse_file(include_str!("../../../../directory/queries.sql"));
        assert_eq!(clubhouse.len(), 6);
    }
}
