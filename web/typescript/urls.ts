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

"use strict";

/* eslint-disable @typescript-eslint/no-unused-vars */

const url_root = "/";
const url_prefix = "/ims";
const url_urlsJS = "/ims/urls.js";
const url_api = "/ims/api";
const url_ping = "/ims/api/ping";
const url_bag = "/ims/api/bag";
const url_actionlogs = "/ims/api/actionlogs";
const url_auth = "/ims/api/auth";
const url_authRefresh = "/ims/api/auth/refresh";
const url_acl = "/ims/api/access";
const url_streets = "/ims/api/streets";
const url_personnel = "/ims/api/personnel";
const url_incidentTypes = "/ims/api/incident_types";
const url_events = "/ims/api/events";
const url_event = "/ims/api/events/<event_id>";
const url_incidents = "/ims/api/events/<event_id>/incidents";
const url_incidentNumber = "/ims/api/events/<event_id>/incidents/<incident_number>";
const url_incident_reportEntries = "/ims/api/events/<event_id>/incidents/<incident_number>/report_entries";
const url_incident_reportEntry = "/ims/api/events/<event_id>/incidents/<incident_number>/report_entries/<report_entry_id>";
const url_incidentAttachments = "/ims/api/events/<event_id>/incidents/<incident_number>/attachments";
const url_incidentAttachmentNumber = "/ims/api/events/<event_id>/incidents/<incident_number>/attachments/<attachment_number>";
const url_fieldReports = "/ims/api/events/<event_id>/field_reports";
const url_fieldReport = "/ims/api/events/<event_id>/field_reports/<field_report_number>";
const url_fieldReport_reportEntries = "/ims/api/events/<event_id>/field_reports/<field_report_number>/report_entries";
const url_fieldReport_reportEntry = "/ims/api/events/<event_id>/field_reports/<field_report_number>/report_entries/<report_entry_id>";
const url_fieldReportAttachments = "/ims/api/events/<event_id>/field_reports/<field_report_number>/attachments";
const url_fieldReportAttachmentNumber = "/ims/api/events/<event_id>/field_reports/<field_report_number>/attachments/<attachment_number>";
const url_eventSource = "/ims/api/eventsource";
const url_debugBuildInfo = "/ims/api/debug/buildinfo";
const url_debugRuntimeMetrics = "/ims/api/debug/runtimemetrics";
const url_debugGC = "/ims/api/debug/gc";
const url_static = "/ims/static";
const url_styleSheet = "/ims/static/style.css";
const url_authApp = "/ims/auth";
const url_login = "/ims/auth/login";
const url_loginJS = "/ims/static/login.js";
const url_logout = "/ims/auth/logout";
const url_external = "/ims/ext";
const url_jqueryBase = "/ims/ext/jquery/";
const url_jqueryJS = "/ims/ext/jquery/jquery.min.js";
const url_jqueryMap = "/ims/ext/jquery/jquery.min.map";
const url_bootstrapBase = "/ims/ext/bootstrap";
const url_bootstrapCSS = "/ims/ext/bootstrap/css/bootstrap.min.css";
const url_bootstrapJS = "/ims/ext/bootstrap/js/bootstrap.bundle.min.js";
const url_dataTablesBase = "/ims/ext/datatables";
const url_dataTablesJS = "/ims/ext/datatables/js/dataTables.min.js";
const url_dataTablesBootstrapCSS = "/ims/ext/datatables/css/dataTables.bootstrap5.min.css";
const url_dataTablesBootstrapJS = "/ims/ext/datatables/js/dataTables.bootstrap5.min.js";
const url_dataTablesResponsiveCSS = "/ims/ext/datatables/css/responsive.dataTables.min.css";
const url_dataTablesResponsiveJS = "/ims/ext/datatables/js/dataTables.responsive.min.js";
const url_app = "/ims/app/";
const url_rootJS = "/ims/static/root.js";
const url_imsJS = "/ims/static/ims.js";
const url_themeJS = "/ims/static/theme.js";
const url_admin = "/ims/app/admin";
const url_adminRootJS = "/ims/static/admin_root.js";
const url_adminEvents = "/ims/app/admin/events";
const url_adminEventsJS = "/ims/static/admin_events.js";
const url_adminIncidentTypes = "/ims/app/admin/types";
const url_adminIncidentTypesJS = "/ims/static/admin_types.js";
const url_adminStreets = "/ims/app/admin/streets";
const url_adminStreetsJS = "/ims/static/admin_streets.js";
const url_adminDebug = "/ims/app/admin/debug";
const url_adminDebugJS = "/ims/app/admin/admin_debug.js";
const url_viewEvents = "/ims/app/events";
const url_viewEvent = "/ims/app/events/<event_id>";
const url_viewIncidents = "/ims/app/events/<event_id>/incidents";
const url_viewIncidentsJS = "/ims/static/incidents.js";
const url_viewIncidentsRelative = "incidents";
const url_viewIncidentNumber = "/ims/app/events/<event_id>/incidents/<number>";
const url_viewIncidentJS = "/ims/static/incident.js";
const url_viewFieldReports = "/ims/app/events/<event_id>/field_reports";
const url_viewFieldReportsJS = "/ims/static/field_reports.js";
const url_viewFieldReportsRelative = "field_reports";
const url_viewFieldReportNew = "/ims/app/events/<event_id>/field_reports/new";
const url_viewFieldReportNumber = "/ims/app/events/<event_id>/field_reports/<number>";
const url_viewFieldReportJS = "/ims/static/field_report.js";
