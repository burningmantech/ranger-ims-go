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

//! Formatting helpers, ported from the Go server's `lib/format`.

use std::sync::LazyLock;

use regex::Regex;

// Pieces of a Black Rock City address, used within case-insensitive
// expressions. Go's \d and \s are ASCII-only, hence the explicit classes.
const RADIAL_PAT: &str = r"(?:([0-9]{1,2})(?::([0-9]{1,2}))?|([0-9]{3,4}))";
const CONCENTRIC_PAT: &str = r"([A-L]|ESPLANADE|ESPL|ESP)\.?";
const DISTANCE_PAT: &str = r"([0-9]{1,5})[\t\n\f\r ]*('|’|FT\.?|FEET)?";
const SEPARATOR_PAT: &str =
    r"(?:[\t\n\f\r ]*[&+/,][\t\n\f\r ]*|[\t\n\f\r ]+AND[\t\n\f\r ]+|[\t\n\f\r ]+)";

static RADIAL_CONCENTRIC: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(&format!(
        "(?i)^{RADIAL_PAT}{SEPARATOR_PAT}{CONCENTRIC_PAT}$"
    ))
    .expect("valid regex")
});
static CONCENTRIC_RADIAL: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(&format!(
        "(?i)^{CONCENTRIC_PAT}{SEPARATOR_PAT}{RADIAL_PAT}$"
    ))
    .expect("valid regex")
});
static RADIAL_DISTANCE: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(&format!("(?i)^{RADIAL_PAT}{SEPARATOR_PAT}{DISTANCE_PAT}$")).expect("valid regex")
});

/// Rewrites a Black Rock City address into its canonical form: "7:00 & E"
/// for an intersection (in whichever order it was written), or "12:00 2500'"
/// for a radial and distance. Anything else is returned unchanged.
pub fn normalize_address(address: &str) -> String {
    let trimmed = address.trim();
    let g = |c: &regex::Captures, i: usize| c.get(i).map_or("", |m| m.as_str()).to_string();
    if let Some(c) = RADIAL_CONCENTRIC.captures(trimmed)
        && let Some(radial) = format_radial(&g(&c, 1), &g(&c, 2), &g(&c, 3))
    {
        return format!("{radial} & {}", format_concentric(&g(&c, 4)));
    }
    if let Some(c) = CONCENTRIC_RADIAL.captures(trimmed)
        && let Some(radial) = format_radial(&g(&c, 2), &g(&c, 3), &g(&c, 4))
    {
        return format!("{} & {radial}", format_concentric(&g(&c, 1)));
    }
    if let Some(c) = RADIAL_DISTANCE.captures(trimmed)
        && let (Some(radial), Some(distance)) = (
            format_radial(&g(&c, 1), &g(&c, 2), &g(&c, 3)),
            format_distance(&g(&c, 4), &g(&c, 5)),
        )
    {
        return format!("{radial} {distance}");
    }
    address.to_string()
}

fn format_radial(hour: &str, minute: &str, colonless: &str) -> Option<String> {
    let (hour, minute) = if colonless.is_empty() {
        (hour, minute)
    } else {
        colonless.split_at(colonless.len() - 2)
    };
    let h: u32 = hour.parse().ok().filter(|h| *h <= 12)?;
    let m: u32 = if minute.is_empty() {
        0
    } else {
        minute.parse().ok().filter(|m| *m <= 59)?
    };
    Some(format!("{h}:{m:02}"))
}

fn format_concentric(street: &str) -> String {
    if street.len() == 1 {
        street.to_uppercase()
    } else {
        "Esplanade".to_string()
    }
}

fn format_distance(feet: &str, unit: &str) -> Option<String> {
    // Without a unit, a short number is more likely a typo than a distance.
    if unit.is_empty() && feet.len() < 3 {
        return None;
    }
    let f: u32 = feet.parse().ok().filter(|f| *f != 0)?;
    Some(format!("{f}'"))
}

/// Go's `%.3g`.
fn format_g3(v: f64) -> String {
    let sci = format!("{v:.2e}");
    let (mantissa, exp) = sci
        .split_once('e')
        .expect("scientific format has an exponent");
    let exp: i32 = exp.parse().expect("integer exponent");
    if !(-4..3).contains(&exp) {
        let mantissa = if mantissa.contains('.') {
            mantissa.trim_end_matches('0').trim_end_matches('.')
        } else {
            mantissa
        };
        let sign = if exp < 0 { '-' } else { '+' };
        return format!("{mantissa}e{sign}{:02}", exp.abs());
    }
    let decimals = (2 - exp).max(0) as usize;
    let s = format!("{v:.decimals$}");
    if s.contains('.') {
        s.trim_end_matches('0').trim_end_matches('.').to_string()
    } else {
        s
    }
}

pub fn human_byte_size(num_bytes: i64) -> String {
    const UNITS: [(&str, i64); 6] = [
        ("KiB", 1 << 10),
        ("MiB", 1 << 20),
        ("GiB", 1 << 30),
        ("TiB", 1 << 40),
        ("PiB", 1 << 50),
        ("EiB", 1 << 60),
    ];
    if num_bytes < 0 {
        return "invalid".to_string();
    }
    if num_bytes < 1 << 10 {
        return format!("{num_bytes} B");
    }
    for (i, (unit, size)) in UNITS.iter().enumerate() {
        let next = UNITS.get(i + 1).map(|(_, s)| *s);
        if next.is_none_or(|n| num_bytes < n) {
            return format!("{} {unit}", format_g3(num_bytes as f64 / *size as f64));
        }
    }
    unreachable!("the last unit has no upper bound")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn radial_then_concentric() {
        assert_eq!(normalize_address("7+e"), "7:00 & E");
        assert_eq!(normalize_address("7:00&e"), "7:00 & E");
        assert_eq!(normalize_address("7 and e"), "7:00 & E");
        assert_eq!(normalize_address("7, e"), "7:00 & E");
        assert_eq!(normalize_address("7/e"), "7:00 & E");
        assert_eq!(normalize_address("7 e"), "7:00 & E");
        assert_eq!(normalize_address("  7:00  &  e.  "), "7:00 & E");
        assert_eq!(normalize_address("07:00 & E"), "7:00 & E");
        assert_eq!(normalize_address("7:5 & a"), "7:05 & A");
        assert_eq!(normalize_address("12&l"), "12:00 & L");
        assert_eq!(normalize_address("0:15 b"), "0:15 & B");
    }

    #[test]
    fn colonless_radial() {
        assert_eq!(normalize_address("813&g"), "8:13 & G");
        assert_eq!(normalize_address("1015 + g"), "10:15 & G");
        assert_eq!(normalize_address("1230 esp"), "12:30 & Esplanade");
        assert_eq!(normalize_address("015 and b"), "0:15 & B");
        assert_eq!(normalize_address("g,813"), "G & 8:13");
        assert_eq!(normalize_address("813 500'"), "8:13 500'");
        assert_eq!(normalize_address("999 & G"), "999 & G");
        assert_eq!(normalize_address("1330 & G"), "1330 & G");
    }

    #[test]
    fn concentric_then_radial_and_esplanade() {
        assert_eq!(normalize_address("e and 7:30"), "E & 7:30");
        assert_eq!(normalize_address("c,3"), "C & 3:00");
        assert_eq!(normalize_address("4 + ESP."), "4:00 & Esplanade");
        assert_eq!(normalize_address("esp&4"), "Esplanade & 4:00");
    }

    #[test]
    fn art_addresses() {
        assert_eq!(normalize_address("12 2500'"), "12:00 2500'");
        assert_eq!(normalize_address("12, 2500ft"), "12:00 2500'");
        assert_eq!(normalize_address("12 & 2500 feet"), "12:00 2500'");
        assert_eq!(normalize_address("12:00 2500"), "12:00 2500'");
        assert_eq!(normalize_address("0:15 0500"), "0:15 500'");
        assert_eq!(normalize_address("6 80ft"), "6:00 80'");
        assert_eq!(normalize_address("6 80’"), "6:00 80'");
    }

    #[test]
    fn leaves_unrecognized_text_alone() {
        assert_eq!(normalize_address(""), "");
        assert_eq!(normalize_address("  "), "  ");
        assert_eq!(
            normalize_address("behind the porta potties"),
            "behind the porta potties"
        );
        assert_eq!(normalize_address("7:00 Plaza"), "7:00 Plaza");
        assert_eq!(normalize_address("7 & M"), "7 & M");
        assert_eq!(normalize_address("13:00 & E"), "13:00 & E");
        assert_eq!(normalize_address("7:60 & E"), "7:60 & E");
        assert_eq!(normalize_address("12 30"), "12 30");
        assert_eq!(normalize_address("7"), "7");
        assert_eq!(normalize_address("7:00 & E & 7:00"), "7:00 & E & 7:00");
    }

    #[test]
    fn human_byte_sizes() {
        let ki: i64 = 1024;
        assert_eq!(human_byte_size(-100), "invalid");
        assert_eq!(human_byte_size(0), "0 B");
        assert_eq!(human_byte_size(2_000), "1.95 KiB");
        assert_eq!(human_byte_size(2_000_000), "1.91 MiB");
        assert_eq!(human_byte_size(2_000_000_000_000_000_000), "1.73 EiB");
        assert_eq!(human_byte_size(567 * ki), "567 KiB");
        assert_eq!(human_byte_size(567 * ki * ki * ki * ki * ki), "567 PiB");
        assert_eq!(human_byte_size(i64::MAX), "8 EiB");
        assert_eq!(human_byte_size(100 * ki), "100 KiB");
        assert_eq!(human_byte_size(1023 * ki + 1000), "1.02e+03 KiB");
    }
}
