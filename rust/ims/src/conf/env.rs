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

//! Applies the `.env` file and environment variables to a config, reading the
//! same variable names as the Go server's `cmd/serveconfig.go`.

use std::path::{Path, PathBuf};

use super::{AttachmentsStoreType, DBStoreType, DeploymentType, DirectoryType, IMSConfig};
use crate::util::goduration;

pub const DEFAULT_ENV_FILE: &str = ".env";

/// Loads `env_file` into the process environment (without overriding
/// variables that are already set, as with godotenv), then applies the
/// environment to `cfg`.
pub fn apply_env_config(cfg: IMSConfig, env_file: &str) -> Result<IMSConfig, String> {
    match dotenvy::from_path(Path::new(env_file)) {
        Ok(()) => {}
        Err(dotenvy::Error::Io(e)) if e.kind() == std::io::ErrorKind::NotFound => {
            if env_file != DEFAULT_ENV_FILE {
                return Err(format!(
                    "envfile '{env_file}' was set by the caller, but the file was not found"
                ));
            }
            tracing::info!(
                "No .env file found. Carrying on with IMSConfig defaults and environment variable overrides"
            );
        }
        Err(e) => return Err(e.to_string()),
    }
    apply_env(cfg, &|key| lookup_env(std::env::var(key).ok()))
}

/// When doing `docker run --env-file .env`, Docker passes values through with
/// their double-quotes still in place. https://github.com/docker/cli/issues/3630
fn lookup_env(v: Option<String>) -> Option<String> {
    let v = v?;
    if v.len() >= 2 && v.starts_with('"') && v.ends_with('"') {
        return Some(v[1..v.len() - 1].to_string());
    }
    Some(v)
}

fn is_true(v: &str) -> bool {
    v.eq_ignore_ascii_case("true")
}

fn parse_int(key: &str, v: &str) -> Result<i32, String> {
    v.parse().map_err(|e| format!("{key}: {e}"))
}

fn parse_duration(key: &str, v: &str) -> Result<std::time::Duration, String> {
    goduration::parse(v).map_err(|e| format!("{key}: {e}"))
}

fn apply_env(
    mut cfg: IMSConfig,
    env: &dyn Fn(&str) -> Option<String>,
) -> Result<IMSConfig, String> {
    if let Some(v) = env("IMS_HOSTNAME") {
        cfg.core.host = v;
    }
    if let Some(v) = env("IMS_PORT") {
        cfg.core.port = parse_int("IMS_PORT", &v)?;
    }
    if let Some(v) = env("IMS_DEPLOYMENT") {
        cfg.core.deployment = DeploymentType::parse(&v.to_lowercase());
    }
    if let Some(v) = env("IMS_TOKEN_LIFETIME") {
        cfg.core.token_lifetime = parse_duration("IMS_TOKEN_LIFETIME", &v)?;
    }
    if let Some(v) = env("IMS_CACHE_CONTROL_SHORT") {
        cfg.core.cache_control_short = parse_duration("IMS_CACHE_CONTROL_SHORT", &v)?;
    }
    if let Some(v) = env("IMS_DIRECTORY_CACHE_TTL") {
        cfg.directory.in_memory_cache_ttl = parse_duration("IMS_DIRECTORY_CACHE_TTL", &v)?;
    }
    if let Some(v) = env("IMS_CACHE_CONTROL_LONG") {
        cfg.core.cache_control_long = parse_duration("IMS_CACHE_CONTROL_LONG", &v)?;
    }
    if let Some(v) = env("IMS_LOG_LEVEL") {
        cfg.core.log_level = v;
    }
    if let Some(v) = env("IMS_ACTION_LOG_ENABLED") {
        cfg.core.action_log_enabled = is_true(&v);
    }
    if let Some(v) = env("IMS_ERROR_LOG_ENABLED") {
        cfg.core.error_log_enabled = is_true(&v);
    }
    if let Some(v) = env("IMS_EVENT_DELETION_ENABLED") {
        cfg.core.event_deletion_enabled = is_true(&v);
    }
    if let Some(v) = env("IMS_INSECURE_COOKIES") {
        cfg.core.insecure_cookies = is_true(&v);
    }
    if let Some(v) = env("IMS_BM_API_URL") {
        cfg.burning_man_api.url = v.strip_suffix('/').unwrap_or(&v).to_string();
    }
    if let Some(v) = env("IMS_BM_API_KEY") {
        cfg.burning_man_api.api_key = v;
    }
    if let Some(v) = env("IMS_DIRECTORY") {
        cfg.directory.directory = DirectoryType::parse(&v.to_lowercase());
    }
    if let Some(v) = env("IMS_ADMINS") {
        cfg.core.admins = v.split(',').map(str::to_string).collect();
    }
    if let Some(v) = env("IMS_JWT_SECRET") {
        cfg.core.jwt_secret = v;
    }
    if let Some(v) = env("IMS_DB_STORE_TYPE") {
        cfg.store.store_type = DBStoreType::parse(&v.to_lowercase());
    }
    if let Some(v) = env("IMS_DB_HOST_NAME") {
        cfg.store.maria_db.host_name = v;
    }
    if let Some(v) = env("IMS_DB_HOST_PORT") {
        cfg.store.maria_db.host_port = parse_int("IMS_DB_HOST_PORT", &v)?;
    }
    if let Some(v) = env("IMS_DB_DATABASE") {
        cfg.store.maria_db.database = v;
    }
    if let Some(v) = env("IMS_DB_USER_NAME") {
        cfg.store.maria_db.username = v;
    }
    if let Some(v) = env("IMS_DB_PASSWORD") {
        cfg.store.maria_db.password = v;
    }
    if let Some(v) = env("IMS_DMS_HOSTNAME") {
        cfg.directory.clubhouse_db.hostname = v;
    }
    if let Some(v) = env("IMS_DMS_DATABASE") {
        cfg.directory.clubhouse_db.database = v;
    }
    if let Some(v) = env("IMS_DMS_USERNAME") {
        cfg.directory.clubhouse_db.username = v;
    }
    if let Some(v) = env("IMS_DMS_PASSWORD") {
        cfg.directory.clubhouse_db.password = v;
    }
    if let Some(v) = env("IMS_ATTACHMENTS_STORE") {
        cfg.attachments_store.store_type = AttachmentsStoreType::parse(&v);
    }
    if let Some(v) = env("IMS_ATTACHMENTS_LOCAL_DIR") {
        create_private_dir(&v)?;
        cfg.attachments_store.local.dir = Some(PathBuf::from(v));
    }
    // These three AWS env vars use the standard names, hence no "IMS_" prefix.
    if let Some(v) = env("AWS_ACCESS_KEY_ID") {
        cfg.attachments_store.s3.aws_access_key_id = v;
    }
    if let Some(v) = env("AWS_SECRET_ACCESS_KEY") {
        cfg.attachments_store.s3.aws_secret_access_key = v;
    }
    if let Some(v) = env("AWS_REGION") {
        cfg.attachments_store.s3.aws_region = v;
    }
    if let Some(v) = env("IMS_ATTACHMENTS_S3_BUCKET") {
        cfg.attachments_store.s3.bucket = v;
    }
    if let Some(v) = env("IMS_ATTACHMENTS_S3_COMMON_KEY_PREFIX") {
        cfg.attachments_store.s3.common_key_prefix = v;
    }

    // Not in the Go server, which has no env vars for these. The API
    // integration harness uses them to match its in-process config.
    if let Some(v) = env("IMS_MAX_REQUEST_BYTES") {
        cfg.core.max_request_bytes = v
            .parse()
            .map_err(|e| format!("IMS_MAX_REQUEST_BYTES: {e}"))?;
    }
    if let Some(v) = env("IMS_DB_MAX_OPEN_CONNS") {
        cfg.store.maria_db.max_open_conns = parse_int("IMS_DB_MAX_OPEN_CONNS", &v)?;
    }
    if let Some(v) = env("IMS_DMS_MAX_OPEN_CONNS") {
        cfg.directory.clubhouse_db.max_open_conns = parse_int("IMS_DMS_MAX_OPEN_CONNS", &v)?;
    }
    // Not in the Go server either. See conf::TestHooks.
    if let Some(v) = env("IMS_TEST_PANIC_ROUTE") {
        cfg.test_hooks.panic_route = is_true(&v);
    }
    if let Some(v) = env("IMS_TEST_SYNC_LOGS") {
        cfg.test_hooks.sync_logs = is_true(&v);
    }
    if let Some(v) = env("IMS_TEST_QUERY_HOOK_URL") {
        cfg.test_hooks.query_hook_url = v;
    }
    Ok(cfg)
}

fn create_private_dir(dir: &str) -> Result<(), String> {
    let mut builder = std::fs::DirBuilder::new();
    builder.recursive(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::DirBuilderExt;
        builder.mode(0o750);
    }
    builder
        .create(dir)
        .map_err(|e| format!("IMS_ATTACHMENTS_LOCAL_DIR: {e}"))
}

#[cfg(test)]
mod tests {
    use std::collections::HashMap;
    use std::time::Duration;

    use super::*;
    use crate::conf::Setting;

    #[test]
    fn apply_env_sets_everything() {
        let temp_dir = tempfile::tempdir().unwrap();
        let attachments_dir = temp_dir.path().join("attachments");
        let vars: HashMap<&str, String> = HashMap::from([
            ("IMS_HOSTNAME", "host".to_string()),
            ("IMS_PORT", "1234".to_string()),
            ("IMS_DEPLOYMENT", "DEV".to_string()),
            ("IMS_TOKEN_LIFETIME", "1000s".to_string()),
            ("IMS_CACHE_CONTROL_SHORT", "3m".to_string()),
            ("IMS_CACHE_CONTROL_LONG", "7m".to_string()),
            ("IMS_DIRECTORY_CACHE_TTL", "15m".to_string()),
            ("IMS_LOG_LEVEL", "WARN".to_string()),
            ("IMS_ACTION_LOG_ENABLED", "true".to_string()),
            ("IMS_ERROR_LOG_ENABLED", "false".to_string()),
            ("IMS_EVENT_DELETION_ENABLED", "TRUE".to_string()),
            ("IMS_INSECURE_COOKIES", "true".to_string()),
            ("IMS_BM_API_URL", "https://bm.example/".to_string()),
            ("IMS_BM_API_KEY", "bmkey".to_string()),
            ("IMS_DIRECTORY", "ClubhouseDB".to_string()),
            ("IMS_ADMINS", "alice,bob".to_string()),
            ("IMS_JWT_SECRET", "shhh".to_string()),
            ("IMS_DB_HOST_NAME", "db".to_string()),
            ("IMS_DB_STORE_TYPE", "mariadb".to_string()),
            ("IMS_DB_HOST_PORT", "555".to_string()),
            ("IMS_DB_DATABASE", "ims".to_string()),
            ("IMS_DB_USER_NAME", "me".to_string()),
            ("IMS_DB_PASSWORD", "boo".to_string()),
            ("IMS_DMS_HOSTNAME", "db2".to_string()),
            ("IMS_DMS_DATABASE", "rangerz".to_string()),
            ("IMS_DMS_USERNAME", "me2".to_string()),
            ("IMS_DMS_PASSWORD", "woo".to_string()),
            ("IMS_ATTACHMENTS_STORE", "local".to_string()),
            (
                "IMS_ATTACHMENTS_LOCAL_DIR",
                attachments_dir.to_str().unwrap().to_string(),
            ),
            ("AWS_ACCESS_KEY_ID", "my name".to_string()),
            ("AWS_SECRET_ACCESS_KEY", "my key".to_string()),
            ("AWS_REGION", "mars".to_string()),
            ("IMS_ATTACHMENTS_S3_BUCKET", "big-bucket".to_string()),
            (
                "IMS_ATTACHMENTS_S3_COMMON_KEY_PREFIX",
                "safe/dir".to_string(),
            ),
        ]);
        let cfg = apply_env(IMSConfig::default(), &|k| vars.get(k).cloned()).unwrap();

        assert_eq!(cfg.core.host, "host");
        assert_eq!(cfg.core.port, 1234);
        assert_eq!(cfg.core.deployment, Setting::Known(DeploymentType::Dev));
        assert_eq!(cfg.core.token_lifetime, Duration::from_secs(1000));
        assert_eq!(cfg.core.cache_control_short, Duration::from_secs(180));
        assert_eq!(cfg.core.cache_control_long, Duration::from_secs(420));
        assert_eq!(cfg.directory.in_memory_cache_ttl, Duration::from_secs(900));
        assert_eq!(cfg.core.log_level, "WARN");
        assert!(cfg.core.action_log_enabled);
        assert!(!cfg.core.error_log_enabled);
        assert!(cfg.core.event_deletion_enabled);
        assert!(cfg.core.insecure_cookies);
        assert_eq!(cfg.burning_man_api.url, "https://bm.example");
        assert_eq!(cfg.burning_man_api.api_key, "bmkey");
        assert_eq!(
            cfg.directory.directory,
            Setting::Known(DirectoryType::ClubhouseDB)
        );
        assert_eq!(cfg.core.admins, vec!["alice", "bob"]);
        assert_eq!(cfg.core.jwt_secret, "shhh");
        assert_eq!(cfg.store.store_type, Setting::Known(DBStoreType::Maria));
        assert_eq!(cfg.store.maria_db.host_name, "db");
        assert_eq!(cfg.store.maria_db.host_port, 555);
        assert_eq!(cfg.store.maria_db.database, "ims");
        assert_eq!(cfg.store.maria_db.username, "me");
        assert_eq!(cfg.store.maria_db.password, "boo");
        assert_eq!(cfg.directory.clubhouse_db.hostname, "db2");
        assert_eq!(cfg.directory.clubhouse_db.database, "rangerz");
        assert_eq!(cfg.directory.clubhouse_db.username, "me2");
        assert_eq!(cfg.directory.clubhouse_db.password, "woo");
        assert_eq!(
            cfg.attachments_store.store_type,
            Setting::Known(AttachmentsStoreType::Local)
        );
        assert_eq!(
            cfg.attachments_store.local.dir.as_deref(),
            Some(attachments_dir.as_path())
        );
        assert!(attachments_dir.is_dir());
        assert_eq!(cfg.attachments_store.s3.aws_access_key_id, "my name");
        assert_eq!(cfg.attachments_store.s3.aws_secret_access_key, "my key");
        assert_eq!(cfg.attachments_store.s3.aws_region, "mars");
        assert_eq!(cfg.attachments_store.s3.bucket, "big-bucket");
        assert_eq!(cfg.attachments_store.s3.common_key_prefix, "safe/dir");
    }

    #[test]
    fn apply_env_leaves_defaults_alone() {
        let cfg = apply_env(IMSConfig::default(), &|_| None).unwrap();
        assert_eq!(cfg.core.host, "localhost");
        assert_eq!(cfg.core.port, 8080);
        assert_eq!(cfg.core.token_lifetime, Duration::from_secs(8 * 3600));
    }

    #[test]
    fn apply_env_rejects_bad_numbers() {
        let err = apply_env(IMSConfig::default(), &|k| {
            (k == "IMS_PORT").then(|| "eighty".to_string())
        });
        assert!(err.unwrap_err().contains("IMS_PORT"));

        // Durations need a unit, as in Go.
        let err = apply_env(IMSConfig::default(), &|k| {
            (k == "IMS_CACHE_CONTROL_LONG").then(|| "20".to_string())
        });
        assert!(err.unwrap_err().contains("IMS_CACHE_CONTROL_LONG"));
    }

    #[test]
    fn lookup_env_strips_docker_quotes() {
        assert_eq!(
            lookup_env(Some("\"localhost\"".to_string())).as_deref(),
            Some("localhost")
        );
        assert_eq!(
            lookup_env(Some("localhost".to_string())).as_deref(),
            Some("localhost")
        );
        assert_eq!(lookup_env(Some("\"".to_string())).as_deref(), Some("\""));
        assert_eq!(lookup_env(None), None);
    }
}
