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

//! The Ranger personnel list. Ported from the Go server's `api/personnel.go`.

use axum::Extension;
use axum::extract::State;
use axum::response::Response;

use super::helpers::*;
use super::middleware::JwtCtx;
use super::state::S;
use crate::authz;
use crate::json;
use crate::util::herr::HttpError;

pub async fn get_personnel(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
) -> HResult<Response> {
    let global = get_global_permissions(&st, &jwt)
        .await
        .map_err(|e| e.from_src("[getGlobalPermissions]"))?;
    if global & authz::GLOBAL_READ_PERSONNEL == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have GlobalReadPersonnel permission",
        ));
    }
    let rangers = st.user_store.get_rangers().await.map_err(|e| {
        HttpError::internal_server_error("Failed to get personnel", e).from_src("[GetRangers]")
    })?;
    // Email addresses and passwords never go out in the API; json::Person has
    // no fields for them.
    let resp: Vec<json::Person> = rangers
        .into_iter()
        .map(|r| json::Person {
            handle: r.handle,
            status: r.status,
            onsite: r.onsite,
            directory_id: r.directory_id,
        })
        .collect();
    Ok(with_cache_control(
        json_response(&resp),
        st.cfg.core.cache_control_short,
    ))
}
