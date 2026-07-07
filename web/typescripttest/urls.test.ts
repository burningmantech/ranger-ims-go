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

// Tests for urls.ts. In production urls.js is loaded as a classic script whose
// top-level "const url_*" declarations become page-wide globals; setup.ts
// reproduces that by installing them as actual globals (see setup.ts). These
// tests sanity-check that mechanism and the URL definitions it exposes.

import { expect, test } from "vitest";

test("urls.ts loads as a classic script without throwing", async (): Promise<void> => {
    // @ts-expect-error TS2306: urls.ts is intentionally not a module; importing
    // it just executes the declarations, like the browser does.
    await expect(import("../typescript/urls.ts")).resolves.toBeDefined();
});

test("representative API URLs are installed as globals with their expected values", (): void => {
    expect(url_auth).toBe("/ims/api/auth");
    expect(url_events).toBe("/ims/api/events");
    expect(url_acl).toBe("/ims/api/access");
    expect(url_logout).toBe("/ims/auth/logout");
    expect(url_app).toBe("/ims/app/");
});

test("every API URL is rooted under the IMS prefix", (): void => {
    const apiUrls = [
        url_auth, url_authRefresh, url_acl, url_accessTargets,
        url_personnel, url_incidentTypes, url_events, url_incidents,
        url_fieldReports, url_visits, url_places, url_eventSource,
    ];
    for (const url of apiUrls) {
        expect(url.startsWith("/ims/")).toBe(true);
    }
});

test("templated URLs carry the placeholders the page code substitutes", (): void => {
    expect(url_event).toContain("<event_id>");
    expect(url_incidentNumber).toContain("<event_id>");
    expect(url_incidentNumber).toContain("<incident_number>");
    expect(url_fieldReport).toContain("<field_report_number>");
    expect(url_visitNumber).toContain("<visit_number>");
    expect(url_incidentRanger).toContain("<ranger_name>");
});
