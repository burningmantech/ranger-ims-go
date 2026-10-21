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

//! Places (camps, art, mutant vehicles, and others). Ported from the Go
//! server's `api/place.go`.

use axum::Extension;
use axum::body::Body;
use axum::extract::{Path, Query, State};
use axum::http::{HeaderMap, StatusCode};
use axum::response::Response;
use serde::{Deserialize, Serialize};

use super::embargo::{place_locations_embargoed, redact_place_location};
use super::helpers::*;
use super::middleware::JwtCtx;
use super::state::{AppState, S};
use crate::authz;
use crate::json;
use crate::store::queries;
use crate::util::bmapi;
use crate::util::conv::GoTime;
use crate::util::gojson;
use crate::util::herr::{self, HttpError};

#[derive(Deserialize)]
pub struct GetPlacesQuery {
    #[serde(default)]
    exclude_external_data: String,
}

pub async fn get_places(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(event_name): Path<String>,
    Query(q): Query<GetPlacesQuery>,
) -> HResult<Response> {
    let (event, event_perms) = get_event_permissions(&st, &jwt, &event_name)
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    let global = get_global_permissions(&st, &jwt)
        .await
        .map_err(|e| e.from_src("[getGlobalPermissions]"))?;
    if event_perms & authz::EVENT_READ_PLACES == 0
        && global & authz::GLOBAL_ADMINISTRATE_PLACES == 0
    {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventReadPlaces permission",
        ));
    }
    let exclude_external_data = q.exclude_external_data.eq_ignore_ascii_case("true");
    let places = queries::places(&st.db, exclude_external_data, event.id)
        .await
        .map_err(db_err("Failed to fetch Places", "[Places]"))?;
    let is_admin = global & authz::GLOBAL_ADMINISTRATE_PLACES != 0;
    let now = GoTime::now();
    let mut resp = json::Places::new();
    for row in places {
        let mut place = json::Place {
            name: row.name,
            location_string: row.location_string,
            external_data: None,
        };
        if !exclude_external_data {
            let raw = row.external_data.unwrap_or_else(|| b"null".to_vec());
            let ed: serde_json::Value = serde_json::from_slice(&raw).map_err(|e| {
                HttpError::internal_server_error("Failed to unmarshal place", e)
                    .from_src("[Unmarshal]")
            })?;
            if !(ed.is_object() || ed.is_null()) {
                return Err(HttpError::internal_server_error(
                    "Failed to unmarshal place",
                    anyhow::anyhow!(
                        "json: cannot unmarshal into Go value of type map[string]interface {{}}"
                    ),
                ));
            }
            place.external_data = Some(ed);
        }
        if !is_admin && place_locations_embargoed(&event, &row.place_type, &now) {
            redact_place_location(&mut place);
        }
        resp.entry(row.place_type).or_default().push(place);
    }
    Ok(with_cache_control(
        json_response(&resp),
        st.cfg.core.cache_control_short,
    ))
}

async fn require_administrate_places(st: &AppState, jwt: &JwtCtx) -> HResult<()> {
    let global = get_global_permissions(st, jwt)
        .await
        .map_err(|e| e.from_src("[getGlobalPermissions]"))?;
    if global & authz::GLOBAL_ADMINISTRATE_PLACES == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have GlobalAdministratePlaces permission",
        ));
    }
    Ok(())
}

pub async fn update_places(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(event_name): Path<String>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    require_administrate_places(&st, &jwt).await?;
    let event = get_event(&st, &event_name)
        .await
        .map_err(|e| e.from_src("[getEvent]"))?;
    let by_type: json::Places = read_body_as(&headers, body)
        .await
        .map_err(|e| e.from_src("[readBodyAs]"))?;
    // Each type supplied has everything it had replaced by what was sent.
    for (place_type, places) in by_type {
        let places: Vec<(json::Place, Option<String>)> =
            places.into_iter().map(|p| (p, None)).collect();
        replace_places(&st, event.id, &place_type, places)
            .await
            .map_err(|e| e.from_src("[replacePlaces]"))?;
    }
    Ok(herr::no_content_response("Success"))
}

/// Swaps out everything the event has of one place type. Each place may
/// come with its external data already serialized, as imported places do.
async fn replace_places(
    st: &AppState,
    event_id: i32,
    place_type: &str,
    places: Vec<(json::Place, Option<String>)>,
) -> HResult<()> {
    queries::remove_places(&st.db, event_id, place_type)
        .await
        .map_err(db_err("Failed to remove places", "[RemovePlaces]"))?;
    for (i, (place, raw)) in places.into_iter().enumerate() {
        let external = match raw {
            Some(raw) => raw,
            None => gojson::to_string(&place.external_data.unwrap_or(serde_json::Value::Null))
                .map_err(|e| {
                    HttpError::internal_server_error("Failed to marshal place", e)
                        .from_src("[Marshal]")
                })?,
        };
        queries::create_place(
            &st.db,
            event_id,
            i as i32,
            place_type,
            &place.name,
            &place.location_string,
            Some(&external),
        )
        .await
        .map_err(db_err("Failed to create place", "[UpdatePlace]"))?;
    }
    Ok(())
}

#[derive(Deserialize)]
pub struct ImportPlacesQuery {
    #[serde(default)]
    place_type: String,
    #[serde(default)]
    year: String,
}

#[derive(Serialize)]
pub struct ImportPlacesResponse {
    /// How many places were stored.
    count: usize,
}

/// Replaces one place type's places for an event with what the Burning Man
/// API has for a given year. The year is a parameter, since an IMS event
/// isn't necessarily named for the year whose data it wants.
pub async fn import_places(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(event_name): Path<String>,
    Query(q): Query<ImportPlacesQuery>,
) -> HResult<Response> {
    require_administrate_places(&st, &jwt).await?;
    let bm = &st.cfg.burning_man_api;
    if !bm.enabled() {
        return Err(HttpError::new(
            StatusCode::SERVICE_UNAVAILABLE,
            "This server has no Burning Man API key configured",
            None,
        ));
    }
    let event = get_event(&st, &event_name)
        .await
        .map_err(|e| e.from_src("[getEvent]"))?;
    let kind = bmapi::Kind::parse(&q.place_type)
        .map_err(|e| HttpError::bad_request(e.clone(), anyhow::anyhow!(e)))?;
    let year: i32 = q
        .year
        .parse()
        .map_err(|e| HttpError::bad_request("The year must be a number", anyhow::anyhow!("{e}")))?;
    let records = bmapi::Client::new(&bm.url, &bm.api_key, st.bm_http.clone())
        .fetch(kind, year)
        .await
        .map_err(|e| {
            HttpError::new(
                StatusCode::BAD_GATEWAY,
                format!("Failed to fetch {kind} data for {year} from the Burning Man API: {e}"),
                Some(e),
            )
            .from_src("[Fetch]")
        })?;
    // A year with no data comes back as an empty array, which would otherwise
    // silently wipe out whatever the event already had.
    if records.is_empty() {
        return Err(HttpError::new(
            StatusCode::BAD_GATEWAY,
            format!("The Burning Man API has no {kind} data for {year}. Nothing was changed."),
            None,
        ));
    }
    let count = records.len();
    let places = records
        .into_iter()
        .map(|r| {
            let place = json::Place {
                name: r.name,
                location_string: r.location_string,
                external_data: None,
            };
            (place, Some(r.raw))
        })
        .collect();
    replace_places(&st, event.id, kind.as_str(), places)
        .await
        .map_err(|e| e.from_src("[replacePlaces]"))?;
    Ok(json_response(&ImportPlacesResponse { count }))
}
