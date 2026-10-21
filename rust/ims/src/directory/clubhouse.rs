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

//! A directory Source backed by a Ranger Clubhouse database, which is what the
//! Black Rock Rangers use. A user's on-duty position comes from the Clubhouse
//! timesheet.

use std::collections::HashMap;
use std::sync::LazyLock;

use sqlx::mysql::MySqlConnectOptions;
use sqlx::{FromRow, MySqlPool};

use super::{Titles, User, Users, apply_memberships};
use crate::conf::ClubhouseDB;
use crate::store::query::Query;
use crate::store::sqlfile::{ParsedQuery, parse_file};

pub const CURRENT_SCHEMA: &str = include_str!("../../../../directory/schema/current.sql");

static QUERIES: LazyLock<HashMap<String, ParsedQuery>> =
    LazyLock::new(|| parse_file(include_str!("../../../../directory/queries.sql")));

fn q(name: &str) -> Query {
    Query::new(
        QUERIES
            .get(name)
            .unwrap_or_else(|| panic!("no query named {name} in directory/queries.sql")),
    )
}

#[derive(FromRow)]
struct PersonRow {
    id: i64,
    callsign: String,
    email: Option<String>,
    status: String,
    on_site: bool,
    password: Option<String>,
}

#[derive(FromRow)]
struct OnDutyRow {
    #[sqlx(try_from = "u64")]
    person_id: i64,
    #[sqlx(try_from = "u64")]
    position_id: i64,
}

#[derive(FromRow)]
struct TitleRow {
    #[sqlx(try_from = "u64")]
    id: i64,
    title: String,
}

#[derive(FromRow)]
struct PersonPositionRow {
    #[sqlx(try_from = "u64")]
    person_id: i64,
    #[sqlx(try_from = "u64")]
    position_id: i64,
}

#[derive(FromRow)]
struct PersonTeamRow {
    #[sqlx(try_from = "i32")]
    person_id: i64,
    #[sqlx(try_from = "i32")]
    team_id: i64,
}

/// Connects to the Clubhouse database. `hostname` is a "host:port" pair, as
/// in the Go server's config.
pub async fn connect(cfg: &ClubhouseDB) -> Result<MySqlPool, String> {
    tracing::info!("Setting up Clubhouse DB connection");
    let (host, port) = match cfg.hostname.rsplit_once(':') {
        Some((h, p)) => (
            h,
            p.parse::<u16>()
                .map_err(|e| format!("bad Clubhouse DB port: {e}"))?,
        ),
        None => (cfg.hostname.as_str(), 3306),
    };
    let host = if host.is_empty() { "localhost" } else { host };
    let options = MySqlConnectOptions::new()
        .host(host)
        .port(port)
        .username(&cfg.username)
        .password(&cfg.password)
        .database(&cfg.database);
    let pool = crate::store::pool_options(cfg.max_open_conns)
        .connect_with(options)
        .await
        .map_err(|e| format!("[connect]: {e}"))?;
    tracing::info!("Connected to Clubhouse MariaDB");
    Ok(pool)
}

pub struct ClubhouseSource {
    db: MySqlPool,
}

impl ClubhouseSource {
    pub fn new(db: MySqlPool) -> Self {
        ClubhouseSource { db }
    }

    pub async fn fetch_users(&self) -> anyhow::Result<Users> {
        let (persons, teams, positions, person_teams, person_positions, on_duty) =
            tokio::try_join!(
                q("Persons").fetch_all::<PersonRow, _>(&self.db),
                self.fetch_teams_raw(),
                self.fetch_positions_raw(),
                q("PersonTeams").fetch_all::<PersonTeamRow, _>(&self.db),
                q("PersonPositions").fetch_all::<PersonPositionRow, _>(&self.db),
                q("PersonsOnDuty").fetch_all::<OnDutyRow, _>(&self.db),
            )
            .map_err(|e| anyhow::anyhow!("[Teams,Positions,PersonTeams,PersonPositions] {e}"))?;

        let mut users: Users = persons
            .into_iter()
            .map(|p| {
                let user = User {
                    id: p.id,
                    handle: p.callsign,
                    email: p.email.unwrap_or_default(),
                    status: p.status,
                    onsite: p.on_site,
                    password: p.password.unwrap_or_default(),
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
            true,
        );
        for pod in on_duty {
            if let Some(user) = users.get_mut(&pod.person_id) {
                user.on_duty_position_id = Some(pod.position_id);
                if let Some(name) = positions.get(&pod.position_id) {
                    user.on_duty_position_name = Some(name.clone());
                }
            }
        }
        Ok(users)
    }

    async fn fetch_positions_raw(&self) -> Result<Titles, sqlx::Error> {
        let rows: Vec<TitleRow> = q("Positions").fetch_all(&self.db).await?;
        Ok(rows.into_iter().map(|r| (r.id, r.title)).collect())
    }

    async fn fetch_teams_raw(&self) -> Result<Titles, sqlx::Error> {
        let rows: Vec<TitleRow> = q("Teams").fetch_all(&self.db).await?;
        Ok(rows.into_iter().map(|r| (r.id, r.title)).collect())
    }

    pub async fn fetch_positions(&self) -> anyhow::Result<Titles> {
        self.fetch_positions_raw()
            .await
            .map_err(|e| anyhow::anyhow!("[Positions]: {e}"))
    }

    pub async fn fetch_teams(&self) -> anyhow::Result<Titles> {
        self.fetch_teams_raw()
            .await
            .map_err(|e| anyhow::anyhow!("[Teams]: {e}"))
    }
}
