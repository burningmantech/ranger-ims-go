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

//! Sanctuary Visits. Ported from the Go server's `api/visit.go`.

use std::collections::HashMap;

use axum::Extension;
use axum::body::Body;
use axum::extract::{Path, Query, State};
use axum::http::{HeaderMap, HeaderValue, header};
use axum::response::Response;

use super::helpers::*;
use super::incident::{
    EntriesQuery, MAX_CAS_ATTEMPTS, apply_string_change, location_path, normalized_address,
    rfc3339_utc,
};
use super::middleware::JwtCtx;
use super::reportentry::{Parent, add_change_report_entries, path_i32, report_entry_to_json};
use super::state::{AppState, S};
use crate::authz;
use crate::json;
use crate::store::models::{Event, ReportEntry, Visit, VisitRanger};
use crate::store::queries;
use crate::util::conv::GoTime;
use crate::util::herr::{self, HttpError};

pub async fn get_visits(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(event_name): Path<String>,
    Query(q): Query<EntriesQuery>,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &event_name)
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & authz::EVENT_READ_VISITS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventReadVisits permission",
        ));
    }
    let (entries, rangers, visits) = tokio::join!(
        queries::visits_report_entries(&st.db, event.id, q.include_system_entries()),
        queries::visits_rangers(&st.db, event.id),
        queries::visits(&st.db, event.id),
    );
    let entries = entries.map_err(db_err(
        "Failed to fetch Visit Report Entries",
        "[Visits_ReportEntries]",
    ))?;
    let rangers = rangers.map_err(db_err("Failed to fetch rangers", "[Visits_Rangers]"))?;
    let visits = visits.map_err(db_err("Failed to fetch Visits", "[Visits]"))?;
    let mut entries_by_visit: HashMap<i32, Vec<ReportEntry>> = HashMap::new();
    for row in entries {
        entries_by_visit
            .entry(row.visit_number)
            .or_default()
            .push(row.report_entry);
    }
    let mut rangers_by_visit: HashMap<i32, Vec<VisitRanger>> = HashMap::new();
    for r in rangers {
        rangers_by_visit.entry(r.visit_number).or_default().push(r);
    }
    let resp: Vec<json::Visit> = visits
        .iter()
        .map(|v| {
            visit_to_json(
                v,
                rangers_by_visit
                    .get(&v.number)
                    .map(Vec::as_slice)
                    .unwrap_or_default(),
                entries_by_visit
                    .get(&v.number)
                    .map(Vec::as_slice)
                    .unwrap_or_default(),
                &event,
                st.attachments_enabled(),
            )
        })
        .collect();
    Ok(json_response(&resp))
}

pub async fn get_visit(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & authz::EVENT_READ_VISITS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventReadVisits permission on this Event",
        ));
    }
    let number = path_i32(&p, "visitNumber", "Failed to parse visit number")?;
    let (visit, entries) = fetch_visit(&st, event.id, number)
        .await
        .map_err(|e| e.from_src("[fetchVisit]"))?;
    let rangers = queries::visit_rangers(&st.db, event.id, number)
        .await
        .map_err(|e| HttpError::internal_server_error("Failed to fetch rangers", e))?;
    Ok(json_response(&visit_to_json(
        &visit,
        &rangers,
        &entries,
        &event,
        st.attachments_enabled(),
    )))
}

pub async fn fetch_visit(
    st: &AppState,
    event_id: i32,
    number: i32,
) -> HResult<(Visit, Vec<ReportEntry>)> {
    let visit = match queries::visit(&st.db, event_id, number).await {
        Ok(Some(v)) => v,
        Ok(None) => {
            return Err(HttpError::not_found(
                "Visit not found",
                anyhow::anyhow!("sql: no rows in result set"),
            )
            .from_src("[Visit]"));
        }
        Err(e) => {
            return Err(
                HttpError::internal_server_error("Failed to fetch Visit", e).from_src("[Visit]")
            );
        }
    };
    let entries = queries::visit_report_entries(&st.db, event_id, number)
        .await
        .map_err(db_err(
            "Failed to fetch report entries",
            "[Visit_ReportEntries]",
        ))?
        .into_iter()
        .map(|r| r.report_entry)
        .collect();
    Ok((visit, entries))
}

fn visit_to_json(
    v: &Visit,
    rangers: &[VisitRanger],
    entries: &[ReportEntry],
    event: &Event,
    attachments_enabled: bool,
) -> json::Visit {
    let result_entries: Vec<json::ReportEntry> = entries
        .iter()
        .map(|re| report_entry_to_json(re, attachments_enabled))
        .collect();
    let created = GoTime::from_float(v.created);
    let last_modified = result_entries
        .iter()
        .map(|re| re.created)
        .fold(created, |a, b| if b.after(&a) { b } else { a });
    json::Visit {
        event: event.name.clone(),
        event_id: event.id,
        number: v.number,
        created,
        last_modified,
        version: v.version,
        incident: v.incident_number,
        guest_preferred_name: v.guest_preferred_name.clone(),
        guest_legal_name: v.guest_legal_name.clone(),
        guest_description: v.guest_description.clone(),
        guest_action_plan: v.guest_action_plan.clone(),
        guest_camp_name: v.guest_camp_name.clone(),
        guest_camp_address: v.guest_camp_address.clone(),
        guest_camp_description: v.guest_camp_description.clone(),
        guest_camp_contacts: v.guest_camp_contacts.clone(),
        arrival_time: v.arrival_time.map(GoTime::from_float),
        arrival_method: v.arrival_method.clone(),
        arrival_state: v.arrival_state.clone(),
        arrival_reason: v.arrival_reason.clone(),
        arrival_belongings: v.arrival_belongings.clone(),
        departure_time: v.departure_time.map(GoTime::from_float),
        departure_method: v.departure_method.clone(),
        departure_state: v.departure_state.clone(),
        resource_sitter: v.resource_sitter.clone(),
        resource_bed_id: v.resource_bed_id.clone(),
        resource_rest: v.resource_rest.clone(),
        resource_clothes: v.resource_clothes.clone(),
        resource_pogs: v.resource_pogs.clone(),
        resource_food_bev: v.resource_food_bev.clone(),
        resource_other: v.resource_other.clone(),
        rangers: Some(
            rangers
                .iter()
                .map(|r| json::VisitRanger {
                    handle: r.ranger_handle.clone(),
                    role: r.role.clone(),
                })
                .collect(),
        ),
        report_entries: Some(result_entries),
    }
}

pub async fn new_visit(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(event_name): Path<String>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &event_name)
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & authz::EVENT_WRITE_VISITS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventWriteVisits permission on this Event",
        ));
    }
    let mut new: json::Visit = read_body_as(&headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    let author = claims_of(&jwt)?.ranger_handle().to_string();
    let now = GoTime::now().to_float();
    let mut attempt = 1;
    let number = loop {
        let number = queries::next_visit_number(&st.db, event.id)
            .await
            .map_err(db_err(
                "Failed to find next Visit number",
                "[NextVisitNumber]",
            ))? as i32;
        match queries::create_visit(&st.db, event.id, number, now).await {
            Ok(()) => break number,
            Err(e) if is_duplicate_key_error(&e) => {
                if attempt == MAX_NUMBER_ALLOC_ATTEMPTS {
                    return Err(HttpError::conflict(
                        "Visits are being created concurrently. Please try again.",
                        e,
                    )
                    .from_src("[CreateVisit]"));
                }
                attempt += 1;
            }
            Err(e) => {
                return Err(
                    HttpError::internal_server_error("Failed to create visit", e)
                        .from_src("[CreateVisit]"),
                );
            }
        }
    };
    new.event_id = event.id;
    new.event = event.name.clone();
    new.number = number;
    update_visit(&st, &new, &author, event.normalize_addresses)
        .await
        .map_err(|e| e.from_src("[updateVisit]").from_src("[newVisit]"))?;
    let mut resp = herr::created_response("Created");
    resp.headers_mut()
        .insert("IMS-Visit-Number", HeaderValue::from(number));
    if let Ok(v) = HeaderValue::from_str(&location_path(&event.name, "visits", number)) {
        resp.headers_mut().insert(header::LOCATION, v);
    }
    Ok(resp)
}

pub async fn edit_visit(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & authz::EVENT_WRITE_VISITS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventWriteVisits permission for this Event",
        ));
    }
    let number = path_i32(&p, "visitNumber", "Invalid Visit Number")?;
    let mut new: json::Visit = read_body_as(&headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    new.event = event.name.clone();
    new.event_id = event.id;
    new.number = number;
    let author = claims_of(&jwt)?.ranger_handle().to_string();
    update_visit(&st, &new, &author, event.normalize_addresses)
        .await
        .map_err(|e| e.from_src("[updateVisit]").from_src("[editVisit]"))?;
    Ok(herr::no_content_response("Success"))
}

async fn update_visit(
    st: &AppState,
    new: &json::Visit,
    author: &str,
    normalize_addresses: bool,
) -> HResult<()> {
    for _ in 0..MAX_CAS_ATTEMPTS {
        let conflict =
            retry_on_deadlock(|| update_visit_attempt(st, new, author, normalize_addresses))
                .await
                .map_err(|e| e.from_src("[updateVisitAttempt]"))?;
        if !conflict {
            return Ok(());
        }
    }
    Err(HttpError::conflict(
        "The visit is being modified concurrently. Please try again.",
        anyhow::anyhow!("version conflict"),
    ))
}

async fn update_visit_attempt(
    st: &AppState,
    new: &json::Visit,
    author: &str,
    normalize_addresses: bool,
) -> HResult<bool> {
    let stored = match queries::visit(&st.db, new.event_id, new.number).await {
        Ok(Some(v)) => v,
        Ok(None) => {
            return Err(HttpError::not_found(
                "Visit not found",
                anyhow::anyhow!("sql: no rows in result set"),
            )
            .from_src("[Visit]"));
        }
        Err(e) => {
            return Err(
                HttpError::internal_server_error("Failed to fetch visit", e).from_src("[Visit]")
            );
        }
    };
    let mut txn = st
        .db
        .begin()
        .await
        .map_err(db_err("Failed to start transaction", "[Begin]"))?;
    let (update, logs) = build_visit_update(&stored, new, normalize_addresses)
        .map_err(|e| e.from_src("[buildVisitUpdate]"))?;
    if !logs.is_empty() {
        let rows = match queries::update_visit(&mut *txn, &update).await {
            Ok(rows) => rows,
            Err(e) if sqlx_error_number(&e) == Some(1452) => {
                return Err(HttpError::not_found("No such Incident", e).from_src("[UpdateVisit]"));
            }
            Err(e) => {
                return Err(
                    HttpError::internal_server_error("Failed to update visit", e)
                        .from_src("[UpdateVisit]"),
                );
            }
        };
        if rows == 0 {
            return match queries::visit_version(&st.db, new.event_id, new.number).await {
                Ok(Some(_)) => Ok(true),
                Ok(None) => Err(HttpError::not_found(
                    "Visit not found",
                    anyhow::anyhow!("sql: no rows in result set"),
                )
                .from_src("[VisitVersion]")),
                Err(e) => Err(HttpError::internal_server_error("Failed to fetch visit", e)
                    .from_src("[VisitVersion]")),
            };
        }
    }
    add_change_report_entries(
        &mut txn,
        Parent::Visit,
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
    st.es.notify_visit_update(stored.event, stored.number);
    st.es.notify_incident_updates(
        stored.event,
        stored.incident_number.unwrap_or(0),
        update.incident_number.unwrap_or(0),
    );
    Ok(false)
}

fn visit_time_log_value(t: &GoTime) -> String {
    if t.is_zero() {
        "(cleared)".to_string()
    } else {
        rfc3339_utc(t)
    }
}

fn build_visit_update(
    stored: &Visit,
    new: &json::Visit,
    normalize_addresses: bool,
) -> HResult<(Visit, Vec<String>)> {
    let mut u = stored.clone();
    let mut logs = Vec::new();
    if let Some(inc) = new.incident {
        u.incident_number = (inc > 0).then_some(inc);
        if inc > 0 {
            logs.push(format!("Changed incident number: {inc}"));
        } else {
            logs.push("Changed incident number: (unassigned)".to_string());
        }
    }
    apply_string_change(
        &mut u.guest_preferred_name,
        new.guest_preferred_name.as_deref(),
        "GuestPreferredName",
        &mut logs,
    );
    apply_string_change(
        &mut u.guest_legal_name,
        new.guest_legal_name.as_deref(),
        "GuestLegalName",
        &mut logs,
    );
    apply_string_change(
        &mut u.guest_description,
        new.guest_description.as_deref(),
        "GuestDescription",
        &mut logs,
    );
    apply_string_change(
        &mut u.guest_action_plan,
        new.guest_action_plan.as_deref(),
        "GuestActionPlan",
        &mut logs,
    );
    apply_string_change(
        &mut u.resource_bed_id,
        new.resource_bed_id.as_deref(),
        "ResourceBedID",
        &mut logs,
    );
    apply_string_change(
        &mut u.guest_camp_name,
        new.guest_camp_name.as_deref(),
        "GuestCampName",
        &mut logs,
    );
    let camp_address = normalized_address(new.guest_camp_address.as_deref(), normalize_addresses);
    apply_string_change(
        &mut u.guest_camp_address,
        camp_address.as_deref(),
        "GuestCampAddress",
        &mut logs,
    );
    apply_string_change(
        &mut u.guest_camp_description,
        new.guest_camp_description.as_deref(),
        "GuestCampDescription",
        &mut logs,
    );
    apply_string_change(
        &mut u.guest_camp_contacts,
        new.guest_camp_contacts.as_deref(),
        "GuestCampContacts",
        &mut logs,
    );
    apply_string_change(
        &mut u.resource_sitter,
        new.resource_sitter.as_deref(),
        "ResourceSitter",
        &mut logs,
    );
    if let Some(t) = &new.arrival_time {
        u.arrival_time = t.to_opt_float();
        logs.push(format!("Changed ArrivalTime: {}", visit_time_log_value(t)));
    }
    apply_string_change(
        &mut u.arrival_method,
        new.arrival_method.as_deref(),
        "ArrivalMethod",
        &mut logs,
    );
    apply_string_change(
        &mut u.arrival_state,
        new.arrival_state.as_deref(),
        "ArrivalState",
        &mut logs,
    );
    apply_string_change(
        &mut u.arrival_reason,
        new.arrival_reason.as_deref(),
        "ArrivalReason",
        &mut logs,
    );
    apply_string_change(
        &mut u.arrival_belongings,
        new.arrival_belongings.as_deref(),
        "ArrivalBelongings",
        &mut logs,
    );
    if let Some(t) = &new.departure_time {
        u.departure_time = t.to_opt_float();
        logs.push(format!(
            "Changed DepartureTime: {}",
            visit_time_log_value(t)
        ));
    }
    if let (Some(arrival), Some(departure)) = (u.arrival_time, u.departure_time)
        && departure < arrival
    {
        let msg = if new.departure_time.is_none() {
            "Arrival time cannot be after departure time"
        } else {
            "Departure time cannot be before arrival time"
        };
        return Err(HttpError::bad_request_msg(msg));
    }
    apply_string_change(
        &mut u.departure_method,
        new.departure_method.as_deref(),
        "DepartureMethod",
        &mut logs,
    );
    apply_string_change(
        &mut u.departure_state,
        new.departure_state.as_deref(),
        "DepartureState",
        &mut logs,
    );
    apply_string_change(
        &mut u.resource_rest,
        new.resource_rest.as_deref(),
        "ResourceRest",
        &mut logs,
    );
    apply_string_change(
        &mut u.resource_clothes,
        new.resource_clothes.as_deref(),
        "ResourceClothes",
        &mut logs,
    );
    apply_string_change(
        &mut u.resource_pogs,
        new.resource_pogs.as_deref(),
        "ResourcePogs",
        &mut logs,
    );
    apply_string_change(
        &mut u.resource_food_bev,
        new.resource_food_bev.as_deref(),
        "ResourceFoodBev",
        &mut logs,
    );
    apply_string_change(
        &mut u.resource_other,
        new.resource_other.as_deref(),
        "ResourceOther",
        &mut logs,
    );
    Ok((u, logs))
}
