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

//! The web UI: static files and server-rendered pages, ported from the Go
//! server's `web` package. The pages are Askama templates generated from the
//! Go server's templ templates by rust/tools/templ2askama.py.

pub mod pages;
pub mod staticfiles;

use axum::Router;
use axum::extract::{Path, State};
use axum::http::header::{self, HeaderValue};
use axum::http::{Method, StatusCode, Uri};
use axum::response::{IntoResponse, Response};
use axum::routing::get;

use crate::api::state::S;
use crate::util::herr;
use pages::Page;

pub fn web_router(_st: S) -> Router<S> {
    let page = |page: Page| get(move |State(st): State<S>| async move { render(&st, page, "") });
    let event_page = |page: Page| {
        get(
            move |State(st): State<S>, Path(p): Path<std::collections::HashMap<String, String>>| async move {
                render(
                    &st,
                    page,
                    p.get("eventName").map(String::as_str).unwrap_or_default(),
                )
            },
        )
    };
    Router::new()
        .route("/ims/static/{*path}", get(staticfiles::serve))
        .route("/ims/app", page(Page::Root))
        .route("/ims/app/admin", page(Page::AdminRoot))
        .route("/ims/app/admin/actionlogs", page(Page::AdminActionLogs))
        .route("/ims/app/admin/errorlogs", page(Page::AdminErrorLogs))
        .route("/ims/app/admin/events", page(Page::AdminEvents))
        .route("/ims/app/admin/types", page(Page::AdminTypes))
        .route("/ims/app/admin/debug", page(Page::AdminDebug))
        .route("/ims/app/admin/directory", page(Page::AdminDirectory))
        .route("/ims/app/admin/places", page(Page::AdminPlaces))
        .route("/ims/app/settings", page(Page::Settings))
        .route("/ims/app/help", page(Page::Help))
        .route("/ims/app/search", page(Page::Search))
        .route(
            "/ims/app/events/{eventName}/places",
            event_page(Page::Places),
        )
        .route(
            "/ims/app/events/{eventName}/field_reports",
            event_page(Page::FieldReports),
        )
        .route(
            "/ims/app/events/{eventName}/field_reports/{fieldReportNumber}",
            event_page(Page::FieldReport),
        )
        .route(
            "/ims/app/events/{eventName}/incidents",
            event_page(Page::Incidents),
        )
        .route(
            "/ims/app/events/{eventName}/incidents/{incidentNumber}",
            event_page(Page::Incident),
        )
        .route(
            "/ims/app/events/{eventName}/visits",
            event_page(Page::SanctuaryVisits),
        )
        .route(
            "/ims/app/events/{eventName}/visits/{visitNumber}",
            event_page(Page::SanctuaryVisit),
        )
        .route("/ims/app/events/{eventName}", get(event_redirect))
        .route("/ims/auth/login", page(Page::Login))
        .route("/ims/auth/logout", get(logout))
        .route("/ims/app/", get(app_catch_all))
        .route("/ims/app/{*anything}", get(app_catch_all))
}

/// A page, with the headers every page gets: long-lived caching (the pages
/// only change when IMS does), no framing by other origins, and no leaking
/// of IMS paths to other sites through links.
fn render(st: &S, page: Page, event_name: &str) -> Response {
    let core = &st.cfg.core;
    let html = match pages::render(
        page,
        core.deployment_str(),
        &pages::version_name(),
        &pages::version_ref(),
        event_name,
    ) {
        Ok(html) => html,
        Err(e) => {
            tracing::error!(error = %e, "Failed to render template");
            return herr::HttpError::internal_server_error("Failed to parse template", e)
                .from_src("[Render]")
                .into_response();
        }
    };
    let mut resp = html.into_response();
    let h = resp.headers_mut();
    h.insert(
        header::CONTENT_TYPE,
        HeaderValue::from_static("text/html; charset=utf-8"),
    );
    if let Ok(v) = HeaderValue::from_str(&format!("max-age={}", core.cache_control_long.as_secs()))
    {
        h.insert(header::CACHE_CONTROL, v);
    }
    h.insert(
        header::CONTENT_SECURITY_POLICY,
        HeaderValue::from_static("frame-ancestors 'self'"),
    );
    h.insert(
        header::REFERRER_POLICY,
        HeaderValue::from_static("same-origin"),
    );
    resp
}

/// Go's `url.PathEscape`.
pub fn path_escape(s: &str) -> String {
    let mut out = String::new();
    for b in s.bytes() {
        if b.is_ascii_alphanumeric() || b"-_.~$&+=:@".contains(&b) {
            out.push(b as char);
        } else {
            out.push_str(&format!("%{b:02X}"));
        }
    }
    out
}

/// A redirect written as Go's `http.Redirect` writes one.
pub fn redirect(method: &Method, status: StatusCode, location: &str) -> Response {
    let mut resp = if *method == Method::GET || *method == Method::HEAD {
        let reason = status.canonical_reason().unwrap_or_default();
        let body = format!("<a href=\"{}\">{reason}</a>.\n\n", html_escape(location));
        let mut r = body.into_response();
        r.headers_mut().insert(
            header::CONTENT_TYPE,
            HeaderValue::from_static("text/html; charset=utf-8"),
        );
        r
    } else {
        Response::new(axum::body::Body::empty())
    };
    *resp.status_mut() = status;
    if let Ok(v) = HeaderValue::from_str(location) {
        resp.headers_mut().insert(header::LOCATION, v);
    }
    resp
}

fn html_escape(s: &str) -> String {
    s.replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&#34;")
        .replace('\'', "&#39;")
}

async fn event_redirect(method: Method, Path(event_name): Path<String>) -> Response {
    redirect(
        &method,
        StatusCode::FOUND,
        &format!("/ims/app/events/{}/incidents", path_escape(&event_name)),
    )
}

async fn logout(State(st): State<S>, method: Method) -> Response {
    tracing::info!("Redirecting from logout");
    let mut resp = redirect(&method, StatusCode::SEE_OTHER, "/ims/app?logout");
    for c in [
        st.cookies.expired_access_token(),
        st.cookies.expired_legacy_refresh_token(),
    ] {
        if let Ok(v) = HeaderValue::from_str(&c.header_value()) {
            resp.headers_mut().append(header::SET_COOKIE, v);
        }
    }
    resp
}

/// A known page with a trailing slash would otherwise be a 404, so it's
/// redirected to the page without one, staying under /ims/app.
async fn app_catch_all(method: Method, uri: Uri) -> Response {
    let path = uri.path();
    if path.ends_with('/') {
        let rest = path.strip_prefix("/ims/app").unwrap_or_default();
        let mut cleaned = clean_path(rest);
        if cleaned == "/" {
            cleaned = String::new();
        }
        return redirect(
            &method,
            StatusCode::MOVED_PERMANENTLY,
            &format!("/ims/app{cleaned}"),
        );
    }
    herr::text_response(StatusCode::NOT_FOUND, "404 page not found")
}

/// Go's `path.Clean`, for an absolute path.
pub fn clean_path(p: &str) -> String {
    let mut parts: Vec<&str> = Vec::new();
    for seg in p.split('/') {
        match seg {
            "" | "." => {}
            ".." => {
                parts.pop();
            }
            s => parts.push(s),
        }
    }
    format!("/{}", parts.join("/"))
}

/// Go's ServeMux redirects a request whose path isn't clean (with "..",
/// "." or doubled slashes) to the clean path, keeping a trailing slash.
pub async fn clean_path_redirect(
    req: axum::extract::Request,
    next: axum::middleware::Next,
) -> Response {
    let path = req.uri().path();
    let mut cleaned = clean_path(path);
    if path.ends_with('/') && cleaned != "/" {
        cleaned.push('/');
    }
    if cleaned != path {
        let location = match req.uri().query() {
            Some(q) => format!("{cleaned}?{q}"),
            None => cleaned,
        };
        return redirect(req.method(), StatusCode::MOVED_PERMANENTLY, &location);
    }
    next.run(req).await
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn go_path_escape() {
        assert_eq!(path_escape("2025"), "2025");
        assert_eq!(path_escape("a b/c"), "a%20b%2Fc");
    }

    #[test]
    fn go_path_clean() {
        assert_eq!(
            clean_path("/ims/app/events/x/incidents/../"),
            "/ims/app/events/x"
        );
        assert_eq!(clean_path("//ims//app"), "/ims/app");
        assert_eq!(clean_path("/"), "/");
    }
}

#[cfg(test)]
mod endpoint_tests {
    use axum::body::Body;
    use axum::http::{Request, StatusCode};
    use http_body_util::BodyExt;
    use tower::ServiceExt;

    use crate::api::testutil::test_state;
    use crate::conf::IMSConfig;

    const TEMPL_ENDPOINTS: [&str; 18] = [
        "/ims/app",
        "/ims/app/admin",
        "/ims/app/admin/errorlogs",
        "/ims/app/admin/places",
        "/ims/app/admin/events",
        "/ims/app/admin/types",
        "/ims/app/events/SomeEvent/places",
        "/ims/app/events/SomeEvent/field_reports",
        "/ims/app/events/SomeEvent/field_reports/123",
        "/ims/app/events/SomeEvent/incidents",
        "/ims/app/events/SomeEvent/incidents/123",
        "/ims/app/events/SomeEvent/visits",
        "/ims/app/events/SomeEvent/visits/123",
        "/ims/app/settings",
        "/ims/app/help",
        "/ims/app/search",
        "/ims/auth/login",
        "/ims/app?logout",
    ];

    fn app() -> axum::Router {
        crate::cmd::serve::app(test_state(IMSConfig::default(), vec![]))
    }

    async fn get(path: &str) -> axum::response::Response {
        app()
            .oneshot(Request::get(path).body(Body::empty()).unwrap())
            .await
            .unwrap()
    }

    #[tokio::test]
    async fn templ_endpoints() {
        for endpoint in TEMPL_ENDPOINTS {
            let resp = get(endpoint).await;
            assert_eq!(resp.status(), StatusCode::OK, "{endpoint}");
            let h = resp.headers();
            assert_eq!(h["content-type"], "text/html; charset=utf-8", "{endpoint}");
            assert_eq!(
                h["content-security-policy"], "frame-ancestors 'self'",
                "{endpoint}"
            );
            assert_eq!(h["referrer-policy"], "same-origin", "{endpoint}");
            assert_eq!(h["cache-control"], "max-age=7200", "{endpoint}");
            let body = resp.into_body().collect().await.unwrap().to_bytes();
            assert!(
                String::from_utf8_lossy(&body).contains("IMS © Burning Man Project"),
                "{endpoint}"
            );
        }
    }

    #[tokio::test]
    async fn logout_expires_cookies_and_redirects() {
        let resp = get("/ims/auth/logout").await;
        assert_eq!(resp.status(), StatusCode::SEE_OTHER);
        assert_eq!(resp.headers()["location"], "/ims/app?logout");
        let cookies: Vec<_> = resp
            .headers()
            .get_all("set-cookie")
            .iter()
            .map(|v| v.to_str().unwrap().to_string())
            .collect();
        assert_eq!(cookies.len(), 2);
        assert!(cookies[0].starts_with("__Host-ims_access_token=;"));
        assert!(cookies[1].starts_with("refresh_token=;"));
    }

    #[tokio::test]
    async fn redirects() {
        // Unclean paths go to the clean path first, as with Go's ServeMux.
        let resp = get("/ims/app/events/SomeEvent/incidents/../").await;
        assert_eq!(resp.status(), StatusCode::MOVED_PERMANENTLY);
        assert_eq!(resp.headers()["location"], "/ims/app/events/SomeEvent/");

        let resp = get("/ims/app/events/SomeEvent/").await;
        assert_eq!(resp.status(), StatusCode::MOVED_PERMANENTLY);
        assert_eq!(resp.headers()["location"], "/ims/app/events/SomeEvent");

        let resp = get("/ims/app/events/SomeEvent").await;
        assert_eq!(resp.status(), StatusCode::FOUND);
        assert_eq!(
            resp.headers()["location"],
            "/ims/app/events/SomeEvent/incidents"
        );

        let resp = get("/ims/app/events/Some%20Event").await;
        assert_eq!(
            resp.headers()["location"],
            "/ims/app/events/Some%20Event/incidents"
        );

        let resp = get("/ims/app/events/SomeEvent/book_reports").await;
        assert_eq!(resp.status(), StatusCode::NOT_FOUND);

        let resp = get("/ims/app/").await;
        assert_eq!(resp.status(), StatusCode::MOVED_PERMANENTLY);
        assert_eq!(resp.headers()["location"], "/ims/app");
    }

    #[tokio::test]
    async fn static_files() {
        let resp = get("/ims/static/style.css").await;
        assert_eq!(resp.status(), StatusCode::OK);
        assert_eq!(resp.headers()["content-type"], "text/css; charset=utf-8");
        assert_eq!(resp.headers()["cache-control"], "max-age=7200");

        let resp = get("/ims/static/ext/bootstrap/bootstrap.bundle.min.js").await;
        assert_eq!(resp.status(), StatusCode::OK);
        assert_eq!(
            resp.headers()["content-type"],
            "text/javascript; charset=utf-8"
        );

        let resp = get("/ims/static/nope.js").await;
        assert_eq!(resp.status(), StatusCode::NOT_FOUND);

        let resp = get("/ims/static/logos").await;
        assert_eq!(resp.status(), StatusCode::MOVED_PERMANENTLY);
        assert_eq!(resp.headers()["location"], "logos/");
    }
}
