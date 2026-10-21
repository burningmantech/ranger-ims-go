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

use std::time::Duration;

/// Pings an IMS server and returns the process exit code: 0 when the server
/// answered "ack", and the same nonzero codes as the Go command otherwise.
pub async fn run(server_url: &str) -> i32 {
    let ping_url = format!("{}/ims/api/ping", server_url.trim_end_matches('/'));
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(5))
        .build()
        .expect("static client config");
    let resp = match client.get(&ping_url).send().await {
        Ok(resp) => resp,
        Err(e) => panic!("got a startup error: {e}"),
    };
    let status = resp.status();
    let body = resp.text().await.unwrap_or_default();
    if status != reqwest::StatusCode::OK {
        println!("wanted status code 200, got {}", status.as_u16());
        return 5;
    }
    if body.trim() != "ack" {
        println!("wanted response of 'ack', got '{body}'");
        return 6;
    }
    println!("OK");
    0
}
