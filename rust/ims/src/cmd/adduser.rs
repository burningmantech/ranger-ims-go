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

//! `add-user`: creates or updates a user in the IMS-native directory, mainly
//! to bootstrap the first admin of a fresh deployment. Ported from the Go
//! server's `cmd/adduser.go`.

use std::io::BufRead;

use sqlx::MySqlPool;

use crate::conf::{DBStoreType, DirectoryType, IMSConfig};
use crate::store::{self, queries};
use crate::util::argon2id;

pub struct Args {
    pub handle: String,
    pub email: String,
    /// None when --onsite wasn't given, which leaves an existing user's flag alone.
    pub onsite: Option<bool>,
    pub password_stdin: bool,
}

pub fn check_config(cfg: &IMSConfig) -> Result<(), String> {
    if !cfg.directory.directory.is(DirectoryType::Ims) {
        let current = match &cfg.directory.directory {
            crate::conf::Setting::Known(d) => d.as_str().to_string(),
            crate::conf::Setting::Unknown(s) => s.clone(),
        };
        return Err(format!(
            "add-user manages the IMS-native directory, but this deployment's IMS_DIRECTORY is {current:?}. \
             Set IMS_DIRECTORY=ims to use the IMS-native directory"
        ));
    }
    if !cfg.store.store_type.is(DBStoreType::Maria) {
        return Err("add-user requires a MariaDB IMS datastore".to_string());
    }
    Ok(())
}

pub async fn run(cfg: IMSConfig, args: Args) -> Result<(), String> {
    check_config(&cfg)?;
    let password = read_password(args.password_stdin, &mut std::io::stdin().lock())
        .map_err(|e| format!("[readPassword]: {e}"))?;
    let hashed = tokio::task::spawn_blocking(move || {
        argon2id::create_hash(&password, &argon2id::SECOND_RECOMMENDED_PARAMS)
    })
    .await
    .map_err(|e| e.to_string())?;
    let db = store::connect(&cfg.store, true)
        .await
        .map_err(|e| format!("[store.SqlDB]: {e}"))?;
    let created =
        upsert_directory_user(&db, &args.handle, &args.email, args.onsite, &hashed).await?;
    if created {
        println!("Created user {}", args.handle);
    } else {
        println!("Updated existing user {}", args.handle);
    }
    println!(
        "To make this user an IMS administrator, add {:?} to IMS_ADMINS",
        args.handle
    );
    Ok(())
}

/// Creates the user, or updates an existing one's password (and email and
/// onsite flag, when given), marking them active. Returns whether a user was
/// created.
pub async fn upsert_directory_user(
    db: &MySqlPool,
    handle: &str,
    email: &str,
    onsite: Option<bool>,
    hashed_password: &str,
) -> Result<bool, String> {
    let existing = queries::directory_person_by_handle(db, handle)
        .await
        .map_err(|e| format!("[DirectoryPersonByHandle]: {e}"))?;
    let Some(mut person) = existing else {
        let email = (!email.is_empty()).then_some(email);
        queries::directory_create_person(
            db,
            handle,
            email,
            hashed_password,
            true,
            onsite.unwrap_or(false),
        )
        .await
        .map_err(|e| format!("[DirectoryCreatePerson]: {e}"))?;
        return Ok(true);
    };
    if !email.is_empty() {
        person.email = Some(email.to_string());
    }
    if let Some(o) = onsite {
        person.onsite = o;
    }
    person.active = true;
    queries::directory_update_person(db, &person)
        .await
        .map_err(|e| format!("[DirectoryUpdatePerson]: {e}"))?;
    queries::directory_set_person_password(db, person.id, hashed_password)
        .await
        .map_err(|e| format!("[DirectorySetPersonPassword]: {e}"))?;
    Ok(false)
}

pub fn read_password(from_stdin: bool, stdin: &mut dyn BufRead) -> Result<String, String> {
    if from_stdin {
        let mut line = String::new();
        stdin
            .read_line(&mut line)
            .map_err(|e| format!("failed to read password from stdin: {e}"))?;
        let password = line.trim_end_matches(['\r', '\n']).to_string();
        if password.is_empty() {
            return Err("empty password provided on stdin".to_string());
        }
        return Ok(password);
    }
    let first = rpassword::prompt_password("Password: ").map_err(|e| {
        format!("failed to read password (use --password-stdin if not on a terminal): {e}")
    })?;
    let second = rpassword::prompt_password("Confirm password: ")
        .map_err(|e| format!("failed to read password confirmation: {e}"))?;
    if first != second {
        return Err("passwords did not match".to_string());
    }
    if first.is_empty() {
        return Err("empty password provided".to_string());
    }
    Ok(first)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::conf::Setting;

    #[test]
    fn requires_ims_directory() {
        let cfg = IMSConfig::default();
        assert!(check_config(&cfg).unwrap_err().contains("IMS_DIRECTORY"));
        let mut cfg = IMSConfig::default();
        cfg.directory.directory = Setting::Known(DirectoryType::Ims);
        assert!(check_config(&cfg).is_ok());
    }

    #[test]
    fn password_from_stdin() {
        let mut input: &[u8] = b"hunter2\r\n";
        assert_eq!(read_password(true, &mut input).unwrap(), "hunter2");
        let mut empty: &[u8] = b"\n";
        assert!(read_password(true, &mut empty).is_err());
    }
}
