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

//! Captures a request's JSON body for the action log, as the Go server's
//! `api/actionlogbody.go` does. It records what the handler actually reads,
//! so a handler that ignores its body (or rejects it as too large) costs
//! nothing.

use std::pin::Pin;
use std::sync::{Arc, LazyLock, Mutex};
use std::task::{Context, Poll};

use axum::body::{Body, Bytes};
use axum::http::HeaderMap;
use axum::http::header::CONTENT_TYPE;
use http_body::{Frame, SizeHint};
use regex::Regex;

use crate::util::gojson;

/// What one action log row keeps of a request body at most.
pub const MAX_LOGGED_BODY_BYTES: usize = 16 * 1024;
const REDACTED: &str = "[redacted]";
const TRUNCATION_MARKER: &str = "…[truncated]";

/// A JSON password field and its string value, for redacting a body that
/// doesn't parse (a truncated or malformed one).
static PASSWORD_VALUE: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r#"(?i)("[^"]*password[^"]*"\s*:\s*)"(?:[^"\\]|\\.)*""#).expect("valid regex")
});

#[derive(Default)]
pub struct Captured {
    kept: Vec<u8>,
    truncated: bool,
}

impl Captured {
    fn record(&mut self, data: &[u8]) {
        let room = MAX_LOGGED_BODY_BYTES.saturating_sub(self.kept.len());
        if data.len() > room {
            self.truncated = true;
        }
        let n = data.len().min(room);
        self.kept.extend_from_slice(&data[..n]);
    }

    /// The captured body as it should be stored: passwords redacted, JSON
    /// compacted, and valid UTF-8. None when there was no body to keep.
    pub fn logged(&self) -> Option<String> {
        if self.kept.is_empty() {
            return None;
        }
        if !self.truncated
            && let Ok(mut parsed) = serde_json::from_slice::<serde_json::Value>(&self.kept)
        {
            redact_passwords(&mut parsed);
            if let Ok(s) = gojson::to_string(&parsed) {
                return Some(s);
            }
        }
        let raw = String::from_utf8_lossy(&self.kept).replace('\u{FFFD}', "");
        let mut s = PASSWORD_VALUE
            .replace_all(&raw, format!("${{1}}\"{REDACTED}\"").as_str())
            .into_owned();
        if self.truncated {
            s.push_str(TRUNCATION_MARKER);
        }
        Some(s)
    }
}

/// Replaces the value of any object field whose name mentions a password.
fn redact_passwords(v: &mut serde_json::Value) {
    match v {
        serde_json::Value::Object(map) => {
            for (k, val) in map.iter_mut() {
                if k.to_lowercase().contains("password") {
                    *val = serde_json::Value::String(REDACTED.to_string());
                } else {
                    redact_passwords(val);
                }
            }
        }
        serde_json::Value::Array(items) => items.iter_mut().for_each(redact_passwords),
        _ => {}
    }
}

/// A request body that copies what's read through it into `Captured`.
pub struct CapturingBody {
    inner: Body,
    captured: Arc<Mutex<Captured>>,
}

impl CapturingBody {
    pub fn wrap(inner: Body) -> (Body, Arc<Mutex<Captured>>) {
        let captured = Arc::new(Mutex::new(Captured::default()));
        let body = Body::new(CapturingBody {
            inner,
            captured: Arc::clone(&captured),
        });
        (body, captured)
    }
}

impl http_body::Body for CapturingBody {
    type Data = Bytes;
    type Error = axum::Error;

    fn poll_frame(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
    ) -> Poll<Option<Result<Frame<Bytes>, axum::Error>>> {
        let polled = Pin::new(&mut self.inner).poll_frame(cx);
        if let Poll::Ready(Some(Ok(frame))) = &polled
            && let Some(data) = frame.data_ref()
        {
            self.captured
                .lock()
                .unwrap_or_else(|e| e.into_inner())
                .record(data);
        }
        polled
    }

    fn is_end_stream(&self) -> bool {
        self.inner.is_end_stream()
    }

    fn size_hint(&self) -> SizeHint {
        self.inner.size_hint()
    }
}

/// Whether the request declares a JSON body, the only kind worth capturing.
pub fn is_json_content_type(headers: &HeaderMap) -> bool {
    headers
        .get(CONTENT_TYPE)
        .and_then(|v| v.to_str().ok())
        .and_then(|v| v.parse::<mime::Mime>().ok())
        .is_some_and(|m| m.essence_str().eq_ignore_ascii_case("application/json"))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn capture(body: &str) -> Option<String> {
        let mut c = Captured::default();
        c.record(body.as_bytes());
        c.logged()
    }

    #[test]
    fn compacts_json() {
        assert_eq!(
            capture(r#"{"summary": "A new summary"}"#).unwrap(),
            r#"{"summary":"A new summary"}"#
        );
    }

    #[test]
    fn sorts_keys_like_go() {
        assert_eq!(capture(r#"{"b": 1, "a": 2}"#).unwrap(), r#"{"a":2,"b":1}"#);
    }

    #[test]
    fn redacts_passwords() {
        assert_eq!(
            capture(r#"{"password":"hunter2"}"#).unwrap(),
            r#"{"password":"[redacted]"}"#
        );
    }

    #[test]
    fn redacts_nested_passwords() {
        let got = capture(r#"{"persons":[{"handle":"Hubcap","new_password":"hunter2"}]}"#).unwrap();
        assert!(got.contains(r#""handle":"Hubcap""#));
        assert!(got.contains(r#""new_password":"[redacted]""#));
        assert!(!got.contains("hunter2"));
    }

    #[test]
    fn truncates_oversized_body() {
        let huge = format!(r#"{{"summary":"{}"}}"#, "x".repeat(32 * 1024));
        let got = capture(&huge).unwrap();
        assert!(got.len() < huge.len());
        assert!(got.ends_with("…[truncated]"));
        assert!(got.starts_with(r#"{"summary":"xxx"#));
    }

    #[test]
    fn truncated_password_is_still_redacted() {
        let body = format!(
            r#"{{"password":"hunter2","notes":"{}"}}"#,
            "y".repeat(32 * 1024)
        );
        let got = capture(&body).unwrap();
        assert!(got.contains(r#""password":"[redacted]""#));
        assert!(!got.contains("hunter2"));
    }

    #[test]
    fn keeps_unparseable_body_as_text() {
        assert_eq!(capture(r#"{"summary": "#).unwrap(), r#"{"summary": "#);
    }

    #[test]
    fn empty_body_is_none() {
        assert_eq!(Captured::default().logged(), None);
    }

    #[test]
    fn json_content_type() {
        let mut h = HeaderMap::new();
        assert!(!is_json_content_type(&h));
        h.insert(
            CONTENT_TYPE,
            "application/json; charset=utf-8".parse().unwrap(),
        );
        assert!(is_json_content_type(&h));
        h.insert(
            CONTENT_TYPE,
            "multipart/form-data; boundary=xyz".parse().unwrap(),
        );
        assert!(!is_json_content_type(&h));
    }
}
