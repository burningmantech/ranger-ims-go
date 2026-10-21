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

use std::time::{Duration, SystemTime, UNIX_EPOCH};

use jsonwebtoken::errors::ErrorKind;
use jsonwebtoken::{Algorithm, DecodingKey, EncodingKey, Header, Validation, decode, encode};
use serde::{Deserialize, Serialize};

/// The only value IMS now issues for the "tok" claim. IMS used to also issue
/// "refresh" tokens, signed with the same key, and those must never pass as
/// access tokens.
pub const TOKEN_TYPE_ACCESS: &str = "access";

#[derive(Debug, Clone, Default, PartialEq, Serialize, Deserialize)]
pub struct IMSClaims {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iss: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub sub: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub exp: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub nbf: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub iat: Option<i64>,
    #[serde(default)]
    pub han: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    pub tok: String,
}

impl IMSClaims {
    pub fn ranger_handle(&self) -> &str {
        &self.han
    }

    /// The Ranger's directory ID, or -1 if it can't be determined.
    pub fn directory_id(&self) -> i64 {
        self.sub
            .as_deref()
            .and_then(|s| s.parse().ok())
            .unwrap_or(-1)
    }
}

#[derive(Debug, PartialEq, Eq)]
pub enum JwtError {
    NoJwtString,
    Expired,
    Invalid(String),
}

impl std::fmt::Display for JwtError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            JwtError::NoJwtString => write!(f, "no JWT string provided"),
            JwtError::Expired => {
                write!(f, "[jwt.Parse]: token has invalid claims: token is expired")
            }
            JwtError::Invalid(s) => write!(f, "{s}"),
        }
    }
}

impl std::error::Error for JwtError {}

#[derive(Clone)]
pub struct JWTer {
    pub secret_key: String,
}

fn unix_now() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or(Duration::ZERO)
        .as_secs() as i64
}

impl JWTer {
    pub fn create_jwt(&self, claims: &IMSClaims) -> anyhow::Result<String> {
        Ok(encode(
            &Header::new(Algorithm::HS256),
            claims,
            &EncodingKey::from_secret(self.secret_key.as_bytes()),
        )?)
    }

    pub fn create_access_token(
        &self,
        handle: &str,
        directory_id: i64,
        expiration: SystemTime,
    ) -> anyhow::Result<String> {
        let exp = expiration
            .duration_since(UNIX_EPOCH)
            .unwrap_or(Duration::ZERO)
            .as_secs() as i64;
        self.create_jwt(&IMSClaims {
            iss: Some("ims".to_string()),
            sub: Some(directory_id.to_string()),
            exp: Some(exp),
            nbf: None,
            iat: Some(unix_now()),
            han: handle.to_string(),
            tok: TOKEN_TYPE_ACCESS.to_string(),
        })
    }

    /// Returns the claims of a valid, authenticated access token. As with
    /// golang-jwt, "exp" and "nbf" are checked when present (with no leeway),
    /// but no claim is required.
    pub fn authenticate_jwt(&self, jwt: &str) -> Result<IMSClaims, JwtError> {
        if jwt.is_empty() {
            return Err(JwtError::NoJwtString);
        }
        let mut validation = Validation::new(Algorithm::HS256);
        validation.leeway = 0;
        validation.required_spec_claims.clear();
        validation.validate_exp = false;
        validation.validate_nbf = false;
        validation.validate_aud = false;
        let data = decode::<IMSClaims>(
            jwt,
            &DecodingKey::from_secret(self.secret_key.as_bytes()),
            &validation,
        )
        .map_err(|e| match e.kind() {
            ErrorKind::InvalidSignature => JwtError::Invalid(
                "[jwt.Parse]: token signature is invalid: signature is invalid".to_string(),
            ),
            _ => JwtError::Invalid(format!("[jwt.Parse]: {e}")),
        })?;
        let claims = data.claims;
        let now = unix_now();
        // golang-jwt treats a token as expired once now >= exp.
        if claims.exp.is_some_and(|exp| now >= exp) {
            return Err(JwtError::Expired);
        }
        if claims.nbf.is_some_and(|nbf| now < nbf) {
            return Err(JwtError::Invalid(
                "[jwt.Parse]: token is not valid yet".to_string(),
            ));
        }
        if claims.han.is_empty() {
            return Err(JwtError::Invalid("ranger handle is required".to_string()));
        }
        if claims.tok != TOKEN_TYPE_ACCESS {
            return Err(JwtError::Invalid(format!(
                "token type {:?} is not valid here",
                claims.tok
            )));
        }
        Ok(claims)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn in_an_hour() -> SystemTime {
        SystemTime::now() + Duration::from_secs(3600)
    }

    #[test]
    fn create_and_get_valid_jwt() {
        let jwter = JWTer {
            secret_key: "some-secret".to_string(),
        };
        let j = jwter
            .create_access_token("Hardware", 12345, in_an_hour())
            .unwrap();
        let claims = jwter.authenticate_jwt(&j).unwrap();
        assert_eq!(claims.ranger_handle(), "Hardware");
        assert_eq!(claims.sub.as_deref(), Some("12345"));
        assert_eq!(claims.directory_id(), 12345);
    }

    #[test]
    fn expired_jwt() {
        let jwter = JWTer {
            secret_key: "some-secret".to_string(),
        };
        let j = jwter
            .create_access_token("Hardware", 1, SystemTime::now() - Duration::from_secs(3600))
            .unwrap();
        let err = jwter.authenticate_jwt(&j).unwrap_err();
        assert_eq!(err, JwtError::Expired);
        assert!(err.to_string().contains("expired"));
    }

    #[test]
    fn jwt_signed_with_different_key() {
        let jwter = JWTer {
            secret_key: "some-secret".to_string(),
        };
        let other = JWTer {
            secret_key: "some-other-secret".to_string(),
        };
        let j = other
            .create_access_token("Hardware", 1, in_an_hour())
            .unwrap();
        assert!(
            jwter
                .authenticate_jwt(&j)
                .unwrap_err()
                .to_string()
                .contains("signature is invalid")
        );
    }

    #[test]
    fn jwt_without_handle() {
        let jwter = JWTer {
            secret_key: "some-secret".to_string(),
        };
        let j = jwter.create_access_token("", 12345, in_an_hour()).unwrap();
        assert!(
            jwter
                .authenticate_jwt(&j)
                .unwrap_err()
                .to_string()
                .contains("ranger handle is required")
        );
    }

    #[test]
    fn empty_jwt() {
        let jwter = JWTer {
            secret_key: "some-secret".to_string(),
        };
        assert_eq!(
            jwter.authenticate_jwt("").unwrap_err(),
            JwtError::NoJwtString
        );
    }

    #[test]
    fn old_refresh_token_is_not_an_access_token() {
        let jwter = JWTer {
            secret_key: "some-secret".to_string(),
        };
        let refresh = jwter
            .create_jwt(&IMSClaims {
                iss: Some("ims".to_string()),
                sub: Some("12345".to_string()),
                exp: Some(unix_now() + 3600),
                han: "Hardware".to_string(),
                tok: "refresh".to_string(),
                ..Default::default()
            })
            .unwrap();
        assert!(
            jwter
                .authenticate_jwt(&refresh)
                .unwrap_err()
                .to_string()
                .contains("token type")
        );
    }

    #[test]
    fn token_without_exp_is_accepted() {
        // golang-jwt doesn't require exp, so neither does this.
        let jwter = JWTer {
            secret_key: "some-secret".to_string(),
        };
        let j = jwter
            .create_jwt(&IMSClaims {
                sub: Some("1".to_string()),
                han: "Hardware".to_string(),
                tok: TOKEN_TYPE_ACCESS.to_string(),
                ..Default::default()
            })
            .unwrap();
        assert!(jwter.authenticate_jwt(&j).is_ok());
    }

    #[test]
    fn directory_id_of_bad_subject() {
        let claims = IMSClaims {
            sub: Some("abc".to_string()),
            ..Default::default()
        };
        assert_eq!(claims.directory_id(), -1);
        assert_eq!(IMSClaims::default().directory_id(), -1);
    }
}
