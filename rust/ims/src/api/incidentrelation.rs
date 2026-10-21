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

//! Incident types and linked Incidents, added and removed one at a time so
//! that concurrent writers commute. Ported from the Go server's
//! `api/incidentrelation.go`.

use std::collections::HashMap;

use axum::Extension;
use axum::extract::{Path, State};
use axum::http::HeaderMap;
use axum::response::Response;

use super::helpers::*;
use super::incident::{fetch_incident, read_extra_incident_row_fields};
use super::middleware::JwtCtx;
use super::reportentry::{NewReportEntry, Parent, add_report_entry, path_i32};
use super::state::{AppState, S};
use crate::authz;
use crate::store::models::Event;
use crate::store::queries;
use crate::util::herr::{self, HttpError};

struct RelationRequest {
    event: Event,
    number: i32,
    author: String,
}

async fn parse_relation_request(
    st: &AppState,
    jwt: &JwtCtx,
    p: &HashMap<String, String>,
    headers: &HeaderMap,
    require_json: bool,
) -> HResult<RelationRequest> {
    let (event, perms) = get_event_permissions(st, jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & authz::EVENT_WRITE_INCIDENTS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventWriteIncidents permission for this Event",
        ));
    }
    if require_json {
        require_json_content_type(headers).map_err(|e| e.from_src("[requireJSONContentType]"))?;
    }
    let number = path_i32(p, "incidentNumber", "Invalid Incident Number")?;
    Ok(RelationRequest {
        event,
        number,
        author: claims_of(jwt)?.ranger_handle().to_string(),
    })
}

async fn set_incident_type(
    st: &AppState,
    jwt: &JwtCtx,
    p: &HashMap<String, String>,
    headers: &HeaderMap,
    attach: bool,
) -> HResult<()> {
    let r = parse_relation_request(st, jwt, p, headers, attach)
        .await
        .map_err(|e| e.from_src("[parseIncidentRelationRequest]"))?;
    let type_id = path_i32(p, "incidentTypeId", "Invalid Incident Type ID")?;
    let (row, _) = fetch_incident(st, r.event.id, r.number)
        .await
        .map_err(|e| e.from_src("[readIncidentRow]"))?;
    let (current, _, _) = read_extra_incident_row_fields(&row).map_err(|e| {
        HttpError::internal_server_error("Failed to read Incident details", e)
            .from_src("[readExtraIncidentRowFields]")
    })?;
    if current.contains(&type_id) == attach {
        return Ok(());
    }
    let types = queries::incident_types(&st.db)
        .await
        .map_err(db_err("Failed to get Incident Types", "[IncidentTypes]"))?;
    let type_name = types
        .iter()
        .find(|t| t.id == type_id)
        .map(|t| t.name.clone())
        .unwrap_or_default();
    if type_name.is_empty() {
        return Err(HttpError::bad_request_msg(format!(
            "There is no Incident Type with id {type_id}"
        )));
    }
    let r = &r;
    let type_name = &type_name;
    retry_on_deadlock(|| async move {
        let mut txn = st
            .db
            .begin()
            .await
            .map_err(db_err("Failed to start transaction", "[Begin]"))?;
        let log_line = if attach {
            match queries::attach_incident_type_to_incident(
                &mut *txn, r.event.id, r.number, type_id,
            )
            .await
            {
                Ok(()) => {}
                Err(e) if is_duplicate_key_error(&e) => return Ok(()),
                Err(e) => {
                    return Err(
                        HttpError::internal_server_error("Failed to add Incident Type", e)
                            .from_src("[AttachIncidentTypeToIncident]"),
                    );
                }
            }
            format!("Added type: {type_name}")
        } else {
            queries::detach_incident_type_from_incident(&mut *txn, r.event.id, r.number, type_id)
                .await
                .map_err(db_err(
                    "Failed to detach Incident Type",
                    "[DetachIncidentTypeFromIncident]",
                ))?;
            format!("Removed type: {type_name}")
        };
        add_report_entry(
            &mut txn,
            Parent::Incident,
            r.event.id,
            r.number,
            &NewReportEntry::generated(&r.author, log_line),
        )
        .await
        .map_err(|e| e.from_src("[addIncidentReportEntry]"))?;
        txn.commit()
            .await
            .map_err(db_err("Failed to commit transaction", "[Commit]"))?;
        Ok(())
    })
    .await?;
    st.es.notify_incident_update(r.event.id, r.number);
    Ok(())
}

async fn set_incident_link(
    st: &AppState,
    jwt: &JwtCtx,
    p: &HashMap<String, String>,
    headers: &HeaderMap,
    link: bool,
) -> HResult<()> {
    let r = parse_relation_request(st, jwt, p, headers, link)
        .await
        .map_err(|e| e.from_src("[parseIncidentRelationRequest]"))?;
    let peer_event = get_event(
        st,
        p.get("linkedEventName")
            .map(String::as_str)
            .unwrap_or_default(),
    )
    .await
    .map_err(|e| e.from_src("[getEvent]"))?;
    let peer_number = path_i32(p, "linkedIncidentNumber", "Invalid linked Incident Number")?;
    if peer_event.id == r.event.id && peer_number == r.number {
        return Err(HttpError::bad_request_msg(
            "An Incident cannot be linked to itself",
        ));
    }
    fetch_incident(st, r.event.id, r.number)
        .await
        .map_err(|e| e.from_src("[readIncidentRow]"))?;
    let current = queries::incident_linked_incidents(&st.db, r.event.id, r.number)
        .await
        .map_err(db_err(
            "Failed to fetch linked Incidents",
            "[Incident_LinkedIncidents]",
        ))?;
    let already = current
        .iter()
        .any(|c| c.linked_event == peer_event.id && c.linked_incident == peer_number);
    if already == link {
        return Ok(());
    }
    let r = &r;
    let peer_event = &peer_event;
    retry_on_deadlock(|| async move {
        let mut txn = st.db.begin().await.map_err(db_err("Failed to start transaction", "[Begin]"))?;
        let (self_line, peer_line) = if link {
            match queries::link_incidents(&mut *txn, r.event.id, r.number, peer_event.id, peer_number).await {
                Ok(()) => {}
                Err(e) if is_duplicate_key_error(&e) => return Ok(()),
                Err(e) => {
                    return Err(HttpError::bad_request(
                        format!("Failed to link Incident. There may be no IMS #{peer_number} for the given event."),
                        e,
                    )
                    .from_src("[LinkIncidents]"));
                }
            }
            queries::link_incidents(&mut *txn, peer_event.id, peer_number, r.event.id, r.number)
                .await
                .map_err(db_err("Failed to link Incident", "[LinkIncidents]"))?;
            (
                format!("Incident linked: {} #{peer_number}", peer_event.name),
                format!("Incident linked: {} #{}", r.event.name, r.number),
            )
        } else {
            queries::unlink_incidents(&mut *txn, r.event.id, r.number, peer_event.id, peer_number)
                .await
                .map_err(db_err("Failed to unlink Incident", "[UnlinkIncidents]"))?;
            queries::unlink_incidents(&mut *txn, peer_event.id, peer_number, r.event.id, r.number)
                .await
                .map_err(db_err("Failed to unlink Incident", "[UnlinkIncidents]"))?;
            (
                format!("Incident unlinked: {} #{peer_number}", peer_event.name),
                format!("Incident unlinked: {} #{}", r.event.name, r.number),
            )
        };
        add_report_entry(&mut txn, Parent::Incident, r.event.id, r.number, &NewReportEntry::generated(&r.author, self_line))
            .await
            .map_err(|e| e.from_src("[addIncidentReportEntry]"))?;
        add_report_entry(&mut txn, Parent::Incident, peer_event.id, peer_number, &NewReportEntry::generated(&r.author, peer_line))
            .await
            .map_err(|e| e.from_src("[addIncidentReportEntry]"))?;
        txn.commit().await.map_err(db_err("Failed to commit transaction", "[Commit]"))?;
        Ok(())
    })
    .await?;
    st.es.notify_incident_update(r.event.id, r.number);
    st.es.notify_incident_update(peer_event.id, peer_number);
    Ok(())
}

pub async fn attach_type_to_incident(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    headers: HeaderMap,
) -> HResult<Response> {
    set_incident_type(&st, &jwt, &p, &headers, true)
        .await
        .map_err(|e| e.from_src("[setIncidentType]"))?;
    Ok(herr::no_content_response("Success"))
}

pub async fn detach_type_from_incident(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    headers: HeaderMap,
) -> HResult<Response> {
    set_incident_type(&st, &jwt, &p, &headers, false)
        .await
        .map_err(|e| e.from_src("[setIncidentType]"))?;
    Ok(herr::no_content_response("Success"))
}

pub async fn link_incident(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    headers: HeaderMap,
) -> HResult<Response> {
    set_incident_link(&st, &jwt, &p, &headers, true)
        .await
        .map_err(|e| e.from_src("[setIncidentLink]"))?;
    Ok(herr::no_content_response("Success"))
}

pub async fn unlink_incident(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    headers: HeaderMap,
) -> HResult<Response> {
    set_incident_link(&st, &jwt, &p, &headers, false)
        .await
        .map_err(|e| e.from_src("[setIncidentLink]"))?;
    Ok(herr::no_content_response("Success"))
}
