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

use common::MariaDB;
use ims::store::migrate;
use sqlx::MySqlPool;

const SCHEMA_06: &str = include_str!("../../../store/integration/06.sql");

async fn tables(pool: &MySqlPool) -> Vec<(String, String)> {
    let mut names: Vec<String> = sqlx::query_scalar("show tables")
        .fetch_all(pool)
        .await
        .unwrap();
    names.sort();
    let mut out = Vec::new();
    for name in names {
        let (_, create): (String, String) = sqlx::query_as(&format!("show create table `{name}`"))
            .fetch_one(pool)
            .await
            .unwrap();
        out.push((name, create));
    }
    out
}

#[tokio::test]
async fn migrating_from_06_matches_current_schema() {
    let (db1, db2) = tokio::join!(MariaDB::start(), MariaDB::start());
    let (pool1, pool2) = tokio::join!(db1.pool(), db2.pool());

    sqlx::raw_sql(SCHEMA_06).execute(&pool1).await.unwrap();
    migrate::migrate_db(&pool1).await.unwrap();

    // A fresh database gets current.sql, and a second migration is a no-op.
    migrate::migrate_db(&pool2).await.unwrap();
    migrate::migrate_db(&pool2).await.unwrap();

    let repo_version = migrate::repo_schema_version().unwrap();
    assert_eq!(
        migrate::db_schema_version(&pool1).await.unwrap(),
        repo_version
    );
    assert_eq!(
        migrate::db_schema_version(&pool2).await.unwrap(),
        repo_version
    );
    assert_eq!(tables(&pool1).await, tables(&pool2).await);
}
