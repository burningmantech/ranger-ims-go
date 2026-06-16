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

// Tests for incidents.ts against the real templ-rendered incidents list page
// (incidents.templ). The page loads incident types, streets, field reports,
// visits, and incidents before handing rows to a DataTables grid; a stand-in
// DataTable (MockDataTable) captures those rows.

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { jsonResponse, loadFixture, MockDataTable, mockFetch } from "./helpers.ts";

const eventName = "2025";
const eventId = 1;
const incidentsUrl = `/ims/api/events/${eventName}/incidents?exclude_system_entries=true`;

let serverEventAccess: ims.AuthInfoEventAccess;
let serverIncidents: ims.Incident[];
let serverTypes: ims.IncidentType[];
let serverEvents: ims.EventData[];

beforeEach((): void => {
    vi.resetModules();
    loadFixture("incidents.html");
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/incidents`);

    vi.stubGlobal("isSecureContext", true);
    vi.stubGlobal("DataTable", MockDataTable);
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
    serverIncidents = [
        { number: 1, event: eventName, state: "on_scene", priority: 3, summary: "Dust storm", incident_type_ids: [1], report_entries: [] },
        { number: 2, event: eventName, state: "closed", priority: 5, summary: "Lost shoe", incident_type_ids: [], report_entries: [] },
    ];
    serverTypes = [
        { id: 1, name: "Junk", hidden: false, description: "" },
        { id: 2, name: "Lost Child", hidden: false, description: "" },
    ];
    serverEvents = [{ id: eventId, name: eventName }];
});

function incidentsRoutes(url: string, _init?: RequestInit): Response | undefined {
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
    if (url === "/ims/api/incident_types") {
        return jsonResponse(serverTypes);
    }
    if (url === `/ims/api/streets?event_id=${eventId}`) {
        return jsonResponse({ [eventId]: { "100": "Esplanade" } });
    }
    if (url === `/ims/api/events/${eventName}/field_reports?exclude_system_entries=true`) {
        return jsonResponse([]);
    }
    if (url === `/ims/api/events/${eventName}/visits?exclude_system_entries=true`) {
        return jsonResponse([]);
    }
    if (url === incidentsUrl) {
        return jsonResponse(serverIncidents);
    }
    return undefined;
}

async function initIncidentsPage(handler: (url: string, init?: RequestInit) => Response | undefined = incidentsRoutes) {
    MockDataTable.lastInstance = null;
    const mock = mockFetch(handler);
    await import("../typescript/incidents.ts");
    await vi.waitFor((): void => {
        const errored = !document.getElementById("error_info")!.classList.contains("hidden");
        expect(errored || MockDataTable.lastInstance != null).toBe(true);
    });
    return mock;
}

test("page init loads the event's incidents into the table", async (): Promise<void> => {
    await initIncidentsPage();

    await vi.waitFor((): void => {
        expect(MockDataTable.lastInstance?.data().length).toBe(2);
    });
    const numbers = MockDataTable.lastInstance!.data().map((i: ims.Incident) => i.number);
    expect(numbers).toEqual([1, 2]);
    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(true);
});

test("the filter controls and per-type checkboxes are wired", async (): Promise<void> => {
    await initIncidentsPage();

    expect(window.showState).toBeTypeOf("function");
    expect(window.showDays).toBeTypeOf("function");
    expect(window.showRows).toBeTypeOf("function");
    expect(window.toggleCheckAllTypes).toBeTypeOf("function");

    // The type filter list gets one checkable entry per visible incident type.
    await vi.waitFor((): void => {
        expect(document.querySelectorAll("#ul_show_type a[data-incident-type-id]").length).toBe(2);
    });
});

test("a viewer with neither incident read nor field-report write access sees an authorization error", async (): Promise<void> => {
    serverEventAccess.readIncidents = false;
    serverEventAccess.writeFieldReports = false;

    await initIncidentsPage();

    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    expect(document.getElementById("error_text")!.textContent).toContain("not currently authorized");
});

test("a field-report writer without incident access is redirected to the field reports page", async (): Promise<void> => {
    serverEventAccess.readIncidents = false;
    serverEventAccess.writeFieldReports = true;
    const replace = vi.spyOn(window.location, "replace").mockImplementation((): void => {});

    const mock = mockFetch(incidentsRoutes);
    await import("../typescript/incidents.ts");
    await vi.waitFor((): void => {
        expect(replace).toHaveBeenCalled();
    });
    expect((replace.mock.calls[0]![0] as string)).toContain("field_reports");
    // The incidents list is never fetched on the redirect path.
    expect(mock.mock.calls.some(([url]) => url === incidentsUrl)).toBe(false);
});
