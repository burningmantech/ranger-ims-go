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

//! Row types for the IMS database, mirroring sqlc's generated `store/imsdb`
//! models. Enum columns stay as strings here; the API layer validates them.

use sqlx::FromRow;

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct Event {
    pub id: i32,
    pub name: String,
    pub is_group: bool,
    pub parent_group: Option<i32>,
    pub map_url: Option<String>,
    pub camp_locations_release: Option<f64>,
    pub art_locations_release: Option<f64>,
    pub map_url_release: Option<f64>,
    pub normalize_addresses: bool,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct EventAccess {
    pub id: i32,
    pub event: i32,
    pub expression: String,
    pub mode: String,
    pub validity: String,
    pub not_after: Option<f64>,
    pub not_before: Option<f64>,
    pub description: String,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct Incident {
    pub event: i32,
    pub number: i32,
    pub created: f64,
    pub priority: i8,
    pub state: String,
    pub started: f64,
    pub closed: Option<f64>,
    pub summary: Option<String>,
    pub location_name: Option<String>,
    pub location_address: Option<String>,
    pub location_description: Option<String>,
    pub version: i32,
}

/// An Incident plus its JSON-aggregated associations, as the `Incident` and
/// `Incidents` queries return it.
#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct IncidentWithAssociations {
    #[sqlx(flatten)]
    pub incident: Incident,
    pub incident_type_ids: String,
    pub field_report_numbers: String,
    pub visit_numbers: String,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct IncidentRanger {
    pub id: i32,
    pub event: i32,
    pub incident_number: i32,
    pub ranger_handle: String,
    pub role: Option<String>,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct LinkedIncident {
    pub linked_event: i32,
    pub linked_event_name: String,
    pub linked_incident: i32,
    pub linked_incident_summary: Option<String>,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct IncidentType {
    pub id: i32,
    pub name: String,
    pub hidden: bool,
    pub description: Option<String>,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct ReportEntry {
    pub id: i32,
    pub author: String,
    pub text: String,
    pub created: f64,
    pub generated: bool,
    pub stricken: bool,
    pub attached_file: Option<String>,
    pub attached_file_original_name: Option<String>,
    pub attached_file_media_type: Option<String>,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct IncidentReportEntry {
    pub incident_number: i32,
    #[sqlx(flatten)]
    pub report_entry: ReportEntry,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct FieldReportReportEntry {
    pub field_report_number: i32,
    #[sqlx(flatten)]
    pub report_entry: ReportEntry,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct VisitReportEntry {
    pub visit_number: i32,
    #[sqlx(flatten)]
    pub report_entry: ReportEntry,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct FieldReport {
    pub event: i32,
    pub number: i32,
    pub created: f64,
    pub summary: Option<String>,
    pub incident_number: Option<i32>,
    pub version: i32,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct Visit {
    pub event: i32,
    pub number: i32,
    pub created: f64,
    pub incident_number: Option<i32>,
    pub guest_preferred_name: Option<String>,
    pub guest_legal_name: Option<String>,
    pub guest_description: Option<String>,
    pub guest_action_plan: Option<String>,
    pub guest_camp_name: Option<String>,
    pub guest_camp_address: Option<String>,
    pub guest_camp_description: Option<String>,
    pub guest_camp_contacts: Option<String>,
    pub arrival_time: Option<f64>,
    pub arrival_method: Option<String>,
    pub arrival_state: Option<String>,
    pub arrival_reason: Option<String>,
    pub arrival_belongings: Option<String>,
    pub departure_time: Option<f64>,
    pub departure_method: Option<String>,
    pub departure_state: Option<String>,
    pub resource_sitter: Option<String>,
    pub resource_bed_id: Option<String>,
    pub resource_rest: Option<String>,
    pub resource_clothes: Option<String>,
    pub resource_pogs: Option<String>,
    pub resource_food_bev: Option<String>,
    pub resource_other: Option<String>,
    pub version: i32,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct VisitRanger {
    pub id: i32,
    pub event: i32,
    pub visit_number: i32,
    pub ranger_handle: String,
    pub role: Option<String>,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct Place {
    pub event: i32,
    #[sqlx(rename = "TYPE")]
    pub place_type: String,
    pub number: i32,
    pub name: String,
    pub location_string: String,
    /// JSON text. The Places query hands this back as a BLOB.
    pub external_data: Option<Vec<u8>>,
}

#[derive(Debug, Clone, PartialEq, Default, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct ActionLog {
    pub id: i64,
    pub created_at: f64,
    pub action_type: String,
    pub method: Option<String>,
    pub path: Option<String>,
    pub referrer: Option<String>,
    pub request_body: Option<String>,
    pub user_id: Option<i64>,
    pub user_name: Option<String>,
    pub position_id: Option<i64>,
    pub position_name: Option<String>,
    pub client_address: Option<String>,
    pub http_status: Option<i16>,
    pub duration_micros: Option<i64>,
}

#[derive(Debug, Clone, PartialEq, Default, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct ErrorLog {
    pub id: i64,
    pub created_at: f64,
    pub http_status: i16,
    pub response_message: Option<String>,
    pub internal_error: Option<String>,
    pub stack_trace: Option<String>,
    pub method: Option<String>,
    pub path: Option<String>,
    pub referrer: Option<String>,
    pub user_id: Option<i64>,
    pub user_name: Option<String>,
    pub position_id: Option<i64>,
    pub position_name: Option<String>,
    pub client_address: Option<String>,
    pub duration_micros: Option<i64>,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct DirectoryActivePerson {
    pub id: i64,
    pub handle: String,
    pub email: Option<String>,
    pub password: String,
    pub onsite: bool,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct DirectoryPerson {
    pub id: i64,
    pub handle: String,
    pub email: Option<String>,
    pub active: bool,
    pub onsite: bool,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct DirectoryTitled {
    pub id: i64,
    pub title: String,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct DirectoryTitledWithActive {
    pub id: i64,
    pub title: String,
    pub active: bool,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct DirectoryPersonPosition {
    pub person_id: i64,
    pub position_id: i64,
}

#[derive(Debug, Clone, PartialEq, FromRow)]
#[sqlx(rename_all = "SCREAMING_SNAKE_CASE")]
pub struct DirectoryPersonTeam {
    pub person_id: i64,
    pub team_id: i64,
}
