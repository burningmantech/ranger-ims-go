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

//! Request/response helpers shared by the handlers, ported from the Go
//! server's `api/helpers.go`.

use std::collections::HashMap;
use std::time::Duration;

use axum::body::Body;
use axum::http::header::CONTENT_TYPE;
use axum::http::{HeaderMap, HeaderValue};
use axum::response::{IntoResponse, Response};
use serde::Serialize;
use serde::de::DeserializeOwned;

use super::middleware::JwtCtx;
use super::state::AppState;
use crate::authz::{self, EventPermissionMask, GlobalPermissionMask, IMSClaims, Requestor};
use crate::store::models::{Event, EventAccess};
use crate::store::queries;
use crate::util::gojson;
use crate::util::herr::HttpError;

/// How many times to retry creating a record whose freshly allocated number
/// a concurrent creator claimed first.
pub const MAX_NUMBER_ALLOC_ATTEMPTS: usize = 3;
/// How many times to retry a transaction InnoDB picked as a deadlock victim.
pub const MAX_DEADLOCK_ATTEMPTS: usize = 6;
const DEADLOCK_RETRY_BACKOFF: Duration = Duration::from_millis(2);

pub type HResult<T> = Result<T, HttpError>;

fn mysql_error_number(err: &HttpError) -> Option<u16> {
    err.internal.chain().find_map(|e| {
        e.downcast_ref::<sqlx::Error>().and_then(|e| match e {
            sqlx::Error::Database(db) => db
                .try_downcast_ref::<sqlx::mysql::MySqlDatabaseError>()
                .map(|m| m.number()),
            _ => None,
        })
    })
}

pub fn sqlx_error_number(err: &sqlx::Error) -> Option<u16> {
    match err {
        sqlx::Error::Database(db) => db
            .try_downcast_ref::<sqlx::mysql::MySqlDatabaseError>()
            .map(|m| m.number()),
        _ => None,
    }
}

/// ER_DUP_ENTRY: an INSERT lost a race for a unique key.
pub fn is_duplicate_key_error(err: &sqlx::Error) -> bool {
    sqlx_error_number(err) == Some(1062)
}

/// ER_LOCK_DEADLOCK: InnoDB rolled this transaction back as a deadlock victim.
pub fn is_deadlock_error(err: &HttpError) -> bool {
    mysql_error_number(err) == Some(1213)
}

/// Jitters a duration by up to ±50%, as the Go server's `rand.Jitter` does.
fn jitter(d: Duration) -> Duration {
    use rand::Rng;
    let factor = rand::rng().random_range(0.5..1.5);
    d.mul_f64(factor)
}

/// Runs `body` again if InnoDB rolled its transaction back as a deadlock
/// victim. `body` must do all its work in one transaction and publish nothing
/// outside the database until that transaction has committed.
pub async fn retry_on_deadlock<T, F, Fut>(mut body: F) -> HResult<T>
where
    F: FnMut() -> Fut,
    Fut: std::future::Future<Output = HResult<T>>,
{
    let mut backoff = DEADLOCK_RETRY_BACKOFF;
    let mut attempt = 1;
    loop {
        match body().await {
            Err(e) if is_deadlock_error(&e) => {
                if attempt == MAX_DEADLOCK_ATTEMPTS {
                    return Err(e.from_src(&format!(
                        "[retryOnDeadlock] gave up after {attempt} attempts"
                    )));
                }
                tokio::time::sleep(jitter(backoff)).await;
                backoff *= 2;
                attempt += 1;
            }
            other => return other,
        }
    }
}

/// A 500 for a failed database call.
pub fn db_err(msg: &str, src: &str) -> impl FnOnce(sqlx::Error) -> HttpError {
    let msg = msg.to_string();
    let src = src.to_string();
    move |e| HttpError::internal_server_error(msg, e).from_src(&src)
}

pub fn json_response<T: Serialize + ?Sized>(v: &T) -> Response {
    match gojson::to_string(v) {
        Ok(s) => {
            let mut resp = s.into_response();
            resp.headers_mut()
                .insert(CONTENT_TYPE, HeaderValue::from_static("application/json"));
            resp
        }
        Err(e) => HttpError::internal_server_error("Failed to marshal JSON", e)
            .from_src("[Marshal]")
            .into_response(),
    }
}

/// Rejects a request whose body isn't declared as JSON. That keeps another
/// site's HTML form from forging a request, since a form can't send JSON and
/// a cross-origin script would need a CORS preflight IMS never approves.
pub fn require_json_content_type(headers: &HeaderMap) -> HResult<()> {
    const MSG: &str = "Request Content-Type must be application/json";
    let Some(header) = headers
        .get(CONTENT_TYPE)
        .and_then(|v| v.to_str().ok())
        .filter(|v| !v.is_empty())
    else {
        return Err(HttpError::unsupported_media_type(
            MSG,
            anyhow::anyhow!("request has no Content-Type header"),
        ));
    };
    let mime: mime::Mime = header.parse().map_err(|e| {
        HttpError::unsupported_media_type(
            MSG,
            anyhow::anyhow!("request Content-Type is not application/json: {e}"),
        )
        .from_src("[ParseMediaType]")
    })?;
    if !mime.essence_str().eq_ignore_ascii_case("application/json") {
        return Err(HttpError::unsupported_media_type(
            MSG,
            anyhow::anyhow!(
                "request Content-Type is not application/json: got {}",
                mime.essence_str()
            ),
        ));
    }
    Ok(())
}

/// Whether a body read failed because the body was over the size limit.
pub fn is_length_limit_error(err: &axum::Error) -> bool {
    let mut source: Option<&(dyn std::error::Error + 'static)> = Some(err);
    while let Some(e) = source {
        if e.downcast_ref::<http_body_util::LengthLimitError>()
            .is_some()
        {
            return true;
        }
        source = e.source();
    }
    false
}

pub async fn read_body_bytes(body: Body) -> Result<axum::body::Bytes, axum::Error> {
    axum::body::to_bytes(body, usize::MAX).await
}

pub async fn read_body_as<T: DeserializeOwned>(headers: &HeaderMap, body: Body) -> HResult<T> {
    require_json_content_type(headers).map_err(|e| e.from_src("[requireJSONContentType]"))?;
    let bytes = read_body_bytes(body).await.map_err(|e| {
        HttpError::bad_request("Failed to read request body", e).from_src("[io.ReadAll]")
    })?;
    decode_like_go(&bytes).map_err(|e| {
        HttpError::bad_request("Failed to unmarshal request body", e).from_src("[Unmarshal]")
    })
}

/// Decodes JSON the way Go's encoding/json does in one respect that clients
/// rely on: a null leaves a field at its zero value rather than being an
/// error. Every request field defaults when absent, so dropping the null
/// members of objects gets the same result.
pub fn decode_like_go<T: DeserializeOwned>(bytes: &[u8]) -> serde_json::Result<T> {
    fn strip_nulls(v: &mut serde_json::Value) {
        match v {
            serde_json::Value::Object(map) => {
                map.retain(|_, v| !v.is_null());
                map.values_mut().for_each(strip_nulls);
            }
            serde_json::Value::Array(items) => items.iter_mut().for_each(strip_nulls),
            _ => {}
        }
    }
    let mut value: serde_json::Value = serde_json::from_slice(bytes)?;
    if value.is_null() {
        value = serde_json::Value::Object(serde_json::Map::new());
    }
    strip_nulls(&mut value);
    serde_json::from_value(value)
}

pub async fn get_event(state: &AppState, event_name: &str) -> HResult<Event> {
    if event_name.is_empty() {
        return Err(HttpError::bad_request_msg("No eventName was provided"));
    }
    match queries::query_event_id(&state.db, event_name).await {
        Ok(Some(e)) => Ok(e),
        Ok(None) => Err(HttpError::not_found(
            "Event not found",
            anyhow::anyhow!("sql: no rows in result set"),
        )),
        Err(e) => {
            Err(HttpError::internal_server_error("Failed to fetch Event", e)
                .from_src("[QueryEventID]"))
        }
    }
}

pub fn claims_of(jwt: &JwtCtx) -> HResult<&IMSClaims> {
    jwt.claims.as_ref().ok_or_else(|| {
        HttpError::internal_server_error(
            "This endpoint has been misconfigured",
            anyhow::anyhow!("no claims"),
        )
    })
}

pub async fn get_event_permissions(
    state: &AppState,
    jwt: &JwtCtx,
    event_name: &str,
) -> HResult<(Event, EventPermissionMask)> {
    let event = get_event(state, event_name)
        .await
        .map_err(|e| e.from_src("[getEvent]"))?;
    let claims = claims_of(jwt)?;
    let (perms, _) = authz::event_permissions(
        Some(event.id),
        &state.db,
        &state.user_store,
        state.admins(),
        claims,
    )
    .await
    .map_err(|e| {
        HttpError::internal_server_error("Failed to compute permissions", e)
            .from_src("[EventPermissions]")
    })?;
    let mask = perms.get(&event.id).copied().unwrap_or(0);
    Ok((event, mask))
}

pub async fn get_global_permissions(
    state: &AppState,
    jwt: &JwtCtx,
) -> HResult<GlobalPermissionMask> {
    let claims = claims_of(jwt)?;
    let (_, global) =
        authz::event_permissions(None, &state.db, &state.user_store, state.admins(), claims)
            .await
            .map_err(|e| {
                HttpError::internal_server_error("Failed to compute permissions", e)
                    .from_src("[EventPermissions]")
            })?;
    Ok(global)
}

/// The requestor's permissions on every event, with parent-group access
/// rules applied to their child events.
pub async fn permissions_by_event(
    state: &AppState,
    jwt: &JwtCtx,
) -> HResult<HashMap<i32, EventPermissionMask>> {
    let access_rows = queries::event_access_all(&state.db).await.map_err(|e| {
        HttpError::internal_server_error("Failed to fetch event access", e)
            .from_src("[EventAccessAll]")
    })?;
    let mut by_event: HashMap<i32, Vec<EventAccess>> = HashMap::new();
    for ar in access_rows {
        by_event.entry(ar.event).or_default().push(ar);
    }
    let events = queries::events(&state.db).await.map_err(|e| {
        HttpError::internal_server_error("Failed to fetch Events", e).from_src("[Events]")
    })?;
    for e in &events {
        if let Some(parent) = e.parent_group {
            let parent_rules = by_event.get(&parent).cloned().unwrap_or_default();
            by_event.entry(e.id).or_default().extend(parent_rules);
        }
    }
    let claims = claims_of(jwt)?;
    let requestor = Requestor::load(&state.user_store, claims)
        .await
        .map_err(|e| HttpError::internal_server_error("Failed to fetch positions and teams", e))?;
    Ok(authz::many_event_permissions(&by_event, state.admins(), &requestor).0)
}

/// Sets Cache-Control to a private max-age, as handlers for slow-changing
/// data do (overriding the no-store default).
pub fn with_cache_control(mut resp: Response, max_age: Duration) -> Response {
    let v = format!("max-age={}, private", max_age.as_secs());
    if let Ok(v) = HeaderValue::from_str(&v) {
        resp.headers_mut()
            .insert(axum::http::header::CACHE_CONTROL, v);
    }
    resp
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn nulls_decode_to_zero_values_like_go() {
        let rule: crate::json::AccessRule =
            decode_like_go(br#"{"expression":"*","validity":null,"description":null}"#).unwrap();
        assert_eq!(rule.expression, "*");
        assert_eq!(rule.validity, "");
        let inc: crate::json::Incident =
            decode_like_go(br#"{"summary":null,"location":null,"priority":null}"#).unwrap();
        assert_eq!(inc.summary, None);
        assert_eq!(inc.priority, 0);
        let places: crate::json::Places = decode_like_go(b"null").unwrap();
        assert!(places.is_empty());
    }

    #[test]
    fn malformed_json_is_still_an_error() {
        assert!(decode_like_go::<crate::json::Incident>(br#"{"summary": "#).is_err());
        assert!(decode_like_go::<crate::json::Incident>(br#"{"priority": "high"}"#).is_err());
    }
}
