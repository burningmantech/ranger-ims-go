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

//! Cross-event search over Incidents, Field Reports, and Visits. Ported from
//! the Go server's `api/search.go`.

use std::time::Duration;

use axum::Extension;
use axum::extract::{Query, State};
use axum::http::StatusCode;
use axum::response::Response;
use regex::Regex;
use serde::Deserialize;
use sqlx::FromRow;

use super::helpers::*;
use super::middleware::JwtCtx;
use super::state::S;
use crate::authz;
use crate::json;
use crate::store::queries::q;
use crate::util::conv::GoTime;
use crate::util::herr::HttpError;

const MIN_QUERY_RUNES: usize = 2;
const DEFAULT_LIMIT: i32 = 100;
const MAX_LIMIT: i64 = 1000;
const QUERY_TIMEOUT: Duration = Duration::from_secs(50);
const SLOW_THRESHOLD: Duration = Duration::from_secs(2);
const SNIPPET_PREFIX: usize = 40;
const SNIPPET_MAX_LEN: usize = 200;
const SNIPPET_MARKER: &str = "…";
const ALL_KINDS: &str = "incident,field_report,visit";

#[derive(Deserialize)]
pub struct SearchQuery {
    #[serde(default)]
    q: String,
    #[serde(default)]
    regex: String,
    #[serde(default)]
    kinds: String,
    #[serde(default)]
    limit: String,
}

#[derive(FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
struct IncidentHit {
    event: i32,
    event_name: Option<String>,
    number: i32,
    created: f64,
    summary: Option<String>,
    matched_entry_text: Option<String>,
}

#[derive(FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
struct FieldReportHit {
    event: i32,
    event_name: Option<String>,
    number: i32,
    created: f64,
    summary: Option<String>,
    incident_number: Option<i32>,
    matched_entry_text: Option<String>,
}

#[derive(FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
struct VisitHit {
    event: i32,
    event_name: Option<String>,
    number: i32,
    created: f64,
    incident_number: Option<i32>,
    guest_preferred_name: Option<String>,
    guest_legal_name: Option<String>,
    matched_entry_text: Option<String>,
}

/// Go's strconv.ParseBool.
fn parse_bool(s: &str) -> Option<bool> {
    match s {
        "1" | "t" | "T" | "TRUE" | "true" | "True" => Some(true),
        "0" | "f" | "F" | "FALSE" | "false" | "False" => Some(false),
        _ => None,
    }
}

pub fn escape_like_pattern(s: &str) -> String {
    s.replace('\\', "\\\\")
        .replace('%', "\\%")
        .replace('_', "\\_")
}

fn is_rune_start(b: u8) -> bool {
    b & 0xC0 != 0x80
}

/// An excerpt of `text` around the match, with markers where it was cut.
pub fn search_snippet(text: &str, match_at: Option<usize>) -> String {
    if text.is_empty() {
        return String::new();
    }
    let bytes = text.as_bytes();
    let match_at = match_at.unwrap_or(0);
    let mut start = match_at.saturating_sub(SNIPPET_PREFIX);
    let mut end = (start + SNIPPET_MAX_LEN).min(text.len());
    while start > 0 && !is_rune_start(bytes[start]) {
        start -= 1;
    }
    while end < text.len() && !is_rune_start(bytes[end]) {
        end += 1;
    }
    let mut snippet = text[start..end].to_string();
    if start > 0 {
        snippet = format!("{SNIPPET_MARKER}{snippet}");
    }
    if end < text.len() {
        snippet.push_str(SNIPPET_MARKER);
    }
    snippet
}

/// The byte index of the first case-insensitive match of `sub`, comparing
/// lowercased runes as Go's `unicode.ToLower` does.
pub fn index_fold(s: &str, sub: &str) -> Option<usize> {
    let lower = |c: char| c.to_lowercase().next().unwrap_or(c);
    s.char_indices().map(|(i, _)| i).find(|&i| {
        let mut rest = s[i..].chars();
        sub.chars()
            .all(|pc| rest.next().is_some_and(|c| lower(c) == lower(pc)))
    })
}

fn search_query_error(what: &str, err: sqlx::Error, timed_out: bool) -> HttpError {
    const ER_REGEXP_ERROR: u16 = 1139;
    if sqlx_error_number(&err) == Some(ER_REGEXP_ERROR) {
        return HttpError::bad_request("Invalid regular expression", err);
    }
    if timed_out {
        return HttpError::new(
            StatusCode::SERVICE_UNAVAILABLE,
            "Search took too long. Try again now, as this may have been due to a cold database read",
            Some(err.into()),
        );
    }
    HttpError::internal_server_error(format!("Failed to search {what}"), err)
}

async fn with_timeout<T>(
    fut: impl std::future::Future<Output = Result<T, sqlx::Error>>,
) -> Result<T, (sqlx::Error, bool)> {
    match tokio::time::timeout(QUERY_TIMEOUT, fut).await {
        Ok(r) => r.map_err(|e| (e, false)),
        Err(_) => Err((sqlx::Error::PoolTimedOut, true)),
    }
}

pub async fn get_search(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Query(params): Query<SearchQuery>,
) -> HResult<Response> {
    let query = params.q.trim().to_string();
    if query.chars().count() < MIN_QUERY_RUNES {
        return Err(HttpError::bad_request_msg(
            "The 'q' parameter must be at least 2 characters long",
        ));
    }
    let regex = if params.regex.is_empty() {
        false
    } else {
        parse_bool(&params.regex)
            .ok_or_else(|| HttpError::bad_request_msg("The 'regex' parameter must be a boolean"))?
    };
    let kinds = if params.kinds.is_empty() {
        ALL_KINDS.to_string()
    } else {
        params.kinds.clone()
    };
    let (mut want_incidents, mut want_frs, mut want_visits) = (false, false, false);
    for kind in kinds.split(',') {
        match kind.trim() {
            json::SEARCH_RESULT_KIND_INCIDENT => want_incidents = true,
            json::SEARCH_RESULT_KIND_FIELD_REPORT => want_frs = true,
            json::SEARCH_RESULT_KIND_VISIT => want_visits = true,
            _ => {
                return Err(HttpError::bad_request_msg(format!(
                    "The 'kinds' parameter must be a comma-separated subset of {ALL_KINDS}"
                )));
            }
        }
    }
    let mut limit = DEFAULT_LIMIT;
    if !params.limit.is_empty() {
        match params.limit.parse::<i64>() {
            Ok(n) if (1..=MAX_LIMIT).contains(&n) => limit = n as i32,
            _ => {
                return Err(HttpError::bad_request_msg(
                    "The 'limit' parameter must be an integer between 1 and 1000",
                ));
            }
        }
    }

    let perms_by_event = permissions_by_event(&st, &jwt)
        .await
        .map_err(|e| e.from_src("[permissionsByEvent]"))?;
    let events = crate::store::queries::events(&st.db)
        .await
        .map_err(db_err("Failed to fetch Events", "[Events]"))?;
    let (mut incident_events, mut fr_events, mut visit_events) =
        (Vec::new(), Vec::new(), Vec::new());
    for e in events.iter().filter(|e| !e.is_group) {
        let perms = perms_by_event.get(&e.id).copied().unwrap_or(0);
        if perms & authz::EVENT_READ_INCIDENTS != 0 {
            incident_events.push(e.id);
        }
        if perms & authz::EVENT_READ_ALL_FIELD_REPORTS != 0 {
            fr_events.push(e.id);
        }
        if perms & authz::EVENT_READ_VISITS != 0 {
            visit_events.push(e.id);
        }
    }

    let (text_like, text_regexp, query_regexp) = if regex {
        let re = Regex::new(&format!("(?i){query}"))
            .map_err(|e| HttpError::bad_request("Invalid regular expression", e))?;
        (None, Some(query.clone()), Some(re))
    } else {
        (
            Some(format!("%{}%", escape_like_pattern(&query))),
            None,
            None,
        )
    };
    let snippet = |matched: &Option<String>| {
        let text = matched.as_deref().unwrap_or_default();
        let at = match &query_regexp {
            Some(re) => re.find(text).map(|m| m.start()),
            None => index_fold(text, &query),
        };
        search_snippet(text, at)
    };
    let search = |name: &str, ids: &[i32]| {
        q(name)
            .named("text_like", text_like.clone())
            .named("text_regexp", text_regexp.clone())
            .slice("event_ids", ids.to_vec())
            .arg(limit)
    };

    let started = std::time::Instant::now();
    let mut resp = json::SearchResults::default();
    if want_incidents && !incident_events.is_empty() {
        let rows: Vec<IncidentHit> =
            with_timeout(search("SearchIncidents", &incident_events).fetch_all(&st.db))
                .await
                .map_err(|(e, t)| {
                    search_query_error("Incidents", e, t).from_src("[SearchIncidents]")
                })?;
        resp.truncated |= rows.len() == limit as usize;
        for r in rows {
            resp.hits.push(json::SearchResult {
                kind: json::SEARCH_RESULT_KIND_INCIDENT.to_string(),
                event: r.event_name.unwrap_or_default(),
                event_id: r.event,
                number: r.number,
                created: GoTime::from_float(r.created),
                summary: r.summary.unwrap_or_default(),
                snippet: snippet(&r.matched_entry_text),
                incident: None,
            });
        }
    }
    if want_frs && !fr_events.is_empty() {
        let rows: Vec<FieldReportHit> =
            with_timeout(search("SearchFieldReports", &fr_events).fetch_all(&st.db))
                .await
                .map_err(|(e, t)| {
                    search_query_error("Field Reports", e, t).from_src("[SearchFieldReports]")
                })?;
        resp.truncated |= rows.len() == limit as usize;
        for r in rows {
            resp.hits.push(json::SearchResult {
                kind: json::SEARCH_RESULT_KIND_FIELD_REPORT.to_string(),
                event: r.event_name.unwrap_or_default(),
                event_id: r.event,
                number: r.number,
                created: GoTime::from_float(r.created),
                summary: r.summary.unwrap_or_default(),
                snippet: snippet(&r.matched_entry_text),
                incident: r.incident_number,
            });
        }
    }
    if want_visits && !visit_events.is_empty() {
        let rows: Vec<VisitHit> =
            with_timeout(search("SearchVisits", &visit_events).fetch_all(&st.db))
                .await
                .map_err(|(e, t)| search_query_error("Visits", e, t).from_src("[SearchVisits]"))?;
        resp.truncated |= rows.len() == limit as usize;
        for r in rows {
            let mut summary = r.guest_preferred_name.unwrap_or_default();
            if summary.is_empty() {
                summary = r.guest_legal_name.unwrap_or_default();
            }
            resp.hits.push(json::SearchResult {
                kind: json::SEARCH_RESULT_KIND_VISIT.to_string(),
                event: r.event_name.unwrap_or_default(),
                event_id: r.event,
                number: r.number,
                created: GoTime::from_float(r.created),
                summary,
                snippet: snippet(&r.matched_entry_text),
                incident: r.incident_number,
            });
        }
    }
    resp.hits
        .sort_by(|a, b| b.event_id.cmp(&a.event_id).then(b.number.cmp(&a.number)));
    let elapsed = started.elapsed();
    if elapsed >= SLOW_THRESHOLD {
        tracing::warn!(q = %query, regex, limit, total_millis = elapsed.as_millis() as u64, "Served a cross-event search");
    } else {
        tracing::debug!(q = %query, regex, limit, total_millis = elapsed.as_millis() as u64, "Served a cross-event search");
    }
    Ok(json_response(&resp))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn like_escaping() {
        assert_eq!(escape_like_pattern(r"50%_off\now"), r"50\%\_off\\now");
    }

    #[test]
    fn fold_index() {
        assert_eq!(index_fold("Hello World", "world"), Some(6));
        assert_eq!(index_fold("Hello", "xyz"), None);
        assert_eq!(index_fold("ÅNGSTRÖM", "ström"), Some(4));
    }

    #[test]
    fn snippets() {
        assert_eq!(search_snippet("", Some(0)), "");
        assert_eq!(search_snippet("short text", Some(6)), "short text");
        let long = format!("{}needle{}", "a".repeat(100), "b".repeat(300));
        let s = search_snippet(&long, Some(100));
        assert!(s.starts_with("…"));
        assert!(s.ends_with("…"));
        assert!(s.contains("needle"));
        // A cut never lands inside a multi-byte character.
        let multi = "é".repeat(150);
        let s = search_snippet(&multi, Some(100));
        assert!(s.starts_with("…"));
    }

    #[test]
    fn go_parse_bool() {
        assert_eq!(parse_bool("T"), Some(true));
        assert_eq!(parse_bool("False"), Some(false));
        assert_eq!(parse_bool("yes"), None);
    }
}
