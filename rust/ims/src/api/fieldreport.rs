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

//! Field Reports. Ported from the Go server's `api/fieldreport.go`.

use std::collections::HashMap;

use axum::Extension;
use axum::body::Body;
use axum::extract::{Path, Query, State};
use axum::http::{HeaderMap, HeaderValue, header};
use axum::response::Response;
use serde::Deserialize;

use super::helpers::*;
use super::incident::{EntriesQuery, MAX_CAS_ATTEMPTS, location_path};
use super::middleware::JwtCtx;
use super::reportentry::{
    NewReportEntry, Parent, add_change_report_entries, add_report_entry, path_i32,
    report_entry_to_json,
};
use super::state::{AppState, S};
use crate::authz;
use crate::json;
use crate::store::models::{Event, FieldReport, ReportEntry};
use crate::store::queries;
use crate::util::conv::{GoTime, string_to_sql};
use crate::util::herr::{self, HttpError};

pub fn contains_author(entries: &[ReportEntry], author: &str) -> bool {
    entries.iter().any(|e| e.author == author)
}

pub async fn get_field_reports(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(event_name): Path<String>,
    Query(q): Query<EntriesQuery>,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &event_name)
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & (authz::EVENT_READ_ALL_FIELD_REPORTS | authz::EVENT_READ_OWN_FIELD_REPORTS) == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have permission to read Field Reports on this Event",
        ));
    }
    let limited_access = perms & authz::EVENT_READ_ALL_FIELD_REPORTS == 0;
    let entries =
        queries::field_reports_report_entries(&st.db, event.id, q.include_system_entries())
            .await
            .map_err(db_err(
                "Failed to get FR report entries",
                "[FieldReports_ReportEntries]",
            ))?;
    let mut entries_by_fr: HashMap<i32, Vec<ReportEntry>> = HashMap::new();
    for row in entries {
        entries_by_fr
            .entry(row.field_report_number)
            .or_default()
            .push(row.report_entry);
    }
    let frs = queries::field_reports(&st.db, event.id)
        .await
        .map_err(db_err("Failed to fetch Field Reports", "[FieldReports]"))?;
    let handle = claims_of(&jwt)?.ranger_handle().to_string();
    let resp: Vec<json::FieldReport> = frs
        .iter()
        .filter(|fr| {
            !limited_access
                || contains_author(
                    entries_by_fr
                        .get(&fr.number)
                        .map(Vec::as_slice)
                        .unwrap_or_default(),
                    &handle,
                )
        })
        .map(|fr| {
            field_report_to_json(
                fr,
                entries_by_fr
                    .get(&fr.number)
                    .map(Vec::as_slice)
                    .unwrap_or_default(),
                &event,
                st.attachments_enabled(),
            )
        })
        .collect();
    Ok(json_response(&resp))
}

pub async fn get_field_report(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & (authz::EVENT_READ_ALL_FIELD_REPORTS | authz::EVENT_READ_OWN_FIELD_REPORTS) == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have permission to read Field Reports on this Event",
        ));
    }
    let limited_access = perms & authz::EVENT_READ_ALL_FIELD_REPORTS == 0;
    let number = path_i32(&p, "fieldReportNumber", "Invalid field report number")?;
    let (fr, entries) = fetch_field_report(&st, event.id, number)
        .await
        .map_err(|e| e.from_src("[fetchFieldReport]"))?;
    if limited_access && !contains_author(&entries, claims_of(&jwt)?.ranger_handle()) {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have permission to access this particular Field Report",
        ));
    }
    Ok(json_response(&field_report_to_json(
        &fr,
        &entries,
        &event,
        st.attachments_enabled(),
    )))
}

fn field_report_to_json(
    fr: &FieldReport,
    entries: &[ReportEntry],
    event: &Event,
    attachments_enabled: bool,
) -> json::FieldReport {
    json::FieldReport {
        event: event.name.clone(),
        number: fr.number,
        created: GoTime::from_float(fr.created),
        version: fr.version,
        summary: fr.summary.clone(),
        incident: fr.incident_number,
        report_entries: Some(
            entries
                .iter()
                .map(|re| report_entry_to_json(re, attachments_enabled))
                .collect(),
        ),
    }
}

pub async fn fetch_field_report(
    st: &AppState,
    event_id: i32,
    number: i32,
) -> HResult<(FieldReport, Vec<ReportEntry>)> {
    let fr = match queries::field_report(&st.db, event_id, number).await {
        Ok(Some(fr)) => fr,
        Ok(None) => {
            return Err(HttpError::not_found(
                "Field Report does not exist",
                anyhow::anyhow!("sql: no rows in result set"),
            )
            .from_src("[FieldReport]"));
        }
        Err(e) => {
            return Err(
                HttpError::internal_server_error("Failed to fetch Field Report", e)
                    .from_src("[FieldReport]"),
            );
        }
    };
    let entries = queries::field_report_report_entries(&st.db, event_id, number)
        .await
        .map_err(db_err(
            "Failed to fetch Report Entries",
            "[FieldReport_ReportEntries]",
        ))?;
    Ok((fr, entries))
}

#[derive(Deserialize)]
pub struct EditFieldReportQuery {
    #[serde(default)]
    action: String,
    #[serde(default)]
    incident: String,
}

pub async fn edit_field_report(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    Query(q): Query<EditFieldReportQuery>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & (authz::EVENT_WRITE_ALL_FIELD_REPORTS | authz::EVENT_WRITE_OWN_FIELD_REPORTS) == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have permission to edit Field Reports on this Event",
        ));
    }
    let limited_access = perms & authz::EVENT_WRITE_ALL_FIELD_REPORTS == 0;
    let number = path_i32(&p, "fieldReportNumber", "Invalid field report number")?;
    let author = claims_of(&jwt)?.ranger_handle().to_string();
    if limited_access {
        let entries = queries::field_report_report_entries(&st.db, event.id, number)
            .await
            .map_err(db_err(
                "Failed to fetch Field Report ReportEntries",
                "[FieldReport_ReportEntries]",
            ))?;
        if !contains_author(&entries, &author) {
            return Err(HttpError::forbidden_msg(
                "The requestor does not have permission to edit this Field Report",
            ));
        }
    }
    let stored = match queries::field_report(&st.db, event.id, number).await {
        Ok(Some(fr)) => fr,
        Ok(None) => {
            return Err(HttpError::not_found(
                "Field Report does not exist",
                anyhow::anyhow!("sql: no rows in result set"),
            )
            .from_src("[FieldReport]"));
        }
        Err(e) => {
            return Err(
                HttpError::internal_server_error("Failed to fetch Field Report", e)
                    .from_src("[FieldReport]"),
            );
        }
    };

    // An "action" links or unlinks this Field Report and an Incident.
    if !q.action.is_empty() {
        handle_link_to_incident(&st, &stored, &event, &q.action, &q.incident, &author)
            .await
            .map_err(|e| e.from_src("[handleLinkToIncident]"))?;
    }

    let request: json::FieldReport = read_body_as(&headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    // That's fine: it may be that only a link/unlink was requested.
    if request.number == 0 {
        return Ok(herr::no_content_response("Success"));
    }
    for _ in 0..MAX_CAS_ATTEMPTS {
        let conflict = retry_on_deadlock(|| {
            update_field_report_attempt(&st, &event, number, &request, &author)
        })
        .await
        .map_err(|e| e.from_src("[updateFieldReportAttempt]"))?;
        if !conflict {
            return Ok(herr::no_content_response("Success"));
        }
    }
    Err(HttpError::conflict(
        "The field report is being modified concurrently. Please try again.",
        anyhow::anyhow!("version conflict"),
    ))
}

async fn update_field_report_attempt(
    st: &AppState,
    event: &Event,
    number: i32,
    request: &json::FieldReport,
    author: &str,
) -> HResult<bool> {
    let mut stored = match queries::field_report(&st.db, event.id, number).await {
        Ok(Some(fr)) => fr,
        Ok(None) => {
            return Err(HttpError::not_found(
                "Field Report does not exist",
                anyhow::anyhow!("sql: no rows in result set"),
            )
            .from_src("[FieldReport]"));
        }
        Err(e) => {
            return Err(
                HttpError::internal_server_error("Failed to fetch Field Report", e)
                    .from_src("[FieldReport]"),
            );
        }
    };
    let mut txn = st
        .db
        .begin()
        .await
        .map_err(db_err("Failed to begin transaction", "[Begin]"))?;
    let mut logs = Vec::new();
    if let Some(summary) = &request.summary {
        stored.summary = string_to_sql(Some(summary), 0);
        logs.push(format!("Changed summary to: {summary}"));
    }
    if !logs.is_empty() {
        let rows = queries::update_field_report(&mut *txn, &stored)
            .await
            .map_err(db_err(
                "Failed to update Field Report",
                "[UpdateFieldReport]",
            ))?;
        if rows == 0 {
            return match queries::field_report_version(&st.db, event.id, number).await {
                Ok(Some(_)) => Ok(true),
                Ok(None) => Err(HttpError::not_found(
                    "Field Report does not exist",
                    anyhow::anyhow!("sql: no rows in result set"),
                )
                .from_src("[FieldReportVersion]")),
                Err(e) => Err(
                    HttpError::internal_server_error("Failed to fetch Field Report", e)
                        .from_src("[FieldReportVersion]"),
                ),
            };
        }
    }
    add_change_report_entries(
        &mut txn,
        Parent::FieldReport,
        event.id,
        stored.number,
        author,
        &logs,
        request.report_entries.as_deref(),
    )
    .await
    .map_err(|e| e.from_src("[addChangeReportEntries]"))?;
    txn.commit()
        .await
        .map_err(db_err("Failed to commit transaction", "[Commit]"))?;
    st.es.notify_field_report_update(event.id, stored.number);
    Ok(false)
}

async fn handle_link_to_incident(
    st: &AppState,
    stored: &FieldReport,
    event: &Event,
    action: &str,
    target_incident: &str,
    actor: &str,
) -> HResult<()> {
    let previous = stored.incident_number;
    let (new_incident, entry_text) = match action {
        "attach" => {
            let num: i32 = target_incident.parse().map_err(|e| {
                HttpError::bad_request(
                    "Invalid incident number for attachment of FR",
                    anyhow::anyhow!("{e}"),
                )
                .from_src("[ParseInt32]")
            })?;
            (Some(num), format!("Attached to incident: {num}"))
        }
        "detach" => (
            None,
            format!("Detached from incident: {}", previous.unwrap_or(0)),
        ),
        _ => {
            return Err(HttpError::bad_request(
                "Invalid action",
                anyhow::anyhow!("provided bad action was {action}"),
            ));
        }
    };
    if let Err(e) =
        queries::attach_field_report_to_incident(&st.db, new_incident, event.id, stored.number)
            .await
    {
        const ER_NO_REFERENCED_ROW_2: u16 = 1452;
        if sqlx_error_number(&e) == Some(ER_NO_REFERENCED_ROW_2) {
            return Err(HttpError::not_found("No such Incident", e)
                .from_src("[AttachFieldReportToIncident]"));
        }
        return Err(HttpError::internal_server_error(
            "Failed to attach Field Report to incident",
            e,
        )
        .from_src("[AttachFieldReportToIncident]"));
    }
    let mut conn = st.db.acquire().await.map_err(db_err(
        "Failed to attach Field Report to incident",
        "[Acquire]",
    ))?;
    add_report_entry(
        &mut conn,
        Parent::FieldReport,
        event.id,
        stored.number,
        &NewReportEntry::generated(actor, entry_text),
    )
    .await
    .map_err(|e| e.from_src("[addFRReportEntry]"))?;
    st.es
        .notify_incident_updates(event.id, previous.unwrap_or(0), new_incident.unwrap_or(0));
    st.es.notify_field_report_update(event.id, stored.number);
    tracing::info!(event = event.id, new_incident = ?new_incident, previous_incident = ?previous, field_report = stored.number, "Attached Field Report to newIncident");
    Ok(())
}

pub async fn new_field_report(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(event_name): Path<String>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &event_name)
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & (authz::EVENT_WRITE_ALL_FIELD_REPORTS | authz::EVENT_WRITE_OWN_FIELD_REPORTS) == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have permission to write Field Reports on this Event",
        ));
    }
    let fr: json::FieldReport = read_body_as(&headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    if fr.incident.is_some() {
        return Err(HttpError::bad_request_msg(
            "A new Field Report may not be attached to an incident",
        ));
    }
    let author = claims_of(&jwt)?.ranger_handle().to_string();

    // The number allocation can race a concurrent creator; see new_incident.
    let created = GoTime::now().to_float();
    let mut attempt = 1;
    let number = loop {
        let number = queries::next_field_report_number(&st.db, event.id)
            .await
            .map_err(db_err(
                "Failed to find next Field Report number",
                "[NextFieldReportNumber]",
            ))? as i32;
        let summary = string_to_sql(fr.summary.as_deref(), 0);
        match queries::create_field_report(
            &st.db,
            event.id,
            number,
            created,
            summary.as_deref(),
            None,
        )
        .await
        {
            Ok(()) => break number,
            Err(e) if is_duplicate_key_error(&e) => {
                if attempt == MAX_NUMBER_ALLOC_ATTEMPTS {
                    return Err(HttpError::conflict(
                        "Field Reports are being created concurrently. Please try again.",
                        e,
                    )
                    .from_src("[CreateFieldReport]"));
                }
                attempt += 1;
            }
            Err(e) => {
                return Err(
                    HttpError::internal_server_error("Failed to create Field Report", e)
                        .from_src("[CreateFieldReport]"),
                );
            }
        }
    };

    let st_ref = &st;
    let fr_ref = &fr;
    let author_ref = author.as_str();
    retry_on_deadlock(|| async move {
        let mut txn = st_ref
            .db
            .begin()
            .await
            .map_err(db_err("Failed to begin transaction", "[Begin]"))?;
        let logs: Vec<String> = fr_ref
            .summary
            .iter()
            .map(|s| format!("Changed summary to: {s}"))
            .collect();
        add_change_report_entries(
            &mut txn,
            Parent::FieldReport,
            event.id,
            number,
            author_ref,
            &logs,
            fr_ref.report_entries.as_deref(),
        )
        .await
        .map_err(|e| e.from_src("[addChangeReportEntries]"))?;
        txn.commit()
            .await
            .map_err(db_err("Failed to commit transaction", "[Commit]"))?;
        Ok(())
    })
    .await?;

    st.es.notify_field_report_update(event.id, number);
    let mut resp = herr::created_response("Created");
    resp.headers_mut()
        .insert("IMS-Field-Report-Number", HeaderValue::from(number));
    if let Ok(v) = HeaderValue::from_str(&location_path(&event.name, "field_reports", number)) {
        resp.headers_mut().insert(header::LOCATION, v);
    }
    Ok(resp)
}
