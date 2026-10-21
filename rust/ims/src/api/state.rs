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

use std::sync::Arc;

use sqlx::MySqlPool;

use super::eventsource::EventSourcer;
use crate::authz::{JWTer, TokenCookies};
use crate::conf::{AttachmentsStoreType, DirectoryType, IMSConfig};
use crate::directory::UserStore;
use crate::store::actionlog::ActionLogger;
use crate::store::errorlog::ErrorLogger;
use crate::util::attachment::S3Client;

/// Everything a handler might need, shared across requests.
pub struct AppState {
    pub cfg: Arc<IMSConfig>,
    pub db: MySqlPool,
    pub user_store: Arc<UserStore>,
    pub jwter: JWTer,
    pub cookies: TokenCookies,
    pub action_logger: ActionLogger,
    pub error_logger: ErrorLogger,
    pub es: Arc<EventSourcer>,
    pub s3: Option<S3Client>,
    pub bm_http: reqwest::Client,
}

pub type S = Arc<AppState>;

impl AppState {
    pub fn admins(&self) -> &[String] {
        &self.cfg.core.admins
    }

    pub fn attachments_enabled(&self) -> bool {
        !self
            .cfg
            .attachments_store
            .store_type
            .is(AttachmentsStoreType::None)
    }

    pub fn directory_is_ims(&self) -> bool {
        self.cfg.directory.directory.is(DirectoryType::Ims)
    }
}
