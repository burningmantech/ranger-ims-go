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

//! A directory Source backed by IMS's own DIRECTORY_* tables, used when
//! IMS_DIRECTORY is "ims". Its users never have an on-duty position, so
//! "onduty:" access expressions never match them.

use sqlx::MySqlPool;

use super::{Titles, User, Users, apply_memberships};
use crate::store::queries;

/// The status reported for every user from the IMS-native directory, which
/// has just an active flag rather than the Clubhouse's status taxonomy.
pub const IMS_USER_STATUS: &str = "active";

pub struct IMSSource {
    db: MySqlPool,
}

impl IMSSource {
    pub fn new(db: MySqlPool) -> Self {
        IMSSource { db }
    }

    pub async fn fetch_users(&self) -> anyhow::Result<Users> {
        let (persons, positions, teams, person_positions, person_teams) = tokio::try_join!(
            queries::directory_active_persons(&self.db),
            self.fetch_positions_raw(),
            self.fetch_teams_raw(),
            queries::directory_person_positions(&self.db),
            queries::directory_person_teams(&self.db),
        )
        .map_err(|e| anyhow::anyhow!("[DirectoryPersons,Positions,Teams,Memberships] {e}"))?;

        let mut users: Users = persons
            .into_iter()
            .map(|p| {
                let user = User {
                    id: p.id,
                    handle: p.handle,
                    email: p.email.unwrap_or_default(),
                    status: IMS_USER_STATUS.to_string(),
                    onsite: p.onsite,
                    password: p.password,
                    ..Default::default()
                };
                (user.id, user)
            })
            .collect();
        apply_memberships(
            &mut users,
            &positions,
            &teams,
            person_positions
                .into_iter()
                .map(|pp| (pp.person_id, pp.position_id)),
            person_teams
                .into_iter()
                .map(|pt| (pt.person_id, pt.team_id)),
            false,
        );
        Ok(users)
    }

    async fn fetch_positions_raw(&self) -> Result<Titles, sqlx::Error> {
        let rows = queries::directory_active_positions(&self.db).await?;
        Ok(rows.into_iter().map(|r| (r.id, r.title)).collect())
    }

    async fn fetch_teams_raw(&self) -> Result<Titles, sqlx::Error> {
        let rows = queries::directory_active_teams(&self.db).await?;
        Ok(rows.into_iter().map(|r| (r.id, r.title)).collect())
    }

    pub async fn fetch_positions(&self) -> anyhow::Result<Titles> {
        self.fetch_positions_raw()
            .await
            .map_err(|e| anyhow::anyhow!("[DirectoryActivePositions]: {e}"))
    }

    pub async fn fetch_teams(&self) -> anyhow::Result<Titles> {
        self.fetch_teams_raw()
            .await
            .map_err(|e| anyhow::anyhow!("[DirectoryActiveTeams]: {e}"))
    }
}
