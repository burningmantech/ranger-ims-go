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

//! Checks every page the Rust port renders against the Go server's own
//! rendering of the same templ templates, for each kind of deployment.
//! The comparison is of the parsed documents, with whitespace in text
//! normalized, since templ and Askama lay out whitespace differently.

use std::path::{Path, PathBuf};
use std::process::Command;

use ims::web::pages::{self, Page};
use scraper::{Html, Node};

const PAGES: [(Page, &str); 20] = [
    (Page::Login, "login.html"),
    (Page::Root, "root.html"),
    (Page::AdminRoot, "adminroot.html"),
    (Page::AdminActionLogs, "adminactionlogs.html"),
    (Page::AdminErrorLogs, "adminerrorlogs.html"),
    (Page::AdminEvents, "adminevents.html"),
    (Page::AdminTypes, "admintypes.html"),
    (Page::AdminDebug, "admindebug.html"),
    (Page::AdminDirectory, "admindirectory.html"),
    (Page::AdminPlaces, "adminplaces.html"),
    (Page::Settings, "settings.html"),
    (Page::Help, "help.html"),
    (Page::Search, "search.html"),
    (Page::Places, "places.html"),
    (Page::FieldReports, "field_reports.html"),
    (Page::FieldReport, "field_report.html"),
    (Page::Incidents, "incidents.html"),
    (Page::Incident, "incident.html"),
    (Page::SanctuaryVisits, "sanctuary_visits.html"),
    (Page::SanctuaryVisit, "sanctuary_visit.html"),
];

fn repo_root() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("../..")
        .canonicalize()
        .unwrap()
}

fn collapse(s: &str) -> String {
    s.split_whitespace().collect::<Vec<_>>().join(" ")
}

/// A canonical rendering of the document tree.
fn canonical(html: &str) -> Vec<String> {
    let doc = Html::parse_document(html);
    let mut out = Vec::new();
    for edge in doc.root_element().traverse() {
        match edge {
            ego_tree::iter::Edge::Open(node) => match node.value() {
                Node::Element(e) => {
                    let attrs: Vec<String> = e
                        .attrs()
                        .map(|(k, v)| format!("{k}={:?}", collapse(v)))
                        .collect();
                    out.push(format!("<{} {}>", e.name(), attrs.join(" ")));
                }
                Node::Text(t) => {
                    let t = collapse(t);
                    if !t.is_empty() {
                        out.push(format!("text:{t}"));
                    }
                }
                Node::Comment(c) => out.push(format!("comment:{}", collapse(c))),
                _ => {}
            },
            ego_tree::iter::Edge::Close(node) => {
                if let Node::Element(e) = node.value() {
                    out.push(format!("</{}>", e.name()));
                }
            }
        }
    }
    out
}

fn render_golden(deployment: &str, event: &str) -> PathBuf {
    let tag: String = event.chars().filter(char::is_ascii_alphanumeric).collect();
    let out = std::env::temp_dir().join(format!(
        "ims-golden-{}-{deployment}-{tag}",
        std::process::id()
    ));
    let status = Command::new("go")
        .args(["run", "./rust/tools/rendergolden", "-out"])
        .arg(&out)
        .args(["-deployment", deployment, "-event", event])
        .current_dir(repo_root())
        .status()
        .expect("running go; the Go toolchain is needed for this test");
    assert!(status.success(), "rendergolden failed");
    out
}

fn check_deployment(deployment: &str) {
    check(deployment, "2025");
}

fn check(deployment: &str, event: &str) {
    let golden_dir = render_golden(deployment, event);
    for (page, file) in PAGES {
        let golden = std::fs::read_to_string(golden_dir.join(file)).unwrap();
        let ours =
            pages::render(page, deployment, "test-version", "0123456789abcdef", event).unwrap();
        let (g, o) = (canonical(&golden), canonical(&ours));
        if let Some(i) = (0..g.len().max(o.len())).find(|&i| g.get(i) != o.get(i)) {
            let lo = i.saturating_sub(3);
            panic!(
                "{file} ({deployment}) differs at node {i}:\n  go:   {:?}\n  rust: {:?}",
                &g[lo..(i + 3).min(g.len())],
                &o[lo..(i + 3).min(o.len())]
            );
        }
    }
    let _ = std::fs::remove_dir_all(golden_dir);
}

#[test]
fn pages_match_go_for_dev() {
    check_deployment("dev");
}

#[test]
fn pages_match_go_for_production() {
    check_deployment("production");
}

#[test]
fn pages_match_go_for_training() {
    check_deployment("training");
}

#[test]
fn event_names_are_escaped_like_go() {
    // An event name comes straight from the URL path.
    check("dev", r#"<b>"Tom" & 'Jerry'</b>"#);
}

#[test]
fn pages_match_go_for_staging() {
    check_deployment("staging");
}
