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

//! The user directory, ported from the Go server's `directory` package.
//! `UserStore` caches what a `Source` fetches.

pub mod clubhouse;
pub mod ims;

use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;

use crate::util::cache::InMemory;

pub use clubhouse::ClubhouseSource;
pub use ims::IMSSource;

#[derive(Debug, Clone, Default, PartialEq)]
pub struct User {
    pub id: i64,
    pub handle: String,
    pub email: String,
    pub status: String,
    pub onsite: bool,
    pub password: String,
    pub position_ids: Vec<i64>,
    pub position_names: Vec<String>,
    pub team_ids: Vec<i64>,
    pub team_names: Vec<String>,
    pub on_duty_position_id: Option<i64>,
    pub on_duty_position_name: Option<String>,
}

pub type Users = HashMap<i64, User>;
pub type Titles = HashMap<i64, String>;

/// Where directory data comes from. Each fetch returns fully-populated data;
/// caching is UserStore's job.
pub enum Source {
    Clubhouse(ClubhouseSource),
    Ims(IMSSource),
    /// Fixed data, for tests that need a directory without a database.
    Static {
        users: Users,
        positions: Titles,
        teams: Titles,
    },
}

impl Source {
    async fn fetch_users(&self) -> anyhow::Result<Users> {
        match self {
            Source::Clubhouse(s) => s.fetch_users().await,
            Source::Ims(s) => s.fetch_users().await,
            Source::Static { users, .. } => Ok(users.clone()),
        }
    }

    async fn fetch_positions(&self) -> anyhow::Result<Titles> {
        match self {
            Source::Clubhouse(s) => s.fetch_positions().await,
            Source::Ims(s) => s.fetch_positions().await,
            Source::Static { positions, .. } => Ok(positions.clone()),
        }
    }

    async fn fetch_teams(&self) -> anyhow::Result<Titles> {
        match self {
            Source::Clubhouse(s) => s.fetch_teams().await,
            Source::Ims(s) => s.fetch_teams().await,
            Source::Static { teams, .. } => Ok(teams.clone()),
        }
    }
}

/// A person as the personnel API and access checks see them.
#[derive(Debug, Clone, PartialEq)]
pub struct Person {
    pub handle: String,
    pub email: String,
    pub password: String,
    pub status: String,
    pub onsite: bool,
    pub directory_id: i64,
}

pub struct UserStore {
    source: Source,
    user_cache: InMemory<Users>,
    position_cache: InMemory<Titles>,
    team_cache: InMemory<Titles>,
}

impl UserStore {
    pub fn new(source: Source, cache_ttl: Duration) -> Arc<Self> {
        Arc::new(UserStore {
            source,
            user_cache: InMemory::new(cache_ttl),
            position_cache: InMemory::new(cache_ttl),
            team_cache: InMemory::new(cache_ttl),
        })
    }

    /// Invalidates all cached directory data. Call this after any write to the
    /// backing directory.
    pub fn flush(&self) {
        self.user_cache.invalidate();
        self.position_cache.invalidate();
        self.team_cache.invalidate();
    }

    pub async fn get_all_users(&self) -> anyhow::Result<Arc<Users>> {
        self.user_cache
            .get(|| self.source.fetch_users())
            .await
            .map_err(|e| e.context("[userCache.Get]"))
    }

    pub async fn get_rangers(&self) -> anyhow::Result<Vec<Person>> {
        let users = self.get_all_users().await?;
        Ok(users
            .values()
            .map(|u| Person {
                handle: u.handle.clone(),
                email: u.email.clone(),
                password: u.password.clone(),
                status: u.status.clone(),
                onsite: u.onsite,
                directory_id: u.id,
            })
            .collect())
    }

    pub async fn get_positions_and_teams(&self) -> anyhow::Result<(Arc<Titles>, Arc<Titles>)> {
        let positions = self
            .position_cache
            .get(|| self.source.fetch_positions())
            .await;
        let teams = self.team_cache.get(|| self.source.fetch_teams()).await;
        match (positions, teams) {
            (Ok(p), Ok(t)) => Ok((p, t)),
            (Err(e), _) | (_, Err(e)) => Err(e.context("[GetPositionsAndTeams]")),
        }
    }

    async fn with_user<R>(
        &self,
        person_id: i64,
        default: R,
        f: impl FnOnce(&User) -> R,
    ) -> anyhow::Result<R> {
        let users = self
            .get_all_users()
            .await
            .map_err(|e| e.context("[GetAllUsers]"))?;
        Ok(users.get(&person_id).map_or(default, f))
    }

    pub async fn positions_for_ranger(&self, person_id: i64) -> anyhow::Result<Vec<String>> {
        self.with_user(person_id, Vec::new(), |u| u.position_names.clone())
            .await
    }

    pub async fn teams_for_ranger(&self, person_id: i64) -> anyhow::Result<Vec<String>> {
        self.with_user(person_id, Vec::new(), |u| u.team_names.clone())
            .await
    }

    pub async fn onsite_for_ranger(&self, person_id: i64) -> anyhow::Result<bool> {
        self.with_user(person_id, false, |u| u.onsite).await
    }

    /// The Ranger's on-duty position ID and name, or zero values when they
    /// aren't on duty (or aren't in the directory at all).
    pub async fn on_duty_for_ranger(&self, person_id: i64) -> anyhow::Result<(i64, String)> {
        self.with_user(person_id, (0, String::new()), |u| {
            (
                u.on_duty_position_id.unwrap_or(0),
                u.on_duty_position_name.clone().unwrap_or_default(),
            )
        })
        .await
    }
}

/// Fills in each user's positions and teams from the membership rows. A
/// membership naming an unknown person is skipped; one naming an unknown
/// position or team is kept only when `keep_unknown_titles` (the Clubhouse
/// source's behavior, which records an empty name for it).
fn apply_memberships(
    users: &mut Users,
    positions: &Titles,
    teams: &Titles,
    person_positions: impl IntoIterator<Item = (i64, i64)>,
    person_teams: impl IntoIterator<Item = (i64, i64)>,
    keep_unknown_titles: bool,
) {
    for (person_id, position_id) in person_positions {
        let Some(user) = users.get_mut(&person_id) else {
            continue;
        };
        match positions.get(&position_id) {
            Some(name) => {
                user.position_ids.push(position_id);
                user.position_names.push(name.clone());
            }
            None if keep_unknown_titles => {
                user.position_ids.push(position_id);
                user.position_names.push(String::new());
            }
            None => {}
        }
    }
    for (person_id, team_id) in person_teams {
        let Some(user) = users.get_mut(&person_id) else {
            continue;
        };
        match teams.get(&team_id) {
            Some(name) => {
                user.team_ids.push(team_id);
                user.team_names.push(name.clone());
            }
            None if keep_unknown_titles => {
                user.team_ids.push(team_id);
                user.team_names.push(String::new());
            }
            None => {}
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn user(id: i64, handle: &str) -> User {
        User {
            id,
            handle: handle.to_string(),
            status: "active".to_string(),
            ..Default::default()
        }
    }

    #[test]
    fn apply_memberships_skips_unknown_titles_for_ims() {
        let mut users = Users::from([(1, user(1, "A"))]);
        let positions = Titles::from([(10, "Dirt".to_string())]);
        let teams = Titles::from([(20, "Council".to_string())]);
        apply_memberships(
            &mut users,
            &positions,
            &teams,
            [(1, 10), (1, 11), (2, 10)],
            [(1, 20), (1, 21)],
            false,
        );
        assert_eq!(users[&1].position_ids, vec![10]);
        assert_eq!(users[&1].position_names, vec!["Dirt"]);
        assert_eq!(users[&1].team_ids, vec![20]);
        assert_eq!(users[&1].team_names, vec!["Council"]);
    }

    #[test]
    fn apply_memberships_keeps_unknown_titles_for_clubhouse() {
        let mut users = Users::from([(1, user(1, "A"))]);
        let positions = Titles::from([(10, "Dirt".to_string())]);
        apply_memberships(
            &mut users,
            &positions,
            &Titles::new(),
            [(1, 10), (1, 11)],
            [(1, 21)],
            true,
        );
        assert_eq!(users[&1].position_ids, vec![10, 11]);
        assert_eq!(users[&1].position_names, vec!["Dirt", ""]);
        assert_eq!(users[&1].team_ids, vec![21]);
        assert_eq!(users[&1].team_names, vec![""]);
    }

    #[tokio::test]
    async fn user_store_lookups() {
        let mut alice = user(1, "Alice");
        alice.onsite = true;
        alice.position_names = vec!["Dirt".to_string()];
        alice.team_names = vec!["Council".to_string()];
        alice.on_duty_position_id = Some(10);
        alice.on_duty_position_name = Some("Dirt".to_string());
        let store = UserStore::new(
            Source::Static {
                users: Users::from([(1, alice)]),
                positions: Titles::from([(10, "Dirt".to_string())]),
                teams: Titles::from([(20, "Council".to_string())]),
            },
            Duration::from_secs(60),
        );

        assert_eq!(store.positions_for_ranger(1).await.unwrap(), vec!["Dirt"]);
        assert_eq!(store.teams_for_ranger(1).await.unwrap(), vec!["Council"]);
        assert!(store.onsite_for_ranger(1).await.unwrap());
        assert_eq!(
            store.on_duty_for_ranger(1).await.unwrap(),
            (10, "Dirt".to_string())
        );

        // Unknown people get zero values, not errors.
        assert!(store.positions_for_ranger(2).await.unwrap().is_empty());
        assert!(!store.onsite_for_ranger(2).await.unwrap());
        assert_eq!(
            store.on_duty_for_ranger(2).await.unwrap(),
            (0, String::new())
        );

        let rangers = store.get_rangers().await.unwrap();
        assert_eq!(rangers.len(), 1);
        assert_eq!(rangers[0].handle, "Alice");
        assert_eq!(rangers[0].directory_id, 1);

        let (positions, teams) = store.get_positions_and_teams().await.unwrap();
        assert_eq!(positions[&10], "Dirt");
        assert_eq!(teams[&20], "Council");
    }
}
