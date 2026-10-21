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

//! An AppState for handler tests that don't touch the database.

use std::sync::Arc;
use std::time::Duration;

use sqlx::mysql::MySqlPoolOptions;

use super::eventsource::EventSourcer;
use super::state::AppState;
use crate::authz::{JWTer, TokenCookies};
use crate::conf::IMSConfig;
use crate::directory::{Source, Titles, User, UserStore, Users};
use crate::store::actionlog::ActionLogger;
use crate::store::errorlog::ErrorLogger;

pub fn test_state(cfg: IMSConfig, users: Vec<User>) -> AppState {
    // Never connects unless a query runs.
    let db = MySqlPoolOptions::new()
        .connect_lazy("mysql://nobody@127.0.0.1:1/none")
        .unwrap();
    let users: Users = users.into_iter().map(|u| (u.id, u)).collect();
    AppState {
        action_logger: ActionLogger::new(db.clone(), false, true),
        error_logger: ErrorLogger::new(db.clone(), false, true),
        jwter: JWTer {
            secret_key: cfg.core.jwt_secret.clone(),
        },
        cookies: TokenCookies {
            insecure: cfg.core.insecure_cookies,
        },
        es: Arc::new(EventSourcer::new()),
        bm_http: reqwest::Client::new(),
        s3: None,
        user_store: UserStore::new(
            Source::Static {
                users,
                positions: Titles::new(),
                teams: Titles::new(),
            },
            Duration::from_secs(60),
        ),
        db,
        cfg: Arc::new(cfg),
    }
}
