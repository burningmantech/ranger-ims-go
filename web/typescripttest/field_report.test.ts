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

// Tests for field_report.ts against the real templ-rendered field report page
// (field_report.templ).

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

const eventName = "2025";
const eventId = 1;
const frUrl = `/ims/api/events/${eventName}/field_reports`;

let serverEventAccess: ims.AuthInfoEventAccess;
let serverFieldReport: ims.FieldReport;
let serverEvents: ims.EventData[];

beforeEach((): void => {
    vi.resetModules();
    loadFixture("field_report.html");
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/field_reports/7`);

    // The event source lock needs a secure context and the Web Locks API, which
    // happy-dom doesn't provide; park the request so init doesn't loop. (Same
    // approach as incident.test.ts.)
    vi.stubGlobal("isSecureContext", true);
    Object.defineProperty(navigator, "locks", {
        configurable: true,
        value: { request: (): Promise<undefined> => new Promise<undefined>((): void => {}) },
    });

    serverEventAccess = {
        event_id: eventId,
        readIncidents: true,
        writeIncidents: true,
        writeFieldReports: true,
        readVisits: true,
        writeVisits: true,
        attachFiles: true,
    };
    serverFieldReport = {
        number: 7,
        summary: "Lost child near center camp",
        incident: null,
        report_entries: [
            { id: 1, created: "2025-08-25T10:00:00Z", author: "Tool", text: "Found them", system_entry: false },
        ],
    };
    serverEvents = [{ id: eventId, name: eventName }];
});

function frRoutes(url: string, init?: RequestInit): Response | undefined {
    const hasBody = init?.body != null;
    if (url === `/ims/api/auth?event_id=${eventName}`) {
        return jsonResponse({
            authenticated: true,
            user: "Tester",
            admin: false,
            event_access: { [eventName]: serverEventAccess },
        });
    }
    if (url === "/ims/api/events") {
        return jsonResponse(serverEvents);
    }
    if (url === `${frUrl}/7` && !hasBody) {
        return jsonResponse(serverFieldReport);
    }
    if (url.startsWith(`${frUrl}/7`) && hasBody) {
        // Edits and attach/detach.
        return new Response(null, { status: 204 });
    }
    if (url === `/ims/api/events/${eventName}/incidents` && hasBody) {
        return new Response(null, { status: 201, headers: { "IMS-Incident-Number": "42" } });
    }
    return undefined;
}

async function initFieldReportPage(handler: (url: string, init?: RequestInit) => Response | undefined = frRoutes) {
    const mock = mockFetch(handler);
    await import("../typescript/field_report.ts");
    await vi.waitFor((): void => {
        expect(document.getElementById("loading-overlay")!.style.display).toBe("none");
    });
    return mock;
}

function inputValue(id: string): string {
    return (document.getElementById(id) as HTMLInputElement).value;
}

test("page init draws the field report number and summary from the API", async (): Promise<void> => {
    await initFieldReportPage();

    expect(inputValue("field_report_number")).toBe("7");
    expect(inputValue("field_report_summary")).toBe("Lost child near center camp");
    expect(document.title).toContain("Lost child near center camp");
    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(true);
});

test("an unattached field report offers the create-incident button to incident writers", async (): Promise<void> => {
    await initFieldReportPage();

    expect(document.getElementById("create_incident")!.classList.contains("hidden")).toBe(false);
    // No incident is linked yet.
    expect(inputValue("incident_number")).toBe("");
});

test("makeIncident creates an incident, attaches the report, and links it", async (): Promise<void> => {
    const mock = await initFieldReportPage();

    // After creation the reload should report the FR attached to incident 42.
    serverFieldReport.incident = 42;
    await window.makeIncident();

    const incidentCreate = mock.mock.calls.find(([url, init]) =>
        url === `/ims/api/events/${eventName}/incidents` && init?.body != null)!;
    expect(JSON.parse(incidentCreate[1]!.body as string)).toEqual({
        summary: "Lost child near center camp",
        ranger_handles: ["Tool"],
    });
    // The FR is then attached to the freshly-created incident 42.
    expect(mock.mock.calls.some(([url, init]) =>
        url === `${frUrl}/7?action=attach&incident=42` && init?.body != null)).toBe(true);
});

test("updateIncident attaches the field report to a typed-in incident number", async (): Promise<void> => {
    const mock = await initFieldReportPage();

    const incidentInput = document.getElementById("incident_number") as HTMLInputElement;
    incidentInput.value = "13";
    await window.updateIncident(incidentInput);

    expect(mock.mock.calls.some(([url, init]) =>
        url === `${frUrl}/7?action=attach&incident=13` && init?.body != null)).toBe(true);
});

test("a viewer without field-report read access sees an authorization error", async (): Promise<void> => {
    serverEventAccess.readIncidents = false;
    serverEventAccess.writeFieldReports = false;

    await initFieldReportPage();

    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    expect(document.getElementById("error_text")!.textContent).toContain("not currently authorized");
});
