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

mod common;

use std::time::Duration;

use common::MariaDB;
use ims::directory::{self, ClubhouseSource, IMSSource, Source, UserStore};
use ims::store::migrate;

const CLUBHOUSE_TEST_SEED: &str =
    include_str!("../../../api/integration/clubhousedb_test_seed.sql");

#[tokio::test]
async fn clubhouse_source() {
    let db = MariaDB::start().await;
    let pool = db.pool().await;
    sqlx::raw_sql(directory::clubhouse::CURRENT_SCHEMA)
        .execute(&pool)
        .await
        .unwrap();
    sqlx::raw_sql(CLUBHOUSE_TEST_SEED)
        .execute(&pool)
        .await
        .unwrap();

    let store = UserStore::new(
        Source::Clubhouse(ClubhouseSource::new(pool)),
        Duration::from_secs(60),
    );
    let users = store.get_all_users().await.unwrap();
    assert_eq!(users.len(), 2);

    let admin = &users[&6000];
    assert_eq!(admin.handle, "AdminTestRanger");
    assert_eq!(admin.email, "admintestranger@example.com");
    assert_eq!(admin.status, "active");
    assert!(admin.onsite);
    assert_eq!(admin.team_names, vec!["Brown Dot"]);
    assert_eq!(admin.on_duty_position_id, None);

    let alice = &users[&6001];
    assert_eq!(alice.position_ids, vec![7000]);
    assert_eq!(alice.position_names, vec!["Nooperator"]);
    assert_eq!(alice.on_duty_position_id, Some(7000));
    assert_eq!(alice.on_duty_position_name.as_deref(), Some("Nooperator"));
    assert_eq!(
        store.on_duty_for_ranger(6001).await.unwrap(),
        (7000, "Nooperator".to_string())
    );

    let (positions, teams) = store.get_positions_and_teams().await.unwrap();
    assert_eq!(positions[&7000], "Nooperator");
    assert_eq!(teams[&8000], "Brown Dot");
}

#[tokio::test]
async fn ims_source() {
    let db = MariaDB::start().await;
    let pool = db.pool().await;
    migrate::migrate_db(&pool).await.unwrap();
    sqlx::raw_sql(
        "insert into DIRECTORY_PERSON (ID, HANDLE, EMAIL, PASSWORD, ACTIVE, ONSITE) values
            (1, 'Active', 'a@example.com', 'hash', true, true),
            (2, 'Gone', null, 'hash', false, false);
         insert into DIRECTORY_POSITION (ID, TITLE, ACTIVE) values (10, 'Dirt', true), (11, 'Old', false);
         insert into DIRECTORY_TEAM (ID, TITLE, ACTIVE) values (20, 'Council', true);
         insert into DIRECTORY_PERSON__POSITION (PERSON_ID, POSITION_ID) values (1, 10), (1, 11), (2, 10);
         insert into DIRECTORY_PERSON__TEAM (PERSON_ID, TEAM_ID) values (1, 20);",
    )
    .execute(&pool)
    .await
    .unwrap();

    let store = UserStore::new(Source::Ims(IMSSource::new(pool)), Duration::from_secs(60));
    let users = store.get_all_users().await.unwrap();
    assert_eq!(users.len(), 1);
    let user = &users[&1];
    assert_eq!(user.handle, "Active");
    assert_eq!(user.status, directory::ims::IMS_USER_STATUS);
    assert!(user.onsite);
    assert_eq!(user.password, "hash");
    // The inactive position is left out.
    assert_eq!(user.position_names, vec!["Dirt"]);
    assert_eq!(user.team_names, vec!["Council"]);
    assert_eq!(user.on_duty_position_id, None);
}
