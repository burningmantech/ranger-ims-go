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

//! The server-rendered pages.

use askama::Template;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Page {
    Login,
    Root,
    AdminRoot,
    AdminActionLogs,
    AdminErrorLogs,
    AdminEvents,
    AdminTypes,
    AdminDebug,
    AdminDirectory,
    AdminPlaces,
    Settings,
    Help,
    Search,
    Places,
    FieldReports,
    FieldReport,
    Incidents,
    Incident,
    SanctuaryVisits,
    SanctuaryVisit,
}

/// Every page takes the same parameters; those without an event ignore it.
macro_rules! pages {
    ($($variant:ident => $file:literal),+ $(,)?) => {
        mod tmpl {
            use askama::Template;
            use super::filters;
            $(
                #[derive(Template)]
                #[template(path = $file)]
                #[allow(dead_code)]
                pub struct $variant<'a> {
                    pub deployment: &'a str,
                    pub version_name: &'a str,
                    pub version_ref: &'a str,
                    pub event_name: &'a str,
                }
            )+
        }

        pub fn render(page: Page, deployment: &str, version_name: &str, version_ref: &str, event_name: &str) -> askama::Result<String> {
            match page {
                $(Page::$variant => tmpl::$variant { deployment, version_name, version_ref, event_name }.render(),)+
            }
        }
    };
}

pages! {
    Login => "login.html",
    Root => "root.html",
    AdminRoot => "adminroot.html",
    AdminActionLogs => "adminactionlogs.html",
    AdminErrorLogs => "adminerrorlogs.html",
    AdminEvents => "adminevents.html",
    AdminTypes => "admintypes.html",
    AdminDebug => "admindebug.html",
    AdminDirectory => "admindirectory.html",
    AdminPlaces => "adminplaces.html",
    Settings => "settings.html",
    Help => "help.html",
    Search => "search.html",
    Places => "places.html",
    FieldReports => "field_reports.html",
    FieldReport => "field_report.html",
    Incidents => "incidents.html",
    Incident => "incident.html",
    SanctuaryVisits => "sanctuary_visits.html",
    SanctuaryVisit => "sanctuary_visit.html",
}

pub mod filters {
    /// The first 8 characters of the version ref, used to bust caches.
    pub fn short_version<T: std::fmt::Display>(
        v: T,
        _: &dyn askama::Values,
    ) -> askama::Result<String> {
        Ok(v.to_string().chars().take(8).collect())
    }
}

/// e.g. "20250629122355" from the build, or the time the server started.
pub fn version_name() -> String {
    static NAME: std::sync::LazyLock<String> = std::sync::LazyLock::new(|| {
        option_env!("IMS_VERSION_NAME")
            .map(str::to_string)
            .unwrap_or_else(|| chrono::Utc::now().format("%Y%m%d%H%M%S").to_string())
    });
    NAME.clone()
}

/// The git commit the server was built from, or a placeholder.
pub fn version_ref() -> String {
    option_env!("IMS_GIT_REVISION")
        .unwrap_or("deadbeef")
        .to_string()
}
