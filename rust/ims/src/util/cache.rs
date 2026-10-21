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

//! A single-value in-memory cache with a TTL, ported from the Go server's
//! `lib/cache`. Reads of a fresh value never wait; a stale value is refreshed
//! by exactly one caller while any others wait for its result.

use std::future::Future;
use std::sync::{Arc, RwLock};
use std::time::{Duration, Instant};

pub struct InMemory<T> {
    ttl: Duration,
    value: RwLock<Option<(Arc<T>, Instant)>>,
    refresh_lock: tokio::sync::Mutex<()>,
}

impl<T> InMemory<T> {
    pub fn new(ttl: Duration) -> Self {
        InMemory {
            ttl,
            value: RwLock::new(None),
            refresh_lock: tokio::sync::Mutex::new(()),
        }
    }

    fn fresh(&self) -> Option<Arc<T>> {
        let guard = self.value.read().unwrap_or_else(|e| e.into_inner());
        match guard.as_ref() {
            Some((v, at)) if at.elapsed() < self.ttl => Some(Arc::clone(v)),
            _ => None,
        }
    }

    /// Returns the cached value, calling `refresher` for a new one if the
    /// cached one has expired or been invalidated.
    pub async fn get<F, Fut, E>(&self, refresher: F) -> Result<Arc<T>, E>
    where
        F: FnOnce() -> Fut,
        Fut: Future<Output = Result<T, E>>,
    {
        if let Some(v) = self.fresh() {
            return Ok(v);
        }
        let _refreshing = self.refresh_lock.lock().await;
        // Another caller may have refreshed it while this one waited.
        if let Some(v) = self.fresh() {
            return Ok(v);
        }
        let new_val = Arc::new(refresher().await?);
        *self.value.write().unwrap_or_else(|e| e.into_inner()) =
            Some((Arc::clone(&new_val), Instant::now()));
        Ok(new_val)
    }

    pub fn invalidate(&self) {
        *self.value.write().unwrap_or_else(|e| e.into_inner()) = None;
    }
}

#[cfg(test)]
mod tests {
    use std::sync::atomic::{AtomicI64, Ordering};

    use super::*;

    async fn count_refreshes(ttl: Duration, rounds: usize) -> i64 {
        let cache = Arc::new(InMemory::<i64>::new(ttl));
        let refreshes = Arc::new(AtomicI64::new(0));
        let mut tasks = Vec::new();
        for _ in 0..rounds {
            let cache = Arc::clone(&cache);
            let refreshes = Arc::clone(&refreshes);
            tasks.push(tokio::spawn(async move {
                cache
                    .get(|| async { Ok::<_, ()>(refreshes.fetch_add(1, Ordering::SeqCst) + 1) })
                    .await
                    .unwrap()
            }));
        }
        for t in tasks {
            t.await.unwrap();
        }
        refreshes.load(Ordering::SeqCst)
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 4)]
    async fn zero_ttl_refreshes_every_time() {
        assert_eq!(count_refreshes(Duration::ZERO, 1000).await, 1000);
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 4)]
    async fn long_ttl_refreshes_once() {
        assert_eq!(count_refreshes(Duration::from_secs(3600), 1000).await, 1);
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 4)]
    async fn short_ttl_refreshes_between_once_and_every_time() {
        let n = count_refreshes(Duration::from_micros(5), 1000).await;
        assert!((1..=1000).contains(&n));
    }

    #[tokio::test]
    async fn invalidate_forces_a_refresh() {
        let cache = InMemory::<i64>::new(Duration::from_secs(3600));
        let first = cache.get(|| async { Ok::<_, ()>(1) }).await.unwrap();
        assert_eq!(*first, 1);
        let cached = cache.get(|| async { Ok::<_, ()>(2) }).await.unwrap();
        assert_eq!(*cached, 1);

        cache.invalidate();
        let refreshed = cache.get(|| async { Ok::<_, ()>(3) }).await.unwrap();
        assert_eq!(*refreshed, 3);
    }

    #[tokio::test]
    async fn refresh_error_is_returned_and_not_cached() {
        let cache = InMemory::<i64>::new(Duration::from_secs(3600));
        let err = cache
            .get(|| async { Err::<i64, _>("boom") })
            .await
            .unwrap_err();
        assert_eq!(err, "boom");
        let v = cache.get(|| async { Ok::<_, &str>(7) }).await.unwrap();
        assert_eq!(*v, 7);
    }
}
