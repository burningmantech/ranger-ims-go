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

//! Adding and removing Rangers on Incidents and Visits, one at a time.
//! Ported from the Go server's `api/ranger.go`.

use std::collections::HashMap;

use axum::Extension;
use axum::body::Body;
use axum::extract::{Path, State};
use axum::http::HeaderMap;
use axum::response::Response;
use serde::Deserialize;
use sqlx::MySqlConnection;

use super::helpers::*;
use super::middleware::JwtCtx;
use super::reportentry::{NewReportEntry, Parent, add_report_entry, path_i32};
use super::state::{AppState, S};
use crate::authz::{self, EventPermissionMask};

use crate::store::queries;
use crate::util::conv::string_to_sql;
use crate::util::herr::{self, HttpError};

#[derive(Clone, Copy)]
struct Roster {
    parent: Parent,
    write_permission: EventPermissionMask,
    write_permission_name: &'static str,
    number_path_key: &'static str,
    noun: &'static str,
}

const INCIDENT_ROSTER: Roster = Roster {
    parent: Parent::Incident,
    write_permission: authz::EVENT_WRITE_INCIDENTS,
    write_permission_name: "EventWriteIncidents",
    number_path_key: "incidentNumber",
    noun: "Incident",
};

const VISIT_ROSTER: Roster = Roster {
    parent: Parent::Visit,
    write_permission: authz::EVENT_WRITE_VISITS,
    write_permission_name: "EventWriteVisits",
    number_path_key: "visitNumber",
    noun: "Visit",
};

impl Roster {
    async fn detach(
        self,
        conn: &mut MySqlConnection,
        event: i32,
        number: i32,
        handle: &str,
    ) -> Result<(), sqlx::Error> {
        match self.parent {
            Parent::Incident => {
                queries::detach_ranger_handle_from_incident(conn, event, number, handle).await
            }
            _ => queries::detach_ranger_from_visit(conn, event, number, handle).await,
        }
    }

    async fn attach(
        self,
        conn: &mut MySqlConnection,
        event: i32,
        number: i32,
        handle: &str,
        role: Option<&str>,
    ) -> Result<(), sqlx::Error> {
        match self.parent {
            Parent::Incident => {
                queries::attach_ranger_handle_to_incident(conn, event, number, handle, role).await
            }
            _ => queries::attach_ranger_to_visit(conn, event, number, handle, role).await,
        }
    }

    /// The Ranger's role on the record, and whether they're on it at all.
    async fn current_role(
        self,
        conn: &mut MySqlConnection,
        event: i32,
        number: i32,
        handle: &str,
    ) -> Result<(Option<String>, bool), sqlx::Error> {
        let found = match self.parent {
            Parent::Incident => queries::incident_rangers(conn, event, number)
                .await?
                .into_iter()
                .find(|r| r.ranger_handle == handle)
                .map(|r| r.role),
            _ => queries::visit_rangers(conn, event, number)
                .await?
                .into_iter()
                .find(|r| r.ranger_handle == handle)
                .map(|r| r.role),
        };
        Ok(match found {
            Some(role) => (role, true),
            None => (None, false),
        })
    }

    fn notify(self, st: &AppState, event: i32, number: i32) {
        match self.parent {
            Parent::Incident => st.es.notify_incident_update(event, number),
            _ => st.es.notify_visit_update(event, number),
        }
    }
}

struct RosterRequest {
    event_id: i32,
    number: i32,
    ranger_name: String,
    author: String,
}

async fn parse_roster_request(
    st: &AppState,
    jwt: &JwtCtx,
    p: &HashMap<String, String>,
    roster: Roster,
) -> HResult<RosterRequest> {
    let (event, perms) = get_event_permissions(st, jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & roster.write_permission == 0 {
        return Err(HttpError::forbidden_msg(format!(
            "The requestor does not have {} permission for this Event",
            roster.write_permission_name
        )));
    }
    let number = path_i32(
        p,
        roster.number_path_key,
        &format!("Invalid {} Number", roster.noun),
    )?;
    let ranger_name = p.get("rangerName").cloned().unwrap_or_default();
    if ranger_name.is_empty() {
        return Err(HttpError::bad_request_msg("Empty Ranger Name"));
    }
    Ok(RosterRequest {
        event_id: event.id,
        number,
        ranger_name,
        author: claims_of(jwt)?.ranger_handle().to_string(),
    })
}

#[derive(Deserialize)]
struct RosterBody {
    #[serde(default)]
    role: Option<String>,
}

async fn attach_ranger(
    st: &AppState,
    jwt: &JwtCtx,
    p: &HashMap<String, String>,
    headers: &HeaderMap,
    body: Body,
    roster: Roster,
) -> HResult<()> {
    let r = parse_roster_request(st, jwt, p, roster)
        .await
        .map_err(|e| e.from_src("[parseRangerRosterRequest]"))?;
    let body: RosterBody = read_body_as(headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    let new_role = string_to_sql(body.role.as_deref(), 128);
    let r = &r;
    let new_role = &new_role;
    let changed = retry_on_deadlock(|| async move {
        let mut txn = st
            .db
            .begin()
            .await
            .map_err(db_err("Failed to start transaction", "[Begin]"))?;
        let (old_role, was_attached) = roster
            .current_role(&mut txn, r.event_id, r.number, &r.ranger_name)
            .await
            .map_err(db_err(
                &format!("Failed to read {} roster", roster.noun),
                "[currentRole]",
            ))?;
        if was_attached && old_role == *new_role {
            return Ok(false);
        }
        let log_line = match (was_attached, new_role) {
            (false, Some(role)) => format!("Added Ranger: {} (role: {role})", r.ranger_name),
            (false, None) => format!("Added Ranger: {}", r.ranger_name),
            (true, Some(role)) => format!("Set role for {}: {role}", r.ranger_name),
            (true, None) => format!("Removed role for {}", r.ranger_name),
        };
        roster
            .detach(&mut txn, r.event_id, r.number, &r.ranger_name)
            .await
            .map_err(db_err(
                &format!("Failed to detach Ranger from {}", roster.noun),
                "[detach]",
            ))?;
        roster
            .attach(
                &mut txn,
                r.event_id,
                r.number,
                &r.ranger_name,
                new_role.as_deref(),
            )
            .await
            .map_err(db_err(
                &format!("Failed to attach Ranger to {}", roster.noun),
                "[attach]",
            ))?;
        add_report_entry(
            &mut txn,
            roster.parent,
            r.event_id,
            r.number,
            &NewReportEntry::generated(&r.author, log_line),
        )
        .await
        .map_err(|e| e.from_src("[addReportEntry]"))?;
        txn.commit()
            .await
            .map_err(db_err("Failed to commit transaction", "[Commit]"))?;
        Ok(true)
    })
    .await?;
    if changed {
        roster.notify(st, r.event_id, r.number);
    }
    Ok(())
}

async fn detach_ranger(
    st: &AppState,
    jwt: &JwtCtx,
    p: &HashMap<String, String>,
    roster: Roster,
) -> HResult<()> {
    let r = parse_roster_request(st, jwt, p, roster)
        .await
        .map_err(|e| e.from_src("[parseRangerRosterRequest]"))?;
    let r = &r;
    retry_on_deadlock(|| async move {
        let mut txn = st
            .db
            .begin()
            .await
            .map_err(db_err("Failed to start transaction", "[Begin]"))?;
        roster
            .detach(&mut txn, r.event_id, r.number, &r.ranger_name)
            .await
            .map_err(db_err(
                &format!("Failed to detach Ranger from {}", roster.noun),
                "[detach]",
            ))?;
        let text = format!("Removed Ranger: {}", r.ranger_name);
        add_report_entry(
            &mut txn,
            roster.parent,
            r.event_id,
            r.number,
            &NewReportEntry::generated(&r.author, text),
        )
        .await
        .map_err(|e| e.from_src("[addReportEntry]"))?;
        txn.commit()
            .await
            .map_err(db_err("Failed to commit transaction", "[Commit]"))?;
        Ok(())
    })
    .await?;
    roster.notify(st, r.event_id, r.number);
    Ok(())
}

pub async fn attach_ranger_to_incident(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    attach_ranger(&st, &jwt, &p, &headers, body, INCIDENT_ROSTER)
        .await
        .map_err(|e| e.from_src("[attachRanger]"))?;
    Ok(herr::no_content_response("Success"))
}

pub async fn detach_ranger_from_incident(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
) -> HResult<Response> {
    detach_ranger(&st, &jwt, &p, INCIDENT_ROSTER)
        .await
        .map_err(|e| e.from_src("[detachRanger]"))?;
    Ok(herr::no_content_response("Success"))
}

pub async fn attach_ranger_to_visit(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    attach_ranger(&st, &jwt, &p, &headers, body, VISIT_ROSTER)
        .await
        .map_err(|e| e.from_src("[attachRanger]"))?;
    Ok(herr::no_content_response("Success"))
}

pub async fn detach_ranger_from_visit(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
) -> HResult<Response> {
    detach_ranger(&st, &jwt, &p, VISIT_ROSTER)
        .await
        .map_err(|e| e.from_src("[detachRanger]"))?;
    Ok(herr::no_content_response("Success"))
}
