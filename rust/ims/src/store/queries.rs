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

//! Typed wrappers around the queries in `store/queries.sql`, one per sqlc
//! `-- name:`, named as sqlc names its Go methods but in snake_case.

use std::collections::HashMap;
use std::sync::LazyLock;

use sqlx::MySqlExecutor;

use super::models::*;
use super::query::Query;
use super::sqlfile::{ParsedQuery, parse_file};

static QUERIES: LazyLock<HashMap<String, ParsedQuery>> =
    LazyLock::new(|| parse_file(include_str!("../../../../store/queries.sql")));

pub fn q(name: &str) -> Query {
    Query::new(
        QUERIES
            .get(name)
            .unwrap_or_else(|| panic!("no query named {name} in store/queries.sql")),
    )
}

type Result<T> = std::result::Result<T, sqlx::Error>;

pub async fn schema_version<'e>(db: impl MySqlExecutor<'e>) -> Result<i16> {
    q("SchemaVersion").fetch_one_scalar(db).await
}

pub async fn directory_active_persons<'e>(
    db: impl MySqlExecutor<'e>,
) -> Result<Vec<DirectoryActivePerson>> {
    q("DirectoryActivePersons").fetch_all(db).await
}

pub async fn directory_active_positions<'e>(
    db: impl MySqlExecutor<'e>,
) -> Result<Vec<DirectoryTitled>> {
    q("DirectoryActivePositions").fetch_all(db).await
}

pub async fn directory_active_teams<'e>(
    db: impl MySqlExecutor<'e>,
) -> Result<Vec<DirectoryTitled>> {
    q("DirectoryActiveTeams").fetch_all(db).await
}

pub async fn directory_person_positions<'e>(
    db: impl MySqlExecutor<'e>,
) -> Result<Vec<DirectoryPersonPosition>> {
    q("DirectoryPersonPositions").fetch_all(db).await
}

pub async fn directory_person_teams<'e>(
    db: impl MySqlExecutor<'e>,
) -> Result<Vec<DirectoryPersonTeam>> {
    q("DirectoryPersonTeams").fetch_all(db).await
}

pub async fn query_event_id<'e>(db: impl MySqlExecutor<'e>, name: &str) -> Result<Option<Event>> {
    q("QueryEventID").arg(name).fetch_optional(db).await
}

pub async fn event<'e>(db: impl MySqlExecutor<'e>, id: i32) -> Result<Option<Event>> {
    q("Event").arg(id).fetch_optional(db).await
}

pub async fn events<'e>(db: impl MySqlExecutor<'e>) -> Result<Vec<Event>> {
    q("Events").fetch_all(db).await
}

pub async fn event_and_parent_access<'e>(
    db: impl MySqlExecutor<'e>,
    event_id: i32,
) -> Result<Vec<EventAccess>> {
    q("EventAndParentAccess")
        .named("event_id", event_id)
        .fetch_all(db)
        .await
}

pub async fn event_access_all<'e>(db: impl MySqlExecutor<'e>) -> Result<Vec<EventAccess>> {
    q("EventAccessAll").fetch_all(db).await
}

pub async fn add_action_log<'e>(db: impl MySqlExecutor<'e>, r: &ActionLog) -> Result<u64> {
    let res = q("AddActionLog")
        .arg(r.created_at)
        .arg(r.action_type.as_str())
        .arg(r.method.as_ref())
        .arg(r.path.as_ref())
        .arg(r.referrer.as_ref())
        .arg(r.request_body.as_ref())
        .arg(r.user_id)
        .arg(r.user_name.as_ref())
        .arg(r.position_id)
        .arg(r.position_name.as_ref())
        .arg(r.client_address.as_ref())
        .arg(r.http_status)
        .arg(r.duration_micros)
        .execute(db)
        .await?;
    Ok(res.last_insert_id())
}

pub async fn action_logs<'e>(
    db: impl MySqlExecutor<'e>,
    min_time: f64,
    max_time: f64,
) -> Result<Vec<ActionLog>> {
    q("ActionLogs")
        .named("min_time", min_time)
        .named("max_time", max_time)
        .fetch_all(db)
        .await
}

pub async fn add_error_log<'e>(db: impl MySqlExecutor<'e>, r: &ErrorLog) -> Result<u64> {
    let res = q("AddErrorLog")
        .arg(r.created_at)
        .arg(r.http_status)
        .arg(r.response_message.as_ref())
        .arg(r.internal_error.as_ref())
        .arg(r.stack_trace.as_ref())
        .arg(r.method.as_ref())
        .arg(r.path.as_ref())
        .arg(r.referrer.as_ref())
        .arg(r.user_id)
        .arg(r.user_name.as_ref())
        .arg(r.position_id)
        .arg(r.position_name.as_ref())
        .arg(r.client_address.as_ref())
        .arg(r.duration_micros)
        .execute(db)
        .await?;
    Ok(res.last_insert_id())
}

pub async fn error_logs<'e>(
    db: impl MySqlExecutor<'e>,
    min_time: f64,
    max_time: f64,
) -> Result<Vec<ErrorLog>> {
    q("ErrorLogs")
        .named("min_time", min_time)
        .named("max_time", max_time)
        .fetch_all(db)
        .await
}

// ---- Events

pub async fn create_event<'e>(
    db: impl MySqlExecutor<'e>,
    name: &str,
    is_group: bool,
    parent_group: Option<i32>,
) -> Result<u64> {
    Ok(q("CreateEvent")
        .arg(name)
        .arg(is_group)
        .arg(parent_group)
        .execute(db)
        .await?
        .last_insert_id())
}

pub async fn event_one<'e>(db: impl MySqlExecutor<'e>, id: i32) -> Result<Event> {
    q("Event").arg(id).fetch_one(db).await
}

pub async fn update_event<'e>(db: impl MySqlExecutor<'e>, e: &Event) -> Result<()> {
    q("UpdateEvent")
        .arg(e.name.as_str())
        .arg(e.is_group)
        .arg(e.parent_group)
        .arg(e.map_url.as_ref())
        .arg(e.camp_locations_release)
        .arg(e.art_locations_release)
        .arg(e.map_url_release)
        .arg(e.normalize_addresses)
        .arg(e.id)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn event_report_entry_ids<'e>(
    db: impl MySqlExecutor<'e>,
    event_id: i32,
) -> Result<Vec<i32>> {
    q("EventReportEntryIDs")
        .named("event_id", event_id)
        .fetch_all_scalar(db)
        .await
}

/// Runs one of the single-argument `DeleteEvent*` queries.
pub async fn exec_by_event<'e>(
    db: impl MySqlExecutor<'e>,
    name: &str,
    event_id: i32,
) -> Result<()> {
    let query = q(name);
    let query = if name == "DeleteEventLinkedIncidents" {
        query.named("event_id", event_id)
    } else {
        query.arg(event_id)
    };
    query.execute(db).await?;
    Ok(())
}

pub async fn delete_report_entries<'e>(db: impl MySqlExecutor<'e>, ids: &[i32]) -> Result<()> {
    q("DeleteReportEntries")
        .slice("ids", ids.iter().copied())
        .execute(db)
        .await?;
    Ok(())
}

pub async fn detach_children_from_event_group<'e>(
    db: impl MySqlExecutor<'e>,
    parent: i32,
) -> Result<()> {
    q("DetachChildrenFromEventGroup")
        .arg(parent)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn delete_event<'e>(db: impl MySqlExecutor<'e>, id: i32) -> Result<()> {
    q("DeleteEvent").arg(id).execute(db).await?;
    Ok(())
}

// ---- Event access

pub async fn clear_event_access_for_mode<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    mode: &str,
) -> Result<()> {
    q("ClearEventAccessForMode")
        .arg(event)
        .arg(mode)
        .execute(db)
        .await?;
    Ok(())
}

#[allow(clippy::too_many_arguments)]
pub async fn add_event_access<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    expression: &str,
    mode: &str,
    validity: &str,
    not_after: Option<f64>,
    not_before: Option<f64>,
    description: &str,
) -> Result<()> {
    q("AddEventAccess")
        .arg(event)
        .arg(expression)
        .arg(mode)
        .arg(validity)
        .arg(not_after)
        .arg(not_before)
        .arg(description)
        .execute(db)
        .await?;
    Ok(())
}

// ---- Incident types

pub async fn incident_types<'e>(db: impl MySqlExecutor<'e>) -> Result<Vec<IncidentType>> {
    q("IncidentTypes").fetch_all(db).await
}

pub async fn incident_type<'e>(db: impl MySqlExecutor<'e>, id: i32) -> Result<IncidentType> {
    q("IncidentType").arg(id).fetch_one(db).await
}

pub async fn create_incident_type<'e>(
    db: impl MySqlExecutor<'e>,
    name: &str,
    hidden: bool,
) -> Result<u64> {
    Ok(q("CreateIncidentType")
        .arg(name)
        .arg(hidden)
        .execute(db)
        .await?
        .last_insert_id())
}

pub async fn update_incident_type<'e>(db: impl MySqlExecutor<'e>, t: &IncidentType) -> Result<()> {
    q("UpdateIncidentType")
        .arg(t.hidden)
        .arg(t.name.as_str())
        .arg(t.description.as_ref())
        .arg(t.id)
        .execute(db)
        .await?;
    Ok(())
}

// ---- Incidents

pub async fn create_incident<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
    created: f64,
    priority: i8,
    state: &str,
    started: f64,
) -> Result<()> {
    super::testhook::before(
        "CreateIncident",
        serde_json::json!({"event": event, "number": number, "created": created, "priority": priority, "state": state, "started": started}),
    )
    .await;
    q("CreateIncident")
        .arg(event)
        .arg(number)
        .arg(created)
        .arg(priority)
        .arg(state)
        .arg(started)
        .execute(db)
        .await?;
    Ok(())
}

/// The version-guarded update. Zero rows affected means the version moved.
pub async fn update_incident<'e>(db: impl MySqlExecutor<'e>, i: &Incident) -> Result<u64> {
    super::testhook::before(
        "UpdateIncident",
        serde_json::json!({"event": i.event, "number": i.number, "version": i.version}),
    )
    .await;
    Ok(q("UpdateIncident")
        .arg(i.priority)
        .arg(i.state.as_str())
        .arg(i.started)
        .arg(i.closed)
        .arg(i.summary.as_ref())
        .arg(i.location_name.as_ref())
        .arg(i.location_address.as_ref())
        .arg(i.location_description.as_ref())
        .arg(i.event)
        .arg(i.number)
        .arg(i.version)
        .execute(db)
        .await?
        .rows_affected())
}

pub async fn incident_version<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
) -> Result<Option<i32>> {
    let (sql, args) = q("IncidentVersion").arg(event).arg(number).build()?;
    sqlx::query_scalar_with(&sql, args).fetch_optional(db).await
}

pub async fn incident<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
) -> Result<Option<IncidentWithAssociations>> {
    q("Incident")
        .arg(event)
        .arg(number)
        .fetch_optional(db)
        .await
}

pub async fn incidents<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
) -> Result<Vec<IncidentWithAssociations>> {
    q("Incidents").arg(event).fetch_all(db).await
}

pub async fn incidents_rangers<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
) -> Result<Vec<IncidentRanger>> {
    q("Incidents_Rangers").arg(event).fetch_all(db).await
}

pub async fn incident_rangers<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
) -> Result<Vec<IncidentRanger>> {
    q("Incident_Rangers")
        .arg(event)
        .arg(number)
        .fetch_all(db)
        .await
}

pub async fn incident_linked_incidents<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
) -> Result<Vec<LinkedIncident>> {
    q("Incident_LinkedIncidents")
        .arg(event)
        .arg(number)
        .fetch_all(db)
        .await
}

pub async fn incidents_report_entries<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    generated: bool,
) -> Result<Vec<IncidentReportEntry>> {
    q("Incidents_ReportEntries")
        .arg(event)
        .arg(generated)
        .fetch_all(db)
        .await
}

pub async fn incident_report_entries<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
) -> Result<Vec<IncidentReportEntry>> {
    q("Incident_ReportEntries")
        .arg(event)
        .arg(number)
        .fetch_all(db)
        .await
}

pub async fn next_incident_number<'e>(db: impl MySqlExecutor<'e>, event: i32) -> Result<i64> {
    q("NextIncidentNumber")
        .arg(event)
        .fetch_one_scalar(db)
        .await
}

pub async fn attach_ranger_handle_to_incident<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
    handle: &str,
    role: Option<&str>,
) -> Result<()> {
    q("AttachRangerHandleToIncident")
        .arg(event)
        .arg(number)
        .arg(handle)
        .arg(role)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn detach_ranger_handle_from_incident<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
    handle: &str,
) -> Result<()> {
    q("DetachRangerHandleFromIncident")
        .arg(event)
        .arg(number)
        .arg(handle)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn link_incidents<'e>(
    db: impl MySqlExecutor<'e>,
    e1: i32,
    n1: i32,
    e2: i32,
    n2: i32,
) -> Result<()> {
    q("LinkIncidents")
        .arg(e1)
        .arg(n1)
        .arg(e2)
        .arg(n2)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn unlink_incidents<'e>(
    db: impl MySqlExecutor<'e>,
    e1: i32,
    n1: i32,
    e2: i32,
    n2: i32,
) -> Result<()> {
    q("UnlinkIncidents")
        .arg(e1)
        .arg(n1)
        .arg(e2)
        .arg(n2)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn attach_incident_type_to_incident<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
    itype: i32,
) -> Result<()> {
    q("AttachIncidentTypeToIncident")
        .arg(event)
        .arg(number)
        .arg(itype)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn detach_incident_type_from_incident<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
    itype: i32,
) -> Result<()> {
    q("DetachIncidentTypeFromIncident")
        .arg(event)
        .arg(number)
        .arg(itype)
        .execute(db)
        .await?;
    Ok(())
}

// ---- Report entries

#[allow(clippy::too_many_arguments)]
pub async fn create_report_entry<'e>(
    db: impl MySqlExecutor<'e>,
    author: &str,
    text: &str,
    created: f64,
    generated: bool,
    stricken: bool,
    attached_file: Option<&str>,
    attached_file_original_name: Option<&str>,
    attached_file_media_type: Option<&str>,
) -> Result<u64> {
    Ok(q("CreateReportEntry")
        .arg(author)
        .arg(text)
        .arg(created)
        .arg(generated)
        .arg(stricken)
        .arg(attached_file)
        .arg(attached_file_original_name)
        .arg(attached_file_media_type)
        .execute(db)
        .await?
        .last_insert_id())
}

/// Runs one of the `AttachReportEntryTo*` queries: (event, number, entry).
pub async fn attach_report_entry<'e>(
    db: impl MySqlExecutor<'e>,
    query: &str,
    event: i32,
    number: i32,
    entry: i32,
) -> Result<()> {
    q(query)
        .arg(event)
        .arg(number)
        .arg(entry)
        .execute(db)
        .await?;
    Ok(())
}

/// Runs one of the `Set*ReportEntryStricken` queries.
pub async fn set_report_entry_stricken<'e>(
    db: impl MySqlExecutor<'e>,
    query: &str,
    stricken: bool,
    event: i32,
    number: i32,
    entry: i32,
) -> Result<()> {
    q(query)
        .arg(stricken)
        .arg(event)
        .arg(number)
        .arg(entry)
        .execute(db)
        .await?;
    Ok(())
}

// ---- Field reports

pub async fn field_reports<'e>(db: impl MySqlExecutor<'e>, event: i32) -> Result<Vec<FieldReport>> {
    q("FieldReports").arg(event).fetch_all(db).await
}

pub async fn field_report<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
) -> Result<Option<FieldReport>> {
    q("FieldReport")
        .arg(event)
        .arg(number)
        .fetch_optional(db)
        .await
}

pub async fn field_reports_report_entries<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    generated: bool,
) -> Result<Vec<FieldReportReportEntry>> {
    q("FieldReports_ReportEntries")
        .arg(event)
        .arg(generated)
        .fetch_all(db)
        .await
}

pub async fn field_report_report_entries<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
) -> Result<Vec<ReportEntry>> {
    q("FieldReport_ReportEntries")
        .arg(event)
        .arg(number)
        .fetch_all(db)
        .await
}

pub async fn attach_field_report_to_incident<'e>(
    db: impl MySqlExecutor<'e>,
    incident: Option<i32>,
    event: i32,
    number: i32,
) -> Result<()> {
    q("AttachFieldReportToIncident")
        .arg(incident)
        .arg(event)
        .arg(number)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn next_field_report_number<'e>(db: impl MySqlExecutor<'e>, event: i32) -> Result<i64> {
    q("NextFieldReportNumber")
        .arg(event)
        .fetch_one_scalar(db)
        .await
}

pub async fn create_field_report<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
    created: f64,
    summary: Option<&str>,
    incident: Option<i32>,
) -> Result<()> {
    super::testhook::before(
        "CreateFieldReport",
        serde_json::json!({"event": event, "number": number, "created": created, "summary": summary, "incident_number": incident}),
    )
    .await;
    q("CreateFieldReport")
        .arg(event)
        .arg(number)
        .arg(created)
        .arg(summary)
        .arg(incident)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn field_report_version<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
) -> Result<Option<i32>> {
    let (sql, args) = q("FieldReportVersion").arg(event).arg(number).build()?;
    sqlx::query_scalar_with(&sql, args).fetch_optional(db).await
}

pub async fn update_field_report<'e>(db: impl MySqlExecutor<'e>, fr: &FieldReport) -> Result<u64> {
    super::testhook::before(
        "UpdateFieldReport",
        serde_json::json!({"event": fr.event, "number": fr.number, "version": fr.version}),
    )
    .await;
    Ok(q("UpdateFieldReport")
        .arg(fr.summary.as_ref())
        .arg(fr.incident_number)
        .arg(fr.event)
        .arg(fr.number)
        .arg(fr.version)
        .execute(db)
        .await?
        .rows_affected())
}

// ---- Visits

pub async fn attach_visit_to_incident<'e>(
    db: impl MySqlExecutor<'e>,
    incident: Option<i32>,
    event: i32,
    number: i32,
) -> Result<()> {
    q("AttachVisitToIncident")
        .arg(incident)
        .arg(event)
        .arg(number)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn create_visit<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
    created: f64,
) -> Result<()> {
    super::testhook::before(
        "CreateVisit",
        serde_json::json!({"event": event, "number": number, "created": created}),
    )
    .await;
    q("CreateVisit")
        .arg(event)
        .arg(number)
        .arg(created)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn update_visit<'e>(db: impl MySqlExecutor<'e>, v: &Visit) -> Result<u64> {
    super::testhook::before(
        "UpdateVisit",
        serde_json::json!({"event": v.event, "number": v.number, "version": v.version}),
    )
    .await;
    Ok(q("UpdateVisit")
        .arg(v.incident_number)
        .arg(v.guest_preferred_name.as_ref())
        .arg(v.guest_legal_name.as_ref())
        .arg(v.guest_description.as_ref())
        .arg(v.guest_action_plan.as_ref())
        .arg(v.guest_camp_name.as_ref())
        .arg(v.guest_camp_address.as_ref())
        .arg(v.guest_camp_description.as_ref())
        .arg(v.guest_camp_contacts.as_ref())
        .arg(v.arrival_time)
        .arg(v.arrival_method.as_ref())
        .arg(v.arrival_state.as_ref())
        .arg(v.arrival_reason.as_ref())
        .arg(v.arrival_belongings.as_ref())
        .arg(v.departure_time)
        .arg(v.departure_method.as_ref())
        .arg(v.departure_state.as_ref())
        .arg(v.resource_sitter.as_ref())
        .arg(v.resource_bed_id.as_ref())
        .arg(v.resource_rest.as_ref())
        .arg(v.resource_clothes.as_ref())
        .arg(v.resource_pogs.as_ref())
        .arg(v.resource_food_bev.as_ref())
        .arg(v.resource_other.as_ref())
        .arg(v.event)
        .arg(v.number)
        .arg(v.version)
        .execute(db)
        .await?
        .rows_affected())
}

pub async fn visit_version<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
) -> Result<Option<i32>> {
    let (sql, args) = q("VisitVersion").arg(event).arg(number).build()?;
    sqlx::query_scalar_with(&sql, args).fetch_optional(db).await
}

pub async fn visit<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
) -> Result<Option<Visit>> {
    q("Visit").arg(event).arg(number).fetch_optional(db).await
}

pub async fn visits<'e>(db: impl MySqlExecutor<'e>, event: i32) -> Result<Vec<Visit>> {
    q("Visits").arg(event).fetch_all(db).await
}

pub async fn visits_rangers<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
) -> Result<Vec<VisitRanger>> {
    q("Visits_Rangers").arg(event).fetch_all(db).await
}

pub async fn visit_rangers<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
) -> Result<Vec<VisitRanger>> {
    q("Visit_Rangers")
        .arg(event)
        .arg(number)
        .fetch_all(db)
        .await
}

pub async fn attach_ranger_to_visit<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
    handle: &str,
    role: Option<&str>,
) -> Result<()> {
    q("AttachRangerToVisit")
        .arg(event)
        .arg(number)
        .arg(handle)
        .arg(role)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn detach_ranger_from_visit<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
    handle: &str,
) -> Result<()> {
    q("DetachRangerFromVisit")
        .arg(event)
        .arg(number)
        .arg(handle)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn visit_report_entries<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
) -> Result<Vec<VisitReportEntry>> {
    q("Visit_ReportEntries")
        .arg(event)
        .arg(number)
        .fetch_all(db)
        .await
}

pub async fn visits_report_entries<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    generated: bool,
) -> Result<Vec<VisitReportEntry>> {
    q("Visits_ReportEntries")
        .arg(event)
        .arg(generated)
        .fetch_all(db)
        .await
}

pub async fn next_visit_number<'e>(db: impl MySqlExecutor<'e>, event: i32) -> Result<i64> {
    q("NextVisitNumber").arg(event).fetch_one_scalar(db).await
}

// ---- Places

#[allow(clippy::too_many_arguments)]
pub async fn create_place<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    number: i32,
    place_type: &str,
    name: &str,
    location_string: &str,
    external_data: Option<&str>,
) -> Result<()> {
    q("CreatePlace")
        .arg(event)
        .arg(number)
        .arg(place_type)
        .arg(name)
        .arg(location_string)
        .arg(external_data)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn remove_places<'e>(
    db: impl MySqlExecutor<'e>,
    event: i32,
    place_type: &str,
) -> Result<()> {
    q("RemovePlaces")
        .arg(event)
        .arg(place_type)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn places<'e>(
    db: impl MySqlExecutor<'e>,
    exclude_external_data: bool,
    event: i32,
) -> Result<Vec<Place>> {
    q("Places")
        .named("exclude_external_data", exclude_external_data)
        .arg(event)
        .fetch_all(db)
        .await
}

// ---- IMS-native directory administration

pub async fn directory_all_persons<'e>(db: impl MySqlExecutor<'e>) -> Result<Vec<DirectoryPerson>> {
    q("DirectoryAllPersons").fetch_all(db).await
}

pub async fn directory_all_positions<'e>(
    db: impl MySqlExecutor<'e>,
) -> Result<Vec<DirectoryTitledWithActive>> {
    q("DirectoryAllPositions").fetch_all(db).await
}

pub async fn directory_all_teams<'e>(
    db: impl MySqlExecutor<'e>,
) -> Result<Vec<DirectoryTitledWithActive>> {
    q("DirectoryAllTeams").fetch_all(db).await
}

pub async fn directory_person_by_handle<'e>(
    db: impl MySqlExecutor<'e>,
    handle: &str,
) -> Result<Option<DirectoryPerson>> {
    q("DirectoryPersonByHandle")
        .arg(handle)
        .fetch_optional(db)
        .await
}

pub async fn directory_person_by_id<'e>(
    db: impl MySqlExecutor<'e>,
    id: i64,
) -> Result<Option<DirectoryPerson>> {
    q("DirectoryPersonByID").arg(id).fetch_optional(db).await
}

pub async fn directory_create_person<'e>(
    db: impl MySqlExecutor<'e>,
    handle: &str,
    email: Option<&str>,
    password: &str,
    active: bool,
    onsite: bool,
) -> Result<u64> {
    Ok(q("DirectoryCreatePerson")
        .arg(handle)
        .arg(email)
        .arg(password)
        .arg(active)
        .arg(onsite)
        .execute(db)
        .await?
        .last_insert_id())
}

pub async fn directory_update_person<'e>(
    db: impl MySqlExecutor<'e>,
    p: &DirectoryPerson,
) -> Result<()> {
    q("DirectoryUpdatePerson")
        .arg(p.handle.as_str())
        .arg(p.email.as_ref())
        .arg(p.active)
        .arg(p.onsite)
        .arg(p.id)
        .execute(db)
        .await?;
    Ok(())
}

pub async fn directory_set_person_password<'e>(
    db: impl MySqlExecutor<'e>,
    id: i64,
    password: &str,
) -> Result<()> {
    q("DirectorySetPersonPassword")
        .arg(password)
        .arg(id)
        .execute(db)
        .await?;
    Ok(())
}

/// Runs one of the single-ID directory queries, e.g. `DirectoryDeletePerson`.
pub async fn directory_exec_by_id<'e>(
    db: impl MySqlExecutor<'e>,
    query: &str,
    id: i64,
) -> Result<u64> {
    Ok(q(query).arg(id).execute(db).await?.rows_affected())
}

/// `DirectoryCreateTeam` or `DirectoryCreatePosition`.
pub async fn directory_create_group<'e>(
    db: impl MySqlExecutor<'e>,
    query: &str,
    title: &str,
    active: bool,
) -> Result<u64> {
    Ok(q(query)
        .arg(title)
        .arg(active)
        .execute(db)
        .await?
        .last_insert_id())
}

/// `DirectoryUpdateTeam` or `DirectoryUpdatePosition`.
pub async fn directory_update_group<'e>(
    db: impl MySqlExecutor<'e>,
    query: &str,
    title: &str,
    active: bool,
    id: i64,
) -> Result<()> {
    q(query).arg(title).arg(active).arg(id).execute(db).await?;
    Ok(())
}

/// `DirectoryAddPersonTeam` or `DirectoryAddPersonPosition`.
pub async fn directory_add_membership<'e>(
    db: impl MySqlExecutor<'e>,
    query: &str,
    person: i64,
    group: i64,
) -> Result<()> {
    q(query).arg(person).arg(group).execute(db).await?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_query_builds_its_placeholders() {
        // Every query must at least parse; the typed wrappers are exercised
        // against a real database by the integration tests.
        assert!(QUERIES.len() > 100);
        for (name, parsed) in QUERIES.iter() {
            assert!(!parsed.segments.is_empty(), "{name} is empty");
        }
    }
}
