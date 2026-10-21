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

//! Event-scoped roles and permission bitmasks.

use std::collections::HashMap;

use sqlx::MySqlPool;

use super::IMSClaims;
use crate::directory::UserStore;
use crate::store::models::EventAccess;
use crate::store::queries;
use crate::util::conv::GoTime;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum Role {
    AnyAuthenticatedUser,
    EventReporter,
    EventReader,
    EventWriter,
    EventVisitWriter,
    Administrator,
}

pub type EventPermissionMask = u16;
pub type GlobalPermissionMask = u16;

pub const EVENT_NO_PERMISSIONS: EventPermissionMask = 0;
pub const GLOBAL_NO_PERMISSIONS: GlobalPermissionMask = 0;

pub const EVENT_READ_INCIDENTS: EventPermissionMask = 1 << 0;
pub const EVENT_WRITE_INCIDENTS: EventPermissionMask = 1 << 1;
pub const EVENT_READ_ALL_FIELD_REPORTS: EventPermissionMask = 1 << 2;
pub const EVENT_READ_OWN_FIELD_REPORTS: EventPermissionMask = 1 << 3;
pub const EVENT_WRITE_ALL_FIELD_REPORTS: EventPermissionMask = 1 << 4;
pub const EVENT_WRITE_OWN_FIELD_REPORTS: EventPermissionMask = 1 << 5;
pub const EVENT_READ_EVENT_NAME: EventPermissionMask = 1 << 6;
pub const EVENT_READ_PLACES: EventPermissionMask = 1 << 7;
pub const EVENT_READ_VISITS: EventPermissionMask = 1 << 8;
pub const EVENT_WRITE_VISITS: EventPermissionMask = 1 << 9;

pub const GLOBAL_LIST_EVENTS: GlobalPermissionMask = 1 << 0;
pub const GLOBAL_READ_INCIDENT_TYPES: GlobalPermissionMask = 1 << 1;
pub const GLOBAL_READ_PERSONNEL: GlobalPermissionMask = 1 << 2;
pub const GLOBAL_ADMINISTRATE_EVENTS: GlobalPermissionMask = 1 << 3;
pub const GLOBAL_ADMINISTRATE_INCIDENT_TYPES: GlobalPermissionMask = 1 << 4;
pub const GLOBAL_ADMINISTRATE_PLACES: GlobalPermissionMask = 1 << 5;
pub const GLOBAL_ADMINISTRATE_DEBUGGING: GlobalPermissionMask = 1 << 6;
pub const GLOBAL_ADMINISTRATE_DIRECTORY: GlobalPermissionMask = 1 << 7;

pub fn role_global_perms(role: Role) -> GlobalPermissionMask {
    match role {
        Role::AnyAuthenticatedUser => {
            GLOBAL_LIST_EVENTS | GLOBAL_READ_INCIDENT_TYPES | GLOBAL_READ_PERSONNEL
        }
        Role::Administrator => {
            GLOBAL_ADMINISTRATE_EVENTS
                | GLOBAL_ADMINISTRATE_INCIDENT_TYPES
                | GLOBAL_ADMINISTRATE_PLACES
                | GLOBAL_ADMINISTRATE_DEBUGGING
                | GLOBAL_ADMINISTRATE_DIRECTORY
        }
        _ => GLOBAL_NO_PERMISSIONS,
    }
}

pub fn role_event_perms(role: Role) -> EventPermissionMask {
    match role {
        Role::EventReporter => {
            EVENT_READ_EVENT_NAME
                | EVENT_READ_OWN_FIELD_REPORTS
                | EVENT_WRITE_OWN_FIELD_REPORTS
                | EVENT_READ_PLACES
        }
        Role::EventReader => {
            EVENT_READ_EVENT_NAME
                | EVENT_READ_INCIDENTS
                | EVENT_READ_OWN_FIELD_REPORTS
                | EVENT_READ_ALL_FIELD_REPORTS
                | EVENT_READ_VISITS
                | EVENT_READ_PLACES
        }
        Role::EventWriter => {
            EVENT_READ_EVENT_NAME
                | EVENT_READ_INCIDENTS
                | EVENT_WRITE_INCIDENTS
                | EVENT_READ_ALL_FIELD_REPORTS
                | EVENT_READ_OWN_FIELD_REPORTS
                | EVENT_WRITE_ALL_FIELD_REPORTS
                | EVENT_WRITE_OWN_FIELD_REPORTS
                | EVENT_READ_VISITS
                | EVENT_WRITE_VISITS
                | EVENT_READ_PLACES
        }
        Role::EventVisitWriter => {
            EVENT_READ_EVENT_NAME | EVENT_READ_VISITS | EVENT_WRITE_VISITS | EVENT_READ_PLACES
        }
        _ => EVENT_NO_PERMISSIONS,
    }
}

pub fn mode_to_role(mode: &str) -> Option<Role> {
    match mode {
        "read" => Some(Role::EventReader),
        "write" => Some(Role::EventWriter),
        "report" => Some(Role::EventReporter),
        "write_visits" => Some(Role::EventVisitWriter),
        _ => None,
    }
}

/// What a Ranger looks like to an access rule.
#[derive(Debug, Clone, Default)]
pub struct Requestor {
    pub handle: String,
    pub onsite: bool,
    pub positions: Vec<String>,
    pub teams: Vec<String>,
    pub on_duty_position: String,
}

impl Requestor {
    /// Looks up the Ranger's positions, teams, and on-site/on-duty status in
    /// the directory, not the token, so directory changes take effect on the
    /// next cache refresh.
    pub async fn load(user_store: &UserStore, claims: &IMSClaims) -> anyhow::Result<Requestor> {
        let id = claims.directory_id();
        let positions = user_store
            .positions_for_ranger(id)
            .await
            .map_err(|e| e.context("[PositionsForRanger]"))?;
        let teams = user_store
            .teams_for_ranger(id)
            .await
            .map_err(|e| e.context("[TeamsForRanger]"))?;
        let (_, on_duty_position) = user_store
            .on_duty_for_ranger(id)
            .await
            .map_err(|e| e.context("[OnDutyForRanger]"))?;
        let onsite = user_store
            .onsite_for_ranger(id)
            .await
            .map_err(|e| e.context("[OnSiteForRanger]"))?;
        Ok(Requestor {
            handle: claims.ranger_handle().to_string(),
            onsite,
            positions,
            teams,
            on_duty_position,
        })
    }
}

/// The requestor's permissions on one event (if given) and globally.
pub async fn event_permissions(
    event_id: Option<i32>,
    db: &MySqlPool,
    user_store: &UserStore,
    admins: &[String],
    claims: &IMSClaims,
) -> anyhow::Result<(HashMap<i32, EventPermissionMask>, GlobalPermissionMask)> {
    let mut access_by_event: HashMap<i32, Vec<EventAccess>> = HashMap::new();
    if let Some(event_id) = event_id {
        // For an event group, this query returns no rows. That keeps anyone
        // from adding records under a group as though it were an event.
        let rows = queries::event_and_parent_access(db, event_id)
            .await
            .map_err(|e| anyhow::Error::new(e).context("[EventAccess]"))?;
        access_by_event.insert(event_id, rows);
    }
    let requestor = Requestor::load(user_store, claims).await?;
    Ok(many_event_permissions(&access_by_event, admins, &requestor))
}

pub fn many_event_permissions(
    access_by_event: &HashMap<i32, Vec<EventAccess>>,
    admins: &[String],
    requestor: &Requestor,
) -> (HashMap<i32, EventPermissionMask>, GlobalPermissionMask) {
    let mut event_perms = HashMap::new();
    let mut global_perms = GLOBAL_NO_PERMISSIONS;
    if !requestor.handle.is_empty() {
        global_perms |= role_global_perms(Role::AnyAuthenticatedUser);
    }
    if admins.contains(&requestor.handle) {
        global_perms |= role_global_perms(Role::Administrator);
    }
    for (event_id, accesses) in access_by_event {
        let mut perms = EVENT_NO_PERMISSIONS;
        for ea in accesses {
            if person_matches(ea, requestor) {
                perms |= mode_to_role(&ea.mode).map_or(EVENT_NO_PERMISSIONS, role_event_perms);
            }
        }
        event_perms.insert(*event_id, perms);
    }
    (event_perms, global_perms)
}

pub fn person_matches(ea: &EventAccess, r: &Requestor) -> bool {
    let now = GoTime::now();
    if ea
        .not_after
        .is_some_and(|t| GoTime::from_float(t).before(&now))
    {
        return false;
    }
    if ea
        .not_before
        .is_some_and(|t| GoTime::from_float(t).after(&now))
    {
        return false;
    }
    let expr = ea.expression.as_str();
    let match_expr = expr == "*"
        || expr.strip_prefix("person:").is_some_and(|h| h == r.handle)
        || expr
            .strip_prefix("position:")
            .is_some_and(|p| r.positions.iter().any(|x| x == p))
        || expr
            .strip_prefix("onduty:")
            .is_some_and(|p| r.on_duty_position == p)
        || expr
            .strip_prefix("team:")
            .is_some_and(|t| r.teams.iter().any(|x| x == t));
    let match_validity = ea.validity == "always" || (ea.validity == "onsite" && r.onsite);
    match_expr && match_validity
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::*;

    const READER: EventPermissionMask = EVENT_READ_EVENT_NAME
        | EVENT_READ_INCIDENTS
        | EVENT_READ_OWN_FIELD_REPORTS
        | EVENT_READ_ALL_FIELD_REPORTS
        | EVENT_READ_VISITS
        | EVENT_READ_PLACES;
    const REPORTER: EventPermissionMask = EVENT_READ_EVENT_NAME
        | EVENT_READ_OWN_FIELD_REPORTS
        | EVENT_WRITE_OWN_FIELD_REPORTS
        | EVENT_READ_PLACES;
    const VISIT_WRITER: EventPermissionMask =
        EVENT_READ_EVENT_NAME | EVENT_READ_VISITS | EVENT_WRITE_VISITS | EVENT_READ_PLACES;
    const AUTHENTICATED: GlobalPermissionMask =
        GLOBAL_LIST_EVENTS | GLOBAL_READ_INCIDENT_TYPES | GLOBAL_READ_PERSONNEL;

    fn admins() -> Vec<String> {
        vec!["AdminCat".to_string(), "AdminDog".to_string()]
    }

    fn access(event: i32, expr: &str, mode: &str, validity: &str) -> EventAccess {
        EventAccess {
            id: 0,
            event,
            expression: expr.to_string(),
            mode: mode.to_string(),
            validity: validity.to_string(),
            not_after: None,
            not_before: None,
            description: String::new(),
        }
    }

    fn ranger(handle: &str) -> Requestor {
        Requestor {
            handle: handle.to_string(),
            ..Default::default()
        }
    }

    fn perms_for(rules: Vec<EventAccess>, r: &Requestor) -> EventPermissionMask {
        let map = HashMap::from([(123, rules)]);
        many_event_permissions(&map, &admins(), r).0[&123]
    }

    fn hour_from_now(sign: i64) -> Option<f64> {
        let secs = Duration::from_secs(3600).as_secs() as f64;
        Some(GoTime::now().to_float() + sign as f64 * secs)
    }

    #[test]
    fn not_after_and_not_before_guards() {
        let r = ranger("AnyRanger");
        let rule = |not_after: Option<f64>, not_before: Option<f64>| {
            let mut ea = access(123, "*", "read", "always");
            ea.not_after = not_after;
            ea.not_before = not_before;
            perms_for(vec![ea], &r)
        };
        assert_eq!(rule(None, None), READER);
        assert_eq!(rule(hour_from_now(-1), None), EVENT_NO_PERMISSIONS);
        assert_eq!(rule(hour_from_now(1), None), READER);
        assert_eq!(rule(None, hour_from_now(1)), EVENT_NO_PERMISSIONS);
        assert_eq!(rule(None, hour_from_now(-1)), READER);
        assert_eq!(rule(hour_from_now(1), hour_from_now(-1)), READER);
        assert_eq!(
            rule(hour_from_now(1), hour_from_now(1)),
            EVENT_NO_PERMISSIONS
        );
        assert_eq!(
            rule(hour_from_now(-1), hour_from_now(-1)),
            EVENT_NO_PERMISSIONS
        );
    }

    #[test]
    fn person_rules() {
        let rules = vec![
            access(123, "person:EventReaderGuy", "read", "always"),
            access(123, "person:EventReporterPerson", "report", "always"),
            access(
                123,
                "person:EventVisitWriterPerson",
                "write_visits",
                "always",
            ),
        ];
        assert_eq!(perms_for(rules.clone(), &ranger("EventReaderGuy")), READER);
        assert_eq!(
            perms_for(rules.clone(), &ranger("EventReporterPerson")),
            REPORTER
        );
        assert_eq!(
            perms_for(rules.clone(), &ranger("EventVisitWriterPerson")),
            VISIT_WRITER
        );
        assert_eq!(perms_for(rules, &ranger("Nobody")), EVENT_NO_PERMISSIONS);
    }

    #[test]
    fn position_team_and_onduty_rules() {
        let mut r = ranger("Someone");
        r.positions = vec!["Dirt".to_string()];
        r.teams = vec!["Council".to_string()];
        r.on_duty_position = "Operator".to_string();
        assert_eq!(
            perms_for(vec![access(123, "position:Dirt", "read", "always")], &r),
            READER
        );
        assert_eq!(
            perms_for(vec![access(123, "position:Other", "read", "always")], &r),
            EVENT_NO_PERMISSIONS
        );
        assert_eq!(
            perms_for(vec![access(123, "team:Council", "read", "always")], &r),
            READER
        );
        assert_eq!(
            perms_for(vec![access(123, "onduty:Operator", "read", "always")], &r),
            READER
        );
        assert_eq!(
            perms_for(vec![access(123, "onduty:Dirt", "read", "always")], &r),
            EVENT_NO_PERMISSIONS
        );
    }

    #[test]
    fn onsite_validity() {
        let mut r = ranger("Someone");
        assert_eq!(
            perms_for(vec![access(123, "*", "read", "onsite")], &r),
            EVENT_NO_PERMISSIONS
        );
        r.onsite = true;
        assert_eq!(
            perms_for(vec![access(123, "*", "read", "onsite")], &r),
            READER
        );
    }

    #[test]
    fn one_expression_many_modes() {
        let rules = vec![
            access(123, "person:Both", "read", "always"),
            access(123, "person:Both", "report", "always"),
        ];
        assert_eq!(perms_for(rules, &ranger("Both")), READER | REPORTER);
    }

    #[test]
    fn global_perms() {
        let empty = HashMap::new();
        assert_eq!(
            many_event_permissions(&empty, &admins(), &ranger("Someone")).1,
            AUTHENTICATED
        );
        assert_eq!(
            many_event_permissions(&empty, &admins(), &ranger("AdminCat")).1,
            AUTHENTICATED | role_global_perms(Role::Administrator)
        );
        assert_eq!(
            many_event_permissions(&empty, &admins(), &ranger("")).1,
            GLOBAL_NO_PERMISSIONS
        );
    }
}
