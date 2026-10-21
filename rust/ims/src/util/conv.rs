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

//! Conversions ported from the Go server's `lib/conv`, plus `GoTime`, which
//! stands in for Go's `time.Time` in the JSON API.

use chrono::{DateTime, Datelike, NaiveDate, Timelike, Utc};
use serde::{Deserialize, Deserializer, Serialize, Serializer};

/// A point in time that serializes the way Go's `time.Time` does in JSON
/// (RFC 3339 with trailing zeros trimmed from the nanoseconds), and whose zero
/// value is Go's: 0001-01-01T00:00:00Z. Clients use that zero value to mean
/// "clear this", e.g. for an Event's release times.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash)]
pub struct GoTime(pub DateTime<Utc>);

impl GoTime {
    pub fn zero() -> GoTime {
        GoTime(
            NaiveDate::from_ymd_opt(1, 1, 1)
                .expect("valid date")
                .and_hms_opt(0, 0, 0)
                .expect("valid time")
                .and_utc(),
        )
    }

    pub fn now() -> GoTime {
        GoTime(Utc::now())
    }

    pub fn is_zero(&self) -> bool {
        *self == GoTime::zero()
    }

    /// Like Go's `conv.FloatToTime`, for the epoch-seconds doubles stored in
    /// the database.
    pub fn from_float(f: f64) -> GoTime {
        let secs = f as i64;
        let nanos = ((f * 1e9) as i64) % 1_000_000_000;
        // Go's time.Unix normalizes a negative nanosecond part into the
        // previous second.
        let (secs, nanos) = if nanos < 0 {
            (secs - 1, nanos + 1_000_000_000)
        } else {
            (secs, nanos)
        };
        GoTime(DateTime::from_timestamp(secs, nanos as u32).unwrap_or_default())
    }

    pub fn from_opt_float(f: Option<f64>) -> GoTime {
        f.map_or_else(GoTime::zero, GoTime::from_float)
    }

    /// Like Go's `conv.TimeToFloat`.
    pub fn to_float(&self) -> f64 {
        f64::from(self.0.nanosecond() % 1_000_000_000) / 1e9 + self.0.timestamp() as f64
    }

    /// Like Go's `conv.TimeToNullFloat`: the zero time is stored as null.
    pub fn to_opt_float(&self) -> Option<f64> {
        if self.is_zero() {
            None
        } else {
            Some(self.to_float())
        }
    }

    /// Go's `time.RFC3339Nano` layout.
    pub fn format_rfc3339_nano(&self) -> String {
        let t = self.0;
        let mut s = format!(
            "{:04}-{:02}-{:02}T{:02}:{:02}:{:02}",
            t.year(),
            t.month(),
            t.day(),
            t.hour(),
            t.minute(),
            t.second()
        );
        let nanos = t.nanosecond() % 1_000_000_000;
        if nanos != 0 {
            let frac = format!("{nanos:09}");
            s.push('.');
            s.push_str(frac.trim_end_matches('0'));
        }
        s.push('Z');
        s
    }

    pub fn before(&self, other: &GoTime) -> bool {
        self.0 < other.0
    }

    pub fn after(&self, other: &GoTime) -> bool {
        self.0 > other.0
    }
}

impl Default for GoTime {
    fn default() -> Self {
        GoTime::zero()
    }
}

impl Serialize for GoTime {
    fn serialize<S: Serializer>(&self, s: S) -> Result<S::Ok, S::Error> {
        s.serialize_str(&self.format_rfc3339_nano())
    }
}

impl<'de> Deserialize<'de> for GoTime {
    fn deserialize<D: Deserializer<'de>>(d: D) -> Result<Self, D::Error> {
        let s = String::deserialize(d)?;
        DateTime::parse_from_rfc3339(&s)
            .map(|t| GoTime(t.with_timezone(&Utc)))
            .map_err(|e| serde::de::Error::custom(format!("parsing time {s:?} as RFC 3339: {e}")))
    }
}

/// Like Go's `conv.StringToSql`: empty and missing strings become null, and
/// long ones are cut to at most `max_len` bytes (on a character boundary,
/// where Go would cut mid-character). A `max_len` of 0 means no limit.
pub fn string_to_sql(s: Option<&str>, max_len: usize) -> Option<String> {
    let s = s?;
    if s.is_empty() {
        return None;
    }
    if max_len > 0 && s.len() > max_len {
        let mut end = max_len;
        while !s.is_char_boundary(end) {
            end -= 1;
        }
        return Some(s[..end].to_string());
    }
    Some(s.to_string())
}

pub fn empty_to_none(s: &str) -> Option<&str> {
    if s.is_empty() { None } else { Some(s) }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn float_round_trip() {
        let t = GoTime::from_float(1748459852.644699);
        assert_eq!(t.0.timestamp(), 1748459852);
        assert!((t.to_float() - 1748459852.644699).abs() < 1e-6);
    }

    #[test]
    fn formats_like_go() {
        assert_eq!(GoTime::zero().format_rfc3339_nano(), "0001-01-01T00:00:00Z");
        let t = GoTime(DateTime::from_timestamp(1700000000, 120_000_000).unwrap());
        assert_eq!(t.format_rfc3339_nano(), "2023-11-14T22:13:20.12Z");
        let t = GoTime(DateTime::from_timestamp(1700000000, 0).unwrap());
        assert_eq!(t.format_rfc3339_nano(), "2023-11-14T22:13:20Z");
    }

    #[test]
    fn parses_offsets_and_zero() {
        let t: GoTime = serde_json::from_str("\"2023-11-14T14:13:20-08:00\"").unwrap();
        assert_eq!(t.0.timestamp(), 1700000000);
        let z: GoTime = serde_json::from_str("\"0001-01-01T00:00:00Z\"").unwrap();
        assert!(z.is_zero());
        assert_eq!(z.to_opt_float(), None);
    }

    #[test]
    fn string_to_sql_truncates_and_nulls() {
        assert_eq!(string_to_sql(None, 0), None);
        assert_eq!(string_to_sql(Some(""), 0), None);
        assert_eq!(string_to_sql(Some("abcdef"), 3).as_deref(), Some("abc"));
        assert_eq!(string_to_sql(Some("abcdef"), 0).as_deref(), Some("abcdef"));
        // "é" is two bytes, so a cut at 2 would split it.
        assert_eq!(string_to_sql(Some("aé"), 2).as_deref(), Some("a"));
    }
}
