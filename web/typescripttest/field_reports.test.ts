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
    // window globals persist across tests; clear the one the page assigns only
    // once init fully settles, so waitFor tracks this test's init, not a prior one.
    (window as unknown as Record<string, unknown>)["toggleMultisearchModal"] = undefined;
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

// Pull a column's render function off the table the page configured.
function renderColumn(name: string): (value: any, type: string, row: ims.FieldReport) => unknown {
    const column = MockDataTable.lastInstance!.column(name)!;
    return column.render!;
}

test("the summary column renders display, filter, and sort text", async (): Promise<void> => {
    await initFieldReportsPage();
    const render = renderColumn("field_report_summary");
    const report = serverFieldReports[0]!; // summary "Lost child"

    expect(render(report.summary, "display", report)).toBe("Lost child");
    expect(render(report.summary, "sort", report)).toBe("Lost child");
    expect(render(report.summary, "filter", report)).toContain("Lost child");
    expect(render(report.summary, "bogus", report)).toBeUndefined();
});

test("the summary column falls back to the first report entry line", async (): Promise<void> => {
    serverFieldReports[0] = {
        number: 7, summary: "", incident: null,
        report_entries: [{ text: "First line\nSecond line", system_entry: false }],
    };
    await initFieldReportsPage();
    const render = renderColumn("field_report_summary");

    expect(render("", "display", serverFieldReports[0]!)).toBe("First line");
});

test("the modification-date filter passes everything until a window is set", async (): Promise<void> => {
    serverFieldReports[0]!.created = new Date().toISOString();
    serverFieldReports[1]!.created = "2000-01-01T00:00:00Z";
    await initFieldReportsPage();
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    // "all" days (the default): both pass.
    expect(table.fixedSearch("modification_date", 0)).toBe(true);
    expect(table.fixedSearch("modification_date", 1)).toBe(true);

    // Restrict to the last day: the year-2000 report is filtered out.
    window.frShowDays(1, false);
    expect(table.fixedSearch("modification_date", 0)).toBe(true);
    expect(table.fixedSearch("modification_date", 1)).toBe(false);
});

test("the modification-date filter keeps reports with a recent entry", async (): Promise<void> => {
    serverFieldReports[0] = {
        number: 7, summary: "Old report", incident: null, created: "2000-01-01T00:00:00Z",
        report_entries: [{ text: "fresh note", system_entry: false, created: new Date().toISOString() }],
    };
    await initFieldReportsPage();
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    window.frShowDays(1, false);
    // Created long ago, but a report entry lands within the window.
    expect(table.fixedSearch("modification_date", 0)).toBe(true);
});

test("show* controls update the dropdown labels and the URL hash", async (): Promise<void> => {
    await initFieldReportsPage();

    window.frShowDays(1, true);
    expect(document.getElementById("show_days")!.querySelector(".selection")!.textContent).not.toBe("");
    expect(window.location.hash).toContain("days=1");

    window.frShowRows("50", true);
    expect(window.location.hash).toContain("rows=50");
    expect(MockDataTable.lastInstance!.pageLen).toBe(50);
});

test("'all' rows sets the page length to unlimited", async (): Promise<void> => {
    await initFieldReportsPage();
    window.frShowRows("all", false);
    expect(MockDataTable.lastInstance!.pageLen).toBe(-1);
});

test("pressing Enter on an integer search jumps to that field report", async (): Promise<void> => {
    await initFieldReportsPage();
    const input = document.getElementById("search_input") as HTMLInputElement;
    input.value = "55";
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));

    expect(input.value).toBe("");
    expect(window.location.href).toContain("/field_reports/55");
});

test("a /regex/ search is handed to the table as a regex query", async (): Promise<void> => {
    await initFieldReportsPage();
    const table = MockDataTable.lastInstance!;
    const input = document.getElementById("search_input") as HTMLInputElement;
    input.value = "/wallet/";
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));

    expect(table.lastSearch).toEqual(["wallet", true, false]);
    expect(window.location.hash).toContain("q=");
});

test("a field report update broadcast reloads the table", async (): Promise<void> => {
    await initFieldReportsPage();
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    serverFieldReports.push({ number: 9, summary: "New report", incident: null, report_entries: [] });
    const channel = new BroadcastChannel("field_report_update");
    channel.postMessage({ field_report_number: 9, event_id: eventId });
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(3);
    });
    channel.close();
});

test("an update_all field report broadcast reloads the table", async (): Promise<void> => {
    await initFieldReportsPage();
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    serverFieldReports.push({ number: 9, summary: "New report", incident: null, report_entries: [] });
    const channel = new BroadcastChannel("field_report_update");
    channel.postMessage({ update_all: true });
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(3);
    });
    channel.close();
});

test("a field report broadcast for a different event is ignored", async (): Promise<void> => {
    await initFieldReportsPage();
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    const channel = new BroadcastChannel("field_report_update");
    channel.postMessage({ field_report_number: 7, event_id: eventId + 999 });
    await new Promise((resolve): void => { setTimeout(resolve, 20); });
    expect(table.data().length).toBe(2);
    channel.close();
});

test("toggling the multisearch modal lists the available events", async (): Promise<void> => {
    await initFieldReportsPage();
    await vi.waitFor((): void => {
        expect(window.toggleMultisearchModal).toBeTypeOf("function");
    });
    window.toggleMultisearchModal();

    const links = document.querySelectorAll("#multisearch-events-list a");
    expect(links.length).toBe(1);
    expect(links[0]!.textContent).toBe(eventName);
});

test("keyboard shortcuts trigger new-field-report and focus the search box", async (): Promise<void> => {
    await initFieldReportsPage();
    await vi.waitFor((): void => {
        expect(window.toggleMultisearchModal).toBeTypeOf("function");
    });

    const newClicked = vi.fn();
    document.getElementById("new_field_report")!.addEventListener("click", newClicked);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "n" }));
    expect(newClicked).toHaveBeenCalled();

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "/" }));
    expect(document.activeElement).toBe(document.getElementById("search_input"));
});
