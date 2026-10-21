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

//! Route registration, ported from the Go server's `api/mux.go`. Every route
//! goes through the middleware stack in `middleware.rs`, configured per route.

use axum::Router;
use axum::http::StatusCode;
use axum::middleware::from_fn_with_state;
use axum::response::Response;
use axum::routing::{MethodRouter, get};

use super::middleware::{ActionLogMode, AuthMode, RouteOpts, api_stack};
use super::state::S;
use super::*;
use crate::util::herr;

/// A test-only route that always panics, so the error log tests can drive a
/// genuine 500 through the whole middleware stack.
pub const PANIC_PATH: &str = "/ims/api/test/panic";

use ActionLogMode::{LogMetadata, LogMutation, LogNothing};

struct Routes {
    router: Router<S>,
    st: S,
}

impl Routes {
    fn add(mut self, path: &str, auth: AuthMode, log: ActionLogMode, mr: MethodRouter<S>) -> Self {
        let opts = RouteOpts { auth, log };
        self.router = self.router.route(
            path,
            mr.route_layer(from_fn_with_state((self.st.clone(), opts), api_stack)),
        );
        self
    }

    fn authed(self, path: &str, log: ActionLogMode, mr: MethodRouter<S>) -> Self {
        self.add(path, AuthMode::Required, log, mr)
    }
}

pub fn api_router(st: S) -> Router<S> {
    use axum::routing::{delete, post};
    let r = Routes {
        router: Router::new(),
        st: st.clone(),
    };
    let mut r = r
        .authed("/ims/api/access", LogMetadata, get(eventaccess::get_event_accesses))
        .authed("/ims/api/access_targets", LogMetadata, get(eventaccess::get_access_targets))
        .authed("/ims/api/access", LogMutation, post(eventaccess::post_event_access))
        .authed("/ims/api/actionlogs", LogMetadata, get(logs::get_action_logs))
        .authed("/ims/api/errorlogs", LogMetadata, get(logs::get_error_logs))
        // Doesn't consider the requestor's token at all, since the point is
        // to make a new one. Its body is a password, so only metadata is logged.
        .add("/ims/api/auth", AuthMode::None, LogMetadata, post(auth::post_auth))
        .add("/ims/api/auth", AuthMode::Optional, LogMetadata, get(auth::get_auth))
        .authed("/ims/api/events/{eventName}/incidents", LogNothing, get(incident::get_incidents))
        .authed("/ims/api/events/{eventName}/incidents", LogMutation, post(incident::new_incident))
        .authed("/ims/api/events/{eventName}/incidents/{incidentNumber}", LogNothing, get(incident::get_incident))
        .authed("/ims/api/events/{eventName}/incidents/{incidentNumber}", LogMutation, post(incident::edit_incident))
        .authed(
            "/ims/api/events/{eventName}/incidents/{incidentNumber}/attachments/{attachmentNumber}",
            LogMetadata,
            get(attachment::get_incident_attachment),
        )
        .authed("/ims/api/events/{eventName}/incidents/{incidentNumber}/attachments", LogMetadata, post(attachment::attach_to_incident))
        .authed("/ims/api/events/{eventName}/incidents/{incidentNumber}/rangers/{rangerName}", LogMutation, post(ranger::attach_ranger_to_incident))
        .authed("/ims/api/events/{eventName}/incidents/{incidentNumber}/rangers/{rangerName}", LogMutation, delete(ranger::detach_ranger_from_incident))
        .authed(
            "/ims/api/events/{eventName}/incidents/{incidentNumber}/report_entries/{reportEntryId}",
            LogMutation,
            post(reportentry::edit_incident_report_entry),
        )
        .authed(
            "/ims/api/events/{eventName}/incidents/{incidentNumber}/incident_types/{incidentTypeId}",
            LogMutation,
            post(incidentrelation::attach_type_to_incident),
        )
        .authed(
            "/ims/api/events/{eventName}/incidents/{incidentNumber}/incident_types/{incidentTypeId}",
            LogMutation,
            delete(incidentrelation::detach_type_from_incident),
        )
        .authed(
            "/ims/api/events/{eventName}/incidents/{incidentNumber}/linked_incidents/{linkedEventName}/{linkedIncidentNumber}",
            LogMutation,
            post(incidentrelation::link_incident),
        )
        .authed(
            "/ims/api/events/{eventName}/incidents/{incidentNumber}/linked_incidents/{linkedEventName}/{linkedIncidentNumber}",
            LogMutation,
            delete(incidentrelation::unlink_incident),
        )
        .authed("/ims/api/events/{eventName}/field_reports", LogNothing, get(fieldreport::get_field_reports))
        .authed("/ims/api/events/{eventName}/field_reports", LogMutation, post(fieldreport::new_field_report))
        .authed("/ims/api/events/{eventName}/field_reports/{fieldReportNumber}", LogNothing, get(fieldreport::get_field_report))
        .authed("/ims/api/events/{eventName}/field_reports/{fieldReportNumber}", LogMutation, post(fieldreport::edit_field_report))
        .authed(
            "/ims/api/events/{eventName}/field_reports/{fieldReportNumber}/attachments/{attachmentNumber}",
            LogMetadata,
            get(attachment::get_field_report_attachment),
        )
        .authed(
            "/ims/api/events/{eventName}/field_reports/{fieldReportNumber}/attachments",
            LogMetadata,
            post(attachment::attach_to_field_report),
        )
        .authed(
            "/ims/api/events/{eventName}/field_reports/{fieldReportNumber}/report_entries/{reportEntryId}",
            LogMutation,
            post(reportentry::edit_field_report_report_entry),
        )
        .authed("/ims/api/events/{eventName}/visits", LogNothing, get(visit::get_visits))
        .authed("/ims/api/events/{eventName}/visits/{visitNumber}", LogNothing, get(visit::get_visit))
        .authed("/ims/api/events/{eventName}/visits", LogMutation, post(visit::new_visit))
        .authed("/ims/api/events/{eventName}/visits/{visitNumber}", LogMutation, post(visit::edit_visit))
        .authed("/ims/api/events/{eventName}/visits/{visitNumber}/rangers/{rangerName}", LogMutation, post(ranger::attach_ranger_to_visit))
        .authed("/ims/api/events/{eventName}/visits/{visitNumber}/rangers/{rangerName}", LogMutation, delete(ranger::detach_ranger_from_visit))
        .authed(
            "/ims/api/events/{eventName}/visits/{visitNumber}/attachments/{attachmentNumber}",
            LogMetadata,
            get(attachment::get_visit_attachment),
        )
        .authed("/ims/api/events/{eventName}/visits/{visitNumber}/attachments", LogMetadata, post(attachment::attach_to_visit))
        .authed(
            "/ims/api/events/{eventName}/visits/{visitNumber}/report_entries/{reportEntryId}",
            LogMutation,
            post(reportentry::edit_visit_report_entry),
        )
        // UpdatePlaces is metadata only, since its body is the whole place list.
        .authed("/ims/api/events/{eventName}/places", LogMetadata, get(place::get_places))
        .authed("/ims/api/events/{eventName}/places", LogMetadata, post(place::update_places))
        .authed("/ims/api/events/{eventName}/places/import", LogMutation, post(place::import_places))
        .authed("/ims/api/events", LogNothing, get(event::get_events))
        .authed("/ims/api/events", LogMutation, post(event::edit_event))
        .authed("/ims/api/events/{eventName}", LogMutation, delete(event::delete_event))
        .authed("/ims/api/search", LogNothing, get(search::get_search))
        .authed("/ims/api/incident_types", LogNothing, get(itype::get_incident_types))
        .authed("/ims/api/incident_types", LogMutation, post(itype::edit_incident_types))
        .authed("/ims/api/personnel", LogNothing, get(personnel::get_personnel))
        .authed("/ims/api/directory", LogMetadata, get(directory::get_directory))
        .authed("/ims/api/directory/persons", LogMutation, post(directory::edit_directory_person))
        .authed("/ims/api/directory/persons/{personId}/password", LogMutation, post(directory::set_directory_person_password))
        .authed("/ims/api/directory/persons/{personId}", LogMutation, delete(directory::delete_directory_person))
        .authed("/ims/api/directory/teams", LogMutation, post(directory::edit_directory_team))
        .authed("/ims/api/directory/teams/{teamId}", LogMutation, delete(directory::delete_directory_team))
        .authed("/ims/api/directory/positions", LogMutation, post(directory::edit_directory_position))
        .authed("/ims/api/directory/positions/{positionId}", LogMutation, delete(directory::delete_directory_position))
        // The stream only carries notification metadata, not record contents.
        // The token is only checked on connecting.
        .authed(
            "/ims/api/eventsource",
            LogNothing,
            get(|axum::extract::State(st): axum::extract::State<S>| async move { st.es.stream_response() }),
        )
        .authed("/ims/api/debug/buildinfo", LogMetadata, get(logs::debug::get_build_info))
        .authed("/ims/api/debug/runtimemetrics", LogMetadata, get(logs::debug::get_runtime_metrics))
        .authed("/ims/api/debug/gc", LogMetadata, post(logs::debug::perform_gc))
        .authed("/ims/api/debug/config", LogMetadata, get(logs::debug::get_config));

    if st.cfg.test_hooks.panic_route {
        r = r.authed(
            PANIC_PATH,
            LogNothing,
            get(|| async { panic!("this handler always panics") as Response }),
        );
    }
    add_basic_handlers(r.router)
}

/// Routes that need no config, database, or authentication.
pub fn add_basic_handlers(router: Router<S>) -> Router<S> {
    router
        .route("/", get(|| async { herr::ok_response("IMS") }))
        .route("/ims/api/ping", get(|| async { herr::ok_response("ack") }))
}

/// Go's ServeMux answers an unknown path this way.
pub async fn not_found() -> Response {
    herr::text_response(StatusCode::NOT_FOUND, "404 page not found")
}

/// And a known path with the wrong method this way (axum adds the Allow header).
pub async fn method_not_allowed() -> Response {
    herr::text_response(StatusCode::METHOD_NOT_ALLOWED, "Method Not Allowed")
}
