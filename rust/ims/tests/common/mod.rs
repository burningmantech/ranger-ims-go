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

//! A throwaway MariaDB for integration tests, matching lib/testctr in the Go tree.

#![allow(dead_code)]

use sqlx::MySqlPool;
use sqlx::mysql::{MySqlConnectOptions, MySqlPoolOptions};
use testcontainers::core::{ContainerPort, WaitFor};
use testcontainers::runners::AsyncRunner;
use testcontainers::{ContainerAsync, GenericImage, ImageExt};

pub const DATABASE: &str = "ims";
pub const USERNAME: &str = "rangers";
pub const PASSWORD: &str = "password";

pub struct MariaDB {
    pub container: ContainerAsync<GenericImage>,
    pub port: u16,
}

impl MariaDB {
    pub async fn start() -> MariaDB {
        let container = GenericImage::new("mariadb", "10.5.29")
            .with_exposed_port(ContainerPort::Tcp(3306))
            .with_wait_for(WaitFor::message_on_stderr(
                "port: 3306  mariadb.org binary distribution",
            ))
            .with_env_var("MARIADB_RANDOM_ROOT_PASSWORD", "true")
            .with_env_var("MARIADB_DATABASE", DATABASE)
            .with_env_var("MARIADB_USER", USERNAME)
            .with_env_var("MARIADB_PASSWORD", PASSWORD)
            .start()
            .await
            .expect("starting MariaDB container");
        let port = container
            .get_host_port_ipv4(3306)
            .await
            .expect("mapped port");
        MariaDB { container, port }
    }

    pub async fn pool(&self) -> MySqlPool {
        MySqlPoolOptions::new()
            .max_connections(5)
            .connect_with(
                MySqlConnectOptions::new()
                    .host("127.0.0.1")
                    .port(self.port)
                    .username(USERNAME)
                    .password(PASSWORD)
                    .database(DATABASE),
            )
            .await
            .expect("connecting to MariaDB")
    }
}
