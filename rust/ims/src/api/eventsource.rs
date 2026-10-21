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

//! Server-Sent Events that tell web clients which records changed, ported
//! from the Go server's `api/eventsource.go`. Only notification metadata goes
//! out, never record contents.

use std::convert::Infallible;
use std::sync::atomic::{AtomicI64, Ordering};

use axum::body::{Body, Bytes};
use axum::http::{HeaderValue, StatusCode, header};
use axum::response::Response;
use futures::StreamExt;
use serde::Serialize;
use tokio::sync::broadcast;
use tokio_stream::wrappers::BroadcastStream;

#[derive(Debug, Clone, Default, PartialEq, Serialize)]
pub struct IMSEventData {
    #[serde(skip_serializing_if = "is_zero")]
    pub event_id: i32,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub comment: String,
    #[serde(skip_serializing_if = "is_zero")]
    pub incident_number: i32,
    #[serde(skip_serializing_if = "is_zero")]
    pub field_report_number: i32,
    #[serde(skip_serializing_if = "is_zero")]
    pub visit_number: i32,
    #[serde(skip_serializing_if = "std::ops::Not::not")]
    pub initial_event: bool,
}

fn is_zero(i: &i32) -> bool {
    *i == 0
}

#[derive(Debug, Clone, PartialEq)]
pub struct IMSEvent {
    pub id: i64,
    pub data: IMSEventData,
}

impl IMSEvent {
    pub fn event_type(&self) -> &'static str {
        if self.data.incident_number > 0 {
            "Incident"
        } else if self.data.field_report_number > 0 {
            "FieldReport"
        } else if self.data.visit_number > 0 {
            "Visit"
        } else if self.data.initial_event {
            "InitialEvent"
        } else {
            "UnknownEvent"
        }
    }

    /// The wire form, as the launchdarkly/eventsource encoder writes it.
    pub fn encode(&self) -> String {
        let data = serde_json::to_string(&self.data).unwrap_or_default();
        format!(
            "id: {}\nevent: {}\ndata: {}\n\n",
            self.id,
            self.event_type(),
            data
        )
    }
}

pub struct EventSourcer {
    id_counter: AtomicI64,
    tx: broadcast::Sender<IMSEvent>,
}

impl Default for EventSourcer {
    fn default() -> Self {
        EventSourcer::new()
    }
}

impl EventSourcer {
    pub fn new() -> EventSourcer {
        let (tx, _) = broadcast::channel(1024);
        EventSourcer {
            id_counter: AtomicI64::new(0),
            tx,
        }
    }

    fn publish(&self, data: IMSEventData) {
        let id = self.id_counter.fetch_add(1, Ordering::SeqCst) + 1;
        // No subscribers is fine.
        let _ = self.tx.send(IMSEvent { id, data });
    }

    pub fn notify_field_report_update(&self, event_id: i32, number: i32) {
        if number != 0 {
            self.publish(IMSEventData {
                event_id,
                field_report_number: number,
                ..Default::default()
            });
        }
    }

    pub fn notify_incident_update(&self, event_id: i32, number: i32) {
        if number != 0 {
            self.publish(IMSEventData {
                event_id,
                incident_number: number,
                ..Default::default()
            });
        }
    }

    pub fn notify_incident_updates(&self, event_id: i32, incident1: i32, incident2: i32) {
        self.notify_incident_update(event_id, incident1);
        if incident2 != incident1 {
            self.notify_incident_update(event_id, incident2);
        }
    }

    pub fn notify_visit_update(&self, event_id: i32, number: i32) {
        if number != 0 {
            self.publish(IMSEventData {
                event_id,
                visit_number: number,
                ..Default::default()
            });
        }
    }

    /// Opens a stream for one client. It starts with an InitialEvent carrying
    /// the most recent event ID, as the Go server's Replay does.
    pub fn stream_response(&self) -> Response {
        let rx = self.tx.subscribe();
        let initial = IMSEvent {
            id: self.id_counter.load(Ordering::SeqCst),
            data: IMSEventData {
                initial_event: true,
                comment: "The most recent SSE ID is provided in this message".to_string(),
                ..Default::default()
            },
        };
        let first =
            futures::stream::once(
                async move { Ok::<_, Infallible>(Bytes::from(initial.encode())) },
            );
        // A lagging client misses some notifications, as a slow client of the
        // Go server would; it refetches on the next one anyway.
        let rest = BroadcastStream::new(rx)
            .filter_map(|r| async move { r.ok().map(|e| Ok(Bytes::from(e.encode()))) });
        let mut resp = Response::new(Body::from_stream(first.chain(rest)));
        *resp.status_mut() = StatusCode::OK;
        let h = resp.headers_mut();
        h.insert(
            header::CONTENT_TYPE,
            HeaderValue::from_static("text/event-stream; charset=utf-8"),
        );
        h.insert(
            header::CACHE_CONTROL,
            HeaderValue::from_static("no-cache, no-store, must-revalidate"),
        );
        h.insert(header::CONNECTION, HeaderValue::from_static("keep-alive"));
        resp
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn event(data: IMSEventData) -> IMSEvent {
        IMSEvent { id: 1, data }
    }

    #[test]
    fn event_types() {
        assert_eq!(
            event(IMSEventData {
                incident_number: 1,
                ..Default::default()
            })
            .event_type(),
            "Incident"
        );
        assert_eq!(
            event(IMSEventData {
                field_report_number: 1,
                ..Default::default()
            })
            .event_type(),
            "FieldReport"
        );
        assert_eq!(
            event(IMSEventData {
                visit_number: 1,
                ..Default::default()
            })
            .event_type(),
            "Visit"
        );
        assert_eq!(
            event(IMSEventData {
                initial_event: true,
                ..Default::default()
            })
            .event_type(),
            "InitialEvent"
        );
        assert_eq!(event(IMSEventData::default()).event_type(), "UnknownEvent");
    }

    #[test]
    fn encodes_like_launchdarkly() {
        let e = IMSEvent {
            id: 7,
            data: IMSEventData {
                event_id: 2,
                incident_number: 3,
                ..Default::default()
            },
        };
        assert_eq!(
            e.encode(),
            "id: 7\nevent: Incident\ndata: {\"event_id\":2,\"incident_number\":3}\n\n"
        );
    }

    #[tokio::test]
    async fn publishes_to_subscribers() {
        let es = EventSourcer::new();
        let mut rx = es.tx.subscribe();
        es.notify_incident_updates(1, 5, 5);
        es.notify_visit_update(1, 0);
        es.notify_field_report_update(1, 9);
        assert_eq!(rx.recv().await.unwrap().data.incident_number, 5);
        let next = rx.recv().await.unwrap();
        assert_eq!(next.data.field_report_number, 9);
        assert_eq!(next.id, 2);
    }
}
