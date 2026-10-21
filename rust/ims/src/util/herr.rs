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

//! HTTP errors and plain responses, ported from the Go server's `lib/herr`.
//! An `HttpError` carries both a client-safe message and an internal error;
//! only the former goes out, as an RFC 9457 problem document.

use std::fmt;

use axum::http::{HeaderValue, StatusCode, header};
use axum::response::{IntoResponse, Response};
use serde::Serialize;

use crate::util::conv::GoTime;
use crate::util::gojson;

pub const APPLICATION_PROBLEM_MEDIA_TYPE: &str = "application/problem+json";

pub struct HttpError {
    pub code: StatusCode,
    pub response_message: String,
    pub internal: anyhow::Error,
    /// This error happens in normal operation, so isn't worth logging.
    pub expected: bool,
}

/// What the error-log middleware learns about an error response. It's stashed
/// in the response's extensions, since by then the HttpError itself is gone.
#[derive(Clone, Debug)]
pub struct RecordedError {
    pub code: StatusCode,
    pub response_message: String,
    pub internal_error: String,
    pub canceled: bool,
}

/// Marks an internal error as a client that hung up mid-request, which the
/// error log ignores.
#[derive(Debug)]
pub struct Canceled;

impl fmt::Display for Canceled {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "context canceled")
    }
}

impl std::error::Error for Canceled {}

impl HttpError {
    pub fn new(
        code: StatusCode,
        message: impl Into<String>,
        internal: Option<anyhow::Error>,
    ) -> HttpError {
        let response_message = message.into();
        let internal = internal.unwrap_or_else(|| anyhow::anyhow!(response_message.clone()));
        HttpError {
            code,
            response_message,
            internal,
            expected: false,
        }
    }

    pub fn internal_server_error(
        msg: impl Into<String>,
        err: impl Into<anyhow::Error>,
    ) -> HttpError {
        HttpError::new(StatusCode::INTERNAL_SERVER_ERROR, msg, Some(err.into()))
    }

    pub fn bad_request(msg: impl Into<String>, err: impl Into<anyhow::Error>) -> HttpError {
        HttpError::new(StatusCode::BAD_REQUEST, msg, Some(err.into()))
    }

    pub fn bad_request_msg(msg: impl Into<String>) -> HttpError {
        HttpError::new(StatusCode::BAD_REQUEST, msg, None)
    }

    pub fn request_entity_too_large(
        msg: impl Into<String>,
        err: impl Into<anyhow::Error>,
    ) -> HttpError {
        HttpError::new(StatusCode::PAYLOAD_TOO_LARGE, msg, Some(err.into()))
    }

    pub fn unsupported_media_type(
        msg: impl Into<String>,
        err: impl Into<anyhow::Error>,
    ) -> HttpError {
        HttpError::new(StatusCode::UNSUPPORTED_MEDIA_TYPE, msg, Some(err.into()))
    }

    pub fn unauthorized(msg: impl Into<String>, err: impl Into<anyhow::Error>) -> HttpError {
        HttpError::new(StatusCode::UNAUTHORIZED, msg, Some(err.into()))
    }

    pub fn forbidden(msg: impl Into<String>, err: impl Into<anyhow::Error>) -> HttpError {
        HttpError::new(StatusCode::FORBIDDEN, msg, Some(err.into()))
    }

    pub fn forbidden_msg(msg: impl Into<String>) -> HttpError {
        HttpError::new(StatusCode::FORBIDDEN, msg, None)
    }

    pub fn not_found(msg: impl Into<String>, err: impl Into<anyhow::Error>) -> HttpError {
        HttpError::new(StatusCode::NOT_FOUND, msg, Some(err.into()))
    }

    pub fn not_found_msg(msg: impl Into<String>) -> HttpError {
        HttpError::new(StatusCode::NOT_FOUND, msg, None)
    }

    pub fn conflict(msg: impl Into<String>, err: impl Into<anyhow::Error>) -> HttpError {
        HttpError::new(StatusCode::CONFLICT, msg, Some(err.into()))
    }

    /// Wraps the internal error with the name of the function it came from,
    /// like the Go server's `HTTPError.From`.
    pub fn from_src(mut self, source: &str) -> HttpError {
        self.internal = self.internal.context(source.to_string());
        self
    }

    pub fn set_expected(mut self) -> HttpError {
        self.expected = true;
        self
    }

    pub fn internal_error_string(&self) -> String {
        format!("{:#}", self.internal)
    }

    /// Finds an error of type E anywhere in the internal error's chain.
    pub fn find<E: std::error::Error + Send + Sync + 'static>(&self) -> Option<&E> {
        self.internal.chain().find_map(|e| e.downcast_ref::<E>())
    }
}

impl fmt::Debug for HttpError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "HTTP {}: ResponseMessage:'{}', InternalError:'{:#}'",
            self.code.as_u16(),
            self.response_message,
            self.internal
        )
    }
}

impl fmt::Display for HttpError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        fmt::Debug::fmt(self, f)
    }
}

#[derive(Serialize)]
struct Problem<'a> {
    #[serde(rename = "type")]
    problem_type: &'a str,
    status: u16,
    #[serde(skip_serializing_if = "str::is_empty")]
    detail: &'a str,
    instance: &'a str,
    timestamp: GoTime,
}

impl IntoResponse for HttpError {
    fn into_response(self) -> Response {
        if !self.expected {
            tracing::error!(
                code = self.code.as_u16(),
                message = %self.response_message,
                internal_error = %format!("{:#}", self.internal),
                "Writing error HTTP response"
            );
        }
        let problem = Problem {
            problem_type: "",
            status: self.code.as_u16(),
            detail: &self.response_message,
            instance: "",
            timestamp: GoTime::now(),
        };
        let body = gojson::to_string(&problem)
            .unwrap_or_else(|_| r#"{"detail":"Failed to marshal problem response"}"#.to_string());
        let mut resp = (self.code, body).into_response();
        resp.headers_mut().insert(
            header::CONTENT_TYPE,
            HeaderValue::from_static(APPLICATION_PROBLEM_MEDIA_TYPE),
        );
        resp.extensions_mut().insert(RecordedError {
            code: self.code,
            response_message: self.response_message.clone(),
            internal_error: format!("{:#}", self.internal),
            canceled: self.find::<Canceled>().is_some(),
        });
        resp
    }
}

/// A text/plain response written the way Go's `http.Error` writes one, which
/// is what the Go server's `WriteOKResponse` and friends use: the text plus a
/// trailing newline, and a nosniff header.
pub fn text_response(status: StatusCode, text: &str) -> Response {
    let mut resp = (status, format!("{text}\n")).into_response();
    let headers = resp.headers_mut();
    headers.insert(
        header::CONTENT_TYPE,
        HeaderValue::from_static("text/plain; charset=utf-8"),
    );
    headers.insert(
        header::X_CONTENT_TYPE_OPTIONS,
        HeaderValue::from_static("nosniff"),
    );
    resp
}

pub fn ok_response(text: &str) -> Response {
    text_response(StatusCode::OK, text)
}

pub fn no_content_response(text: &str) -> Response {
    // Go's http.Error writes a body even for a 204, but net/http then drops
    // it, since a 204 can't have one. The headers stay.
    let mut resp = text_response(StatusCode::NO_CONTENT, text);
    *resp.body_mut() = axum::body::Body::empty();
    resp
}

pub fn created_response(text: &str) -> Response {
    text_response(StatusCode::CREATED, text)
}

#[cfg(test)]
mod tests {
    use http_body_util::BodyExt;

    use super::*;

    #[test]
    fn from_src_wraps_internal_error() {
        let e = HttpError::bad_request("Bad thing", anyhow::anyhow!("root cause"))
            .from_src("[inner]")
            .from_src("[outer]");
        assert_eq!(e.internal_error_string(), "[outer]: [inner]: root cause");
        assert_eq!(e.response_message, "Bad thing");
        assert_eq!(e.code, StatusCode::BAD_REQUEST);
    }

    #[test]
    fn missing_internal_error_uses_message() {
        let e = HttpError::not_found_msg("No such thing");
        assert_eq!(e.internal_error_string(), "No such thing");
    }

    #[test]
    fn find_sees_through_context() {
        let e = HttpError::internal_server_error("x", anyhow::Error::new(Canceled)).from_src("[a]");
        assert!(e.find::<Canceled>().is_some());
    }

    #[tokio::test]
    async fn renders_problem_json() {
        let resp = HttpError::forbidden_msg("No <way>").into_response();
        assert_eq!(resp.status(), StatusCode::FORBIDDEN);
        assert_eq!(
            resp.headers()["content-type"],
            APPLICATION_PROBLEM_MEDIA_TYPE
        );
        assert!(resp.extensions().get::<RecordedError>().is_some());
        let body = resp.into_body().collect().await.unwrap().to_bytes();
        let v: serde_json::Value = serde_json::from_slice(&body).unwrap();
        assert_eq!(v["status"], 403);
        assert_eq!(v["detail"], "No <way>");
        assert_eq!(v["type"], "");
        assert_eq!(v["instance"], "");
        assert!(
            String::from_utf8(body.to_vec())
                .unwrap()
                .contains("\\u003cway\\u003e")
        );
    }
}
