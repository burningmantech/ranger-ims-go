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

//! Report entries: creating them for Incidents, Field Reports, and Visits,
//! and striking them. Ported from the Go server's `api/reportentry.go`.

use std::collections::HashMap;

use axum::Extension;
use axum::body::Body;
use axum::extract::{Path, State};
use axum::http::HeaderMap;
use axum::response::Response;
use sqlx::MySqlConnection;

use super::attachment::previewable_content_type;
use super::fieldreport::{contains_author, fetch_field_report};
use super::helpers::*;
use super::middleware::JwtCtx;
use super::state::S;
use crate::authz;
use crate::json;
use crate::store::models::ReportEntry;
use crate::store::queries;
use crate::util::conv::{GoTime, string_to_sql};
use crate::util::herr::{self, HttpError};

/// What a report entry belongs to.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Parent {
    Incident,
    FieldReport,
    Visit,
}

impl Parent {
    fn attach_query(self) -> &'static str {
        match self {
            Parent::Incident => "AttachReportEntryToIncident",
            Parent::FieldReport => "AttachReportEntryToFieldReport",
            Parent::Visit => "AttachReportEntryToVisit",
        }
    }

    fn strike_query(self) -> &'static str {
        match self {
            Parent::Incident => "SetIncidentReportEntryStricken",
            Parent::FieldReport => "SetFieldReportReportEntryStricken",
            Parent::Visit => "SetVisitReportEntryStricken",
        }
    }
}

/// A report entry to be created and attached to its parent.
#[derive(Debug, Clone, Default)]
pub struct NewReportEntry {
    pub author: String,
    pub text: String,
    pub generated: bool,
    /// Set only when the entry records a file attachment.
    pub attached_file: String,
    pub attached_file_original_name: String,
    pub attached_file_media_type: String,
}

impl NewReportEntry {
    pub fn generated(author: &str, text: impl Into<String>) -> NewReportEntry {
        NewReportEntry {
            author: author.to_string(),
            text: text.into(),
            generated: true,
            ..Default::default()
        }
    }
}

pub async fn add_report_entry(
    conn: &mut MySqlConnection,
    parent: Parent,
    event_id: i32,
    number: i32,
    entry: &NewReportEntry,
) -> HResult<i32> {
    let id = queries::create_report_entry(
        &mut *conn,
        &entry.author,
        &entry.text,
        GoTime::now().to_float(),
        entry.generated,
        false,
        string_to_sql(Some(&entry.attached_file), 128).as_deref(),
        string_to_sql(Some(&entry.attached_file_original_name), 128).as_deref(),
        string_to_sql(Some(&entry.attached_file_media_type), 128).as_deref(),
    )
    .await
    .map_err(db_err(
        "Failed to create report entry",
        "[CreateReportEntry]",
    ))? as i32;
    queries::attach_report_entry(&mut *conn, parent.attach_query(), event_id, number, id)
        .await
        .map_err(db_err(
            "Failed to attach report entry",
            "[AttachReportEntry]",
        ))?;
    Ok(id)
}

/// Records the change-log lines as one generated entry, then adds each
/// nonempty entry the client sent.
pub async fn add_change_report_entries(
    conn: &mut MySqlConnection,
    parent: Parent,
    event_id: i32,
    number: i32,
    author: &str,
    logs: &[String],
    entries: Option<&[json::ReportEntry]>,
) -> HResult<()> {
    if !logs.is_empty() {
        add_report_entry(
            &mut *conn,
            parent,
            event_id,
            number,
            &NewReportEntry::generated(author, logs.join("\n")),
        )
        .await
        .map_err(|e| e.from_src("[addReportEntry]"))?;
    }
    for entry in entries.unwrap_or_default() {
        if entry.text.is_empty() {
            continue;
        }
        let new = NewReportEntry {
            author: author.to_string(),
            text: entry.text.clone(),
            ..Default::default()
        };
        add_report_entry(&mut *conn, parent, event_id, number, &new)
            .await
            .map_err(|e| e.from_src("[addReportEntry]"))?;
    }
    Ok(())
}

/// Report entries go out with whole-second creation times, as in Go.
pub fn report_entry_to_json(re: &ReportEntry, attachments_enabled: bool) -> json::ReportEntry {
    let mut attachment = json::Attachment::default();
    if attachments_enabled && let Some(name) = &re.attached_file_original_name {
        attachment.name = name.clone();
        attachment.previewable =
            previewable_content_type(re.attached_file_media_type.as_deref().unwrap_or_default());
    }
    json::ReportEntry {
        id: re.id,
        created: GoTime::from_float((re.created as i64) as f64),
        author: re.author.clone(),
        system_entry: re.generated,
        text: re.text.clone(),
        stricken: Some(re.stricken),
        attachment,
    }
}

pub fn path_i32(p: &HashMap<String, String>, key: &str, msg: &str) -> HResult<i32> {
    let raw = p.get(key).map(String::as_str).unwrap_or_default();
    raw.parse::<i32>().map_err(|e| {
        HttpError::bad_request(
            msg.to_string(),
            anyhow::anyhow!("strconv.ParseInt: parsing {raw:?}: {e}"),
        )
        .from_src("[ParseInt32]")
    })
}

/// Strikes or unstrikes an entry, recording that in a generated entry. The
/// parent's VERSION doesn't move: entries are their own table, so no edit of
/// the parent can clobber them.
async fn set_stricken(
    st: &S,
    parent: Parent,
    event_id: i32,
    number: i32,
    entry_id: i32,
    stricken: bool,
    author: &str,
) -> HResult<()> {
    retry_on_deadlock(|| async {
        let mut txn = st
            .db
            .begin()
            .await
            .map_err(db_err("Error beginning transaction", "[Begin]"))?;
        queries::set_report_entry_stricken(
            &mut *txn,
            parent.strike_query(),
            stricken,
            event_id,
            number,
            entry_id,
        )
        .await
        .map_err(db_err(
            "Error setting report entry",
            "[SetReportEntryStricken]",
        ))?;
        let verb = if stricken { "Struck" } else { "Unstruck" };
        add_report_entry(
            &mut txn,
            parent,
            event_id,
            number,
            &NewReportEntry::generated(author, format!("{verb} reportEntry {entry_id}")),
        )
        .await
        .map_err(|e| e.from_src("[addReportEntry]"))?;
        txn.commit()
            .await
            .map_err(db_err("Error committing transaction", "[Commit]"))?;
        Ok(())
    })
    .await
}

pub async fn edit_field_report_report_entry(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & (authz::EVENT_WRITE_ALL_FIELD_REPORTS | authz::EVENT_WRITE_OWN_FIELD_REPORTS) == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have permission to write Field Reports on this Event",
        ));
    }
    let limited_access = perms & authz::EVENT_WRITE_ALL_FIELD_REPORTS == 0;
    let author = claims_of(&jwt)?.ranger_handle().to_string();
    let number = path_i32(&p, "fieldReportNumber", "Failed to parse fieldReportNumber")?;
    let entry_id = path_i32(&p, "reportEntryId", "Failed to parse reportEntryId")?;
    let re: json::ReportEntry = read_body_as(&headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    let (_, entries) = fetch_field_report(&st, event.id, number)
        .await
        .map_err(|e| e.from_src("[fetchFieldReport]"))?;
    // Striking is an edit, so a Ranger who may only write their own Field
    // Reports has to be one of its authors.
    if limited_access && !contains_author(&entries, &author) {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have permission to edit this particular Field Report",
        ));
    }
    let Some(stricken) = re.stricken else {
        return Ok(herr::no_content_response("Success"));
    };
    set_stricken(
        &st,
        Parent::FieldReport,
        event.id,
        number,
        entry_id,
        stricken,
        &author,
    )
    .await?;
    st.es.notify_field_report_update(event.id, number);
    Ok(herr::no_content_response("Success"))
}

pub async fn edit_incident_report_entry(
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
            "The requestor does not have permission to write Report Entries on this Event",
        ));
    }
    let author = claims_of(&jwt)?.ranger_handle().to_string();
    let number = path_i32(&p, "incidentNumber", "Failed to parse incidentNumber")?;
    let entry_id = path_i32(&p, "reportEntryId", "Failed to parse reportEntryId")?;
    let re: json::ReportEntry = read_body_as(&headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    let Some(stricken) = re.stricken else {
        return Ok(herr::no_content_response("Success"));
    };
    set_stricken(
        &st,
        Parent::Incident,
        event.id,
        number,
        entry_id,
        stricken,
        &author,
    )
    .await?;
    st.es.notify_incident_update(event.id, number);
    Ok(herr::no_content_response("Success"))
}

pub async fn edit_visit_report_entry(
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
            "The requestor does not have permission to write Visits on this Event",
        ));
    }
    let author = claims_of(&jwt)?.ranger_handle().to_string();
    let number = path_i32(&p, "visitNumber", "Failed to parse visitNumber")?;
    let entry_id = path_i32(&p, "reportEntryId", "Failed to parse reportEntryId")?;
    let re: json::ReportEntry = read_body_as(&headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    match queries::visit(&st.db, event.id, number).await {
        Ok(Some(_)) => {}
        Ok(None) => {
            return Err(HttpError::not_found(
                "There is no Visit for the provided ID",
                anyhow::anyhow!("sql: no rows in result set"),
            ));
        }
        Err(e) => {
            return Err(
                HttpError::not_found("There is no Visit for the provided ID", e)
                    .from_src("[Visit]"),
            );
        }
    }
    let Some(stricken) = re.stricken else {
        return Ok(herr::no_content_response("Success"));
    };
    set_stricken(
        &st,
        Parent::Visit,
        event.id,
        number,
        entry_id,
        stricken,
        &author,
    )
    .await?;
    st.es.notify_visit_update(event.id, number);
    Ok(herr::no_content_response("Success"))
}
