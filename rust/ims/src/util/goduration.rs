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

//! Go's `time.ParseDuration` and `time.Duration.String`, because the env vars
//! that hold durations (e.g. `IMS_TOKEN_LIFETIME=8h`) are shared with the Go
//! server and must be read the same way.

use std::time::Duration;

const NANOS_PER_UNIT: [(&str, u128); 8] = [
    ("ns", 1),
    ("us", 1_000),
    ("µs", 1_000),
    ("μs", 1_000),
    ("ms", 1_000_000),
    ("s", 1_000_000_000),
    ("m", 60 * 1_000_000_000),
    ("h", 3600 * 1_000_000_000),
];

/// Parses a duration string such as "300ms", "1.5h" or "2h45m". Negative
/// durations are rejected, since none of IMS's settings can use one.
pub fn parse(s: &str) -> Result<Duration, String> {
    let orig = s;
    let err = || format!("time: invalid duration {orig:?}");
    let mut rest = s.strip_prefix('+').unwrap_or(s);
    if rest.starts_with('-') {
        return Err(format!("time: negative duration {orig:?} is not allowed"));
    }
    if rest == "0" {
        return Ok(Duration::ZERO);
    }
    if rest.is_empty() {
        return Err(err());
    }
    let mut total_nanos: u128 = 0;
    while !rest.is_empty() {
        let int_len = rest.bytes().take_while(u8::is_ascii_digit).count();
        let int_part = &rest[..int_len];
        rest = &rest[int_len..];
        let mut frac_part = "";
        if let Some(after_dot) = rest.strip_prefix('.') {
            let frac_len = after_dot.bytes().take_while(u8::is_ascii_digit).count();
            frac_part = &after_dot[..frac_len];
            rest = &after_dot[frac_len..];
        }
        if int_part.is_empty() && frac_part.is_empty() {
            return Err(err());
        }
        let unit_len = rest
            .char_indices()
            .find(|(_, c)| *c == '.' || c.is_ascii_digit())
            .map_or(rest.len(), |(i, _)| i);
        let unit = &rest[..unit_len];
        rest = &rest[unit_len..];
        if unit.is_empty() {
            return Err(format!("time: missing unit in duration {orig:?}"));
        }
        let Some(&(_, per_unit)) = NANOS_PER_UNIT.iter().find(|(u, _)| *u == unit) else {
            return Err(format!("time: unknown unit {unit:?} in duration {orig:?}"));
        };
        let int_val: u128 = if int_part.is_empty() {
            0
        } else {
            int_part.parse().map_err(|_| err())?
        };
        let mut nanos = int_val.checked_mul(per_unit).ok_or_else(err)?;
        if !frac_part.is_empty() {
            let digits = frac_part.len().min(30) as u32;
            let frac_val: u128 = frac_part[..digits as usize].parse().map_err(|_| err())?;
            nanos += frac_val * per_unit / 10u128.pow(digits);
        }
        total_nanos = total_nanos.checked_add(nanos).ok_or_else(err)?;
    }
    if total_nanos > i64::MAX as u128 {
        return Err(err());
    }
    Ok(Duration::from_nanos(total_nanos as u64))
}

/// Formats a duration the way Go's `time.Duration.String` does, e.g. "8h0m0s".
pub fn format(d: Duration) -> String {
    let nanos = d.as_nanos();
    if nanos == 0 {
        return "0s".to_string();
    }
    if nanos < 1_000_000_000 {
        let (unit, per_unit) = if nanos < 1_000 {
            ("ns", 1)
        } else if nanos < 1_000_000 {
            ("µs", 1_000)
        } else {
            ("ms", 1_000_000)
        };
        return format!("{}{unit}", format_frac(nanos, per_unit));
    }
    let hours = nanos / (3600 * 1_000_000_000);
    let minutes = (nanos / (60 * 1_000_000_000)) % 60;
    let secs_nanos = nanos % (60 * 1_000_000_000);
    let secs = format_frac(secs_nanos, 1_000_000_000);
    if hours > 0 {
        format!("{hours}h{minutes}m{secs}s")
    } else if minutes > 0 {
        format!("{minutes}m{secs}s")
    } else {
        format!("{secs}s")
    }
}

fn format_frac(value: u128, per_unit: u128) -> String {
    let whole = value / per_unit;
    let frac = value % per_unit;
    if frac == 0 {
        return whole.to_string();
    }
    let width = per_unit.ilog10() as usize;
    let frac_str = format!("{frac:0width$}");
    format!("{whole}.{}", frac_str.trim_end_matches('0'))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_simple_units() {
        assert_eq!(parse("1000s").unwrap(), Duration::from_secs(1000));
        assert_eq!(parse("3m").unwrap(), Duration::from_secs(180));
        assert_eq!(parse("8h").unwrap(), Duration::from_secs(8 * 3600));
        assert_eq!(parse("250ms").unwrap(), Duration::from_millis(250));
        assert_eq!(parse("7us").unwrap(), Duration::from_micros(7));
        assert_eq!(parse("7µs").unwrap(), Duration::from_micros(7));
        assert_eq!(parse("9ns").unwrap(), Duration::from_nanos(9));
    }

    #[test]
    fn parse_compound_and_fractional() {
        assert_eq!(
            parse("2h45m").unwrap(),
            Duration::from_secs(2 * 3600 + 45 * 60)
        );
        assert_eq!(parse("5m10s").unwrap(), Duration::from_secs(310));
        assert_eq!(parse("1.5h").unwrap(), Duration::from_secs(5400));
        assert_eq!(parse(".5s").unwrap(), Duration::from_millis(500));
        assert_eq!(parse("+1s").unwrap(), Duration::from_secs(1));
    }

    #[test]
    fn parse_zero() {
        assert_eq!(parse("0").unwrap(), Duration::ZERO);
        assert_eq!(parse("0s").unwrap(), Duration::ZERO);
    }

    #[test]
    fn parse_rejects_bad_input() {
        // A bare nonzero number has no unit, just as in Go.
        assert!(parse("20").is_err());
        assert!(parse("").is_err());
        assert!(parse("s").is_err());
        assert!(parse("1d").is_err());
        assert!(parse("-1s").is_err());
        assert!(parse("1.s.").is_err());
    }

    #[test]
    fn format_matches_go() {
        assert_eq!(format(Duration::ZERO), "0s");
        assert_eq!(format(Duration::from_secs(8 * 3600)), "8h0m0s");
        assert_eq!(format(Duration::from_secs(20 * 60)), "20m0s");
        assert_eq!(format(Duration::from_secs(5)), "5s");
        assert_eq!(format(Duration::from_millis(1500)), "1.5s");
        assert_eq!(format(Duration::from_millis(250)), "250ms");
        assert_eq!(format(Duration::from_micros(3)), "3µs");
        assert_eq!(format(Duration::from_nanos(12)), "12ns");
        assert_eq!(format(Duration::from_secs(3600 + 61)), "1h1m1s");
    }
}
