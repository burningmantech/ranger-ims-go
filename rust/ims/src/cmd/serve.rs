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

use std::net::SocketAddr;
use std::sync::Arc;
use std::time::Duration;

use axum::Router;
use tokio::net::TcpListener;

use super::goio;
use crate::api::{self, eventsource::EventSourcer, state::AppState};
use crate::authz::{JWTer, TokenCookies};
use crate::conf::{DirectoryType, IMSConfig};
use crate::directory::{self, Source, UserStore};
use crate::store::{self, actionlog::ActionLogger, errorlog::ErrorLogger};
use crate::util::attachment;

/// Builds the shared state the handlers use, connecting to the databases.
pub async fn build_state(cfg: IMSConfig) -> Result<AppState, String> {
    let db = store::connect(&cfg.store, true).await?;
    if !cfg.test_hooks.query_hook_url.is_empty() {
        store::testhook::install(&cfg.test_hooks.query_hook_url);
    }
    let source = if cfg.directory.directory.is(DirectoryType::Ims) {
        // The IMS-native directory lives in the IMS database itself.
        Source::Ims(directory::IMSSource::new(db.clone()))
    } else {
        let clubhouse = directory::clubhouse::connect(&cfg.directory.clubhouse_db).await?;
        Source::Clubhouse(directory::ClubhouseSource::new(clubhouse))
    };
    let user_store = UserStore::new(source, cfg.directory.in_memory_cache_ttl);
    let s3 = attachment::S3Client::from_config(&cfg).await?;
    let sync_logs = cfg.test_hooks.sync_logs;
    Ok(AppState {
        action_logger: ActionLogger::new(db.clone(), cfg.core.action_log_enabled, sync_logs),
        error_logger: ErrorLogger::new(db.clone(), cfg.core.error_log_enabled, sync_logs),
        jwter: JWTer {
            secret_key: cfg.core.jwt_secret.clone(),
        },
        cookies: TokenCookies {
            insecure: cfg.core.insecure_cookies,
        },
        es: Arc::new(EventSourcer::new()),
        bm_http: reqwest::Client::builder()
            .timeout(Duration::from_secs(30))
            .build()
            .map_err(|e| e.to_string())?,
        s3,
        user_store,
        db,
        cfg: Arc::new(cfg),
    })
}

/// The whole application: the API plus the web UI.
pub fn app(state: AppState) -> Router {
    let st = Arc::new(state);
    let router = api::api_router(st.clone()).merge(crate::web::web_router(st.clone()));
    router
        .fallback(api::mux::not_found)
        .method_not_allowed_fallback(api::mux::method_not_allowed)
        .layer(axum::middleware::from_fn(crate::web::clean_path_redirect))
        .with_state(st)
}

/// Starts the server and blocks until it's told to shut down, returning the
/// process exit code.
pub async fn run(mut cfg: IMSConfig, print_config: bool) -> i32 {
    if let Err(e) = cfg.validate() {
        panic!("got a startup error: {e}");
    }
    configure_logger(&cfg.core.log_level);
    if print_config {
        eprintln!(
            "Here's the final redacted IMSConfig:\n\n{}\n",
            cfg.print_redacted()
        );
        let secret = &cfg.core.jwt_secret;
        if let (Some(first), Some(last)) = (secret.chars().next(), secret.chars().last()) {
            eprintln!("With JWTSecret: {first}...{last}");
        }
    }

    let host = cfg.core.host.clone();
    let port = cfg.core.port as u16;
    let state = build_state(cfg)
        .await
        .unwrap_or_else(|e| panic!("got a startup error: {e}"));
    let router = app(state);

    let listener = TcpListener::bind((host.as_str(), port))
        .await
        .unwrap_or_else(|e| panic!("got a startup error: {e}"));
    let addr = listener
        .local_addr()
        .expect("bound listener has an address");
    tracing::info!(%addr, "IMS server is ready for connections");
    tracing::info!("Visit the web frontend at http://{addr}/ims/app");
    // The API integration harness reads this line to learn the port when
    // IMS_PORT=0, so keep its format stable.
    println!("ims listening on {addr}");

    serve_until_shutdown(listener, router).await;
    69
}

/// Serves connections until SIGINT/SIGTERM, then gives in-flight requests
/// some time to finish. This is axum::serve, done by hand so that each
/// connection's IO can go through GoErrorIo.
async fn serve_until_shutdown(listener: TcpListener, router: Router) {
    use hyper_util::rt::{TokioExecutor, TokioIo};
    use hyper_util::server::conn::auto::Builder;
    use hyper_util::server::graceful::GracefulShutdown;
    use tower::Service;

    let mut make_service = router.into_make_service_with_connect_info::<SocketAddr>();
    let graceful = GracefulShutdown::new();
    let builder = Builder::new(TokioExecutor::new());
    let signal = shutdown_signal();
    tokio::pin!(signal);
    loop {
        let (stream, remote) = tokio::select! {
            accepted = listener.accept() => match accepted {
                Ok(conn) => conn,
                Err(e) => {
                    tracing::error!(error = %e, "accept failed");
                    continue;
                }
            },
            () = &mut signal => break,
        };
        let _ = stream.set_nodelay(true);
        let tower_service = match make_service.call(remote).await {
            Ok(svc) => svc,
            Err(never) => match never {},
        };
        let hyper_service =
            hyper::service::service_fn(move |req: hyper::Request<hyper::body::Incoming>| {
                tower_service.clone().call(req.map(axum::body::Body::new))
            });
        let conn = builder
            .serve_connection_with_upgrades(
                TokioIo::new(goio::GoErrorIo::new(stream)),
                hyper_service,
            )
            .into_owned();
        let conn = graceful.watch(conn);
        tokio::spawn(async move {
            let _ = conn.await;
        });
    }
    tracing::error!("Shutting down gracefully, press Ctrl+C again to force");
    match tokio::time::timeout(Duration::from_secs(10), graceful.shutdown()).await {
        Ok(()) => tracing::error!("Server shut down"),
        Err(_) => tracing::error!("Server shut down after timing out on in-flight requests"),
    }
}

async fn shutdown_signal() {
    let ctrl_c = tokio::signal::ctrl_c();
    #[cfg(unix)]
    {
        let mut term = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
            .expect("installing SIGTERM handler");
        tokio::select! {
            _ = ctrl_c => {},
            _ = term.recv() => {},
        }
    }
    #[cfg(not(unix))]
    {
        let _ = ctrl_c.await;
    }
}

fn configure_logger(level: &str) {
    let filter = match level.to_ascii_uppercase().as_str() {
        "DEBUG" => "debug",
        "INFO" => "info",
        "WARN" => "warn",
        "ERROR" => "error",
        other => panic!("got a startup error: unknown log level {other}"),
    };
    let _ = tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::new(filter))
        .with_writer(std::io::stderr)
        .try_init();
}
