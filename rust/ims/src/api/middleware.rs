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

//! The middleware stack every API route goes through, in the Go server's
//! order (see `authed` in its `api/mux.go`):
//! RecordErrors → RecoverFromPanic → RejectCrossOrigin → NoStoreByDefault →
//! RequireAuthN/OptionalAuthN → LogRequest → LimitRequestBytes → handler.

use std::any::Any;
use std::backtrace::Backtrace;
use std::cell::RefCell;
use std::net::SocketAddr;
use std::panic::AssertUnwindSafe;
use std::sync::{Arc, Mutex, Once};
use std::time::Instant;

use axum::body::Body;
use axum::extract::{ConnectInfo, Request, State};
use axum::http::header::{AUTHORIZATION, CACHE_CONTROL, HOST, ORIGIN, REFERER};
use axum::http::{HeaderMap, HeaderValue, Method, StatusCode};
use axum::middleware::Next;
use axum::response::{IntoResponse, Response};
use futures::FutureExt;
use http_body_util::Limited;

use super::actionlogbody::{CapturingBody, is_json_content_type};
use super::state::S;
use crate::authz::IMSClaims;
use crate::authz::jwt::JwtError;
use crate::store::models::{ActionLog, ErrorLog};
use crate::util::conv::{GoTime, string_to_sql};
use crate::util::herr::{HttpError, RecordedError};

/// How much of a request a route writes to the action log.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ActionLogMode {
    /// Keeps the chatty read endpoints out of the action log entirely.
    LogNothing,
    /// Records who made the request and how it went, but not what they sent.
    LogMetadata,
    /// Additionally records the request's JSON body, i.e. the mutation.
    LogMutation,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AuthMode {
    Required,
    Optional,
    /// Ignores the requestor's token entirely.
    None,
}

#[derive(Clone, Copy)]
pub struct RouteOpts {
    pub auth: AuthMode,
    pub log: ActionLogMode,
}

/// The requestor's authentication, as handlers see it.
#[derive(Clone, Debug, Default)]
pub struct JwtCtx {
    pub claims: Option<IMSClaims>,
    pub error: Option<String>,
}

#[derive(Debug)]
struct CrossOrigin(String);

impl std::fmt::Display for CrossOrigin {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "cross-origin request{}", self.0)
    }
}

impl std::error::Error for CrossOrigin {}

#[derive(Debug)]
struct DirectoryLookup(String);

impl std::fmt::Display for DirectoryLookup {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "failed to fetch users: {}", self.0)
    }
}

impl std::error::Error for DirectoryLookup {}

#[derive(Debug)]
struct UserNotInDirectory(String);

impl std::fmt::Display for UserNotInDirectory {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "user is not in the directory: {}", self.0)
    }
}

impl std::error::Error for UserNotInDirectory {}

/// Who made the request, captured before the handler runs so that the error
/// log can still name them if the handler panics.
#[derive(Clone, Default)]
struct Identity {
    user_id: Option<i64>,
    user_name: Option<String>,
    position_id: Option<i64>,
    position_name: Option<String>,
}

/// The client's address, preferring what a proxy in front of IMS says.
pub fn client_address(headers: &HeaderMap, remote: Option<SocketAddr>) -> String {
    if let Some(v) = headers
        .get("CF-Connecting-IP")
        .and_then(|v| v.to_str().ok())
        .filter(|v| !v.is_empty())
    {
        return v.to_string();
    }
    if let Some(v) = headers
        .get("X-Forwarded-For")
        .and_then(|v| v.to_str().ok())
        .filter(|v| !v.is_empty())
    {
        return v.to_string();
    }
    remote.map(|a| a.ip().to_string()).unwrap_or_default()
}

/// The referrer, trimmed down to the IMS-relative part.
pub fn request_referrer(headers: &HeaderMap) -> Option<String> {
    let r = headers
        .get(REFERER)
        .and_then(|v| v.to_str().ok())
        .unwrap_or_default();
    let r = match r.find("/ims") {
        Some(i) => &r[i..],
        None => r,
    };
    if r.is_empty() {
        None
    } else {
        Some(r.to_string())
    }
}

thread_local! {
    static LAST_PANIC_BACKTRACE: RefCell<Option<String>> = const { RefCell::new(None) };
}

/// Keeps the backtrace of each panic, for the error log. A panic in a handler
/// unwinds on the thread polling it, which is where catch_unwind picks this up.
fn install_panic_hook() {
    static ONCE: Once = Once::new();
    ONCE.call_once(|| {
        let default_hook = std::panic::take_hook();
        std::panic::set_hook(Box::new(move |info| {
            let bt = Backtrace::force_capture().to_string();
            LAST_PANIC_BACKTRACE.with(|b| *b.borrow_mut() = Some(bt));
            default_hook(info);
        }));
    });
}

fn panic_message(payload: &(dyn Any + Send)) -> String {
    if let Some(s) = payload.downcast_ref::<&str>() {
        (*s).to_string()
    } else if let Some(s) = payload.downcast_ref::<String>() {
        s.clone()
    } else {
        "unknown panic".to_string()
    }
}

/// Go's http.CrossOriginProtection check: state-changing requests a browser
/// sent from another origin are refused.
fn cross_origin_check(method: &Method, headers: &HeaderMap) -> Result<(), &'static str> {
    if matches!(*method, Method::GET | Method::HEAD | Method::OPTIONS) {
        return Ok(());
    }
    match headers
        .get("Sec-Fetch-Site")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("")
    {
        "" => {}
        "same-origin" | "none" => return Ok(()),
        _ => return Err("cross-origin request detected from Sec-Fetch-Site header"),
    }
    let origin = headers
        .get(ORIGIN)
        .and_then(|v| v.to_str().ok())
        .unwrap_or("");
    if origin.is_empty() {
        return Ok(());
    }
    let host = headers
        .get(HOST)
        .and_then(|v| v.to_str().ok())
        .unwrap_or("");
    let origin_host = origin.split_once("://").map(|(_, rest)| rest).unwrap_or("");
    let origin_host = origin_host.split(['/', '?', '#']).next().unwrap_or("");
    if !origin_host.is_empty() && origin_host == host {
        return Ok(());
    }
    Err(
        "cross-origin request detected, and/or browser is out of date: Sec-Fetch-Site is missing, and Origin does not match Host",
    )
}

/// The requestor's access token. A Bearer token beats the cookie; any other
/// Authorization scheme (e.g. a proxy's Basic auth) isn't for IMS.
fn request_token(state: &S, headers: &HeaderMap) -> Result<String, anyhow::Error> {
    if let Some(auth) = headers.get(AUTHORIZATION).and_then(|v| v.to_str().ok())
        && let Some((scheme, token)) = auth.split_once(' ')
        && scheme.eq_ignore_ascii_case("Bearer")
    {
        return Ok(token.to_string());
    }
    let token = state.cookies.access_token_from(headers)?;
    if token.is_empty() {
        return Ok(token);
    }
    // SameSite=Strict still lets a sibling subdomain send the cookie, and
    // cross-origin protection lets GETs through, so only IMS's own pages (or a
    // typed-in URL) may use it. No header at all means an old browser.
    match headers
        .get("Sec-Fetch-Site")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("")
    {
        "" | "same-origin" | "none" => Ok(token),
        site => Err(anyhow::Error::new(CrossOrigin(format!(
            ": cookie sent with Sec-Fetch-Site {site:?}"
        )))),
    }
}

/// The claims of the requestor's valid access token. Tokens can't be revoked,
/// so the user must also still be in the directory under the same handle.
async fn authenticate(state: &S, headers: &HeaderMap) -> Result<IMSClaims, anyhow::Error> {
    let token = request_token(state, headers)?;
    let claims = state.jwter.authenticate_jwt(&token)?;
    let users = state
        .user_store
        .get_all_users()
        .await
        .map_err(|e| anyhow::Error::new(DirectoryLookup(format!("{e:#}"))))?;
    match users.get(&claims.directory_id()) {
        Some(u) if u.handle == claims.ranger_handle() => Ok(claims),
        _ => Err(anyhow::Error::new(UserNotInDirectory(format!(
            "{} (ID {})",
            claims.ranger_handle(),
            claims.directory_id()
        )))),
    }
}

fn authn_error(err: anyhow::Error) -> HttpError {
    if err.downcast_ref::<CrossOrigin>().is_some() {
        return HttpError::forbidden("Cross-origin request rejected", err);
    }
    if err.downcast_ref::<DirectoryLookup>().is_some() {
        return HttpError::internal_server_error("Failed to fetch personnel", err);
    }
    if err.downcast_ref::<UserNotInDirectory>().is_some() {
        return HttpError::unauthorized(
            "Please log in again. User is no longer in the directory",
            err,
        );
    }
    match err.downcast_ref::<JwtError>() {
        Some(JwtError::Expired) => {
            HttpError::unauthorized("Please log in again. Authorization token is expired", err)
        }
        Some(JwtError::NoJwtString) => {
            HttpError::unauthorized("Please log in again. No authorization token", err)
        }
        _ => HttpError::unauthorized("Invalid authorization token", err),
    }
}

/// The whole stack, as one middleware function.
pub async fn api_stack(
    State((state, opts)): State<(S, RouteOpts)>,
    req: Request,
    next: Next,
) -> Response {
    install_panic_hook();
    let start = Instant::now();
    let method = req.method().clone();
    let path = req.uri().path().to_string();
    let referrer = request_referrer(req.headers());
    let remote = req
        .extensions()
        .get::<ConnectInfo<SocketAddr>>()
        .map(|c| c.0);
    let client_addr = client_address(req.headers(), remote);
    let identity = Arc::new(Mutex::new(Identity::default()));

    // RecoverFromPanic
    let inner = inner_stack(state.clone(), opts, req, next, Arc::clone(&identity), start);
    let (resp, panic_stack) = match AssertUnwindSafe(inner).catch_unwind().await {
        Ok(resp) => (resp, None),
        Err(payload) => {
            let stack = LAST_PANIC_BACKTRACE
                .with(|b| b.borrow_mut().take())
                .unwrap_or_default();
            let msg = panic_message(payload.as_ref());
            tracing::error!(err = %msg, stack = %stack, "Recovered from panic");
            let resp = HttpError::internal_server_error(
                "The server malfunctioned",
                anyhow::anyhow!("panic: {msg}"),
            )
            .into_response();
            (resp, Some(stack))
        }
    };

    // RecordErrors
    if let Some(rec) = resp.extensions().get::<RecordedError>().cloned()
        && rec.code.as_u16() >= StatusCode::INTERNAL_SERVER_ERROR.as_u16()
        && !rec.canceled
    {
        let who = identity.lock().unwrap_or_else(|e| e.into_inner()).clone();
        state
            .error_logger
            .log(ErrorLog {
                id: 0,
                created_at: GoTime::now().to_float(),
                http_status: rec.code.as_u16() as i16,
                response_message: string_to_sql(Some(&rec.response_message), 1024),
                internal_error: string_to_sql(Some(&rec.internal_error), 1024),
                stack_trace: string_to_sql(panic_stack.as_deref(), 1024),
                method: string_to_sql(Some(method.as_str()), 128),
                path: string_to_sql(Some(&path), 255),
                referrer: string_to_sql(referrer.as_deref(), 255),
                user_id: who.user_id,
                user_name: who.user_name,
                position_id: who.position_id,
                position_name: who.position_name,
                client_address: string_to_sql(Some(&client_addr), 128),
                duration_micros: Some(start.elapsed().as_micros() as i64),
            })
            .await;
    }
    resp
}

async fn inner_stack(
    state: S,
    opts: RouteOpts,
    mut req: Request,
    next: Next,
    identity: Arc<Mutex<Identity>>,
    start: Instant,
) -> Response {
    // RejectCrossOrigin
    if let Err(e) = cross_origin_check(req.method(), req.headers()) {
        return HttpError::forbidden("Cross-origin request rejected", anyhow::anyhow!(e))
            .into_response();
    }

    // NoStoreByDefault, applied to whatever comes back from here on.
    let mut resp = authed_part(&state, opts, &mut req, next, identity, start).await;
    if !resp.headers().contains_key(CACHE_CONTROL) {
        resp.headers_mut()
            .insert(CACHE_CONTROL, HeaderValue::from_static("no-store"));
    }
    resp
}

async fn authed_part(
    state: &S,
    opts: RouteOpts,
    req: &mut Request,
    next: Next,
    identity: Arc<Mutex<Identity>>,
    start: Instant,
) -> Response {
    match opts.auth {
        AuthMode::Required => match authenticate(state, req.headers()).await {
            Ok(claims) => {
                req.extensions_mut().insert(JwtCtx {
                    claims: Some(claims),
                    error: None,
                });
            }
            Err(e) => return authn_error(e).into_response(),
        },
        AuthMode::Optional => {
            let ctx = match authenticate(state, req.headers()).await {
                Ok(claims) => JwtCtx {
                    claims: Some(claims),
                    error: None,
                },
                Err(e) => JwtCtx {
                    claims: None,
                    error: Some(format!("{e:#}")),
                },
            };
            req.extensions_mut().insert(ctx);
        }
        AuthMode::None => {}
    }

    // LogRequest
    let claims = req
        .extensions()
        .get::<JwtCtx>()
        .and_then(|c| c.claims.clone());
    let mut who = Identity::default();
    if let Some(claims) = &claims {
        who.user_name = string_to_sql(Some(claims.ranger_handle()), 128);
        who.user_id = Some(claims.directory_id());
        if let Ok((pos_id, pos_name)) = state
            .user_store
            .on_duty_for_ranger(claims.directory_id())
            .await
        {
            if pos_id != 0 {
                who.position_id = Some(pos_id);
            }
            who.position_name = string_to_sql(Some(&pos_name), 128);
        }
    }
    *identity.lock().unwrap_or_else(|e| e.into_inner()) = who.clone();

    let method = req.method().clone();
    let path = req.uri().path().to_string();
    let referrer = request_referrer(req.headers());
    let remote = req
        .extensions()
        .get::<ConnectInfo<SocketAddr>>()
        .map(|c| c.0);
    let client_addr = client_address(req.headers(), remote);

    let body = std::mem::replace(req.body_mut(), Body::empty());
    let (body, captured) =
        if opts.log == ActionLogMode::LogMutation && is_json_content_type(req.headers()) {
            let (b, c) = CapturingBody::wrap(body);
            (b, Some(c))
        } else {
            (body, None)
        };
    // LimitRequestBytes, inside the capture, so an oversized body gets
    // rejected before much of it has been read.
    let limit = usize::try_from(state.cfg.core.max_request_bytes).unwrap_or(usize::MAX);
    *req.body_mut() = Body::new(Limited::new(body, limit));

    let req = std::mem::replace(req, Request::new(Body::empty()));
    let resp = next.run(req).await;

    if opts.log != ActionLogMode::LogNothing {
        let request_body =
            captured.and_then(|c| c.lock().unwrap_or_else(|e| e.into_inner()).logged());
        state
            .action_logger
            .log(ActionLog {
                id: 0,
                created_at: GoTime::now().to_float(),
                action_type: "api".to_string(),
                method: string_to_sql(Some(method.as_str()), 128),
                path: string_to_sql(Some(&path), 128),
                referrer: string_to_sql(referrer.as_deref(), 128),
                request_body: string_to_sql(request_body.as_deref(), 0),
                user_id: who.user_id,
                user_name: who.user_name,
                position_id: who.position_id,
                position_name: who.position_name,
                client_address: string_to_sql(Some(&client_addr), 128),
                http_status: Some(resp.status().as_u16() as i16),
                duration_micros: Some(start.elapsed().as_micros() as i64),
            })
            .await;
    }
    tracing::debug!(
        method = %method,
        path = %path,
        code = resp.status().as_u16(),
        duration = %format!("{:.3}ms", start.elapsed().as_micros() as f64 / 1000.0),
        "Served request"
    );
    resp
}

#[cfg(test)]
mod tests {
    use super::*;

    fn headers(pairs: &[(&'static str, &str)]) -> HeaderMap {
        let mut h = HeaderMap::new();
        for (k, v) in pairs {
            h.insert(*k, v.parse().unwrap());
        }
        h
    }

    #[test]
    fn cross_origin_allows_safe_methods() {
        let h = headers(&[("Sec-Fetch-Site", "cross-site")]);
        assert!(cross_origin_check(&Method::GET, &h).is_ok());
        assert!(cross_origin_check(&Method::POST, &h).is_err());
    }

    #[test]
    fn cross_origin_trusts_same_origin_and_none() {
        assert!(
            cross_origin_check(
                &Method::POST,
                &headers(&[("Sec-Fetch-Site", "same-origin")])
            )
            .is_ok()
        );
        assert!(cross_origin_check(&Method::POST, &headers(&[("Sec-Fetch-Site", "none")])).is_ok());
        assert!(
            cross_origin_check(&Method::POST, &headers(&[("Sec-Fetch-Site", "same-site")]))
                .is_err()
        );
    }

    #[test]
    fn cross_origin_falls_back_to_origin_vs_host() {
        // Non-browser clients send neither header.
        assert!(cross_origin_check(&Method::POST, &HeaderMap::new()).is_ok());
        let same = headers(&[
            ("Origin", "https://ims.example.org"),
            ("Host", "ims.example.org"),
        ]);
        assert!(cross_origin_check(&Method::POST, &same).is_ok());
        let other = headers(&[
            ("Origin", "https://evil.example.org"),
            ("Host", "ims.example.org"),
        ]);
        assert!(cross_origin_check(&Method::DELETE, &other).is_err());
    }

    #[test]
    fn client_address_prefers_proxy_headers() {
        let remote: SocketAddr = "10.0.0.1:1234".parse().unwrap();
        assert_eq!(client_address(&HeaderMap::new(), Some(remote)), "10.0.0.1");
        assert_eq!(
            client_address(&headers(&[("X-Forwarded-For", "1.2.3.4")]), Some(remote)),
            "1.2.3.4"
        );
        assert_eq!(
            client_address(
                &headers(&[
                    ("X-Forwarded-For", "1.2.3.4"),
                    ("CF-Connecting-IP", "5.6.7.8")
                ]),
                Some(remote)
            ),
            "5.6.7.8"
        );
    }

    #[test]
    fn referrer_is_trimmed_to_ims_path() {
        assert_eq!(
            request_referrer(&headers(&[(
                "Referer",
                "https://host.example/ims/app/events/2026"
            )]))
            .as_deref(),
            Some("/ims/app/events/2026")
        );
        assert_eq!(request_referrer(&HeaderMap::new()), None);
    }
}
