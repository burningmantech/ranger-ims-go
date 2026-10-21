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

//! JSON output that matches Go's `encoding/json`, which escapes `<`, `>`, `&`,
//! U+2028 and U+2029 inside strings. Those characters can only appear inside
//! strings in serde_json's output, so escaping them after the fact is safe.

use serde::Serialize;

pub fn to_string<T: Serialize + ?Sized>(v: &T) -> serde_json::Result<String> {
    Ok(escape_like_go(&serde_json::to_string(v)?))
}

pub fn escape_like_go(json: &str) -> String {
    let mut out = String::with_capacity(json.len());
    for c in json.chars() {
        match c {
            '<' => out.push_str("\\u003c"),
            '>' => out.push_str("\\u003e"),
            '&' => out.push_str("\\u0026"),
            '\u{2028}' => out.push_str("\\u2028"),
            '\u{2029}' => out.push_str("\\u2029"),
            _ => out.push(c),
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn escapes_html_characters() {
        let got = to_string(&serde_json::json!({"a": "<b> & \u{2028}"})).unwrap();
        assert_eq!(got, "{\"a\":\"\\u003cb\\u003e \\u0026 \\u2028\"}");
    }
}
