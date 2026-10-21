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

//! Argon2id hashes in the PHC string format that the Clubhouse, and hence
//! the Go server's `lib/argon2id`, reads and writes:
//! `$argon2id$v=19$m=8192,t=4,p=1$<b64 salt>$<b64 key>`.

use argon2::{Algorithm, Argon2, Version};
use base64::Engine;
use base64::engine::general_purpose::STANDARD_NO_PAD;
use rand::RngCore;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Params {
    pub memory_kib: u32,
    pub iterations: u32,
    pub parallelism: u8,
    pub salt_length: u32,
    pub key_length: u32,
}

pub const CLUBHOUSE_PARAMS: Params = Params {
    memory_kib: 8 * 1024,
    iterations: 4,
    parallelism: 1,
    salt_length: 16,
    key_length: 32,
};

/// What the IMS-native directory hashes new passwords with.
pub const SECOND_RECOMMENDED_PARAMS: Params = Params {
    memory_kib: 64 * 1024,
    iterations: 3,
    parallelism: 4,
    salt_length: 16,
    key_length: 32,
};

/// Only for dev-only users, e.g. those made by `add-user` against a dev stack.
pub fn development_params() -> Params {
    Params {
        memory_kib: 64 * 1024,
        iterations: 1,
        parallelism: std::thread::available_parallelism()
            .map_or(1, |n| n.get().min(u8::MAX as usize) as u8),
        salt_length: 16,
        key_length: 32,
    }
}

#[derive(Debug, PartialEq, Eq)]
pub enum Error {
    InvalidHash,
    IncompatibleVariant,
    IncompatibleVersion,
    Compute(String),
}

impl std::fmt::Display for Error {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Error::InvalidHash => write!(f, "argon2id: hash is not in the correct format"),
            Error::IncompatibleVariant => write!(f, "argon2id: incompatible variant of argon2"),
            Error::IncompatibleVersion => write!(f, "argon2id: incompatible version of argon2"),
            Error::Compute(e) => write!(f, "argon2id: {e}"),
        }
    }
}

impl std::error::Error for Error {}

fn derive_key(password: &str, salt: &[u8], params: &Params) -> Result<Vec<u8>, Error> {
    if params.memory_kib < 8 * u32::from(params.parallelism)
        && params.parallelism > 0
        && params.iterations > 0
    {
        return Ok(super::argon2go::id_key(
            password.as_bytes(),
            salt,
            params.iterations,
            params.memory_kib,
            params.parallelism,
            params.key_length,
        ));
    }
    let argon_params = argon2::Params::new(
        params.memory_kib,
        params.iterations,
        u32::from(params.parallelism),
        Some(params.key_length as usize),
    )
    .map_err(|e| Error::Compute(e.to_string()))?;
    let mut key = vec![0u8; params.key_length as usize];
    Argon2::new(Algorithm::Argon2id, Version::V0x13, argon_params)
        .hash_password_into(password.as_bytes(), salt, &mut key)
        .map_err(|e| Error::Compute(e.to_string()))?;
    Ok(key)
}

pub fn create_hash(password: &str, params: &Params) -> String {
    let mut salt = vec![0u8; params.salt_length as usize];
    rand::rng().fill_bytes(&mut salt);
    let key = derive_key(password, &salt, params).expect("argon2id params are constants");
    format!(
        "$argon2id$v=19$m={},t={},p={}${}${}",
        params.memory_kib,
        params.iterations,
        params.parallelism,
        STANDARD_NO_PAD.encode(&salt),
        STANDARD_NO_PAD.encode(&key),
    )
}

pub fn compare_password_and_hash(password: &str, hash: &str) -> Result<bool, Error> {
    let (params, salt, key) = decode_hash(hash)?;
    let other_key = derive_key(password, &salt, &params)?;
    Ok(constant_time_eq(&key, &other_key))
}

fn constant_time_eq(a: &[u8], b: &[u8]) -> bool {
    if a.len() != b.len() {
        return false;
    }
    a.iter().zip(b).fold(0u8, |acc, (x, y)| acc | (x ^ y)) == 0
}

pub fn decode_hash(hash: &str) -> Result<(Params, Vec<u8>, Vec<u8>), Error> {
    let rest = hash
        .strip_prefix("$argon2id$")
        .ok_or(Error::IncompatibleVariant)?;
    let parts: Vec<&str> = rest.split('$').collect();
    if parts.len() != 4 {
        return Err(Error::InvalidHash);
    }
    let version: u32 = parts[0]
        .strip_prefix("v=")
        .and_then(|v| v.parse().ok())
        .ok_or(Error::InvalidHash)?;
    if version != 19 {
        return Err(Error::IncompatibleVersion);
    }
    let mut memory_kib = None;
    let mut iterations = None;
    let mut parallelism = None;
    for kv in parts[1].split(',') {
        match kv.split_once('=') {
            Some(("m", v)) => memory_kib = v.parse().ok(),
            Some(("t", v)) => iterations = v.parse().ok(),
            Some(("p", v)) => parallelism = v.parse().ok(),
            _ => return Err(Error::InvalidHash),
        }
    }
    let salt = STANDARD_NO_PAD
        .decode(parts[2])
        .map_err(|_| Error::InvalidHash)?;
    let key = STANDARD_NO_PAD
        .decode(parts[3])
        .map_err(|_| Error::InvalidHash)?;
    let params = Params {
        memory_kib: memory_kib.ok_or(Error::InvalidHash)?,
        iterations: iterations.ok_or(Error::InvalidHash)?,
        parallelism: parallelism.ok_or(Error::InvalidHash)?,
        salt_length: salt.len() as u32,
        key_length: key.len() as u32,
    };
    Ok((params, salt, key))
}

#[cfg(test)]
mod tests {
    use super::*;

    const CHEAP: Params = Params {
        memory_kib: 64,
        iterations: 1,
        parallelism: 1,
        salt_length: 16,
        key_length: 32,
    };

    #[test]
    fn round_trip() {
        let hash = create_hash("hunter2", &CHEAP);
        assert!(hash.starts_with("$argon2id$v=19$m=64,t=1,p=1$"));
        assert_eq!(compare_password_and_hash("hunter2", &hash), Ok(true));
        assert_eq!(compare_password_and_hash("hunter3", &hash), Ok(false));
    }

    #[test]
    fn verifies_hash_made_by_go_impl() {
        // Produced by the Go server's `hash_password --password 'correct horse'`.
        let hash = "$argon2id$v=19$m=8192,t=4,p=1$Pz5HcA51bZyiJN5DsJXjAw$trH0H74sgu1X7JZMVOwzzW6mzMbHT3A0HAps3yGI27Q";
        let (params, _, _) = decode_hash(hash).unwrap();
        assert_eq!(params, CLUBHOUSE_PARAMS);
        assert_eq!(compare_password_and_hash("correct horse", hash), Ok(true));
        assert_eq!(compare_password_and_hash("correct horses", hash), Ok(false));
    }

    #[test]
    fn verifies_go_hash_with_memory_below_minimum() {
        // From api/integration/clubhousedb_test_seed.sql; the password is )'(
        let hash = "$argon2id$v=19$m=1,t=1,p=1$51uXrZoFRb6O4Tw4TsAJVQ$SedDwp+hPpIJc42QcnFJy6EOtE+b5kyYFpnuRHl/5qs";
        assert_eq!(compare_password_and_hash(")'(", hash), Ok(true));
        assert_eq!(compare_password_and_hash("password", hash), Ok(false));
    }

    #[test]
    fn decode_rejects_other_variants() {
        assert_eq!(
            decode_hash("$argon2i$v=19$m=64,t=1,p=1$c2FsdA$a2V5").unwrap_err(),
            Error::IncompatibleVariant
        );
    }

    #[test]
    fn decode_rejects_other_versions() {
        assert_eq!(
            decode_hash("$argon2id$v=16$m=64,t=1,p=1$c2FsdA$a2V5").unwrap_err(),
            Error::IncompatibleVersion
        );
    }

    #[test]
    fn decode_rejects_malformed() {
        assert_eq!(
            decode_hash("$argon2id$v=19$m=64,t=1,p=1$c2FsdA").unwrap_err(),
            Error::InvalidHash
        );
        assert_eq!(
            decode_hash("$argon2id$v=19$m=64,t=1$c2FsdA$a2V5").unwrap_err(),
            Error::InvalidHash
        );
        assert_eq!(
            decode_hash("$argon2id$v=19$m=64,t=1,p=1$!!$a2V5").unwrap_err(),
            Error::InvalidHash
        );
    }
}
