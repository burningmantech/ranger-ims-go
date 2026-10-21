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

//! The cookies that carry a web client's access token. See the Go server's
//! `lib/authz/cookie.go` for the reasoning behind each attribute.

use std::time::Duration;

use axum::http::HeaderMap;
use axum::http::header::COOKIE;

/// Browsers only accept a "__Host-" cookie that's Secure, has Path=/, and has
/// no Domain, which means only this exact host can have set it.
pub const ACCESS_TOKEN_COOKIE_NAME: &str = "__Host-ims_access_token";

/// The cookie's name when insecure cookies are on and the request came over
/// plain HTTP, since browsers wouldn't accept the prefix without Secure.
pub const INSECURE_ACCESS_TOKEN_COOKIE_NAME: &str = "ims_access_token";

const LEGACY_REFRESH_TOKEN_COOKIE_NAME: &str = "refresh_token";

#[derive(Debug)]
pub struct MultipleTokenCookies;

impl std::fmt::Display for MultipleTokenCookies {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "multiple access token cookies")
    }
}

impl std::error::Error for MultipleTokenCookies {}

#[derive(Debug, Clone, Copy, Default)]
pub struct TokenCookies {
    /// Allows a cookie that isn't Secure, for local development over plain
    /// HTTP only.
    pub insecure: bool,
}

/// A Set-Cookie value, formatted as Go's `http.Cookie.String` does.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SetCookie {
    pub name: &'static str,
    pub value: String,
    /// Go's MaxAge: 0 means unset, negative means delete now.
    pub max_age: i64,
    pub secure: bool,
}

impl SetCookie {
    pub fn header_value(&self) -> String {
        let mut s = format!("{}={}; Path=/", self.name, quote_if_needed(&self.value));
        if self.max_age > 0 {
            s.push_str(&format!("; Max-Age={}", self.max_age));
        } else if self.max_age < 0 {
            s.push_str("; Max-Age=0");
        }
        s.push_str("; HttpOnly");
        if self.secure {
            s.push_str("; Secure");
        }
        s.push_str("; SameSite=Strict");
        s
    }
}

fn quote_if_needed(v: &str) -> String {
    if v.contains(' ') || v.contains(',') {
        format!("\"{v}\"")
    } else {
        v.to_string()
    }
}

impl TokenCookies {
    /// This server never terminates TLS itself, so a request is secure unless
    /// insecure cookies are allowed.
    fn secure(&self) -> bool {
        !self.insecure
    }

    fn access_token_name(&self) -> &'static str {
        if self.secure() {
            ACCESS_TOKEN_COOKIE_NAME
        } else {
            INSECURE_ACCESS_TOKEN_COOKIE_NAME
        }
    }

    pub fn access_token(&self, token: &str, lifetime: Duration) -> SetCookie {
        SetCookie {
            name: self.access_token_name(),
            value: token.to_string(),
            max_age: lifetime.as_secs() as i64,
            secure: self.secure(),
        }
    }

    pub fn expired_access_token(&self) -> SetCookie {
        SetCookie {
            max_age: -1,
            ..self.access_token("", Duration::ZERO)
        }
    }

    pub fn expired_legacy_refresh_token(&self) -> SetCookie {
        SetCookie {
            name: LEGACY_REFRESH_TOKEN_COOKIE_NAME,
            value: String::new(),
            max_age: -1,
            secure: self.secure(),
        }
    }

    /// The token in the request's access token cookie, or "" if there isn't
    /// one. More than one such cookie means one is a lookalike planted from a
    /// sibling subdomain, so neither is trusted.
    pub fn access_token_from(&self, headers: &HeaderMap) -> Result<String, MultipleTokenCookies> {
        let name = self.access_token_name();
        let found: Vec<String> = request_cookies(headers)
            .into_iter()
            .filter(|(n, _)| n == name)
            .map(|(_, v)| v)
            .collect();
        match found.len() {
            0 => Ok(String::new()),
            1 => Ok(found.into_iter().next().unwrap_or_default()),
            _ => Err(MultipleTokenCookies),
        }
    }
}

/// Parses every Cookie header into (name, value) pairs, as Go's
/// `Request.Cookies` does.
fn request_cookies(headers: &HeaderMap) -> Vec<(String, String)> {
    let mut out = Vec::new();
    for header in headers.get_all(COOKIE) {
        let Ok(line) = header.to_str() else { continue };
        for part in line.split(';') {
            let part = part.trim();
            if part.is_empty() {
                continue;
            }
            let (name, value) = part.split_once('=').unwrap_or((part, ""));
            if name.is_empty() {
                continue;
            }
            let value = value
                .strip_prefix('"')
                .and_then(|v| v.strip_suffix('"'))
                .unwrap_or(value);
            out.push((name.to_string(), value.to_string()));
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use axum::http::HeaderValue;

    use super::*;

    fn headers_with(cookies: &[&str]) -> HeaderMap {
        let mut h = HeaderMap::new();
        for c in cookies {
            h.append(COOKIE, HeaderValue::from_str(c).unwrap());
        }
        h
    }

    #[test]
    fn cookies_are_secure_by_default() {
        let cookies = TokenCookies::default();
        let c = cookies.access_token("tok", Duration::from_secs(3600));
        assert!(c.secure);
        assert_eq!(c.name, ACCESS_TOKEN_COOKIE_NAME);
        assert_eq!(
            c.header_value(),
            "__Host-ims_access_token=tok; Path=/; Max-Age=3600; HttpOnly; Secure; SameSite=Strict"
        );
        let expired = cookies.expired_access_token();
        assert_eq!(
            expired.header_value(),
            "__Host-ims_access_token=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Strict"
        );
        assert!(cookies.expired_legacy_refresh_token().secure);
    }

    #[test]
    fn insecure_cookies_over_plain_http() {
        let cookies = TokenCookies { insecure: true };
        let c = cookies.access_token("tok", Duration::from_secs(3600));
        assert!(!c.secure);
        assert_eq!(c.name, INSECURE_ACCESS_TOKEN_COOKIE_NAME);
        assert_eq!(
            cookies.expired_access_token().name,
            INSECURE_ACCESS_TOKEN_COOKIE_NAME
        );
        assert!(!cookies.expired_legacy_refresh_token().secure);
        assert_eq!(
            cookies.expired_legacy_refresh_token().header_value(),
            "refresh_token=; Path=/; Max-Age=0; HttpOnly; SameSite=Strict"
        );
    }

    #[test]
    fn access_token_from_cookie() {
        let prod = TokenCookies::default();
        let insecure = TokenCookies { insecure: true };

        // No cookie, no token
        assert_eq!(prod.access_token_from(&HeaderMap::new()).unwrap(), "");

        // The prefixed cookie carries the token
        assert_eq!(
            prod.access_token_from(&headers_with(&["__Host-ims_access_token=tok"]))
                .unwrap(),
            "tok"
        );

        // An unprefixed cookie is ignored, since any subdomain could have set it
        assert_eq!(
            prod.access_token_from(&headers_with(&["ims_access_token=planted"]))
                .unwrap(),
            ""
        );

        // Two cookies by that name mean one is a lookalike
        assert!(
            prod.access_token_from(&headers_with(&[
                "__Host-ims_access_token=planted; __Host-ims_access_token=tok"
            ]))
            .is_err()
        );
        assert!(
            prod.access_token_from(&headers_with(&[
                "__Host-ims_access_token=planted",
                "__Host-ims_access_token=tok"
            ]))
            .is_err()
        );

        // With insecure cookies, plain HTTP uses the unprefixed cookie
        assert_eq!(
            insecure
                .access_token_from(&headers_with(&["a=b; ims_access_token=tok"]))
                .unwrap(),
            "tok"
        );
    }
}
