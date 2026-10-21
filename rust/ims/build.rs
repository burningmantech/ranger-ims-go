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

//! Embeds the git revision and commit time, which the Go server gets from
//! its BuildInfo: the pages use them for cache-busting and the footer, and
//! the debug endpoint reports them.

use std::process::Command;

fn git(args: &[&str]) -> Option<String> {
    let out = Command::new("git")
        .args(args)
        .env("TZ", "UTC")
        .output()
        .ok()?;
    out.status
        .success()
        .then(|| String::from_utf8_lossy(&out.stdout).trim().to_string())
}

fn main() {
    println!("cargo:rerun-if-changed=../../.git/HEAD");
    println!("cargo:rerun-if-changed=../../.git/refs");
    println!("cargo:rerun-if-env-changed=IMS_GIT_REVISION");
    println!("cargo:rerun-if-env-changed=IMS_VERSION_NAME");
    // A Docker build has no .git to ask, so it passes these in instead.
    let revision = std::env::var("IMS_GIT_REVISION")
        .ok()
        .or_else(|| git(&["rev-parse", "HEAD"]));
    // Like the tail of Go's pseudo-version, e.g. "20250629122355-7254ff315bc4".
    let version = std::env::var("IMS_VERSION_NAME").ok().or_else(|| {
        let time = git(&[
            "show",
            "-s",
            "--format=%cd",
            "--date=format-local:%Y%m%d%H%M%S",
            "HEAD",
        ])?;
        let rev = revision.clone()?;
        Some(format!("{time}-{}", &rev[..rev.len().min(12)]))
    });
    if let Some(r) = revision.filter(|r| !r.is_empty()) {
        println!("cargo:rustc-env=IMS_GIT_REVISION={r}");
    }
    if let Some(v) = version.filter(|v| !v.is_empty()) {
        println!("cargo:rustc-env=IMS_VERSION_NAME={v}");
    }
    if let Some(v) = Command::new(std::env::var("RUSTC").unwrap_or_else(|_| "rustc".to_string()))
        .arg("--version")
        .output()
        .ok()
        .filter(|o| o.status.success())
    {
        println!(
            "cargo:rustc-env=IMS_RUSTC_VERSION={}",
            String::from_utf8_lossy(&v.stdout).trim()
        );
    }
}
