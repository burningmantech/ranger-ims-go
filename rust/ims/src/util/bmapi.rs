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

//! A small client for the public Burning Man API, which is where IMS gets the
//! camp, art, and mutant vehicle data behind an Event's Places. Ported from
//! the Go server's `lib/bmapi`.

use std::time::Duration;

use serde::Deserialize;
use serde_json::value::RawValue;

/// Caps how much we'll read from the API.
const MAX_RESPONSE_BYTES: usize = 64 << 20;
const REQUEST_TIMEOUT: Duration = Duration::from_secs(120);

/// An API resource that maps onto an IMS place type.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Kind {
    Art,
    Camp,
    MV,
}

impl Kind {
    pub fn as_str(self) -> &'static str {
        match self {
            Kind::Art => "art",
            Kind::Camp => "camp",
            Kind::MV => "mv",
        }
    }

    pub fn parse(place_type: &str) -> Result<Kind, String> {
        match place_type {
            "art" => Ok(Kind::Art),
            "camp" => Ok(Kind::Camp),
            "mv" => Ok(Kind::MV),
            _ => Err(format!(
                "no Burning Man API data for place type {place_type}"
            )),
        }
    }
}

impl std::fmt::Display for Kind {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(self.as_str())
    }
}

/// One camp, art piece, or mutant vehicle. `raw` is the object exactly as
/// the API sent it, which IMS stores as the Place's external data.
#[derive(Debug, Clone)]
pub struct Record {
    pub name: String,
    pub location_string: String,
    pub raw: String,
}

pub struct Client {
    base_url: String,
    api_key: String,
    http: reqwest::Client,
}

fn summarize(body: &[u8]) -> String {
    const LIMIT: usize = 200;
    let s = String::from_utf8_lossy(body);
    let s = s.trim();
    if s.len() > LIMIT {
        let mut end = LIMIT;
        while !s.is_char_boundary(end) {
            end -= 1;
        }
        return format!("{}…", &s[..end]);
    }
    s.to_string()
}

impl Client {
    pub fn new(base_url: &str, api_key: &str, http: reqwest::Client) -> Client {
        Client {
            base_url: base_url.trim_end_matches('/').to_string(),
            api_key: api_key.to_string(),
            http,
        }
    }

    pub async fn fetch(&self, kind: Kind, year: i32) -> anyhow::Result<Vec<Record>> {
        let url = format!("{}/api/{}?year={}", self.base_url, kind, year);
        let resp = self
            .http
            .get(&url)
            .header("X-API-Key", &self.api_key)
            .header("Accept", "application/json")
            .timeout(REQUEST_TIMEOUT)
            .send()
            .await
            .map_err(|e| anyhow::anyhow!("[Do]: {e}"))?;
        let status = resp.status();
        let mut body = Vec::new();
        let mut stream = resp;
        while let Some(chunk) = stream
            .chunk()
            .await
            .map_err(|e| anyhow::anyhow!("[ReadAll]: {e}"))?
        {
            let room = MAX_RESPONSE_BYTES.saturating_sub(body.len());
            body.extend_from_slice(&chunk[..chunk.len().min(room)]);
            if room <= chunk.len() {
                break;
            }
        }
        if status != reqwest::StatusCode::OK {
            anyhow::bail!("burning man API returned {}: {}", status, summarize(&body));
        }
        let raws: Vec<Box<RawValue>> = serde_json::from_slice(&body).map_err(|e| {
            anyhow::anyhow!(
                "[Unmarshal]: burning man API returned something other than a JSON array ({}): {e}",
                summarize(&body)
            )
        })?;
        #[derive(Deserialize)]
        struct Fields {
            #[serde(default)]
            name: String,
            #[serde(default)]
            location_string: String,
        }
        raws.into_iter()
            .map(|raw| {
                let f: Fields = serde_json::from_str(raw.get())
                    .map_err(|e| anyhow::anyhow!("[Unmarshal]: {e}"))?;
                Ok(Record {
                    name: f.name,
                    location_string: f.location_string,
                    raw: raw.get().to_string(),
                })
            })
            .collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_kind() {
        assert_eq!(Kind::parse("camp"), Ok(Kind::Camp));
        assert_eq!(Kind::parse("art"), Ok(Kind::Art));
        assert_eq!(Kind::parse("mv"), Ok(Kind::MV));
        assert!(Kind::parse("other").is_err());
    }

    #[test]
    fn summarize_trims_long_bodies() {
        assert_eq!(summarize(b"  short  "), "short");
        let long = "x".repeat(300);
        assert_eq!(summarize(long.as_bytes()), format!("{}…", "x".repeat(200)));
    }

    async fn serve_once(status: u16, body: &'static str) -> String {
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        tokio::spawn(async move {
            use tokio::io::{AsyncReadExt, AsyncWriteExt};
            let (mut sock, _) = listener.accept().await.unwrap();
            let mut buf = [0u8; 4096];
            let n = sock.read(&mut buf).await.unwrap();
            let req = String::from_utf8_lossy(&buf[..n]).to_string();
            assert!(req.starts_with("GET /api/camp?year=2025 "), "{req}");
            assert!(req.to_lowercase().contains("x-api-key: key"));
            let resp = format!(
                "HTTP/1.1 {status} X\r\ncontent-type: application/json\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
                body.len()
            );
            sock.write_all(resp.as_bytes()).await.unwrap();
        });
        format!("http://{addr}")
    }

    #[tokio::test]
    async fn fetch_records() {
        let url = serve_once(
            200,
            r#"[{"uid":"a","name":"One","location_string":"3:00 & A","year":2025}]"#,
        )
        .await;
        let records = Client::new(&url, "key", reqwest::Client::new())
            .fetch(Kind::Camp, 2025)
            .await
            .unwrap();
        assert_eq!(records.len(), 1);
        assert_eq!(records[0].name, "One");
        assert_eq!(records[0].location_string, "3:00 & A");
        assert!(records[0].raw.contains(r#""uid":"a""#));
    }

    #[tokio::test]
    async fn fetch_error_status() {
        let url = serve_once(500, r#"{"detail":"bad day"}"#).await;
        let err = Client::new(&url, "key", reqwest::Client::new())
            .fetch(Kind::Camp, 2025)
            .await
            .unwrap_err();
        assert!(
            err.to_string().contains("burning man API returned 500"),
            "{err}"
        );
        assert!(err.to_string().contains("bad day"));
    }
}
