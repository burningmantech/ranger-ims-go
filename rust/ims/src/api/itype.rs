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

//! Incident types. Ported from the Go server's `api/itype.go`.

use axum::Extension;
use axum::body::Body;
use axum::extract::State;
use axum::http::{HeaderMap, HeaderValue};
use axum::response::Response;

use super::helpers::*;
use super::middleware::JwtCtx;
use super::state::S;
use crate::authz;
use crate::json;
use crate::store::queries;
use crate::util::conv::string_to_sql;
use crate::util::herr::{self, HttpError};

pub async fn get_incident_types(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
) -> HResult<Response> {
    let global = get_global_permissions(&st, &jwt)
        .await
        .map_err(|e| e.from_src("[getGlobalPermissions]"))?;
    if global & authz::GLOBAL_READ_INCIDENT_TYPES == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have GlobalReadIncidentTypes permission",
        ));
    }
    let rows = queries::incident_types(&st.db)
        .await
        .map_err(db_err("Failed to fetch Incident Types", "[IncidentTypes]"))?;
    let mut resp: Vec<json::IncidentType> = rows
        .into_iter()
        .map(|t| json::IncidentType {
            id: t.id,
            name: Some(t.name),
            description: t.description,
            hidden: Some(t.hidden),
        })
        .collect();
    resp.sort_by_key(|t| t.id);
    Ok(with_cache_control(
        json_response(&resp),
        st.cfg.core.cache_control_short,
    ))
}

pub async fn edit_incident_types(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let global = get_global_permissions(&st, &jwt)
        .await
        .map_err(|e| e.from_src("[getGlobalPermissions]"))?;
    if global & authz::GLOBAL_ADMINISTRATE_INCIDENT_TYPES == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have GlobalAdministrateIncidentTypes permission",
        ));
    }
    let req: json::IncidentType = read_body_as(&headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    // A nameless Incident Type can't be shown to anyone, and a lookup by name
    // can't tell it from a type that doesn't exist.
    if req.name.as_ref().is_some_and(|n| n.trim().is_empty()) {
        return Err(HttpError::bad_request_msg(
            "Incident Type name may not be empty",
        ));
    }
    if req.id == 0 {
        let Some(name) = &req.name else {
            return Err(HttpError::bad_request_msg(
                "Incident Type name is required for a new Incident Type",
            ));
        };
        let id = queries::create_incident_type(&st.db, name, req.hidden == Some(true))
            .await
            .map_err(db_err(
                "Failed to create Incident Type",
                "[CreateIncidentTypeOrIgnore]",
            ))?;
        let mut resp = herr::no_content_response("Success");
        resp.headers_mut()
            .insert("IMS-Incident-Type-ID", HeaderValue::from(id));
        return Ok(resp);
    }

    let mut t = queries::incident_type(&st.db, req.id)
        .await
        .map_err(db_err("Failed to fetch Incident Type", "[IncidentType]"))?;
    if let Some(n) = req.name {
        t.name = n;
    }
    if let Some(h) = req.hidden {
        t.hidden = h;
    }
    if let Some(d) = &req.description {
        t.description = string_to_sql(Some(d), 1023);
    }
    queries::update_incident_type(&st.db, &t)
        .await
        .map_err(|_| {
            HttpError::internal_server_error(
                "Failed to update incident type",
                anyhow::anyhow!("Failed to update incident type"),
            )
            .from_src("[UpdateIncidentType]")
        })?;
    Ok(herr::no_content_response("Success"))
}
