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

//! The JSON API, ported from the Go server's `api` package.

pub mod actionlogbody;
pub mod attachment;
pub mod auth;
pub mod directory;
pub mod embargo;
pub mod event;
pub mod eventaccess;
pub mod eventsource;
pub mod fieldreport;
pub mod helpers;
pub mod incident;
pub mod incidentrelation;
pub mod itype;
pub mod logs;
pub mod middleware;
pub mod mux;
pub mod personnel;
pub mod place;
pub mod ranger;
pub mod reportentry;
pub mod search;
pub mod state;
pub mod visit;

pub use mux::{add_basic_handlers, api_router};
#[cfg(test)]
pub mod testutil;

#[cfg(test)]
mod tests {
    use axum::body::Body;
    use axum::http::{Request, StatusCode};
    use tower::ServiceExt;

    use super::testutil::test_state;
    use crate::conf::IMSConfig;

    #[tokio::test]
    async fn routes_exist() {
        let app = crate::cmd::serve::app(test_state(IMSConfig::default(), vec![]));
        let resp = app
            .clone()
            .oneshot(
                Request::post("/ims/api/auth")
                    .header("content-type", "application/json")
                    .body(Body::from("{}"))
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(resp.status(), StatusCode::UNAUTHORIZED);
        let resp = app
            .clone()
            .oneshot(Request::get("/ims/api/auth").body(Body::empty()).unwrap())
            .await
            .unwrap();
        assert_eq!(resp.status(), StatusCode::OK);
        let resp = app
            .clone()
            .oneshot(Request::get("/nope").body(Body::empty()).unwrap())
            .await
            .unwrap();
        assert_eq!(resp.status(), StatusCode::NOT_FOUND);
        let resp = app
            .oneshot(Request::put("/ims/api/auth").body(Body::empty()).unwrap())
            .await
            .unwrap();
        assert_eq!(resp.status(), StatusCode::METHOD_NOT_ALLOWED);
    }
}
