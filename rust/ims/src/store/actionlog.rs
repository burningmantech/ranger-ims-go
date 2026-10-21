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

//! Writes ACTION_LOG rows from a bounded background queue, like the Go
//! server's `store/actionlog`. A full queue drops rows rather than stalling
//! requests.

use std::sync::Arc;
use std::sync::atomic::{AtomicI64, Ordering};
use std::time::Duration;

use sqlx::MySqlPool;
use tokio::sync::mpsc;

use super::models::ActionLog;
use super::queries;

const WORK_QUEUE_MAX_LENGTH: usize = 1024;
const INSERT_DEADLINE: Duration = Duration::from_secs(10);

#[derive(Clone)]
pub struct ActionLogger {
    inner: Arc<Inner>,
}

struct Inner {
    db: MySqlPool,
    enabled: bool,
    synchronous_for_tests: bool,
    work: mpsc::Sender<ActionLog>,
    dropped: AtomicI64,
}

impl ActionLogger {
    pub fn new(db: MySqlPool, enabled: bool, synchronous_for_tests: bool) -> ActionLogger {
        let (tx, mut rx) = mpsc::channel::<ActionLog>(WORK_QUEUE_MAX_LENGTH);
        let inner = Arc::new(Inner {
            db,
            enabled,
            synchronous_for_tests,
            work: tx,
            dropped: AtomicI64::new(0),
        });
        let worker = Arc::clone(&inner);
        tokio::spawn(async move {
            while let Some(row) = rx.recv().await {
                write_row(&worker.db, row).await;
                let dropped = worker.dropped.swap(0, Ordering::Relaxed);
                if dropped > 0 {
                    tracing::warn!(
                        count = dropped,
                        "action log queue was full; rows were dropped"
                    );
                }
            }
        });
        ActionLogger { inner }
    }

    pub async fn log(&self, row: ActionLog) {
        if !self.inner.enabled {
            return;
        }
        if self.inner.synchronous_for_tests {
            write_row(&self.inner.db, row).await;
            return;
        }
        if self.inner.work.try_send(row).is_err() {
            self.inner.dropped.fetch_add(1, Ordering::Relaxed);
        }
    }
}

async fn write_row(db: &MySqlPool, row: ActionLog) {
    match tokio::time::timeout(INSERT_DEADLINE, queries::add_action_log(db, &row)).await {
        Ok(Ok(_)) => {}
        Ok(Err(e)) => tracing::error!(error = %e, "failed to add action log to db"),
        Err(_) => tracing::error!("timed out adding action log to db"),
    }
}
