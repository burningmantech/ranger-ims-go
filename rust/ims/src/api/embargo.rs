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

//! Burning Man doesn't publish camp and art placement until shortly before
//! the event, so an Event can carry release times that embargo that data (and
//! the event's map link) until then. Only IMS admins see embargoed data.

use crate::json::Place;
use crate::store::models::Event;
use crate::util::conv::GoTime;

/// Whether the given release time is still in the future, i.e. whether the
/// data it guards must be withheld. No release time means no embargo.
pub fn embargoed(release: Option<f64>, now: &GoTime) -> bool {
    release.is_some_and(|r| now.before(&GoTime::from_float(r)))
}

/// Removes the location details from an embargoed Place: the location
/// string, plus the external data's own copies of it.
pub fn redact_place_location(place: &mut Place) {
    place.location_string = String::new();
    if let Some(serde_json::Value::Object(ed)) = place.external_data.as_mut() {
        ed.remove("location");
        ed.remove("location_string");
    }
}

/// Whether the event withholds locations for this type of Place from
/// non-admins right now. Only camps and art are ever embargoed.
pub fn place_locations_embargoed(event: &Event, place_type: &str, now: &GoTime) -> bool {
    match place_type {
        "camp" => embargoed(event.camp_locations_release, now),
        "art" => embargoed(event.art_locations_release, now),
        _ => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn event_with(camp: Option<f64>, art: Option<f64>) -> Event {
        Event {
            id: 1,
            name: "2026".to_string(),
            is_group: false,
            parent_group: None,
            map_url: None,
            camp_locations_release: camp,
            art_locations_release: art,
            map_url_release: None,
            normalize_addresses: false,
        }
    }

    #[test]
    fn embargo_depends_on_release_time() {
        let now = GoTime::now();
        assert!(!embargoed(None, &now));
        assert!(embargoed(Some(now.to_float() + 60.0), &now));
        assert!(!embargoed(Some(now.to_float() - 60.0), &now));
    }

    #[test]
    fn only_camps_and_art_are_embargoed() {
        let now = GoTime::now();
        let later = Some(now.to_float() + 60.0);
        let e = event_with(later, None);
        assert!(place_locations_embargoed(&e, "camp", &now));
        assert!(!place_locations_embargoed(&e, "art", &now));
        assert!(!place_locations_embargoed(&e, "mv", &now));
        assert!(!place_locations_embargoed(&e, "other", &now));
        let e = event_with(None, later);
        assert!(place_locations_embargoed(&e, "art", &now));
    }

    #[test]
    fn redaction_removes_locations() {
        let mut p = Place {
            name: "Camp".to_string(),
            location_string: "3:00 & A".to_string(),
            external_data: Some(
                serde_json::json!({"location": {"frontage": "A"}, "location_string": "3:00 & A", "name": "Camp"}),
            ),
        };
        redact_place_location(&mut p);
        assert_eq!(p.location_string, "");
        assert_eq!(p.external_data, Some(serde_json::json!({"name": "Camp"})));
    }
}
