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
// (incidents.templ). The page loads incident types, field reports,
// visits, and incidents before handing rows to a DataTables grid; a stand-in
// DataTable (MockDataTable) captures those rows.

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { jsonResponse, loadFixture, MockDataTable, mockFetch, problemResponse } from "./helpers.ts";

const eventName = "2025";
const eventId = 1;
const incidentsUrl = `/ims/api/events/${eventName}/incidents?exclude_system_entries=true`;

let serverEventAccess: ims.AuthInfoEventAccess;
let serverIncidents: ims.Incident[];
let serverTypes: ims.IncidentType[];
let serverEvents: ims.EventData[];

beforeEach((): void => {
    vi.resetModules();
    // window globals persist across tests; clear the one the page assigns only
    // once init fully settles, so waitFor tracks this test's init, not a prior one.
    (window as unknown as Record<string, unknown>)["toggleMultisearchModal"] = undefined;
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
        expect(document.querySelectorAll("#ul_show_type button[data-incident-type-id]").length).toBe(2);
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

// Pull a column's render function off the table the page configured.
function renderColumn(name: string): (value: any, type: string, row: ims.Incident) => unknown {
    const column = MockDataTable.lastInstance!.column(name)!;
    return column.render!;
}

test("the summary column renders display, filter, and sort text", async (): Promise<void> => {
    await initIncidentsPage();
    const render = renderColumn("incident_summary");
    const incident = serverIncidents[0]!;

    expect(render(incident.summary, "display", incident)).toBe("Dust storm");
    expect(render(incident.summary, "sort", incident)).toBe("Dust storm");
    // The filter value pulls in attached report text, so it contains the summary.
    expect(render(incident.summary, "filter", incident)).toContain("Dust storm");
    // An unrecognized render type yields undefined.
    expect(render(incident.summary, "bogus", incident)).toBeUndefined();
});

test("the summary column truncates very long summaries for display", async (): Promise<void> => {
    const longSummary = "x".repeat(400);
    serverIncidents[0]!.summary = longSummary;
    await initIncidentsPage();
    const render = renderColumn("incident_summary");

    const display = render(longSummary, "display", serverIncidents[0]!) as string;
    expect(display.length).toBe(250);
    expect(display.endsWith("...")).toBe(true);
});

test("the types column renders type names and handles missing ids", async (): Promise<void> => {
    await initIncidentsPage();
    const render = renderColumn("incident_types");
    const incident = serverIncidents[0]!; // incident_type_ids: [1] -> "Junk"

    const display = render(incident.incident_type_ids, "display", incident) as Node;
    expect(display.textContent).toContain("Junk");
    expect(render(incident.incident_type_ids, "filter", incident)).toBe("Junk");
    // Null ids (no types column data) render as undefined.
    expect(render(null, "display", incident)).toBeUndefined();
    expect(render(incident.incident_type_ids, "bogus", incident)).toBeUndefined();
});

// Render the state column for an incident and return the dropdown it produced,
// or null if the column rendered plain text instead.
function renderStateSelect(incident: ims.Incident): HTMLSelectElement|null {
    const rendered = renderColumn("incident_state")(incident.state, "display", incident);
    return rendered instanceof HTMLSelectElement ? rendered : null;
}

test("the state column renders a dropdown set to the incident's state", async (): Promise<void> => {
    await initIncidentsPage();
    const incident = serverIncidents[0]!; // on_scene

    const select = renderStateSelect(incident)!;
    expect(select).not.toBeNull();
    expect(select.value).toBe("on_scene");
    expect(select.ariaLabel).toContain("Incident 1");
    // Every state IMS knows is offered.
    const states = [...select.options].map((o: HTMLOptionElement) => o.value);
    expect(states).toEqual(["new", "on_hold", "dispatched", "on_scene", "closed"]);
});

test("the state column renders plain text for a user who can't write incidents", async (): Promise<void> => {
    serverEventAccess.writeIncidents = false;
    await initIncidentsPage();
    const incident = serverIncidents[0]!;

    expect(renderStateSelect(incident)).toBeNull();
    expect(renderColumn("incident_state")(incident.state, "display", incident)).toBe("On Scene");
});

test("the state column still renders sort and filter values", async (): Promise<void> => {
    await initIncidentsPage();
    const render = renderColumn("incident_state");
    const incident = serverIncidents[0]!; // on_scene

    expect(render(incident.state, "filter", incident)).toBe("On Scene");
    // Sort keys order the states from new (1) through closed (5).
    expect(render(incident.state, "sort", incident)).toBe(4);
    expect(render(incident.state, "bogus", incident)).toBeUndefined();
});

// Accept a write to any single incident, on top of the usual page routes.
function savingRoutes(url: string, init?: RequestInit): Response | undefined {
    const incident = serverIncidents.find(i => url === `/ims/api/events/${eventName}/incidents/${i.number}`);
    if (incident != null) {
        return jsonResponse(incident);
    }
    return incidentsRoutes(url, init);
}

test("changing the state dropdown saves the new state with an If-Match version", async (): Promise<void> => {
    serverIncidents[0]!.version = 7;
    const mock = await initIncidentsPage(savingRoutes);
    const select = renderStateSelect(serverIncidents[0]!)!;

    select.value = "closed";
    select.dispatchEvent(new Event("change"));

    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url]) => url === `/ims/api/events/${eventName}/incidents/1`)).toBe(true);
    });
    const [, init] = mock.mock.calls.find(([url]) => url === `/ims/api/events/${eventName}/incidents/1`)!;
    expect(init!.method).toBe("POST");
    expect(JSON.parse(init!.body as string)).toEqual({ number: 1, state: "closed" });
    expect(new Headers(init!.headers).get("If-Match")).toBe('"7"');
    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(true);
});

test("a failed state change reverts the dropdown and shows the error", async (): Promise<void> => {
    const handler = (url: string, init?: RequestInit): Response | undefined => {
        if (url === `/ims/api/events/${eventName}/incidents/1`) {
            return problemResponse("Nope", 500);
        }
        return incidentsRoutes(url, init);
    };
    await initIncidentsPage(handler);
    const select = renderStateSelect(serverIncidents[0]!)!; // on_scene

    select.value = "closed";
    select.dispatchEvent(new Event("change"));

    await vi.waitFor((): void => {
        expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    });
    // The dropdown goes back to the state the server last gave us.
    expect(select.value).toBe("on_scene");
    expect(select.classList.contains("is-invalid")).toBe(true);
    expect(select.disabled).toBe(false);
});

test("a conflicting state change reports that someone else edited the incident", async (): Promise<void> => {
    const handler = (url: string, init?: RequestInit): Response | undefined => {
        if (url === `/ims/api/events/${eventName}/incidents/1`) {
            return problemResponse("Version mismatch", 412);
        }
        return incidentsRoutes(url, init);
    };
    await initIncidentsPage(handler);
    const select = renderStateSelect(serverIncidents[0]!)!;

    select.value = "closed";
    select.dispatchEvent(new Event("change"));

    await vi.waitFor((): void => {
        expect(document.getElementById("error_text")!.textContent).toContain("Someone else has edited");
    });
});

test("closing an incident with no incident type warns the user, but still saves", async (): Promise<void> => {
    // happy-dom doesn't implement window.alert.
    const alertSpy = vi.fn();
    vi.stubGlobal("alert", alertSpy);
    const mock = await initIncidentsPage(savingRoutes);
    // serverIncidents[1] has no incident types.
    const select = renderStateSelect(serverIncidents[1]!)!;

    select.value = "closed";
    select.dispatchEvent(new Event("change"));

    expect(alertSpy).toHaveBeenCalledOnce();
    expect(alertSpy.mock.calls[0]![0]).toContain("Please add an incident type");
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url]) => url === `/ims/api/events/${eventName}/incidents/2`)).toBe(true);
    });

    // An incident that has a type draws no warning.
    alertSpy.mockClear();
    const typed = renderStateSelect(serverIncidents[0]!)!;
    typed.value = "closed";
    typed.dispatchEvent(new Event("change"));
    expect(alertSpy).not.toHaveBeenCalled();
});

test("the state filter shows open, active, and all states correctly", async (): Promise<void> => {
    // data[0] on_scene (open+active), data[1] closed, data[2] on_hold (open but not active)
    serverIncidents.push({
        number: 3, event: eventName, state: "on_hold", priority: 3, summary: "On hold", incident_type_ids: [1], report_entries: [],
    });
    await initIncidentsPage();
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(3);
    });

    window.showState("open", false);
    expect(table.fixedSearch("state", 0)).toBe(true); // on_scene
    expect(table.fixedSearch("state", 1)).toBe(false); // closed hidden
    expect(table.fixedSearch("state", 2)).toBe(true); // on_hold still open

    window.showState("active", false);
    expect(table.fixedSearch("state", 0)).toBe(true); // on_scene active
    expect(table.fixedSearch("state", 1)).toBe(false); // closed hidden
    expect(table.fixedSearch("state", 2)).toBe(false); // on_hold not active

    window.showState("all", false);
    expect(table.fixedSearch("state", 0)).toBe(true);
    expect(table.fixedSearch("state", 1)).toBe(true);
    expect(table.fixedSearch("state", 2)).toBe(true);
});

test("toggling all types off then on drives the type filter", async (): Promise<void> => {
    await initIncidentsPage();
    const table = MockDataTable.lastInstance!;

    // Everything is visible by default (all types checked).
    expect(table.fixedSearch("type", 0)).toBe(true);
    expect(table.fixedSearch("type", 1)).toBe(true);

    // Uncheck all types: nothing matches.
    window.toggleCheckAllTypes();
    expect(table.fixedSearch("type", 0)).toBe(false);
    expect(table.fixedSearch("type", 1)).toBe(false);

    // Re-check all types: everything matches again.
    window.toggleCheckAllTypes();
    expect(table.fixedSearch("type", 0)).toBe(true);
    expect(table.fixedSearch("type", 1)).toBe(true);
});

test("clicking a single type checkbox narrows the type filter", async (): Promise<void> => {
    await initIncidentsPage();
    const table = MockDataTable.lastInstance!;

    // Start from a clean slate with nothing checked.
    window.toggleCheckAllTypes();
    // Check only the "Junk" type (id 1), which is on incident[0] but not incident[1].
    const junk = document.querySelector('#ul_show_type button[data-incident-type-id="1"]') as HTMLElement;
    junk.click();

    expect(table.fixedSearch("type", 0)).toBe(true); // has type 1
    expect(table.fixedSearch("type", 1)).toBe(false); // blank type, not checked

    // Now check "(blank)", which matches the typeless incident[1].
    (document.getElementById("show_blank_type") as HTMLElement).click();
    expect(table.fixedSearch("type", 1)).toBe(true);
});

test("the days filter hides incidents not modified within the window", async (): Promise<void> => {
    serverIncidents[0]!.last_modified = new Date().toISOString();
    serverIncidents[1]!.last_modified = "2000-01-01T00:00:00Z";
    await initIncidentsPage();
    const table = MockDataTable.lastInstance!;

    // "all" days (the default): both pass.
    expect(table.fixedSearch("modification_date", 0)).toBe(true);
    expect(table.fixedSearch("modification_date", 1)).toBe(true);

    // Last day only: the year-2000 incident is filtered out.
    window.showDays(1, false);
    expect(table.fixedSearch("modification_date", 0)).toBe(true);
    expect(table.fixedSearch("modification_date", 1)).toBe(false);
});

test("show* controls update the dropdown labels and the URL hash", async (): Promise<void> => {
    await initIncidentsPage();

    window.showState("all", true);
    expect(document.getElementById("show_state")!.querySelector(".selection")!.textContent).toBe("All States");
    expect(window.location.hash).toContain("state=all");

    window.showDays(1, true);
    expect(document.getElementById("show_days")!.querySelector(".selection")!.textContent).not.toBe("");
    expect(window.location.hash).toContain("days=1");

    window.showRows("50", true);
    expect(window.location.hash).toContain("rows=50");
});

test("pressing Enter on an integer search jumps to that incident", async (): Promise<void> => {
    await initIncidentsPage();
    const input = document.getElementById("search_input") as HTMLInputElement;
    input.value = "42";
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));

    // The jump clears the search box after navigating.
    expect(input.value).toBe("");
    expect(window.location.href).toContain("/incidents/42");
});

test("a /regex/ search is handed to the table as a regex query", async (): Promise<void> => {
    await initIncidentsPage();
    const table = MockDataTable.lastInstance!;
    const input = document.getElementById("search_input") as HTMLInputElement;
    input.value = "/dust/";
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));

    expect(table.lastSearch).toEqual(["dust", true, false]);
    expect(window.location.hash).toContain("q=");
});

test("an incident update broadcast refreshes the matching row in place", async (): Promise<void> => {
    const updated: ims.Incident = {
        number: 1, event: eventName, state: "closed", priority: 3, summary: "Dust storm RESOLVED", incident_type_ids: [1], report_entries: [],
    };
    const handler = (url: string, init?: RequestInit): Response | undefined => {
        if (url === `/ims/api/events/${eventName}/incidents/1`) {
            return jsonResponse(updated);
        }
        return incidentsRoutes(url, init);
    };
    await initIncidentsPage(handler);
    const table = MockDataTable.lastInstance!;
    // Wait for the first load to finish, which is when the "init" handler runs
    // and registers the BroadcastChannel listener.
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    const channel = new BroadcastChannel("incident_update");
    channel.postMessage({ incident_number: 1, event_id: eventId });
    await vi.waitFor((): void => {
        const row = table.data().find((i: ims.Incident) => i.number === 1);
        expect(row.summary).toBe("Dust storm RESOLVED");
    });
    channel.close();
});

test("a broadcast for an unknown incident adds a new row", async (): Promise<void> => {
    const created: ims.Incident = {
        number: 9, event: eventName, state: "new", priority: 3, summary: "Brand new", incident_type_ids: [], report_entries: [],
    };
    const handler = (url: string, init?: RequestInit): Response | undefined => {
        if (url === `/ims/api/events/${eventName}/incidents/9`) {
            return jsonResponse(created);
        }
        return incidentsRoutes(url, init);
    };
    await initIncidentsPage(handler);
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    const channel = new BroadcastChannel("incident_update");
    channel.postMessage({ incident_number: 9, event_id: eventId });
    await vi.waitFor((): void => {
        expect(table.data().some((i: ims.Incident) => i.number === 9)).toBe(true);
    });
    channel.close();
});

test("an update_all broadcast reloads the whole table", async (): Promise<void> => {
    await initIncidentsPage();
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    // The server now returns a third incident; update_all reloads everything.
    serverIncidents.push({
        number: 5, event: eventName, state: "new", priority: 3, summary: "Reloaded", incident_type_ids: [], report_entries: [],
    });
    const channel = new BroadcastChannel("incident_update");
    channel.postMessage({ update_all: true });
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(3);
    });
    channel.close();
});

test("a broadcast for a different event is ignored", async (): Promise<void> => {
    await initIncidentsPage();
    const table = MockDataTable.lastInstance!;
    await vi.waitFor((): void => {
        expect(table.data().length).toBe(2);
    });

    const channel = new BroadcastChannel("incident_update");
    channel.postMessage({ incident_number: 1, event_id: eventId + 999 });
    // Give the handler a chance to (not) act.
    await new Promise((resolve): void => { setTimeout(resolve, 20); });
    expect(table.data().length).toBe(2);
    channel.close();
});

test("toggling the multisearch modal lists the available events", async (): Promise<void> => {
    await initIncidentsPage();
    // The modal toggle and keyboard listeners are wired only after the events
    // list resolves, which happens after the table is constructed.
    await vi.waitFor((): void => {
        expect(window.toggleMultisearchModal).toBeTypeOf("function");
    });
    window.toggleMultisearchModal();

    const links = document.querySelectorAll("#multisearch-events-list a");
    expect(links.length).toBe(1);
    expect(links[0]!.textContent).toBe(eventName);
    expect((links[0] as HTMLAnchorElement).href).toContain(eventName);
});

test("keyboard shortcuts trigger new-incident and focus the search box", async (): Promise<void> => {
    await initIncidentsPage();
    await vi.waitFor((): void => {
        expect(window.toggleMultisearchModal).toBeTypeOf("function");
    });

    const newClicked = vi.fn();
    document.getElementById("new_incident")!.addEventListener("click", newClicked);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "n" }));
    expect(newClicked).toHaveBeenCalled();

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "/" }));
    expect(document.activeElement).toBe(document.getElementById("search_input"));
});
