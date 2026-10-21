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

//! Administration of the IMS-native user directory. These endpoints refuse
//! every request unless IMS_DIRECTORY=ims. Ported from the Go server's
//! `api/directory.go`.

use std::collections::HashMap;

use axum::Extension;
use axum::body::Body;
use axum::extract::{Path, State};
use axum::http::{HeaderMap, HeaderValue};
use axum::response::Response;

use super::helpers::*;
use super::middleware::JwtCtx;
use super::state::{AppState, S};
use crate::authz;
use crate::json;
use crate::store::queries;
use crate::util::argon2id;
use crate::util::conv::{empty_to_none, string_to_sql};
use crate::util::herr::{self, HttpError};

const MAX_HANDLE_LEN: usize = 128;
const MAX_EMAIL_LEN: usize = 256;
const MAX_TITLE_LEN: usize = 128;
const MAX_PASSWORD_LEN: usize = 256;

async fn require_directory_admin(st: &AppState, jwt: &JwtCtx) -> HResult<()> {
    if !st.directory_is_ims() {
        return Err(HttpError::forbidden_msg(
            "This deployment's user directory is not managed by IMS (IMS_DIRECTORY is not 'ims'), so it cannot be administered here",
        ));
    }
    let global = get_global_permissions(st, jwt)
        .await
        .map_err(|e| e.from_src("[getGlobalPermissions]"))?;
    if global & authz::GLOBAL_ADMINISTRATE_DIRECTORY == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have GlobalAdministrateDirectory permission",
        ));
    }
    Ok(())
}

async fn hash_password(password: String) -> HResult<String> {
    tokio::task::spawn_blocking(move || {
        argon2id::create_hash(&password, &argon2id::SECOND_RECOMMENDED_PARAMS)
    })
    .await
    .map_err(|e| HttpError::internal_server_error("Failed to hash password", e))
}

/// Like Go's `crypto/rand.Text`.
fn rand_text() -> String {
    use rand::Rng;
    const ALPHABET: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
    let mut rng = rand::rng();
    (0..26)
        .map(|_| ALPHABET[rng.random_range(0..ALPHABET.len())] as char)
        .collect()
}

pub async fn get_directory(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
) -> HResult<Response> {
    require_directory_admin(&st, &jwt)
        .await
        .map_err(|e| e.from_src("[requireDirectoryAdmin]"))?;
    let fail = |e: sqlx::Error| {
        HttpError::internal_server_error("Failed to fetch directory", e).from_src("[DirectoryAll*]")
    };
    let persons = queries::directory_all_persons(&st.db).await.map_err(fail)?;
    let teams = queries::directory_all_teams(&st.db).await.map_err(fail)?;
    let positions = queries::directory_all_positions(&st.db)
        .await
        .map_err(fail)?;
    let person_teams = queries::directory_person_teams(&st.db)
        .await
        .map_err(fail)?;
    let person_positions = queries::directory_person_positions(&st.db)
        .await
        .map_err(fail)?;

    let mut teams_by_person: HashMap<i64, Vec<i64>> = HashMap::new();
    for pt in person_teams {
        teams_by_person
            .entry(pt.person_id)
            .or_default()
            .push(pt.team_id);
    }
    let mut positions_by_person: HashMap<i64, Vec<i64>> = HashMap::new();
    for pp in person_positions {
        positions_by_person
            .entry(pp.person_id)
            .or_default()
            .push(pp.position_id);
    }
    let mut resp = json::Directory {
        persons: persons
            .into_iter()
            .map(|p| {
                let mut team_ids = teams_by_person.remove(&p.id).unwrap_or_default();
                let mut position_ids = positions_by_person.remove(&p.id).unwrap_or_default();
                team_ids.sort_unstable();
                position_ids.sort_unstable();
                json::DirectoryPerson {
                    id: p.id,
                    handle: Some(p.handle),
                    email: p.email,
                    active: Some(p.active),
                    onsite: Some(p.onsite),
                    team_ids: Some(team_ids),
                    position_ids: Some(position_ids),
                }
            })
            .collect(),
        teams: teams
            .into_iter()
            .map(|t| json::DirectoryGroup {
                id: t.id,
                title: Some(t.title),
                active: Some(t.active),
            })
            .collect(),
        positions: positions
            .into_iter()
            .map(|t| json::DirectoryGroup {
                id: t.id,
                title: Some(t.title),
                active: Some(t.active),
            })
            .collect(),
    };
    resp.persons.sort_by_key(|p| p.id);
    resp.teams.sort_by_key(|t| t.id);
    resp.positions.sort_by_key(|p| p.id);
    Ok(json_response(&resp))
}

pub async fn edit_directory_person(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let new_id = edit_person(&st, &jwt, &headers, body)
        .await
        .map_err(|e| e.from_src("[editDirectoryPerson]"))?;
    st.user_store.flush();
    let mut resp = herr::no_content_response("Success");
    if let Some(id) = new_id {
        resp.headers_mut()
            .insert("IMS-Directory-Person-ID", HeaderValue::from(id));
    }
    Ok(resp)
}

async fn edit_person(
    st: &AppState,
    jwt: &JwtCtx,
    headers: &HeaderMap,
    body: Body,
) -> HResult<Option<i64>> {
    require_directory_admin(st, jwt)
        .await
        .map_err(|e| e.from_src("[requireDirectoryAdmin]"))?;
    let req: json::DirectoryPerson = read_body_as(headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    if let Some(h) = &req.handle {
        if h.is_empty() {
            return Err(HttpError::bad_request_msg("Handle must not be empty"));
        }
        if h.len() > MAX_HANDLE_LEN {
            return Err(HttpError::bad_request_msg("Handle is too long"));
        }
    }
    if req.email.as_ref().is_some_and(|e| e.len() > MAX_EMAIL_LEN) {
        return Err(HttpError::bad_request_msg("Email is too long"));
    }
    let person_id = if req.id == 0 {
        let Some(handle) = &req.handle else {
            return Err(HttpError::bad_request_msg(
                "Handle is required for a new person",
            ));
        };
        // A placeholder that can never verify, until an admin sets a password.
        let placeholder = hash_password(rand_text()).await?;
        let email = req
            .email
            .as_deref()
            .and_then(|e| string_to_sql(empty_to_none(e), MAX_EMAIL_LEN));
        queries::directory_create_person(
            &st.db,
            handle,
            email.as_deref(),
            &placeholder,
            req.active.unwrap_or(true),
            req.onsite.unwrap_or(false),
        )
        .await
        .map_err(|e| {
            HttpError::bad_request(
                "Failed to create person. Handles and emails must be unique.",
                e,
            )
            .from_src("[DirectoryCreatePerson]")
        })? as i64
    } else {
        let mut existing = match queries::directory_person_by_id(&st.db, req.id).await {
            Ok(Some(p)) => p,
            Ok(None) => {
                return Err(HttpError::not_found(
                    "Person not found",
                    anyhow::anyhow!("sql: no rows in result set"),
                ));
            }
            Err(e) => {
                return Err(
                    HttpError::internal_server_error("Failed to fetch person", e)
                        .from_src("[DirectoryPersonByID]"),
                );
            }
        };
        if let Some(h) = &req.handle {
            existing.handle = h.clone();
        }
        if let Some(e) = &req.email {
            existing.email = string_to_sql(empty_to_none(e), MAX_EMAIL_LEN);
        }
        if let Some(a) = req.active {
            existing.active = a;
        }
        if let Some(o) = req.onsite {
            existing.onsite = o;
        }
        queries::directory_update_person(&st.db, &existing)
            .await
            .map_err(|e| {
                HttpError::bad_request(
                    "Failed to update person. Handles and emails must be unique.",
                    e,
                )
                .from_src("[DirectoryUpdatePerson]")
            })?;
        req.id
    };
    set_memberships(
        st,
        person_id,
        req.team_ids.as_deref(),
        req.position_ids.as_deref(),
    )
    .await
    .map_err(|e| e.from_src("[setMemberships]"))?;
    Ok((req.id == 0).then_some(person_id))
}

async fn set_memberships(
    st: &AppState,
    person_id: i64,
    team_ids: Option<&[i64]>,
    position_ids: Option<&[i64]>,
) -> HResult<()> {
    if let Some(ids) = team_ids {
        queries::directory_exec_by_id(&st.db, "DirectoryClearPersonTeams", person_id)
            .await
            .map_err(db_err(
                "Failed to update team memberships",
                "[DirectoryClearPersonTeams]",
            ))?;
        for id in ids {
            queries::directory_add_membership(&st.db, "DirectoryAddPersonTeam", person_id, *id)
                .await
                .map_err(|e| {
                    HttpError::bad_request("Failed to add team membership. Does the team exist?", e)
                        .from_src("[DirectoryAddPersonTeam]")
                })?;
        }
    }
    if let Some(ids) = position_ids {
        queries::directory_exec_by_id(&st.db, "DirectoryClearPersonPositions", person_id)
            .await
            .map_err(db_err(
                "Failed to update position memberships",
                "[DirectoryClearPersonPositions]",
            ))?;
        for id in ids {
            queries::directory_add_membership(&st.db, "DirectoryAddPersonPosition", person_id, *id)
                .await
                .map_err(|e| {
                    HttpError::bad_request(
                        "Failed to add position membership. Does the position exist?",
                        e,
                    )
                    .from_src("[DirectoryAddPersonPosition]")
                })?;
        }
    }
    Ok(())
}

fn path_i64(p: &HashMap<String, String>, key: &str, msg: &str) -> HResult<i64> {
    let raw = p.get(key).map(String::as_str).unwrap_or_default();
    raw.parse().map_err(|e| {
        HttpError::bad_request(msg.to_string(), anyhow::anyhow!("{e}")).from_src("[ParseInt64]")
    })
}

pub async fn set_directory_person_password(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    require_directory_admin(&st, &jwt)
        .await
        .map_err(|e| e.from_src("[requireDirectoryAdmin]"))?;
    let person_id = path_i64(&p, "personId", "Invalid person ID")?;
    let req: json::DirectoryPersonPassword = read_body_as(&headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    if req.password.is_empty() {
        return Err(HttpError::bad_request_msg("Password must not be empty"));
    }
    if req.password.len() > MAX_PASSWORD_LEN {
        return Err(HttpError::bad_request_msg("Password is too long"));
    }
    match queries::directory_person_by_id(&st.db, person_id).await {
        Ok(Some(_)) => {}
        Ok(None) => {
            return Err(HttpError::not_found(
                "Person not found",
                anyhow::anyhow!("sql: no rows in result set"),
            ));
        }
        Err(e) => {
            return Err(
                HttpError::internal_server_error("Failed to fetch person", e)
                    .from_src("[DirectoryPersonByID]"),
            );
        }
    }
    let hashed = hash_password(req.password).await?;
    queries::directory_set_person_password(&st.db, person_id, &hashed)
        .await
        .map_err(db_err(
            "Failed to set password",
            "[DirectorySetPersonPassword]",
        ))?;
    st.user_store.flush();
    Ok(herr::no_content_response("Success"))
}

pub async fn delete_directory_person(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
) -> HResult<Response> {
    require_directory_admin(&st, &jwt)
        .await
        .map_err(|e| e.from_src("[requireDirectoryAdmin]"))?;
    let id = path_i64(&p, "personId", "Invalid person ID")?;
    queries::directory_exec_by_id(&st.db, "DirectoryDeletePerson", id)
        .await
        .map_err(db_err("Failed to delete person", "[DirectoryDeletePerson]"))?;
    st.user_store.flush();
    Ok(herr::no_content_response("Success"))
}

async fn edit_group(
    st: &AppState,
    jwt: &JwtCtx,
    headers: &HeaderMap,
    body: Body,
    is_position: bool,
) -> HResult<Option<i64>> {
    require_directory_admin(st, jwt)
        .await
        .map_err(|e| e.from_src("[requireDirectoryAdmin]"))?;
    let req: json::DirectoryGroup = read_body_as(headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    if let Some(t) = &req.title {
        if t.is_empty() {
            return Err(HttpError::bad_request_msg("Title must not be empty"));
        }
        if t.len() > MAX_TITLE_LEN {
            return Err(HttpError::bad_request_msg("Title is too long"));
        }
    }
    if req.id == 0 {
        let Some(title) = &req.title else {
            return Err(HttpError::bad_request_msg(
                "Title is required for a new team or position",
            ));
        };
        let query = if is_position {
            "DirectoryCreatePosition"
        } else {
            "DirectoryCreateTeam"
        };
        let id = queries::directory_create_group(&st.db, query, title, req.active.unwrap_or(true))
            .await
            .map_err(|e| {
                HttpError::bad_request("Failed to create. Titles must be unique.", e)
                    .from_src("[DirectoryCreate]")
            })?;
        return Ok(Some(id as i64));
    }
    let rows = if is_position {
        queries::directory_all_positions(&st.db)
            .await
            .map_err(db_err(
                "Failed to fetch positions",
                "[DirectoryAllPositions]",
            ))?
    } else {
        queries::directory_all_teams(&st.db)
            .await
            .map_err(db_err("Failed to fetch teams", "[DirectoryAllTeams]"))?
    };
    let Some(existing) = rows.into_iter().find(|r| r.id == req.id) else {
        return Err(HttpError::not_found_msg(if is_position {
            "Position not found"
        } else {
            "Team not found"
        })
        .from_src("[findDirectoryGroup]"));
    };
    let title = req.title.unwrap_or(existing.title);
    let active = req.active.unwrap_or(existing.active);
    let query = if is_position {
        "DirectoryUpdatePosition"
    } else {
        "DirectoryUpdateTeam"
    };
    queries::directory_update_group(&st.db, query, &title, active, req.id)
        .await
        .map_err(|e| {
            HttpError::bad_request("Failed to update. Titles must be unique.", e)
                .from_src("[DirectoryUpdate]")
        })?;
    Ok(None)
}

pub async fn edit_directory_team(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let new_id = edit_group(&st, &jwt, &headers, body, false)
        .await
        .map_err(|e| e.from_src("[editDirectoryGroup]"))?;
    st.user_store.flush();
    let mut resp = herr::no_content_response("Success");
    if let Some(id) = new_id {
        resp.headers_mut()
            .insert("IMS-Directory-Team-ID", HeaderValue::from(id));
    }
    Ok(resp)
}

pub async fn edit_directory_position(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let new_id = edit_group(&st, &jwt, &headers, body, true)
        .await
        .map_err(|e| e.from_src("[editDirectoryGroup]"))?;
    st.user_store.flush();
    let mut resp = herr::no_content_response("Success");
    if let Some(id) = new_id {
        resp.headers_mut()
            .insert("IMS-Directory-Position-ID", HeaderValue::from(id));
    }
    Ok(resp)
}

pub async fn delete_directory_team(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
) -> HResult<Response> {
    require_directory_admin(&st, &jwt)
        .await
        .map_err(|e| e.from_src("[requireDirectoryAdmin]"))?;
    let id = path_i64(&p, "teamId", "Invalid team ID")?;
    queries::directory_exec_by_id(&st.db, "DirectoryDeleteTeam", id)
        .await
        .map_err(db_err("Failed to delete team", "[DirectoryDeleteTeam]"))?;
    st.user_store.flush();
    Ok(herr::no_content_response("Success"))
}

pub async fn delete_directory_position(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
) -> HResult<Response> {
    require_directory_admin(&st, &jwt)
        .await
        .map_err(|e| e.from_src("[requireDirectoryAdmin]"))?;
    let id = path_i64(&p, "positionId", "Invalid position ID")?;
    queries::directory_exec_by_id(&st.db, "DirectoryDeletePosition", id)
        .await
        .map_err(db_err(
            "Failed to delete position",
            "[DirectoryDeletePosition]",
        ))?;
    st.user_store.flush();
    Ok(herr::no_content_response("Success"))
}
