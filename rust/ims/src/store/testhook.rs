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

//! A test-only hook that lets the Go API integration suite act in the race
//! windows its in-process query interceptor reaches in the Go server: just
//! before a version-guarded UPDATE, or before a creation's INSERT. When
//! installed, those queries first POST their parameters to the hook URL and
//! wait for it to answer. Only a dev deployment can install it.

use std::sync::OnceLock;

static HOOK: OnceLock<(reqwest::Client, String)> = OnceLock::new();

pub fn install(url: &str) {
    let _ = HOOK.set((reqwest::Client::new(), url.to_string()));
}

pub async fn before(query: &str, mut params: serde_json::Value) {
    let Some((client, url)) = HOOK.get() else {
        return;
    };
    if let Some(obj) = params.as_object_mut() {
        obj.insert(
            "query".to_string(),
            serde_json::Value::String(query.to_string()),
        );
    }
    let body = params.to_string();
    if let Err(e) = client
        .post(url)
        .header("Content-Type", "application/json")
        .body(body)
        .send()
        .await
    {
        tracing::error!(error = %e, query, "test query hook failed");
    }
}
