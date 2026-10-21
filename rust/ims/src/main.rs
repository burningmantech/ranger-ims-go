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

use clap::{Parser, Subcommand};

use ims::cmd;
use ims::conf::{self, IMSConfig};

#[derive(Parser)]
#[command(name = "ims", about = "The Black Rock Ranger IMS server (Rust port)")]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    /// Launch the IMS server. Configuration will be read from .env, and can be
    /// overridden by environment variables.
    Serve {
        /// An env file from which to load IMS server configuration.
        #[arg(long, default_value = conf::DEFAULT_ENV_FILE)]
        envfile: String,
        /// Whether to print the redacted IMSConfig on server startup.
        #[arg(long = "print-config", default_value_t = true, action = clap::ArgAction::Set)]
        print_config: bool,
    },
    /// Perform a health check against an IMS server.
    Healthcheck {
        /// The server URL and port of an IMS server.
        #[arg(long = "server_url")]
        server_url: String,
    },
    /// Add or update a user in the IMS-native directory. This is for
    /// deployments with IMS_DIRECTORY=ims, mainly to bootstrap the first
    /// admin: create the user here, then add their handle to IMS_ADMINS. An
    /// existing user gets the new password (and email, if given) and is
    /// marked active.
    #[command(name = "add-user")]
    AddUser {
        /// An env file from which to load IMS server configuration.
        #[arg(long, default_value = conf::DEFAULT_ENV_FILE)]
        envfile: String,
        /// The user's handle, i.e. the name they use to log in.
        #[arg(long)]
        handle: String,
        /// The user's email address (optional; may also be used to log in).
        #[arg(long, default_value = "")]
        email: String,
        /// Mark the user as onsite (relevant only to 'onsite' validity access rules).
        #[arg(long, num_args = 0..=1, default_missing_value = "true", action = clap::ArgAction::Set)]
        onsite: Option<bool>,
        /// Read the password from stdin rather than prompting for it.
        #[arg(long = "password-stdin")]
        password_stdin: bool,
    },
    /// Get a salted hash of a password.
    #[command(name = "hash_password")]
    HashPassword {
        /// The password to hash.
        #[arg(long)]
        password: String,
    },
}

#[tokio::main]
async fn main() {
    let code = match Cli::parse().command {
        Command::Serve {
            envfile,
            print_config,
        } => {
            let cfg = conf::apply_env_config(IMSConfig::default(), &envfile)
                .unwrap_or_else(|e| panic!("got a startup error: {e}"));
            cmd::serve::run(cfg, print_config).await
        }
        Command::AddUser {
            envfile,
            handle,
            email,
            onsite,
            password_stdin,
        } => {
            let cfg = conf::apply_env_config(IMSConfig::default(), &envfile)
                .unwrap_or_else(|e| panic!("got a startup error: {e}"));
            let args = cmd::adduser::Args {
                handle,
                email,
                onsite,
                password_stdin,
            };
            match cmd::adduser::run(cfg, args).await {
                Ok(()) => 0,
                Err(e) => {
                    eprintln!("Error: {e}");
                    1
                }
            }
        }
        Command::Healthcheck { server_url } => cmd::healthcheck::run(&server_url).await,
        Command::HashPassword { password } => {
            cmd::hashpassword::run(&password);
            0
        }
    };
    std::process::exit(code);
}
