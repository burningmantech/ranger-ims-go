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

//! Events: list, create/edit, delete. Ported from the Go server's `api/event.go`.

use std::sync::LazyLock;

use axum::Extension;
use axum::body::Body;
use axum::extract::{Path, Query, State};
use axum::http::{HeaderMap, HeaderValue};
use axum::response::Response;
use regex::Regex;
use serde::Deserialize;

use super::embargo::embargoed;
use super::helpers::*;
use super::middleware::JwtCtx;
use super::state::S;
use crate::authz;
use crate::json;
use crate::store::queries;
use crate::util::conv::{GoTime, string_to_sql};
use crate::util::herr::{self, HttpError};

/// Event names are used in IMS URLs and in filesystem paths. Go's `\w` is
/// ASCII-only, hence the explicit class.
static ALLOWED_EVENT_NAMES: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r"^[0-9A-Za-z_-]+$").expect("valid regex"));
const ALLOWED_EVENT_NAMES_GO: &str = r"^[\w-]+$";

#[derive(Deserialize)]
pub struct GetEventsQuery {
    #[serde(default)]
    include_groups: String,
}

pub async fn get_events(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Query(q): Query<GetEventsQuery>,
) -> HResult<Response> {
    let resp = get_events_inner(&st, &jwt, &q)
        .await
        .map_err(|e| e.from_src("[getEvents]"))?;
    Ok(with_cache_control(
        json_response(&resp),
        st.cfg.core.cache_control_short,
    ))
}

async fn get_events_inner(st: &S, jwt: &JwtCtx, q: &GetEventsQuery) -> HResult<Vec<json::Event>> {
    let global = get_global_permissions(st, jwt)
        .await
        .map_err(|e| e.from_src("[getGlobalPermissions]"))?;
    if global & authz::GLOBAL_LIST_EVENTS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have GlobalListEvents permission",
        ));
    }
    let exclude_groups = !q.include_groups.eq_ignore_ascii_case("true");
    let all_events = queries::events(&st.db)
        .await
        .map_err(db_err("Failed to get events", "[Events]"))?;
    let perms_by_event = permissions_by_event(st, jwt)
        .await
        .map_err(|e| e.from_src("[permissionsByEvent]"))?;
    let is_admin = global & authz::GLOBAL_ADMINISTRATE_EVENTS != 0;
    let now = GoTime::now();
    let mut resp: Vec<json::Event> = all_events
        .into_iter()
        .filter(|e| !(e.is_group && exclude_groups))
        .filter(|e| {
            perms_by_event.get(&e.id).copied().unwrap_or(0) & authz::EVENT_READ_EVENT_NAME != 0
                || is_admin
        })
        .map(|e| {
            // The map shows placement, so it stays with the admins until its
            // release time arrives.
            let map_url = if !is_admin && embargoed(e.map_url_release, &now) {
                None
            } else {
                e.map_url.clone()
            };
            json::Event {
                id: e.id,
                name: Some(e.name),
                is_group: Some(e.is_group),
                parent_group: e.parent_group,
                map_url,
                camp_locations_release: e.camp_locations_release.map(GoTime::from_float),
                art_locations_release: e.art_locations_release.map(GoTime::from_float),
                map_url_release: e.map_url_release.map(GoTime::from_float),
                normalize_addresses: Some(e.normalize_addresses),
            }
        })
        .collect();
    resp.sort_by_key(|e| e.id);
    Ok(resp)
}

fn bad_event_name(name: &Option<String>) -> HttpError {
    HttpError::bad_request(
        format!("Event names must match the pattern {ALLOWED_EVENT_NAMES_GO}"),
        anyhow::anyhow!("invalid event name: '{name:?}'"),
    )
}

pub async fn edit_event(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let new_id = edit_event_inner(&st, &jwt, &headers, body)
        .await
        .map_err(|e| e.from_src("[editEvents]"))?;
    let mut resp = herr::no_content_response("Success");
    if let Some(id) = new_id {
        resp.headers_mut()
            .insert("IMS-Event-ID", HeaderValue::from(id));
    }
    Ok(resp)
}

async fn edit_event_inner(
    st: &S,
    jwt: &JwtCtx,
    headers: &HeaderMap,
    body: Body,
) -> HResult<Option<i32>> {
    let global = get_global_permissions(st, jwt)
        .await
        .map_err(|e| e.from_src("[getGlobalPermissions]"))?;
    if global & authz::GLOBAL_ADMINISTRATE_EVENTS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have GlobalAdministrateEvents permission",
        ));
    }
    let mut req: json::Event = read_body_as(headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;

    let mut new_event_id = None;
    if req.id == 0 {
        let name = match &req.name {
            Some(n) if ALLOWED_EVENT_NAMES.is_match(n) => n.clone(),
            _ => return Err(bad_event_name(&req.name)),
        };
        let id = queries::create_event(&st.db, &name, false, None)
            .await
            .map_err(db_err("Failed to create event", "[CreateEvent]"))?;
        tracing::info!(event_name = %name, id, "Created event");
        req.id = id as i32;
        new_event_id = Some(req.id);
    }

    let mut update = queries::event_one(&st.db, req.id)
        .await
        .map_err(db_err("Failed to fetch event", "[Event]"))?;

    if let Some(name) = &req.name {
        if !ALLOWED_EVENT_NAMES.is_match(name) {
            return Err(bad_event_name(&req.name));
        }
        update.name = name.clone();
    }
    if let Some(g) = req.is_group {
        update.is_group = g;
    }
    if let Some(parent) = req.parent_group {
        if parent == req.id {
            return Err(HttpError::bad_request_msg(
                "Event parent group cannot be the same as the event itself",
            ));
        }
        if parent > 0 {
            let target = queries::event_one(&st.db, parent)
                .await
                .map_err(db_err("Failed to fetch parent group", "[Event]"))?;
            if !target.is_group {
                return Err(HttpError::bad_request_msg(
                    "Event parent must be an event group",
                ));
            }
            update.parent_group = Some(parent);
        } else {
            update.parent_group = None;
        }
    }
    if update.is_group && update.parent_group.is_some() {
        return Err(HttpError::bad_request_msg(
            "An event group cannot have a parent event group",
        ));
    }
    if let Some(map_url) = &req.map_url {
        if update.is_group && !map_url.is_empty() {
            return Err(HttpError::bad_request_msg(
                "An event group cannot have a map URL",
            ));
        }
        update.map_url = string_to_sql(Some(map_url), 1024);
    }
    // Groups hold no places and get no map URL, so nothing about them can be
    // embargoed.
    if let Some(t) = req.camp_locations_release {
        if update.is_group && !t.is_zero() {
            return Err(HttpError::bad_request_msg(
                "An event group cannot have a camp locations release time",
            ));
        }
        update.camp_locations_release = t.to_opt_float();
    }
    if let Some(t) = req.art_locations_release {
        if update.is_group && !t.is_zero() {
            return Err(HttpError::bad_request_msg(
                "An event group cannot have an art locations release time",
            ));
        }
        update.art_locations_release = t.to_opt_float();
    }
    if let Some(t) = req.map_url_release {
        if update.is_group && !t.is_zero() {
            return Err(HttpError::bad_request_msg(
                "An event group cannot have a map URL release time",
            ));
        }
        update.map_url_release = t.to_opt_float();
    }
    if let Some(n) = req.normalize_addresses {
        if update.is_group && n {
            return Err(HttpError::bad_request_msg(
                "An event group cannot normalize addresses",
            ));
        }
        update.normalize_addresses = n;
    }
    queries::update_event(&st.db, &update)
        .await
        .map_err(db_err("Failed to update event", "[UpdateEvent]"))?;
    Ok(new_event_id)
}

/// Deletes an Event and everything associated with it. Child events of a
/// deleted group are detached from it, not deleted.
pub async fn delete_event(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(event_name): Path<String>,
) -> HResult<Response> {
    let global = get_global_permissions(&st, &jwt)
        .await
        .map_err(|e| e.from_src("[getGlobalPermissions]"))?;
    if global & authz::GLOBAL_ADMINISTRATE_EVENTS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have GlobalAdministrateEvents permission",
        ));
    }
    if !st.cfg.core.event_deletion_enabled {
        return Err(HttpError::forbidden_msg(
            "Event deletion is disabled on this server. It can be enabled by setting IMS_EVENT_DELETION_ENABLED=true",
        ));
    }
    let event = get_event(&st, &event_name)
        .await
        .map_err(|e| e.from_src("[getEvent]"))?;
    let fail = |src: &'static str| db_err("Failed to delete event", src);

    let mut txn = st
        .db
        .begin()
        .await
        .map_err(db_err("Failed to begin transaction", "[BeginTx]"))?;
    // Ordered so that no row goes before the rows holding foreign keys to it.
    let entry_ids = queries::event_report_entry_ids(&mut *txn, event.id)
        .await
        .map_err(fail("[EventReportEntryIDs]"))?;
    for name in [
        "DeleteEventIncidentReportEntries",
        "DeleteEventFieldReportReportEntries",
        "DeleteEventVisitReportEntries",
    ] {
        queries::exec_by_event(&mut *txn, name, event.id)
            .await
            .map_err(fail("[DeleteEvent*ReportEntries]"))?;
    }
    if !entry_ids.is_empty() {
        queries::delete_report_entries(&mut *txn, &entry_ids)
            .await
            .map_err(fail("[DeleteReportEntries]"))?;
    }
    for name in [
        "DeleteEventIncidentRangers",
        "DeleteEventIncidentIncidentTypes",
        "DeleteEventLinkedIncidents",
        "DeleteEventVisitRangers",
        "DeleteEventVisits",
        "DeleteEventFieldReports",
        "DeleteEventIncidents",
        "DeleteEventAccessAll",
        "DeleteEventPlaces",
    ] {
        queries::exec_by_event(&mut *txn, name, event.id)
            .await
            .map_err(fail("[DeleteEvent*]"))?;
    }
    queries::detach_children_from_event_group(&mut *txn, event.id)
        .await
        .map_err(fail("[DetachChildrenFromEventGroup]"))?;
    queries::delete_event(&mut *txn, event.id)
        .await
        .map_err(fail("[DeleteEvent]"))?;
    txn.commit()
        .await
        .map_err(db_err("Failed to commit transaction", "[Commit]"))?;
    tracing::info!(event_name = %event.name, id = event.id, "Deleted event");
    Ok(herr::no_content_response("Success"))
}
