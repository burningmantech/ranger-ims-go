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

// Tests for sanctuary_visits.ts against the real templ-rendered visits list
// page (sanctuary_visits.templ). A stand-in DataTable (MockDataTable) captures
// the rows the page hands the table.

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { jsonResponse, loadFixture, MockDataTable, mockFetch } from "./helpers.ts";

const eventName = "2025";
const eventId = 1;
const visitsUrl = `/ims/api/events/${eventName}/visits`;

let serverEventAccess: ims.AuthInfoEventAccess;
let serverVisits: ims.Visit[];
let serverEvents: ims.EventData[];

beforeEach((): void => {
    vi.resetModules();
    loadFixture("sanctuary_visits.html");
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/visits`);

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
    serverVisits = [
        { number: 2, guest_preferred_name: "Sparkle", report_entries: [] },
        { number: 3, guest_preferred_name: "Wanderer", departure_time: "2025-08-25T12:00:00Z", report_entries: [] },
    ];
    serverEvents = [{ id: eventId, name: eventName }];
});

function visitRoutes(url: string, init?: RequestInit): Response | undefined {
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
    if (url === visitsUrl && init?.body == null) {
        return jsonResponse(serverVisits);
    }
    return undefined;
}

async function initVisitsPage(handler: (url: string, init?: RequestInit) => Response | undefined = visitRoutes) {
    MockDataTable.lastInstance = null;
    const mock = mockFetch(handler);
    await import("../typescript/sanctuary_visits.ts");
    await vi.waitFor((): void => {
        const errored = !document.getElementById("error_info")!.classList.contains("hidden");
        expect(errored || MockDataTable.lastInstance != null).toBe(true);
    });
    return mock;
}

test("page init loads the event's visits into the table", async (): Promise<void> => {
    await initVisitsPage();

    await vi.waitFor((): void => {
        expect(MockDataTable.lastInstance?.data().length).toBe(2);
    });
    const numbers = MockDataTable.lastInstance!.data().map((v: ims.Visit) => v.number);
    expect(numbers).toEqual([2, 3]);
    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(true);
});

test("the show-rows and show-status controls are wired and reflect the default selection", async (): Promise<void> => {
    await initVisitsPage();

    expect(window.showRows).toBeTypeOf("function");
    expect(window.showStatus).toBeTypeOf("function");
    expect(document.getElementById("show_rows")!.querySelector(".selection")!.textContent).not.toBe("");
    expect(document.getElementById("show_status")!.querySelector(".selection")!.textContent).not.toBe("");
});

test("a viewer without visit read access sees an authorization error", async (): Promise<void> => {
    serverEventAccess.readVisits = false;

    await initVisitsPage();

    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    expect(document.getElementById("error_text")!.textContent).toContain("not currently authorized");
});
