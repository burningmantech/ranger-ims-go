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

//! Writes ERROR_LOG rows from a bounded background queue, like the Go
//! server's `store/errorlog`.

use std::sync::Arc;
use std::sync::atomic::{AtomicI64, Ordering};
use std::time::Duration;

use sqlx::MySqlPool;
use tokio::sync::mpsc;

use super::models::ErrorLog;
use super::queries;
use crate::util::conv::string_to_sql;

const WORK_QUEUE_MAX_LENGTH: usize = 1024;
const INSERT_DEADLINE: Duration = Duration::from_secs(10);
const MAX_MESSAGE_LENGTH: usize = 4096;
const MAX_STACK_LENGTH: usize = 8192;

#[derive(Clone)]
pub struct ErrorLogger {
    inner: Arc<Inner>,
}

struct Inner {
    db: MySqlPool,
    enabled: bool,
    synchronous_for_tests: bool,
    work: mpsc::Sender<ErrorLog>,
    dropped: AtomicI64,
}

impl ErrorLogger {
    pub fn new(db: MySqlPool, enabled: bool, synchronous_for_tests: bool) -> ErrorLogger {
        let (tx, mut rx) = mpsc::channel::<ErrorLog>(WORK_QUEUE_MAX_LENGTH);
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
                        "error log queue was full; rows were dropped"
                    );
                }
            }
        });
        ErrorLogger { inner }
    }

    /// Never blocks on a full queue: that's exactly when the database is
    /// struggling and errors come thickest, and blocking would hang the server.
    pub async fn log(&self, mut row: ErrorLog) {
        if !self.inner.enabled {
            return;
        }
        row.response_message = string_to_sql(row.response_message.as_deref(), MAX_MESSAGE_LENGTH);
        row.internal_error = string_to_sql(row.internal_error.as_deref(), MAX_MESSAGE_LENGTH);
        row.stack_trace = string_to_sql(row.stack_trace.as_deref(), MAX_STACK_LENGTH);
        if self.inner.synchronous_for_tests {
            write_row(&self.inner.db, row).await;
            return;
        }
        if self.inner.work.try_send(row).is_err() {
            self.inner.dropped.fetch_add(1, Ordering::Relaxed);
        }
    }
}

async fn write_row(db: &MySqlPool, row: ErrorLog) {
    match tokio::time::timeout(INSERT_DEADLINE, queries::add_error_log(db, &row)).await {
        Ok(Ok(_)) => {}
        Ok(Err(e)) => tracing::error!(error = %e, "failed to add error log to db"),
        Err(_) => tracing::error!("timed out adding error log to db"),
    }
}
