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

//! Action and error log readers. Ported from the Go server's
//! `api/actionlog.go` and `api/errorlog.go`.

use std::time::Duration;

use axum::Extension;
use axum::extract::{Query, State};
use axum::response::Response;
use serde::Deserialize;

use super::helpers::*;
use super::middleware::JwtCtx;
use super::state::{AppState, S};
use crate::authz;
use crate::json;
use crate::store::queries;
use crate::util::conv::GoTime;
use crate::util::goduration;
use crate::util::herr::HttpError;

#[derive(Deserialize)]
pub struct LogQuery {
    #[serde(default, rename = "userName")]
    user_name: String,
    #[serde(default)]
    path: String,
    #[serde(default)]
    page: String,
    #[serde(default, rename = "minTimeUnixMs")]
    min_time_unix_ms: String,
    #[serde(default, rename = "maxTimeUnixMs")]
    max_time_unix_ms: String,
}

impl LogQuery {
    fn time_range(&self) -> HResult<(f64, f64)> {
        let parse = |name: &str, v: &str| -> HResult<Option<f64>> {
            if v.is_empty() {
                return Ok(None);
            }
            let ms: i64 = v.parse().map_err(|e| {
                HttpError::bad_request(name.to_string(), anyhow::anyhow!("{e}"))
                    .from_src("[ParseInt64]")
            })?;
            Ok(Some(ms as f64 / 1e3))
        };
        Ok((
            parse("minTimeUnixMs", &self.min_time_unix_ms)?.unwrap_or(1e0),
            parse("maxTimeUnixMs", &self.max_time_unix_ms)?.unwrap_or(1e100),
        ))
    }
}

async fn require_debugging(st: &AppState, jwt: &JwtCtx) -> HResult<()> {
    let global = get_global_permissions(st, jwt)
        .await
        .map_err(|e| e.from_src("[getGlobalPermissions]"))?;
    if global & authz::GLOBAL_ADMINISTRATE_DEBUGGING == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have GlobalAdministrateDebugging permission",
        ));
    }
    Ok(())
}

fn duration_string(micros: Option<i64>) -> String {
    goduration::format(Duration::from_micros(micros.unwrap_or(0).max(0) as u64))
}

pub async fn get_action_logs(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Query(q): Query<LogQuery>,
) -> HResult<Response> {
    require_debugging(&st, &jwt).await?;
    let (min, max) = q.time_range()?;
    let rows = queries::action_logs(&st.db, min, max)
        .await
        .map_err(db_err("Failed to fetch ActionLogs", "[ActionLogs]"))?;
    let resp: Vec<json::ActionLog> = rows
        .into_iter()
        .filter(|al| {
            q.user_name.is_empty() || al.user_name.as_deref().unwrap_or_default() == q.user_name
        })
        .filter(|al| q.path.is_empty() || al.path.as_deref().unwrap_or_default() == q.path)
        .filter(|al| q.page.is_empty() || al.referrer.as_deref().unwrap_or_default() == q.page)
        .map(|al| json::ActionLog {
            id: al.id,
            created_at: GoTime::from_float(al.created_at),
            action_type: al.action_type,
            method: al.method.unwrap_or_default(),
            path: al.path.unwrap_or_default(),
            referrer: al.referrer.unwrap_or_default(),
            request_body: al.request_body.unwrap_or_default(),
            user_id: al.user_id.unwrap_or_default(),
            user_name: al.user_name.unwrap_or_default(),
            position_id: al.position_id.unwrap_or_default(),
            position_name: al.position_name.unwrap_or_default(),
            client_address: al.client_address.unwrap_or_default(),
            http_status: al.http_status.unwrap_or_default(),
            duration: duration_string(al.duration_micros),
        })
        .collect();
    Ok(json_response(&resp))
}

pub async fn get_error_logs(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Query(q): Query<LogQuery>,
) -> HResult<Response> {
    require_debugging(&st, &jwt).await?;
    let (min, max) = q.time_range()?;
    let rows = queries::error_logs(&st.db, min, max)
        .await
        .map_err(db_err("Failed to fetch ErrorLogs", "[ErrorLogs]"))?;
    let resp: Vec<json::ErrorLog> = rows
        .into_iter()
        .filter(|el| {
            q.user_name.is_empty() || el.user_name.as_deref().unwrap_or_default() == q.user_name
        })
        .filter(|el| q.path.is_empty() || el.path.as_deref().unwrap_or_default() == q.path)
        .map(|el| json::ErrorLog {
            id: el.id,
            created_at: GoTime::from_float(el.created_at),
            http_status: el.http_status,
            response_message: el.response_message.unwrap_or_default(),
            internal_error: el.internal_error.unwrap_or_default(),
            stack_trace: el.stack_trace.unwrap_or_default(),
            method: el.method.unwrap_or_default(),
            path: el.path.unwrap_or_default(),
            referrer: el.referrer.unwrap_or_default(),
            user_id: el.user_id.unwrap_or_default(),
            user_name: el.user_name.unwrap_or_default(),
            position_id: el.position_id.unwrap_or_default(),
            position_name: el.position_name.unwrap_or_default(),
            client_address: el.client_address.unwrap_or_default(),
            duration: duration_string(el.duration_micros),
        })
        .collect();
    Ok(json_response(&resp))
}

pub mod debug {
    //! Admin debugging endpoints. The Go server reports Go runtime details
    //! here; this reports the Rust equivalents that exist.

    use std::sync::LazyLock;
    use std::time::Instant;

    use axum::Extension;
    use axum::extract::State;
    use axum::http::{HeaderValue, header};
    use axum::response::Response;

    use super::require_debugging;
    use crate::api::helpers::HResult;
    use crate::api::middleware::JwtCtx;
    use crate::api::state::S;
    use crate::util::conv::GoTime;
    use crate::util::goduration;
    use crate::util::herr;

    static SERVER_START: LazyLock<(Instant, GoTime)> =
        LazyLock::new(|| (Instant::now(), GoTime::now()));

    pub fn mark_server_start() {
        LazyLock::force(&SERVER_START);
    }

    fn no_cache(mut resp: Response) -> Response {
        resp.headers_mut()
            .insert(header::CACHE_CONTROL, HeaderValue::from_static("no-cache"));
        resp
    }

    pub async fn get_build_info(
        State(st): State<S>,
        Extension(jwt): Extension<JwtCtx>,
    ) -> HResult<Response> {
        require_debugging(&st, &jwt).await?;
        let (started, started_at) = &*SERVER_START;
        let text = format!(
            "Server started at: {}\nUptime: {}\n\npath\tims (Rust port)\nversion\t{}\nvcs.revision\t{}\nrustc\t{}\n",
            started_at.format_rfc3339_nano(),
            goduration::format(started.elapsed()),
            env!("CARGO_PKG_VERSION"),
            option_env!("IMS_GIT_REVISION").unwrap_or("unknown"),
            option_env!("IMS_RUSTC_VERSION").unwrap_or("unknown"),
        );
        Ok(no_cache(herr::ok_response(&text)))
    }

    /// Resident set size, from /proc where there is one.
    fn rss_bytes() -> Option<u64> {
        let status = std::fs::read_to_string("/proc/self/status").ok()?;
        let line = status.lines().find(|l| l.starts_with("VmRSS:"))?;
        let kb: u64 = line.split_whitespace().nth(1)?.parse().ok()?;
        Some(kb * 1024)
    }

    pub async fn get_runtime_metrics(
        State(st): State<S>,
        Extension(jwt): Extension<JwtCtx>,
    ) -> HResult<Response> {
        require_debugging(&st, &jwt).await?;
        let (started, _) = &*SERVER_START;
        let tokio_metrics = tokio::runtime::Handle::current().metrics();
        let mut text = String::new();
        text.push_str(&format!(
            "/process/uptime:seconds: {:.6}\n",
            started.elapsed().as_secs_f64()
        ));
        if let Some(rss) = rss_bytes() {
            text.push_str(&format!("/process/memory/rss:bytes: {rss}\n"));
        }
        text.push_str(&format!(
            "/tokio/workers:threads: {}\n",
            tokio_metrics.num_workers()
        ));
        text.push_str(&format!(
            "/tokio/alive-tasks:tasks: {}\n",
            tokio_metrics.num_alive_tasks()
        ));
        text.push_str(&format!("/db/pool/size:connections: {}\n", st.db.size()));
        text.push_str(&format!(
            "/db/pool/idle:connections: {}\n",
            st.db.num_idle()
        ));
        Ok(no_cache(herr::ok_response(&text)))
    }

    pub async fn get_config(
        State(st): State<S>,
        Extension(jwt): Extension<JwtCtx>,
    ) -> HResult<Response> {
        require_debugging(&st, &jwt).await?;
        Ok(no_cache(herr::ok_response(&st.cfg.print_redacted())))
    }

    /// Rust has no garbage collector, so this only checks permission and
    /// reports the time, as the Go endpoint does after its GC.
    pub async fn perform_gc(
        State(st): State<S>,
        Extension(jwt): Extension<JwtCtx>,
    ) -> HResult<Response> {
        require_debugging(&st, &jwt).await?;
        Ok(herr::ok_response(&format!(
            "Performed GC at {}",
            GoTime::now().format_rfc3339_nano()
        )))
    }
}
