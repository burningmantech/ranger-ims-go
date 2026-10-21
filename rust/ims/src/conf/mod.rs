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

//! Server configuration, ported from the Go server's `conf` package. The env
//! vars that fill it in are in `env.rs`.

mod env;

pub use env::DEFAULT_ENV_FILE;

pub use env::apply_env_config;

use std::fmt::Write as _;
use std::path::PathBuf;
use std::time::Duration;

use crate::util::goduration;

const MIB: i64 = 1 << 20;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DirectoryType {
    ClubhouseDB,
    Ims,
    NoOp,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AttachmentsStoreType {
    Local,
    S3,
    None,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DeploymentType {
    Dev,
    Training,
    Staging,
    Production,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DBStoreType {
    Maria,
    NoOp,
}

/// A config value as given, which may not name a known variant. Keeping the
/// raw string lets `validate` report it, the same way the Go server's typed
/// strings do.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Setting<T> {
    Known(T),
    Unknown(String),
}

impl<T: Copy> Setting<T> {
    pub fn get(&self) -> Option<T> {
        match self {
            Setting::Known(t) => Some(*t),
            Setting::Unknown(_) => None,
        }
    }

    pub fn is(&self, t: T) -> bool
    where
        T: PartialEq,
    {
        self.get() == Some(t)
    }
}

impl DirectoryType {
    pub fn as_str(self) -> &'static str {
        match self {
            DirectoryType::ClubhouseDB => "clubhousedb",
            DirectoryType::Ims => "ims",
            DirectoryType::NoOp => "noop",
        }
    }

    pub fn parse(s: &str) -> Setting<Self> {
        match s {
            "clubhousedb" => Setting::Known(DirectoryType::ClubhouseDB),
            "ims" => Setting::Known(DirectoryType::Ims),
            "noop" => Setting::Known(DirectoryType::NoOp),
            other => Setting::Unknown(other.to_string()),
        }
    }
}

impl AttachmentsStoreType {
    pub fn as_str(self) -> &'static str {
        match self {
            AttachmentsStoreType::Local => "local",
            AttachmentsStoreType::S3 => "s3",
            AttachmentsStoreType::None => "none",
        }
    }

    pub fn parse(s: &str) -> Setting<Self> {
        match s {
            "local" => Setting::Known(AttachmentsStoreType::Local),
            "s3" => Setting::Known(AttachmentsStoreType::S3),
            "none" => Setting::Known(AttachmentsStoreType::None),
            other => Setting::Unknown(other.to_string()),
        }
    }
}

impl DeploymentType {
    pub fn as_str(self) -> &'static str {
        match self {
            DeploymentType::Dev => "dev",
            DeploymentType::Training => "training",
            DeploymentType::Staging => "staging",
            DeploymentType::Production => "production",
        }
    }

    pub fn parse(s: &str) -> Setting<Self> {
        match s {
            "dev" => Setting::Known(DeploymentType::Dev),
            "training" => Setting::Known(DeploymentType::Training),
            "staging" => Setting::Known(DeploymentType::Staging),
            "production" => Setting::Known(DeploymentType::Production),
            other => Setting::Unknown(other.to_string()),
        }
    }
}

impl DBStoreType {
    pub fn as_str(self) -> &'static str {
        match self {
            DBStoreType::Maria => "mariadb",
            DBStoreType::NoOp => "noop",
        }
    }

    pub fn parse(s: &str) -> Setting<Self> {
        match s {
            "mariadb" => Setting::Known(DBStoreType::Maria),
            "noop" => Setting::Known(DBStoreType::NoOp),
            other => Setting::Unknown(other.to_string()),
        }
    }
}

#[derive(Debug, Clone)]
pub struct IMSConfig {
    pub core: ConfigCore,
    pub attachments_store: AttachmentsStore,
    pub store: DBStore,
    pub directory: Directory,
    pub burning_man_api: BurningManAPI,
    pub test_hooks: TestHooks,
}

#[derive(Debug, Clone)]
pub struct ConfigCore {
    pub host: String,
    pub port: i32,
    /// How long a login lasts, from the moment of logging in.
    pub token_lifetime: Duration,
    pub admins: Vec<String>,
    pub jwt_secret: String,
    pub deployment: Setting<DeploymentType>,
    pub cache_control_short: Duration,
    pub cache_control_long: Duration,
    /// One of DEBUG, INFO, WARN, or ERROR.
    pub log_level: String,
    pub max_request_bytes: i64,
    pub action_log_enabled: bool,
    pub error_log_enabled: bool,
    pub event_deletion_enabled: bool,
    pub insecure_cookies: bool,
}

#[derive(Debug, Clone)]
pub struct BurningManAPI {
    /// The API's base URL, without a trailing slash.
    pub url: String,
    pub api_key: String,
}

impl BurningManAPI {
    pub fn enabled(&self) -> bool {
        !self.url.is_empty() && !self.api_key.is_empty()
    }
}

#[derive(Debug, Clone)]
pub struct DBStore {
    pub store_type: Setting<DBStoreType>,
    pub maria_db: DBStoreMaria,
}

#[derive(Debug, Clone, Default)]
pub struct DBStoreMaria {
    pub host_name: String,
    pub host_port: i32,
    pub database: String,
    pub username: String,
    pub password: String,
    pub max_open_conns: i32,
}

#[derive(Debug, Clone)]
pub struct Directory {
    pub directory: Setting<DirectoryType>,
    pub clubhouse_db: ClubhouseDB,
    pub in_memory_cache_ttl: Duration,
}

#[derive(Debug, Clone, Default)]
pub struct ClubhouseDB {
    pub hostname: String,
    pub database: String,
    pub username: String,
    pub password: String,
    pub max_open_conns: i32,
}

#[derive(Debug, Clone)]
pub struct AttachmentsStore {
    pub store_type: Setting<AttachmentsStoreType>,
    pub local: LocalAttachments,
    pub s3: S3Attachments,
}

#[derive(Debug, Clone, Default)]
pub struct LocalAttachments {
    pub dir: Option<PathBuf>,
}

#[derive(Debug, Clone, Default)]
pub struct S3Attachments {
    pub aws_access_key_id: String,
    pub aws_secret_access_key: String,
    pub aws_region: String,
    pub bucket: String,
    pub common_key_prefix: String,
}

/// Switches that exist only so that the Go API integration suite can drive
/// this server the way it drives an in-process Go one. `validate` refuses
/// them outside a dev deployment.
#[derive(Debug, Clone, Default)]
pub struct TestHooks {
    /// Mounts a route that always panics, for the error log tests.
    pub panic_route: bool,
    /// Writes action and error log rows before responding, rather than from
    /// a background queue, so tests can read them back straight away.
    pub sync_logs: bool,
    /// See store::testhook.
    pub query_hook_url: String,
}

impl Default for IMSConfig {
    fn default() -> Self {
        IMSConfig {
            core: ConfigCore {
                host: "localhost".to_string(),
                port: 8080,
                token_lifetime: Duration::from_secs(8 * 3600),
                admins: Vec::new(),
                jwt_secret: random_text(),
                deployment: Setting::Known(DeploymentType::Dev),
                cache_control_short: Duration::from_secs(20 * 60),
                cache_control_long: Duration::from_secs(2 * 3600),
                log_level: "INFO".to_string(),
                max_request_bytes: 100 * MIB,
                action_log_enabled: true,
                error_log_enabled: true,
                event_deletion_enabled: false,
                insecure_cookies: false,
            },
            store: DBStore {
                store_type: Setting::Known(DBStoreType::Maria),
                maria_db: DBStoreMaria {
                    host_name: "localhost".to_string(),
                    host_port: 3306,
                    database: "ims".to_string(),
                    // MariaDB errors out if the server hits it with too many parallel requests.
                    max_open_conns: 20,
                    ..Default::default()
                },
            },
            directory: Directory {
                directory: Setting::Known(DirectoryType::ClubhouseDB),
                clubhouse_db: ClubhouseDB {
                    hostname: "localhost:3306".to_string(),
                    database: "rangers".to_string(),
                    ..Default::default()
                },
                in_memory_cache_ttl: Duration::from_secs(5 * 60),
            },
            attachments_store: AttachmentsStore {
                store_type: Setting::Known(AttachmentsStoreType::None),
                local: LocalAttachments::default(),
                s3: S3Attachments::default(),
            },
            burning_man_api: BurningManAPI {
                url: "https://api.burningman.org".to_string(),
                api_key: String::new(),
            },
            test_hooks: TestHooks::default(),
        }
    }
}

/// Like Go's `crypto/rand.Text`: 26 base32 characters, i.e. 128+ bits.
fn random_text() -> String {
    use rand::Rng;
    const ALPHABET: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
    let mut rng = rand::rng();
    (0..26)
        .map(|_| ALPHABET[rng.random_range(0..ALPHABET.len())] as char)
        .collect()
}

impl ConfigCore {
    pub fn deployment_str(&self) -> &str {
        match &self.deployment {
            Setting::Known(d) => d.as_str(),
            Setting::Unknown(s) => s,
        }
    }
}

impl IMSConfig {
    /// Should be called once the config is fully populated. Like the Go
    /// version, it also clears out sub-configs that the chosen types don't use.
    pub fn validate(&mut self) -> Result<(), String> {
        let mut errs: Vec<String> = Vec::new();

        match &self.store.store_type {
            Setting::Known(DBStoreType::Maria) => {}
            Setting::Known(_) => self.store.maria_db = DBStoreMaria::default(),
            Setting::Unknown(s) => {
                errs.push(format!("unknown DB store type {s}"));
                self.store.maria_db = DBStoreMaria::default();
            }
        }

        match &self.directory.directory {
            Setting::Known(DirectoryType::ClubhouseDB) => {}
            Setting::Known(_) => self.directory.clubhouse_db = ClubhouseDB::default(),
            Setting::Unknown(s) => {
                errs.push(format!("unknown directory type {s}"));
                self.directory.clubhouse_db = ClubhouseDB::default();
            }
        }

        if let Setting::Unknown(s) = &self.core.deployment {
            errs.push(format!("unknown deployment type {s}"));
        }
        if !self.core.deployment.is(DeploymentType::Dev) {
            if self.core.insecure_cookies {
                errs.push("non-dev environments must not use insecure cookies".to_string());
            }
            if !self.directory.directory.is(DirectoryType::ClubhouseDB)
                && !self.directory.directory.is(DirectoryType::Ims)
            {
                errs.push(
                    "non-dev environments must use a ClubhouseDB or IMS directory".to_string(),
                );
            }
            if !self.store.store_type.is(DBStoreType::Maria) {
                errs.push("non-dev environments must use a MariaDB datastore".to_string());
            }
            if self.test_hooks.panic_route
                || self.test_hooks.sync_logs
                || !self.test_hooks.query_hook_url.is_empty()
            {
                errs.push("non-dev environments must not enable test hooks".to_string());
            }
        }

        match &self.attachments_store.store_type {
            Setting::Known(AttachmentsStoreType::Local) => {
                if self.attachments_store.local.dir.is_none() {
                    errs.push("local attachments store requires a local directory".to_string());
                }
                self.attachments_store.s3 = S3Attachments::default();
            }
            Setting::Known(AttachmentsStoreType::S3) => {
                let s3 = &self.attachments_store.s3;
                if s3.aws_region.is_empty() || s3.bucket.is_empty() {
                    errs.push(
                        "s3 attachments store requires Default AWSRegion and Bucket".to_string(),
                    );
                }
                self.attachments_store.local = LocalAttachments::default();
            }
            Setting::Known(AttachmentsStoreType::None) => {}
            Setting::Unknown(s) => errs.push(format!("unknown attachments store type {s}")),
        }

        if self.core.token_lifetime.is_zero() {
            errs.push("token lifetime must be positive".to_string());
        }

        if errs.is_empty() {
            Ok(())
        } else {
            Err(errs.join("\n"))
        }
    }

    /// A human-readable dump of the config, with secrets blanked out. The
    /// layout follows the Go server's reflection-based `lib/redact` output.
    pub fn print_redacted(&self) -> String {
        const R: &str = "🤐🤐🤐";
        let mut w = String::new();
        let c = &self.core;
        let _ = writeln!(w, "Core");
        let _ = writeln!(w, "    Host = {}", c.host);
        let _ = writeln!(w, "    Port = {}", c.port);
        let _ = writeln!(
            w,
            "    TokenLifetime = {}",
            goduration::format(c.token_lifetime)
        );
        let _ = writeln!(w, "    Admins = [{}]", c.admins.join(" "));
        let _ = writeln!(w, "    JWTSecret = {R}");
        let _ = writeln!(
            w,
            "    Deployment = {}",
            setting_str(&c.deployment, DeploymentType::as_str)
        );
        let _ = writeln!(
            w,
            "    CacheControlShort = {}",
            goduration::format(c.cache_control_short)
        );
        let _ = writeln!(
            w,
            "    CacheControlLong = {}",
            goduration::format(c.cache_control_long)
        );
        let _ = writeln!(w, "    LogLevel = {}", c.log_level);
        let _ = writeln!(w, "    MaxRequestBytes = {}", c.max_request_bytes);
        let _ = writeln!(w, "    ActionLogEnabled = {}", c.action_log_enabled);
        let _ = writeln!(w, "    ErrorLogEnabled = {}", c.error_log_enabled);
        let _ = writeln!(w, "    EventDeletionEnabled = {}", c.event_deletion_enabled);
        let _ = writeln!(w, "    InsecureCookies = {}", c.insecure_cookies);

        let a = &self.attachments_store;
        let _ = writeln!(w, "AttachmentsStore");
        let _ = writeln!(
            w,
            "    Type = {}",
            setting_str(&a.store_type, AttachmentsStoreType::as_str)
        );
        match &a.local.dir {
            Some(dir) => {
                let _ = writeln!(w, "    Local");
                let _ = writeln!(w, "        Dir = {}", dir.display());
            }
            None => {
                let _ = writeln!(w, "    Local is zero value");
            }
        }
        let s3 = &a.s3;
        if s3.aws_access_key_id.is_empty()
            && s3.aws_secret_access_key.is_empty()
            && s3.aws_region.is_empty()
            && s3.bucket.is_empty()
            && s3.common_key_prefix.is_empty()
        {
            let _ = writeln!(w, "    S3 is zero value");
        } else {
            let _ = writeln!(w, "    S3");
            let _ = writeln!(w, "        AWSAccessKeyID = {}", s3.aws_access_key_id);
            let _ = writeln!(w, "        AWSSecretAccessKey = {R}");
            let _ = writeln!(w, "        AWSRegion = {}", s3.aws_region);
            let _ = writeln!(w, "        Bucket = {}", s3.bucket);
            let _ = writeln!(w, "        CommonKeyPrefix = {}", s3.common_key_prefix);
        }

        let s = &self.store;
        let _ = writeln!(w, "Store");
        let _ = writeln!(
            w,
            "    Type = {}",
            setting_str(&s.store_type, DBStoreType::as_str)
        );
        let _ = writeln!(w, "    MariaDB");
        let _ = writeln!(w, "        HostName = {}", s.maria_db.host_name);
        let _ = writeln!(w, "        HostPort = {}", s.maria_db.host_port);
        let _ = writeln!(w, "        Database = {}", s.maria_db.database);
        let _ = writeln!(w, "        Username = {}", s.maria_db.username);
        let _ = writeln!(w, "        Password = {R}");
        let _ = writeln!(w, "        MaxOpenConns = {}", s.maria_db.max_open_conns);

        let d = &self.directory;
        let _ = writeln!(w, "Directory");
        let _ = writeln!(
            w,
            "    Directory = {}",
            setting_str(&d.directory, DirectoryType::as_str)
        );
        let _ = writeln!(w, "    ClubhouseDB");
        let _ = writeln!(w, "        Hostname = {}", d.clubhouse_db.hostname);
        let _ = writeln!(w, "        Database = {}", d.clubhouse_db.database);
        let _ = writeln!(w, "        Username = {}", d.clubhouse_db.username);
        let _ = writeln!(w, "        Password = {R}");
        let _ = writeln!(
            w,
            "        MaxOpenConns = {}",
            d.clubhouse_db.max_open_conns
        );
        let _ = writeln!(
            w,
            "    InMemoryCacheTTL = {}",
            goduration::format(d.in_memory_cache_ttl)
        );

        let b = &self.burning_man_api;
        let _ = writeln!(w, "BurningManAPI");
        let _ = writeln!(w, "    URL = {}", b.url);
        let _ = writeln!(w, "    APIKey = {R}");
        w
    }
}

fn setting_str<T: Copy>(s: &Setting<T>, as_str: fn(T) -> &'static str) -> String {
    match s {
        Setting::Known(t) => as_str(*t).to_string(),
        Setting::Unknown(raw) => raw.clone(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn print_redacted() {
        let mut cfg = IMSConfig::default();
        cfg.core.admins = vec!["admin user".to_string()];
        cfg.store.maria_db.username = "db username".to_string();
        cfg.store.maria_db.password = "db password".to_string();
        cfg.directory.clubhouse_db.username = "clubhouse username".to_string();
        cfg.directory.clubhouse_db.password = "clubhouse password".to_string();
        cfg.burning_man_api.api_key = "bm key".to_string();
        let jwt_secret = cfg.core.jwt_secret.clone();

        let redacted = cfg.print_redacted();
        assert!(redacted.contains("admin user"));
        assert!(redacted.contains("db username"));
        assert!(!redacted.contains("db password"));
        assert!(redacted.contains("clubhouse username"));
        assert!(!redacted.contains("clubhouse password"));
        assert!(!redacted.contains("bm key"));
        assert!(!redacted.contains(&jwt_secret));
    }

    #[test]
    fn validate_base() {
        let mut cfg = IMSConfig::default();
        assert!(cfg.validate().is_ok());

        cfg.core.token_lifetime = Duration::ZERO;
        assert!(cfg.validate().is_err());
    }

    #[test]
    fn validate_insecure_cookies() {
        // A dev deployment may use insecure cookies
        let mut cfg = IMSConfig::default();
        cfg.core.insecure_cookies = true;
        assert!(cfg.validate().is_ok());

        // Any other may not
        let mut cfg = IMSConfig::default();
        cfg.core.deployment = Setting::Known(DeploymentType::Production);
        cfg.directory.directory = Setting::Known(DirectoryType::Ims);
        cfg.core.insecure_cookies = true;
        assert!(cfg.validate().unwrap_err().contains("insecure cookies"));

        cfg.core.insecure_cookies = false;
        assert!(cfg.validate().is_ok());
    }

    #[test]
    fn validate_test_hooks() {
        let mut cfg = IMSConfig::default();
        cfg.test_hooks.panic_route = true;
        cfg.test_hooks.sync_logs = true;
        assert!(cfg.validate().is_ok());

        let mut cfg = IMSConfig::default();
        cfg.core.deployment = Setting::Known(DeploymentType::Staging);
        cfg.test_hooks.sync_logs = true;
        assert!(cfg.validate().unwrap_err().contains("test hooks"));
    }

    #[test]
    fn validate_db_store() {
        let mut cfg = IMSConfig::default();
        cfg.store.store_type = DBStoreType::parse("invalid type");
        assert!(cfg.validate().is_err());
    }

    #[test]
    fn validate_directory() {
        let mut cfg = IMSConfig::default();
        cfg.directory.directory = DirectoryType::parse("invalid type");
        assert!(cfg.validate().is_err());
    }

    #[test]
    fn validate_non_dev_deployment() {
        let mut cfg = IMSConfig::default();
        cfg.core.deployment = DeploymentType::parse("not a valid deployment");
        assert!(cfg.validate().is_err());

        // non-dev deployment requires a real directory
        let mut cfg = IMSConfig::default();
        cfg.core.deployment = Setting::Known(DeploymentType::Production);
        cfg.directory.directory = Setting::Known(DirectoryType::NoOp);
        assert!(cfg.validate().is_err());

        // non-dev deployment requires MariaDB store
        let mut cfg = IMSConfig::default();
        cfg.core.deployment = Setting::Known(DeploymentType::Production);
        cfg.store.store_type = Setting::Known(DBStoreType::NoOp);
        assert!(cfg.validate().is_err());
    }

    #[test]
    fn validate_attachments_store() {
        let mut cfg = IMSConfig::default();
        cfg.attachments_store.store_type = Setting::Known(AttachmentsStoreType::S3);
        // this will ultimately be ignored
        cfg.attachments_store.local.dir = Some(PathBuf::from("/tmp"));
        cfg.attachments_store.s3 = S3Attachments {
            aws_access_key_id: "abc".to_string(),
            aws_secret_access_key: "def".to_string(),
            aws_region: "there".to_string(),
            bucket: "buck".to_string(),
            common_key_prefix: "a/b".to_string(),
        };
        assert!(cfg.validate().is_ok());
        assert!(cfg.attachments_store.local.dir.is_none());

        // local attachments store requires a local dir to be set
        let mut cfg = IMSConfig::default();
        cfg.attachments_store.store_type = Setting::Known(AttachmentsStoreType::Local);
        assert!(cfg.validate().is_err());

        let mut cfg = IMSConfig::default();
        cfg.attachments_store.store_type = AttachmentsStoreType::parse("invalid type");
        assert!(cfg.validate().is_err());
    }
}
