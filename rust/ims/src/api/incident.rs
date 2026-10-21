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

//! Incidents. Ported from the Go server's `api/incident.go`.

use std::collections::HashMap;

use axum::Extension;
use axum::body::Body;
use axum::extract::{Path, Query, State};
use axum::http::{HeaderMap, HeaderValue, header};
use axum::response::Response;
use serde::Deserialize;

use super::helpers::*;
use super::middleware::JwtCtx;
use super::reportentry::{Parent, add_change_report_entries, path_i32, report_entry_to_json};
use super::state::{AppState, S};
use crate::authz;
use crate::json;
use crate::store::models::{
    Event, IncidentRanger, IncidentWithAssociations, LinkedIncident, ReportEntry,
};
use crate::store::queries;
use crate::util::conv::{GoTime, string_to_sql};
use crate::util::format::normalize_address;
use crate::util::herr::{self, HttpError};

/// How many read-merge-write retries an edit makes when a concurrent writer
/// moves the record's version out from under it.
pub const MAX_CAS_ATTEMPTS: usize = 3;

pub const INCIDENT_STATES: [&str; 5] = ["new", "on_hold", "dispatched", "on_scene", "closed"];

#[derive(Deserialize)]
pub struct EntriesQuery {
    #[serde(default)]
    exclude_system_entries: String,
}

impl EntriesQuery {
    pub fn include_system_entries(&self) -> bool {
        !self.exclude_system_entries.eq_ignore_ascii_case("true")
    }
}

pub async fn get_incidents(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(event_name): Path<String>,
    Query(q): Query<EntriesQuery>,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &event_name)
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & authz::EVENT_READ_INCIDENTS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventReadIncidents permission",
        ));
    }
    // These both return a lot of data, so they run concurrently.
    let (entries, rangers, incidents) = tokio::join!(
        queries::incidents_report_entries(&st.db, event.id, q.include_system_entries()),
        queries::incidents_rangers(&st.db, event.id),
        queries::incidents(&st.db, event.id),
    );
    let entries = entries.map_err(db_err(
        "Failed to fetch Incident Report Entries",
        "[Incidents_ReportEntries]",
    ))?;
    let rangers = rangers.map_err(db_err("Failed to fetch rangers", "[Incidents_Rangers]"))?;
    let incidents = incidents.map_err(db_err("Failed to fetch Incidents", "[Incidents]"))?;

    let mut entries_by_incident: HashMap<i32, Vec<ReportEntry>> = HashMap::new();
    for row in entries {
        entries_by_incident
            .entry(row.incident_number)
            .or_default()
            .push(row.report_entry);
    }
    let mut rangers_by_incident: HashMap<i32, Vec<IncidentRanger>> = HashMap::new();
    for r in rangers {
        rangers_by_incident
            .entry(r.incident_number)
            .or_default()
            .push(r);
    }
    let mut resp = Vec::with_capacity(incidents.len());
    for row in &incidents {
        let n = row.incident.number;
        // Linked incidents aren't looked up for the list.
        resp.push(
            incident_to_json(
                row,
                rangers_by_incident
                    .get(&n)
                    .map(Vec::as_slice)
                    .unwrap_or_default(),
                entries_by_incident
                    .get(&n)
                    .map(Vec::as_slice)
                    .unwrap_or_default(),
                &[],
                &event,
                st.attachments_enabled(),
            )
            .map_err(|e| e.from_src("[incidentToJSON]"))?,
        );
    }
    Ok(json_response(&resp))
}

pub async fn get_incident(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & authz::EVENT_READ_INCIDENTS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventReadIncidents permission on this Event",
        ));
    }
    let number = path_i32(&p, "incidentNumber", "Failed to parse incident number")?;
    let (row, entries) = fetch_incident(&st, event.id, number)
        .await
        .map_err(|e| e.from_src("[fetchIncident]"))?;
    let perms_by_event = permissions_by_event(&st, &jwt)
        .await
        .map_err(|e| e.from_src("[permissionsByEvent]"))?;
    let rangers = queries::incident_rangers(&st.db, event.id, number)
        .await
        .map_err(|e| HttpError::internal_server_error("Failed to fetch rangers", e))?;
    let mut linked = queries::incident_linked_incidents(&st.db, event.id, number)
        .await
        .map_err(|e| HttpError::internal_server_error("Failed to fetch linked incidents", e))?;
    for li in &mut linked {
        if perms_by_event.get(&li.linked_event).copied().unwrap_or(0) & authz::EVENT_READ_INCIDENTS
            == 0
        {
            li.linked_incident_summary = None;
        }
    }
    let resp = incident_to_json(
        &row,
        &rangers,
        &entries,
        &linked,
        &event,
        st.attachments_enabled(),
    )
    .map_err(|e| e.from_src("[incidentToJSON]"))?;
    Ok(json_response(&resp))
}

pub fn parse_id_list(raw: &str) -> anyhow::Result<Vec<i32>> {
    Ok(serde_json::from_str(raw)?)
}

/// The JSON-aggregated associations of an Incident row.
pub fn read_extra_incident_row_fields(
    row: &IncidentWithAssociations,
) -> anyhow::Result<(Vec<i32>, Vec<i32>, Vec<i32>)> {
    Ok((
        parse_id_list(&row.incident_type_ids)?,
        parse_id_list(&row.field_report_numbers)?,
        parse_id_list(&row.visit_numbers)?,
    ))
}

fn incident_to_json(
    row: &IncidentWithAssociations,
    rangers: &[IncidentRanger],
    entries: &[ReportEntry],
    linked: &[LinkedIncident],
    event: &Event,
    attachments_enabled: bool,
) -> HResult<json::Incident> {
    let result_entries: Vec<json::ReportEntry> = entries
        .iter()
        .map(|re| report_entry_to_json(re, attachments_enabled))
        .collect();
    let (type_ids, fr_numbers, visit_numbers) =
        read_extra_incident_row_fields(row).map_err(|e| {
            HttpError::internal_server_error("Failed to fetch Incident details", e)
                .from_src("[readExtraIncidentRowFields]")
        })?;
    let i = &row.incident;
    let created = GoTime::from_float(i.created);
    let last_modified = result_entries
        .iter()
        .map(|re| re.created)
        .fold(created, |a, b| if b.after(&a) { b } else { a });
    Ok(json::Incident {
        event: event.name.clone(),
        event_id: event.id,
        number: i.number,
        created,
        last_modified,
        version: i.version,
        state: i.state.clone(),
        started: GoTime::from_float(i.started),
        closed: GoTime::from_opt_float(i.closed),
        priority: i.priority,
        summary: i.summary.clone(),
        location: json::Location {
            name: i.location_name.clone(),
            address: i.location_address.clone(),
            description: i.location_description.clone(),
        },
        incident_type_ids: Some(type_ids),
        field_reports: Some(fr_numbers),
        visits: Some(visit_numbers),
        linked_incidents: Some(
            linked
                .iter()
                .map(|li| json::LinkedIncident {
                    event_name: li.linked_event_name.clone(),
                    event_id: li.linked_event,
                    number: li.linked_incident,
                    summary: li.linked_incident_summary.clone().unwrap_or_default(),
                })
                .collect(),
        ),
        rangers: Some(
            rangers
                .iter()
                .map(|r| json::IncidentRanger {
                    handle: r.ranger_handle.clone(),
                    role: r.role.clone(),
                })
                .collect(),
        ),
        report_entries: Some(result_entries),
    })
}

pub async fn fetch_incident(
    st: &AppState,
    event_id: i32,
    number: i32,
) -> HResult<(IncidentWithAssociations, Vec<ReportEntry>)> {
    let row = match queries::incident(&st.db, event_id, number).await {
        Ok(Some(row)) => row,
        Ok(None) => {
            return Err(HttpError::not_found(
                "Incident not found",
                anyhow::anyhow!("sql: no rows in result set"),
            )
            .from_src("[Incident]"));
        }
        Err(e) => {
            return Err(
                HttpError::internal_server_error("Failed to fetch Incident", e)
                    .from_src("[Incident]"),
            );
        }
    };
    let entries = queries::incident_report_entries(&st.db, event_id, number)
        .await
        .map_err(db_err(
            "Failed to fetch report entries",
            "[Incident_ReportEntries]",
        ))?
        .into_iter()
        .map(|r| r.report_entry)
        .collect();
    Ok((row, entries))
}

fn created_response(number_header: &'static str, number: i32, location: String) -> Response {
    let mut resp = herr::created_response("Created");
    resp.headers_mut()
        .insert(number_header, HeaderValue::from(number));
    if let Ok(v) = HeaderValue::from_str(&location) {
        resp.headers_mut().insert(header::LOCATION, v);
    }
    resp
}

pub fn location_path(event_name: &str, kind: &str, number: i32) -> String {
    format!("/ims/api/events/{event_name}/{kind}/{number}")
}

pub async fn new_incident(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(event_name): Path<String>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &event_name)
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & authz::EVENT_WRITE_INCIDENTS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventWriteIncidents permission on this Event",
        ));
    }
    let mut new: json::Incident = read_body_as(&headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    let author = claims_of(&jwt)?.ranger_handle().to_string();

    // The number is allocated with a plain SELECT, so a concurrent creator can
    // claim it first; the primary key turns that into a duplicate-key error,
    // and the INSERT is retried with a fresh number.
    let now = GoTime::now().to_float();
    let mut attempt = 1;
    let number = loop {
        let number = queries::next_incident_number(&st.db, event.id)
            .await
            .map_err(db_err(
                "Failed to find next Incident number",
                "[NextIncidentNumber]",
            ))? as i32;
        match queries::create_incident(
            &st.db,
            event.id,
            number,
            now,
            json::INCIDENT_PRIORITY_NORMAL,
            "new",
            now,
        )
        .await
        {
            Ok(()) => break number,
            Err(e) if is_duplicate_key_error(&e) => {
                if attempt == MAX_NUMBER_ALLOC_ATTEMPTS {
                    return Err(HttpError::conflict(
                        "Incidents are being created concurrently. Please try again.",
                        e,
                    )
                    .from_src("[CreateIncident]"));
                }
                attempt += 1;
            }
            Err(e) => {
                return Err(
                    HttpError::internal_server_error("Failed to create incident", e)
                        .from_src("[CreateIncident]"),
                );
            }
        }
    };
    new.event_id = event.id;
    new.event = event.name.clone();
    new.number = number;
    update_incident(&st, &new, &author, event.normalize_addresses)
        .await
        .map_err(|e| e.from_src("[updateIncident]").from_src("[newIncident]"))?;
    Ok(created_response(
        "IMS-Incident-Number",
        number,
        location_path(&event.name, "incidents", number),
    ))
}

pub async fn edit_incident(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & authz::EVENT_WRITE_INCIDENTS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventWriteIncidents permission for this Event",
        ));
    }
    let number = path_i32(&p, "incidentNumber", "Invalid Incident Number")?;
    let mut new: json::Incident = read_body_as(&headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    new.event = event.name.clone();
    new.event_id = event.id;
    new.number = number;
    let author = claims_of(&jwt)?.ranger_handle().to_string();
    update_incident(&st, &new, &author, event.normalize_addresses)
        .await
        .map_err(|e| e.from_src("[updateIncident]").from_src("[editIncident]"))?;
    Ok(herr::no_content_response("Success"))
}

/// Refuses an edit that carries one of the Incident's set-valued fields.
/// Each is mutated one member at a time through its own endpoint, so that
/// concurrent writers can't undo each other.
fn reject_set_replacement(new: &json::Incident) -> HResult<()> {
    let msg = if new.incident_type_ids.is_some() {
        "incident_type_ids is read-only; use POST/DELETE .../incidents/{number}/incident_types/{incidentTypeId}"
    } else if new.linked_incidents.is_some() {
        "linked_incidents is read-only; use POST/DELETE .../incidents/{number}/linked_incidents/{eventName}/{number}"
    } else if new.field_reports.is_some() {
        "field_reports is read-only; attach or detach via the Field Report endpoint"
    } else if new.visits.is_some() {
        "visits is read-only; set the Visit's incident field via the Visit endpoint"
    } else {
        return Ok(());
    };
    Err(HttpError::bad_request_msg(msg).set_expected())
}

async fn update_incident(
    st: &AppState,
    new: &json::Incident,
    author: &str,
    normalize_addresses: bool,
) -> HResult<()> {
    reject_set_replacement(new).map_err(|e| e.from_src("[rejectSetReplacement]"))?;
    for _ in 0..MAX_CAS_ATTEMPTS {
        let conflict =
            retry_on_deadlock(|| update_incident_attempt(st, new, author, normalize_addresses))
                .await
                .map_err(|e| e.from_src("[updateIncidentAttempt]"))?;
        if !conflict {
            return Ok(());
        }
    }
    Err(HttpError::conflict(
        "The incident is being modified concurrently. Please try again.",
        anyhow::anyhow!("version conflict"),
    ))
}

async fn update_incident_attempt(
    st: &AppState,
    new: &json::Incident,
    author: &str,
    normalize_addresses: bool,
) -> HResult<bool> {
    let stored = match queries::incident(&st.db, new.event_id, new.number).await {
        Ok(Some(row)) => row.incident,
        Ok(None) => {
            return Err(HttpError::not_found(
                "Incident not found",
                anyhow::anyhow!("sql: no rows in result set"),
            )
            .from_src("[Incident]"));
        }
        Err(e) => {
            return Err(
                HttpError::internal_server_error("Failed to fetch incident", e)
                    .from_src("[Incident]"),
            );
        }
    };
    let mut txn = st
        .db
        .begin()
        .await
        .map_err(db_err("Failed to start transaction", "[Begin]"))?;

    let mut update = stored.clone();
    let mut logs: Vec<String> = Vec::new();
    if new.priority != 0 {
        update.priority = new.priority;
        logs.push(format!("Changed priority: {}", update.priority));
    }
    if INCIDENT_STATES.contains(&new.state.as_str()) {
        update.state = new.state.clone();
        logs.push(format!("Changed state: {}", update.state));
        update.closed = if new.state == "closed" {
            Some(GoTime::now().to_float())
        } else {
            None
        };
    }
    if !new.started.is_zero() {
        update.started = new.started.to_float();
        logs.push(format!("Changed start time: {}", rfc3339_utc(&new.started)));
    }
    apply_string_change(
        &mut update.summary,
        new.summary.as_deref(),
        "summary",
        &mut logs,
    );
    apply_string_change(
        &mut update.location_name,
        new.location.name.as_deref(),
        "location name",
        &mut logs,
    );
    let address = normalized_address(new.location.address.as_deref(), normalize_addresses);
    apply_string_change(
        &mut update.location_address,
        address.as_deref(),
        "location address",
        &mut logs,
    );
    apply_string_change(
        &mut update.location_description,
        new.location.description.as_deref(),
        "location description",
        &mut logs,
    );

    // A request that only appends report entries skips the guarded update:
    // appends can't lose data, so they mustn't conflict with field edits.
    if !logs.is_empty() {
        let rows = queries::update_incident(&mut *txn, &update)
            .await
            .map_err(db_err("Failed to update incident", "[UpdateIncident]"))?;
        if rows == 0 {
            // Stale version or vanished row; re-read to tell which.
            return match queries::incident_version(&st.db, new.event_id, new.number).await {
                Ok(Some(_)) => Ok(true),
                Ok(None) => Err(HttpError::not_found(
                    "Incident not found",
                    anyhow::anyhow!("sql: no rows in result set"),
                )
                .from_src("[IncidentVersion]")),
                Err(e) => Err(
                    HttpError::internal_server_error("Failed to fetch incident", e)
                        .from_src("[IncidentVersion]"),
                ),
            };
        }
    }
    add_change_report_entries(
        &mut txn,
        Parent::Incident,
        new.event_id,
        new.number,
        author,
        &logs,
        new.report_entries.as_deref(),
    )
    .await
    .map_err(|e| e.from_src("[addChangeReportEntries]"))?;
    txn.commit()
        .await
        .map_err(db_err("Failed to commit transaction", "[Commit]"))?;
    st.es.notify_incident_update(new.event_id, new.number);
    Ok(false)
}

/// Go's `time.RFC3339` layout, in UTC.
pub fn rfc3339_utc(t: &GoTime) -> String {
    t.0.format("%Y-%m-%dT%H:%M:%SZ").to_string()
}

/// Overwrites `dst` if the client sent a new value, and records a
/// "Changed <label>: <value>" line. An empty string clears the field.
pub fn apply_string_change(
    dst: &mut Option<String>,
    new_val: Option<&str>,
    label: &str,
    logs: &mut Vec<String>,
) {
    let Some(v) = new_val else { return };
    *dst = string_to_sql(Some(v), 0);
    logs.push(format!(
        "Changed {label}: {}",
        dst.as_deref().unwrap_or_default()
    ));
}

/// Puts a client-supplied BRC address into canonical form, when the event
/// asks for that.
pub fn normalized_address(address: Option<&str>, normalize: bool) -> Option<String> {
    match address {
        Some(a) if normalize => Some(normalize_address(a)),
        other => other.map(str::to_string),
    }
}
