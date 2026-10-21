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

//! Event access rules. Ported from the Go server's `api/eventaccess.go`.

use std::collections::{HashMap, HashSet};
use std::sync::LazyLock;
use std::time::Duration;

use axum::Extension;
use axum::body::Body;
use axum::extract::State;
use axum::http::HeaderMap;
use axum::response::Response;

use super::helpers::*;
use super::middleware::JwtCtx;
use super::state::{AppState, S};
use crate::authz::{self, Requestor};
use crate::json;
use crate::store::models::{Event, EventAccess};
use crate::store::queries;
use crate::util::conv::GoTime;
use crate::util::herr::{self, HttpError};

async fn require_administrate_events(st: &AppState, jwt: &JwtCtx) -> HResult<()> {
    let global = get_global_permissions(st, jwt)
        .await
        .map_err(|e| e.from_src("[getGlobalPermissions]"))?;
    if global & authz::GLOBAL_ADMINISTRATE_EVENTS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have GlobalAdministrateEvents permission",
        ));
    }
    Ok(())
}

fn case_insensitive(a: &str, b: &str) -> std::cmp::Ordering {
    a.to_lowercase().cmp(&b.to_lowercase())
}

pub async fn get_event_accesses(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
) -> HResult<Response> {
    require_administrate_events(&st, &jwt).await?;
    let events = queries::events(&st.db)
        .await
        .map_err(db_err("Failed to fetch Events", "[Events]"))?;
    let access_rows = queries::event_access_all(&st.db)
        .await
        .map_err(db_err("Failed to fetch EventAccess", "[EventAccessAll]"))?;
    let mut by_event: HashMap<i32, Vec<EventAccess>> = HashMap::new();
    for ar in access_rows {
        by_event.entry(ar.event).or_default().push(ar);
    }
    let users = st.user_store.get_all_users().await.map_err(|e| {
        HttpError::internal_server_error("Failed to fetch Users", e).from_src("[Users]")
    })?;
    let (positions, teams) = st.user_store.get_positions_and_teams().await.map_err(|e| {
        HttpError::internal_server_error("Failed to fetch Positions and Teams", e)
            .from_src("[GetPositionsAndTeams]")
    })?;
    let all_handles: HashSet<&str> = users.values().map(|u| u.handle.as_str()).collect();
    let all_positions: HashSet<&str> = positions.values().map(String::as_str).collect();
    let all_teams: HashSet<&str> = teams.values().map(String::as_str).collect();

    let now = GoTime::now();
    let mut result = json::EventsAccess::new();
    for e in events {
        let mut readers = Vec::new();
        let mut writers = Vec::new();
        let mut reporters = Vec::new();
        let mut visit_writers = Vec::new();
        for access in by_event.get(&e.id).map(Vec::as_slice).unwrap_or_default() {
            let not_after = GoTime::from_opt_float(access.not_after);
            let not_before = GoTime::from_opt_float(access.not_before);
            let mut rule = json::AccessRule {
                expression: access.expression.clone(),
                validity: access.validity.clone(),
                not_after,
                description: access.description.clone(),
                expired: access.not_after.is_some() && not_after.before(&now),
                not_before,
                pending: access.not_before.is_some() && not_before.after(&now),
                debug_info: json::AccessRuleDebugInfo::default(),
            };
            if access.expression == "*"
                && access.validity == "always"
                && !rule.expired
                && !rule.pending
            {
                rule.debug_info.matches_all_users = true;
            } else {
                for person in users.values() {
                    let requestor = Requestor {
                        handle: person.handle.clone(),
                        onsite: person.onsite,
                        positions: person.position_names.clone(),
                        teams: person.team_names.clone(),
                        on_duty_position: person.on_duty_position_name.clone().unwrap_or_default(),
                    };
                    if authz::person_matches(access, &requestor) {
                        rule.debug_info.matches_users.push(person.handle.clone());
                    }
                }
                if rule.debug_info.matches_users.is_empty() {
                    rule.debug_info.matches_no_one = true;
                }
            }
            rule.debug_info.known_target =
                known_target(&access.expression, &all_handles, &all_positions, &all_teams);
            rule.debug_info
                .matches_users
                .sort_by(|a, b| case_insensitive(a, b));
            match access.mode.as_str() {
                "read" => readers.push(rule),
                "write" => writers.push(rule),
                "report" => reporters.push(rule),
                "write_visits" => visit_writers.push(rule),
                _ => {}
            }
        }
        result.insert(
            e.name,
            json::EventAccess {
                readers: Some(readers),
                writers: Some(writers),
                reporters: Some(reporters),
                visit_writers: Some(visit_writers),
            },
        );
    }
    Ok(json_response(&result))
}

pub async fn get_access_targets(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
) -> HResult<Response> {
    require_administrate_events(&st, &jwt).await?;
    let users = st.user_store.get_all_users().await.map_err(|e| {
        HttpError::internal_server_error("Failed to fetch Users", e).from_src("[GetAllUsers]")
    })?;
    let (positions, teams) = st.user_store.get_positions_and_teams().await.map_err(|e| {
        HttpError::internal_server_error("Failed to fetch Positions and Teams", e)
            .from_src("[GetPositionsAndTeams]")
    })?;
    let mut resp = json::AccessTargets {
        persons: users.values().map(|u| u.handle.clone()).collect(),
        positions: positions.values().cloned().collect(),
        teams: teams.values().cloned().collect(),
    };
    resp.persons.sort_by(|a, b| case_insensitive(a, b));
    resp.positions.sort_by(|a, b| case_insensitive(a, b));
    resp.teams.sort_by(|a, b| case_insensitive(a, b));
    Ok(json_response(&resp))
}

/// Whether the target of an access expression is a known user, position, or team.
fn known_target(
    expression: &str,
    handles: &HashSet<&str>,
    positions: &HashSet<&str>,
    teams: &HashSet<&str>,
) -> bool {
    if expression == "*" {
        return true;
    }
    if let Some(h) = expression.strip_prefix("person:") {
        return handles.contains(h);
    }
    if let Some(p) = expression
        .strip_prefix("position:")
        .or_else(|| expression.strip_prefix("onduty:"))
    {
        return positions.contains(p);
    }
    if let Some(t) = expression.strip_prefix("team:") {
        return teams.contains(t);
    }
    false
}

/// Event-access writes are last-write-wins, admin-only, and rare, so they're
/// simply serialized: they're very prone to transaction deadlock otherwise.
static EVENT_ACCESS_WRITE_MU: LazyLock<tokio::sync::Mutex<()>> =
    LazyLock::new(|| tokio::sync::Mutex::new(()));

pub async fn post_event_access(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    require_administrate_events(&st, &jwt).await?;
    let events_access: json::EventsAccess = read_body_as(&headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    for (event_name, access) in events_access {
        let event = get_event(&st, &event_name)
            .await
            .map_err(|e| e.from_src("[readBodyAs]"))?;
        maybe_set_access(&st, &event, access.readers.as_deref(), "read").await?;
        maybe_set_access(&st, &event, access.writers.as_deref(), "write").await?;
        maybe_set_access(&st, &event, access.reporters.as_deref(), "report").await?;
        maybe_set_access(&st, &event, access.visit_writers.as_deref(), "write_visits").await?;
    }
    Ok(herr::no_content_response("Successfully set event access"))
}

/// Replaces the event's rules for one mode, leaving its other modes alone.
/// None means the caller didn't mention this mode; an empty list clears it.
async fn maybe_set_access(
    st: &AppState,
    event: &Event,
    rules: Option<&[json::AccessRule]>,
    mode: &str,
) -> HResult<()> {
    let Some(rules) = rules else { return Ok(()) };
    let _guard = EVENT_ACCESS_WRITE_MU.lock().await;
    // The Go server's mutex only serializes writers within one process, so a
    // deadlock between servers sharing the database is retried.
    let work = retry_on_deadlock(|| async {
        let mut txn = st
            .db
            .begin()
            .await
            .map_err(db_err("Failed to begin transaction", "[BeginTx]"))?;
        queries::clear_event_access_for_mode(&mut *txn, event.id, mode)
            .await
            .map_err(db_err(
                "Failed to begin transaction",
                "[ClearEventAccessForMode]",
            ))?;
        for rule in rules {
            queries::add_event_access(
                &mut *txn,
                event.id,
                &rule.expression,
                mode,
                &rule.validity,
                rule.not_after.to_opt_float(),
                rule.not_before.to_opt_float(),
                &rule.description,
            )
            .await
            .map_err(db_err("Failed to add event access", "[AddEventAccess]"))?;
        }
        txn.commit()
            .await
            .map_err(db_err("Failed to commit transaction", "[Commit]"))?;
        Ok(())
    });
    match tokio::time::timeout(Duration::from_secs(10), work).await {
        Ok(r) => r.map_err(|e: HttpError| e.from_src(&format!("[maybeSetAccess] {mode}"))),
        Err(_) => Err(HttpError::internal_server_error(
            "Failed to set event access",
            anyhow::anyhow!("context deadline exceeded"),
        )),
    }
}
