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

// Tests for field_reports.ts against the real templ-rendered field reports list
// page (field_reports.templ). The page drives a DataTables grid; a stand-in
// DataTable (MockDataTable) captures the rows the page hands the table.

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { jsonResponse, loadFixture, MockDataTable, mockFetch } from "./helpers.ts";

const eventName = "2025";
const eventId = 1;
const frUrl = `/ims/api/events/${eventName}/field_reports`;

let serverEventAccess: ims.AuthInfoEventAccess;
let serverFieldReports: ims.FieldReport[];
let serverEvents: ims.EventData[];

beforeEach((): void => {
    vi.resetModules();
    loadFixture("field_reports.html");
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/field_reports`);

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
    serverFieldReports = [
        { number: 7, summary: "Lost child", incident: null, report_entries: [] },
        { number: 8, summary: "Found wallet", incident: 3, report_entries: [] },
    ];
    serverEvents = [{ id: eventId, name: eventName }];
});

function frRoutes(url: string, init?: RequestInit): Response | undefined {
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
    if (url === frUrl && init?.body == null) {
        return jsonResponse(serverFieldReports);
    }
    return undefined;
}

async function initFieldReportsPage(handler: (url: string, init?: RequestInit) => Response | undefined = frRoutes) {
    MockDataTable.lastInstance = null;
    const mock = mockFetch(handler);
    await import("../typescript/field_reports.ts");
    // The page has no loading overlay; init has settled once the table is
    // constructed (authorized) or the authorization error is shown.
    await vi.waitFor((): void => {
        const errored = !document.getElementById("error_info")!.classList.contains("hidden");
        expect(errored || MockDataTable.lastInstance != null).toBe(true);
    });
    return mock;
}

test("page init loads the event's field reports into the table", async (): Promise<void> => {
    await initFieldReportsPage();

    await vi.waitFor((): void => {
        expect(MockDataTable.lastInstance?.data().length).toBe(2);
    });
    const numbers = MockDataTable.lastInstance!.data().map((fr: ims.FieldReport) => fr.number);
    expect(numbers).toEqual([7, 8]);
    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(true);
});

test("the show-rows and show-days controls are wired and reflect the default selection", async (): Promise<void> => {
    await initFieldReportsPage();

    expect(window.frShowRows).toBeTypeOf("function");
    expect(window.frShowDays).toBeTypeOf("function");
    // init applies the default filters, which fills in the dropdown labels.
    expect(document.getElementById("show_rows")!.querySelector(".selection")!.textContent).not.toBe("");
});

test("a viewer without field-report read access sees an authorization error", async (): Promise<void> => {
    serverEventAccess.readIncidents = false;
    serverEventAccess.writeFieldReports = false;

    await initFieldReportsPage();

    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    expect(document.getElementById("error_text")!.textContent).toContain("not currently authorized");
});
