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

//! The IMS database layer, ported from the Go server's `store` package.

pub mod actionlog;
pub mod errorlog;
pub mod migrate;
pub mod models;
pub mod queries;
pub mod query;
pub mod sqlfile;
pub mod testhook;

use std::time::Duration;

use sqlx::MySqlPool;
use sqlx::mysql::{MySqlConnectOptions, MySqlPoolOptions};

use crate::conf::{DBStore, DBStoreMaria, DBStoreType};

/// Connects to the IMS database, optionally migrating its schema up to the
/// version in `store/schema/current.sql`.
pub async fn connect(cfg: &DBStore, migrate_db: bool) -> Result<MySqlPool, String> {
    if cfg.store_type.is(DBStoreType::NoOp) {
        return Err("the noop DB store isn't supported by the Rust port".to_string());
    }
    let pool = open(&cfg.maria_db).await?;
    if migrate_db {
        migrate::migrate_db(&pool)
            .await
            .map_err(|e| format!("[MigrateDB]: {e}"))?;
    } else {
        tracing::info!("IMS DB migration not requested");
    }
    tracing::info!("Connected to IMS database");
    Ok(pool)
}

async fn open(cfg: &DBStoreMaria) -> Result<MySqlPool, String> {
    tracing::info!("Setting up IMS DB connection");
    let host = if cfg.host_name.is_empty() {
        "localhost"
    } else {
        &cfg.host_name
    };
    let options = MySqlConnectOptions::new()
        .host(host)
        .port(cfg.host_port as u16)
        .username(&cfg.username)
        .password(&cfg.password)
        .database(&cfg.database);
    pool_options(cfg.max_open_conns)
        .connect_with(options)
        .await
        .map_err(|e| format!("[connect]: {e}"))
}

/// Shared by both databases' pools. Connections are retired well before
/// anything between here and the database (MariaDB's wait_timeout, a load
/// balancer's idle timeout) can drop one out from under us.
pub fn pool_options(max_open_conns: i32) -> MySqlPoolOptions {
    // Go's database/sql treats a zero limit as unlimited, which a pool can't
    // be here, so it gets a modest cap instead.
    let max = if max_open_conns > 0 {
        max_open_conns as u32
    } else {
        10
    };
    MySqlPoolOptions::new()
        .max_connections(max)
        .max_lifetime(Duration::from_secs(5 * 60))
}
