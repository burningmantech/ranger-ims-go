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
    // window globals persist across tests; clear the one the page assigns only
    // once init fully settles, so waitFor tracks this test's init, not a prior one.
    (window as unknown as Record<string, unknown>)["toggleMultisearchModal"] = undefined;
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

// Pull a column's render function off the table the page configured.
function renderColumn(name: string): (value: any, type: string, row: ims.Visit) => unknown {
    const column = MockDataTable.lastInstance!.column(name)!;
    return column.render!;
}

test("the name column renders the guest's preferred name, falling back to legal name", async (): Promise<void> => {
    serverVisits.push({ number: 4, guest_legal_name: "Legal Name", report_entries: [] });
    await initVisitsPage();
    const render = renderColumn("visit_name");

    const preferred = serverVisits[0]!; // guest_preferred_name "Sparkle"
    expect((render(null, "display", preferred) as Node).textContent).toBe("Sparkle");
    expect(render(null, "sort", preferred)).toBe("Sparkle");

    const legalOnly = serverVisits[2]!; // only guest_legal_name
    expect((render(null, "display", legalOnly) as Node).textContent).toBe("Legal Name");
    expect(render(null, "filter", legalOnly)).toBe("Legal Name");
    expect(render(null, "bogus", preferred)).toBeUndefined();
});

test("the string columns render display, filter, and truncated text", async (): Promise<void> => {
    serverVisits[0]!.resource_sitter = "Buddy";
    await initVisitsPage();
    const render = renderColumn("visit_sitter");

    expect(render("Buddy", "display", serverVisits[0]!)).toBe("Buddy");
    expect(render("Buddy", "filter", serverVisits[0]!)).toBe("Buddy");
    expect(render(null, "display", serverVisits[0]!)).toBe("");
    expect(render("Buddy", "bogus", serverVisits[0]!)).toBeUndefined();

    const long = "y".repeat(400);
    const display = render(long, "display", serverVisits[0]!) as string;
    expect(display.length).toBe(250);
    expect(display.endsWith("...")).toBe(true);
});

test("the status filter shows only current visits unless showing all", async (): Promise<void> => {
    await initVisitsPage();
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    // data[0] has no departure_time (current); data[1] has one (departed).
    window.showStatus("current", false);
    expect(table.fixedSearch("status", 0)).toBe(true);
    expect(table.fixedSearch("status", 1)).toBe(false);

    window.showStatus("all", false);
    expect(table.fixedSearch("status", 0)).toBe(true);
    expect(table.fixedSearch("status", 1)).toBe(true);
});

test("the modification-date filter passes everything when no window is set", async (): Promise<void> => {
    await initVisitsPage();
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    expect(table.fixedSearch("modification_date", 0)).toBe(true);
    expect(table.fixedSearch("modification_date", 1)).toBe(true);
});

test("show* controls update the dropdown labels and the URL hash", async (): Promise<void> => {
    await initVisitsPage();

    window.showRows("50", true);
    expect(document.getElementById("show_rows")!.querySelector(".selection")!.textContent).not.toBe("");
    expect(window.location.hash).toContain("rows=50");
    expect(MockDataTable.lastInstance!.pageLen).toBe(50);

    window.showStatus("all", true);
    expect(document.getElementById("show_status")!.querySelector(".selection")!.textContent).not.toBe("");
    expect(window.location.hash).toContain("status=all");
});

test("'all' rows sets the page length to unlimited", async (): Promise<void> => {
    await initVisitsPage();
    window.showRows("all", false);
    expect(MockDataTable.lastInstance!.pageLen).toBe(-1);
});

test("pressing Enter on an integer search jumps to that visit", async (): Promise<void> => {
    await initVisitsPage();
    const input = document.getElementById("search_input") as HTMLInputElement;
    input.value = "13";
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));

    expect(input.value).toBe("");
    expect(window.location.href).toContain("/visits/13");
});

test("a /regex/ search is handed to the table as a regex query", async (): Promise<void> => {
    await initVisitsPage();
    const table = MockDataTable.lastInstance!;
    const input = document.getElementById("search_input") as HTMLInputElement;
    input.value = "/spark/";
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));

    expect(table.lastSearch).toEqual(["spark", true, false]);
    expect(window.location.hash).toContain("q=");
});

test("a visit update broadcast reloads the table", async (): Promise<void> => {
    await initVisitsPage();
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    serverVisits.push({ number: 9, guest_preferred_name: "Newcomer", report_entries: [] });
    const channel = new BroadcastChannel("visit_update");
    channel.postMessage({ visit_number: 9, event_id: eventId });
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(3);
    });
    channel.close();
});

test("an update_all visit broadcast reloads the table", async (): Promise<void> => {
    await initVisitsPage();
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    serverVisits.push({ number: 9, guest_preferred_name: "Newcomer", report_entries: [] });
    const channel = new BroadcastChannel("visit_update");
    channel.postMessage({ update_all: true });
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(3);
    });
    channel.close();
});

test("a visit broadcast for a different event is ignored", async (): Promise<void> => {
    await initVisitsPage();
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    const channel = new BroadcastChannel("visit_update");
    channel.postMessage({ visit_number: 2, event_id: eventId + 999 });
    await new Promise((resolve): void => { setTimeout(resolve, 20); });
    expect(table.data().length).toBe(2);
    channel.close();
});

test("toggling the multisearch modal lists the available events", async (): Promise<void> => {
    await initVisitsPage();
    // The modal toggle and keyboard listeners are wired only after the events
    // list resolves, which happens after the table is constructed.
    await vi.waitFor((): void => {
        expect(window.toggleMultisearchModal).toBeTypeOf("function");
    });
    window.toggleMultisearchModal();

    const links = document.querySelectorAll("#multisearch-events-list a");
    expect(links.length).toBe(1);
    expect(links[0]!.textContent).toBe(eventName);
});

test("keyboard shortcuts trigger new-visit and focus the search box", async (): Promise<void> => {
    await initVisitsPage();
    await vi.waitFor((): void => {
        expect(window.toggleMultisearchModal).toBeTypeOf("function");
    });

    const newClicked = vi.fn();
    document.getElementById("new_visit")!.addEventListener("click", newClicked);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "n" }));
    expect(newClicked).toHaveBeenCalled();

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "/" }));
    expect(document.activeElement).toBe(document.getElementById("search_input"));
});
