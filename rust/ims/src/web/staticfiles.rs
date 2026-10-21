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

//! The static files under web/static (the tsc output, the external client
//! libraries, logos, and CSS), embedded at build time as the Go server embeds
//! them. `make generate` has to have run first.

use axum::extract::{Path, State};
use axum::http::header::{self, HeaderValue};
use axum::http::{Method, StatusCode};
use axum::response::{IntoResponse, Response};
use rust_embed::RustEmbed;

use crate::api::state::S;
use crate::util::herr;

#[derive(RustEmbed)]
#[folder = "../../web/static"]
#[exclude = ".*"]
#[exclude = "*/.*"]
struct StaticFiles;

/// Go's `mime.TypeByExtension` for its built-in types; anything else is
/// sniffed, as `http.ServeContent` does.
fn content_type(path: &str, data: &[u8]) -> String {
    let ext = path
        .rsplit_once('.')
        .map(|(_, e)| e.to_ascii_lowercase())
        .unwrap_or_default();
    let known = match ext.as_str() {
        "avif" => "image/avif",
        "css" => "text/css; charset=utf-8",
        "gif" => "image/gif",
        "htm" | "html" => "text/html; charset=utf-8",
        "jpeg" | "jpg" => "image/jpeg",
        "js" | "mjs" => "text/javascript; charset=utf-8",
        "json" => "application/json",
        "pdf" => "application/pdf",
        "png" => "image/png",
        "svg" => "image/svg+xml",
        "wasm" => "application/wasm",
        "webp" => "image/webp",
        "xml" => "text/xml; charset=utf-8",
        "ico" => "image/x-icon",
        _ => "",
    };
    if !known.is_empty() {
        return known.to_string();
    }
    if std::str::from_utf8(data).is_ok() {
        "text/plain; charset=utf-8".to_string()
    } else {
        "application/octet-stream".to_string()
    }
}

fn is_dir(prefix: &str) -> bool {
    StaticFiles::iter().any(|f| f.starts_with(prefix))
}

/// A directory listing, as Go's http.FileServer writes one.
fn listing(dir: &str) -> String {
    let mut entries: Vec<String> = StaticFiles::iter()
        .filter_map(|f| f.strip_prefix(dir).map(str::to_string))
        .map(|rest| match rest.split_once('/') {
            Some((d, _)) => format!("{d}/"),
            None => rest,
        })
        .collect();
    entries.sort();
    entries.dedup();
    let mut out = String::from(
        "<!doctype html>\n<meta name=\"viewport\" content=\"width=device-width\">\n<pre>\n",
    );
    for e in entries {
        out.push_str(&format!(
            "<a href=\"{}\">{}</a>\n",
            super::path_escape(&e).replace("%2F", "/"),
            e
        ));
    }
    out.push_str("</pre>\n");
    out
}

pub async fn serve(State(st): State<S>, method: Method, Path(path): Path<String>) -> Response {
    let mut resp = serve_file(&method, &path);
    if let Ok(v) = HeaderValue::from_str(&format!(
        "max-age={}",
        st.cfg.core.cache_control_long.as_secs()
    )) {
        resp.headers_mut().insert(header::CACHE_CONTROL, v);
    }
    resp
}

fn serve_file(method: &Method, path: &str) -> Response {
    let name = path.rsplit('/').find(|s| !s.is_empty()).unwrap_or("static");
    if let Some(file) = StaticFiles::get(path.trim_end_matches('/')) {
        if path.ends_with('/') {
            return super::redirect(method, StatusCode::MOVED_PERMANENTLY, &format!("../{name}"));
        }
        let mut resp = file.data.to_vec().into_response();
        if let Ok(v) = HeaderValue::from_str(&content_type(path, &file.data)) {
            resp.headers_mut().insert(header::CONTENT_TYPE, v);
        }
        resp.headers_mut()
            .insert(header::ACCEPT_RANGES, HeaderValue::from_static("bytes"));
        return resp;
    }
    let dir = if path.is_empty() || path.ends_with('/') {
        path.to_string()
    } else {
        format!("{path}/")
    };
    if is_dir(&dir) {
        if !path.is_empty() && !path.ends_with('/') {
            return super::redirect(method, StatusCode::MOVED_PERMANENTLY, &format!("{name}/"));
        }
        let mut resp = listing(&dir).into_response();
        resp.headers_mut().insert(
            header::CONTENT_TYPE,
            HeaderValue::from_static("text/html; charset=utf-8"),
        );
        return resp;
    }
    herr::text_response(StatusCode::NOT_FOUND, "404 page not found")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn content_types() {
        assert_eq!(
            content_type("ims.js", b""),
            "text/javascript; charset=utf-8"
        );
        assert_eq!(content_type("style.css", b""), "text/css; charset=utf-8");
        assert_eq!(content_type("logos/a.png", b""), "image/png");
        assert_eq!(
            content_type("logos/site.webmanifest", b"{}"),
            "text/plain; charset=utf-8"
        );
    }

    #[test]
    fn embeds_the_static_files() {
        assert!(StaticFiles::get("style.css").is_some());
        assert!(
            StaticFiles::get("ext/bootstrap/bootstrap.min.css").is_some(),
            "run `make generate` first"
        );
        assert!(StaticFiles::iter().all(|f| !f.contains("/.") && !f.starts_with('.')));
    }
}
