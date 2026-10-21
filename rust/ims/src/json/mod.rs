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

//! The API's JSON wire types, ported from the Go server's `json` package.
//! Field presence follows Go's encoding/json: `omitzero`/`omitempty` fields
//! are skipped when zero or empty, pointers and slices that Go leaves nil go
//! out as null, and maps are key-sorted (hence BTreeMap).

use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};

use crate::util::conv::GoTime;

pub fn is_zero_i32(v: &i32) -> bool {
    *v == 0
}

pub fn is_zero_i64(v: &i64) -> bool {
    *v == 0
}

pub fn is_zero_i16(v: &i16) -> bool {
    *v == 0
}

pub fn is_false(v: &bool) -> bool {
    !*v
}

pub fn is_zero_time(v: &GoTime) -> bool {
    v.is_zero()
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct ActionLog {
    pub id: i64,
    pub created_at: GoTime,
    pub action_type: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub method: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub path: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub referrer: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub request_body: String,
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub user_id: i64,
    pub user_name: String,
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub position_id: i64,
    pub position_name: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub client_address: String,
    #[serde(default, skip_serializing_if = "is_zero_i16")]
    pub http_status: i16,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub duration: String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct ErrorLog {
    pub id: i64,
    pub created_at: GoTime,
    pub http_status: i16,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub response_message: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub internal_error: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub stack_trace: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub method: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub path: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub referrer: String,
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub user_id: i64,
    pub user_name: String,
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub position_id: i64,
    pub position_name: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub client_address: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub duration: String,
}

/// The admin view of the IMS-native user directory.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Directory {
    pub persons: Vec<DirectoryPerson>,
    pub teams: Vec<DirectoryGroup>,
    pub positions: Vec<DirectoryGroup>,
}

/// A person in the IMS-native directory. Optional fields mean "leave
/// unchanged" when updating.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct DirectoryPerson {
    #[serde(default)]
    pub id: i64,
    #[serde(default)]
    pub handle: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub email: Option<String>,
    #[serde(default)]
    pub active: Option<bool>,
    #[serde(default)]
    pub onsite: Option<bool>,
    #[serde(default)]
    pub team_ids: Option<Vec<i64>>,
    #[serde(default)]
    pub position_ids: Option<Vec<i64>>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct DirectoryGroup {
    #[serde(default)]
    pub id: i64,
    #[serde(default)]
    pub title: Option<String>,
    #[serde(default)]
    pub active: Option<bool>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct DirectoryPersonPassword {
    #[serde(default)]
    pub password: String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Event {
    #[serde(default)]
    pub id: i32,
    #[serde(default)]
    pub name: Option<String>,
    #[serde(default)]
    pub is_group: Option<bool>,
    #[serde(default)]
    pub parent_group: Option<i32>,
    #[serde(default)]
    pub map_url: Option<String>,
    /// In an edit request, None means "leave this as it is", and the zero
    /// time means "clear this release time".
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub camp_locations_release: Option<GoTime>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub art_locations_release: Option<GoTime>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub map_url_release: Option<GoTime>,
    #[serde(default)]
    pub normalize_addresses: Option<bool>,
}

pub type EventsAccess = BTreeMap<String, EventAccess>;

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct AccessRuleDebugInfo {
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub matches_users: Vec<String>,
    #[serde(default, skip_serializing_if = "is_false")]
    pub matches_all_users: bool,
    #[serde(default, skip_serializing_if = "is_false")]
    pub matches_no_one: bool,
    #[serde(default)]
    pub known_target: bool,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct AccessRule {
    #[serde(default)]
    pub expression: String,
    #[serde(default)]
    pub validity: String,
    #[serde(default, skip_serializing_if = "is_zero_time")]
    pub not_after: GoTime,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub description: String,
    #[serde(default, skip_serializing_if = "is_false")]
    pub expired: bool,
    #[serde(default, skip_serializing_if = "is_zero_time")]
    pub not_before: GoTime,
    #[serde(default, skip_serializing_if = "is_false")]
    pub pending: bool,
    #[serde(default)]
    pub debug_info: AccessRuleDebugInfo,
}

/// A None list in an edit request means the mode wasn't mentioned at all.
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct EventAccess {
    #[serde(default)]
    pub readers: Option<Vec<AccessRule>>,
    #[serde(default)]
    pub writers: Option<Vec<AccessRule>>,
    #[serde(default)]
    pub reporters: Option<Vec<AccessRule>>,
    #[serde(default)]
    pub visit_writers: Option<Vec<AccessRule>>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct AccessTargets {
    pub persons: Vec<String>,
    pub positions: Vec<String>,
    pub teams: Vec<String>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct FieldReport {
    #[serde(default)]
    pub event: String,
    #[serde(default)]
    pub number: i32,
    #[serde(default, skip_serializing_if = "is_zero_time")]
    pub created: GoTime,
    #[serde(default, skip_serializing_if = "is_zero_i32")]
    pub version: i32,
    #[serde(default)]
    pub summary: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub incident: Option<i32>,
    #[serde(default)]
    pub report_entries: Option<Vec<ReportEntry>>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Location {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub name: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub address: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
}

pub const INCIDENT_PRIORITY_HIGH: i8 = 5;
pub const INCIDENT_PRIORITY_NORMAL: i8 = 3;
pub const INCIDENT_PRIORITY_LOW: i8 = 1;

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Incident {
    #[serde(default)]
    pub event: String,
    #[serde(default)]
    pub event_id: i32,
    #[serde(default)]
    pub number: i32,
    #[serde(default, skip_serializing_if = "is_zero_time")]
    pub created: GoTime,
    #[serde(default, skip_serializing_if = "is_zero_time")]
    pub last_modified: GoTime,
    #[serde(default, skip_serializing_if = "is_zero_i32")]
    pub version: i32,
    #[serde(default)]
    pub state: String,
    #[serde(default, skip_serializing_if = "is_zero_time")]
    pub started: GoTime,
    #[serde(default, skip_serializing_if = "is_zero_time")]
    pub closed: GoTime,
    #[serde(default)]
    pub priority: i8,
    #[serde(default)]
    pub summary: Option<String>,
    #[serde(default)]
    pub location: Location,
    #[serde(default)]
    pub incident_type_ids: Option<Vec<i32>>,
    #[serde(default)]
    pub field_reports: Option<Vec<i32>>,
    #[serde(default)]
    pub visits: Option<Vec<i32>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub linked_incidents: Option<Vec<LinkedIncident>>,
    #[serde(default)]
    pub rangers: Option<Vec<IncidentRanger>>,
    #[serde(default)]
    pub report_entries: Option<Vec<ReportEntry>>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct IncidentRanger {
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub handle: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub role: Option<String>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct LinkedIncident {
    #[serde(default)]
    pub event_name: String,
    #[serde(default)]
    pub event_id: i32,
    #[serde(default)]
    pub number: i32,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub summary: String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct IncidentType {
    #[serde(default)]
    pub id: i32,
    #[serde(default)]
    pub name: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    #[serde(default)]
    pub hidden: Option<bool>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Person {
    pub handle: String,
    pub status: String,
    pub onsite: bool,
    #[serde(default, skip_serializing_if = "is_zero_i64")]
    pub directory_id: i64,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Place {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub location_string: String,
    /// None is omitted; Some(Null) goes out as an explicit null, which is what
    /// the Go server sends for a place stored without external data.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub external_data: Option<serde_json::Value>,
}

/// Keyed by place type.
pub type Places = BTreeMap<String, Vec<Place>>;

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct ReportEntry {
    #[serde(default)]
    pub id: i32,
    #[serde(default, skip_serializing_if = "is_zero_time")]
    pub created: GoTime,
    #[serde(default)]
    pub author: String,
    #[serde(default)]
    pub system_entry: bool,
    #[serde(default)]
    pub text: String,
    #[serde(default)]
    pub stricken: Option<bool>,
    #[serde(default, skip_serializing_if = "Attachment::is_zero")]
    pub attachment: Attachment,
}

#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct Attachment {
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub previewable: bool,
}

impl Attachment {
    pub fn is_zero(&self) -> bool {
        self.name.is_empty() && !self.previewable
    }
}

pub const SEARCH_RESULT_KIND_INCIDENT: &str = "incident";
pub const SEARCH_RESULT_KIND_FIELD_REPORT: &str = "field_report";
pub const SEARCH_RESULT_KIND_VISIT: &str = "visit";

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct SearchResults {
    pub hits: Vec<SearchResult>,
    pub truncated: bool,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct SearchResult {
    pub kind: String,
    pub event: String,
    pub event_id: i32,
    pub number: i32,
    #[serde(default, skip_serializing_if = "is_zero_time")]
    pub created: GoTime,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub summary: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub snippet: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub incident: Option<i32>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct Visit {
    #[serde(default)]
    pub event: String,
    #[serde(default)]
    pub event_id: i32,
    #[serde(default)]
    pub number: i32,
    #[serde(default, skip_serializing_if = "is_zero_time")]
    pub created: GoTime,
    #[serde(default, skip_serializing_if = "is_zero_time")]
    pub last_modified: GoTime,
    #[serde(default, skip_serializing_if = "is_zero_i32")]
    pub version: i32,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub incident: Option<i32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub guest_preferred_name: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub guest_legal_name: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub guest_description: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub guest_action_plan: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub guest_camp_name: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub guest_camp_address: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub guest_camp_description: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub guest_camp_contacts: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub arrival_time: Option<GoTime>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub arrival_method: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub arrival_state: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub arrival_reason: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub arrival_belongings: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub departure_time: Option<GoTime>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub departure_method: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub departure_state: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub resource_sitter: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub resource_bed_id: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub resource_rest: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub resource_clothes: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub resource_pogs: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub resource_food_bev: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub resource_other: Option<String>,
    #[serde(default)]
    pub rangers: Option<Vec<VisitRanger>>,
    #[serde(default)]
    pub report_entries: Option<Vec<ReportEntry>>,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct VisitRanger {
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub handle: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub role: Option<String>,
}
