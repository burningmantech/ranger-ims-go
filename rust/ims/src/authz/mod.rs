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

//! Authentication tokens and authorization rules, ported from the Go
//! server's `lib/authz` and `lib/authn`.

pub mod cookie;
pub mod jwt;
pub mod permission;

pub use cookie::TokenCookies;
pub use jwt::{IMSClaims, JWTer};
pub use permission::*;

use std::sync::LazyLock;

use crate::util::argon2id;

/// Only one Argon2id computation may run at a time. The Clubhouse's params
/// use 8 MiB apiece, and a burst of logins could otherwise push the process
/// over its container's memory limit.
static ARGON_LOCK: LazyLock<tokio::sync::Semaphore> =
    LazyLock::new(|| tokio::sync::Semaphore::new(1));

/// Checks a password against a stored Argon2id hash.
pub async fn verify_password(password: &str, stored: &str) -> anyhow::Result<bool> {
    if !stored.starts_with("$argon2id") {
        anyhow::bail!("unsupported non-argon2id stored password");
    }
    let _permit = ARGON_LOCK
        .acquire()
        .await
        .expect("semaphore is never closed");
    let password = password.to_string();
    let stored = stored.to_string();
    let result = tokio::task::spawn_blocking(move || {
        argon2id::compare_password_and_hash(&password, &stored)
    })
    .await
    .map_err(|e| anyhow::anyhow!("argon2id task failed: {e}"))?;
    Ok(result?)
}

/// A hash for dev-only users, e.g. those made by `add-user` against a dev stack.
pub fn new_salted_argon2id_dev_only(password: &str) -> String {
    argon2id::create_hash(password, &argon2id::development_params())
}
