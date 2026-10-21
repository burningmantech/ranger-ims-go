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

//! POST/GET /ims/api/auth, ported from the Go server's `api/auth.go`.

use std::collections::BTreeMap;
use std::time::{SystemTime, UNIX_EPOCH};

use axum::Extension;
use axum::body::Body;
use axum::extract::{Query, State};
use axum::http::header::SET_COOKIE;
use axum::http::{HeaderMap, HeaderValue, StatusCode};
use axum::response::Response;
use serde::{Deserialize, Serialize};

use super::helpers::{HResult, get_event, json_response, read_body_as};
use super::middleware::JwtCtx;
use super::state::S;
use crate::authz::{self, cookie::SetCookie};
use crate::util::herr::HttpError;

/// A throwaway hash, verified against when no user matches, so a login for a
/// nonexistent user takes as long as one with a wrong password.
const DUMMY_HASH: &str = "$argon2id$v=19$m=8192,t=4,p=1$Ke9wio+D+PfBYlVzJ3CTAA$/kNb/yXgSLyFpfmwIfwKwcNnBRRrUqJp8YXPtDKfNTE";

#[derive(Deserialize)]
pub struct PostAuthRequest {
    #[serde(default)]
    identification: String,
    #[serde(default)]
    password: String,
    /// Asks for the token in the response body, for a client that will send
    /// it back in an Authorization header, rather than in an HttpOnly cookie.
    #[serde(default)]
    token_in_body: bool,
}

#[derive(Serialize)]
pub struct PostAuthResponse {
    #[serde(skip_serializing_if = "String::is_empty")]
    token: String,
    expires_unix_ms: i64,
}

fn add_cookie(resp: &mut Response, c: &SetCookie) {
    if let Ok(v) = HeaderValue::from_str(&c.header_value()) {
        resp.headers_mut().append(SET_COOKIE, v);
    }
}

pub async fn post_auth(State(st): State<S>, headers: HeaderMap, body: Body) -> HResult<Response> {
    let (resp, cookie) = post_auth_inner(&st, &headers, body)
        .await
        .map_err(|e| e.from_src("[postAuth]"))?;
    let mut out = json_response(&resp);
    if let Some(c) = cookie {
        add_cookie(&mut out, &c);
    }
    add_cookie(&mut out, &st.cookies.expired_legacy_refresh_token());
    Ok(out)
}

async fn post_auth_inner(
    st: &S,
    headers: &HeaderMap,
    body: Body,
) -> HResult<(PostAuthResponse, Option<SetCookie>)> {
    let vals: PostAuthRequest = read_body_as(headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    let users = st.user_store.get_all_users().await.map_err(|e| {
        HttpError::internal_server_error("Failed to fetch personnel", e).from_src("[GetRangers]")
    })?;
    let matched = users.values().find(|p| {
        (!p.handle.is_empty() && p.handle.eq_ignore_ascii_case(&vals.identification))
            || (!p.email.is_empty() && p.email.eq_ignore_ascii_case(&vals.identification))
    });

    // See https://instatunnel.my/blog/the-1mb-password-crashing-backends-via-hashing-exhaustion
    if vals.password.len() > 256 {
        return Err(HttpError::bad_request(
            "Outrageously long passwords are disallowed",
            anyhow::anyhow!("rejected very long password"),
        ));
    }

    let Some(person) = matched else {
        let _ = authz::verify_password(&vals.password, DUMMY_HASH).await;
        return Err(HttpError::unauthorized(
            "Failed login attempt (bad credentials)",
            anyhow::anyhow!(
                "login attempt for nonexistent user. Identification: {}",
                vals.identification
            ),
        ));
    };

    let correct = authz::verify_password(&vals.password, &person.password).await.map_err(|e| {
        HttpError::internal_server_error(
            "Stored credentials are stale. A simple login to Clubhouse should fix this. You do not need to change your password.",
            anyhow::anyhow!("{e}. Identification: {}", vals.identification),
        )
        .from_src("[Verify]")
    })?;
    if !correct {
        return Err(HttpError::unauthorized(
            "Failed login attempt (bad credentials)",
            anyhow::anyhow!(
                "bad password for valid user. Identification: {}",
                vals.identification
            ),
        ));
    }

    tracing::info!(identification = %person.handle, "Successful login for Ranger");

    let lifetime = st.cfg.core.token_lifetime;
    let expiration = SystemTime::now() + lifetime;
    let jwt = st
        .jwter
        .create_access_token(&person.handle, person.id, expiration)
        .map_err(|e| {
            HttpError::internal_server_error("Failed to create access token", e)
                .from_src("[CreateAccessToken]")
        })?;
    let expires_unix_ms = expiration
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0);
    if vals.token_in_body {
        return Ok((
            PostAuthResponse {
                token: jwt,
                expires_unix_ms,
            },
            None,
        ));
    }
    Ok((
        PostAuthResponse {
            token: String::new(),
            expires_unix_ms,
        },
        Some(st.cookies.access_token(&jwt, lifetime)),
    ))
}

#[derive(Serialize, Default)]
pub struct GetAuthResponse {
    authenticated: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    user: String,
    admin: bool,
    event_access: Option<BTreeMap<String, AccessForEvent>>,
    event_deletion_allowed: bool,
    places_import_allowed: bool,
}

#[derive(Serialize, Default)]
pub struct AccessForEvent {
    event_id: i32,
    #[serde(rename = "readIncidents")]
    read_incidents: bool,
    #[serde(rename = "writeIncidents")]
    write_incidents: bool,
    #[serde(rename = "writeFieldReports")]
    write_field_reports: bool,
    #[serde(rename = "readVisits")]
    read_visits: bool,
    #[serde(rename = "writeVisits")]
    write_visits: bool,
    #[serde(rename = "attachFiles")]
    attach_files: bool,
}

#[derive(Deserialize)]
pub struct GetAuthQuery {
    #[serde(default)]
    event_id: String,
}

/// Reports whether the requestor is authenticated, and optionally their
/// access to one event. This endpoint is unauthenticated by design.
pub async fn get_auth(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Query(q): Query<GetAuthQuery>,
) -> HResult<Response> {
    let Some(claims) = jwt.claims.as_ref().filter(|_| jwt.error.is_none()) else {
        return Ok(json_response(&GetAuthResponse::default()));
    };
    let handle = claims.ranger_handle().to_string();
    let mut resp = GetAuthResponse {
        authenticated: true,
        admin: st.admins().contains(&handle),
        user: handle,
        event_access: None,
        event_deletion_allowed: st.cfg.core.event_deletion_enabled,
        places_import_allowed: st.cfg.burning_man_api.enabled(),
    };
    let event_name = q.event_id;
    if !event_name.is_empty() {
        let event = match get_event(&st, &event_name).await {
            Ok(e) => e,
            Err(e) if e.code == StatusCode::NOT_FOUND => {
                // Don't reveal whether the event exists; just show no access.
                resp.event_access = Some(BTreeMap::from([(event_name, AccessForEvent::default())]));
                return Ok(json_response(&resp));
            }
            Err(e) => return Err(e.from_src("[getEvent]").from_src("[getAuth]")),
        };
        let (perms, _) =
            authz::event_permissions(Some(event.id), &st.db, &st.user_store, st.admins(), claims)
                .await
                .map_err(|e| {
                    HttpError::internal_server_error("Failed to fetch event permissions", e)
                        .from_src("[EventPermissions]")
                })?;
        let p = perms.get(&event.id).copied().unwrap_or(0);
        resp.event_access = Some(BTreeMap::from([(
            event_name,
            AccessForEvent {
                event_id: event.id,
                read_incidents: p & authz::EVENT_READ_INCIDENTS != 0,
                write_incidents: p & authz::EVENT_WRITE_INCIDENTS != 0,
                write_field_reports: p
                    & (authz::EVENT_WRITE_OWN_FIELD_REPORTS | authz::EVENT_WRITE_ALL_FIELD_REPORTS)
                    != 0,
                read_visits: p & authz::EVENT_READ_VISITS != 0,
                write_visits: p & authz::EVENT_WRITE_VISITS != 0,
                attach_files: st.attachments_enabled(),
            },
        )]));
    }
    Ok(json_response(&resp))
}
