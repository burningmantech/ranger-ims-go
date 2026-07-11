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

// Tests for incident.ts against the real templ-rendered incident page
// (incident.templ).

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { jsonResponse, loadFixture, MockFlatpickr, mockFetch, problemResponse } from "./helpers.ts";

const eventName = "2025";
const eventId = 1;

let serverEventAccess: ims.AuthInfoEventAccess;
let serverIncident: ims.Incident;
// The incident's current ETag; edits through incidentRoutes bump it, the way
// the server's version counter would.
let serverETag: string;
let serverPersonnel: ims.Personnel[];
let serverTypes: ims.IncidentType[];
let serverEvents: ims.EventData[];
let serverPlaces: ims.Places;
let serverFieldReports: ims.FieldReport[];
let serverVisits: ims.Visit[];

beforeEach((): void => {
    vi.resetModules();
    loadFixture("incident.html");
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/incidents/1`);

    // requestEventSourceLock requires a secure context, which happy-dom
    // doesn't claim to be, and the Web Locks API, which happy-dom leaves
    // null. Park the lock request forever so the page doesn't loop
    // reacquiring the EventSource.
    vi.stubGlobal("isSecureContext", true);
    Object.defineProperty(navigator, "locks", {
        configurable: true,
        value: { request: (): Promise<undefined> => new Promise<undefined>((): void => {}) },
    });

    serverETag = `"5"`;
    serverEventAccess = {
        event_id: eventId,
        readIncidents: true,
        writeIncidents: true,
        writeFieldReports: true,
        readVisits: true,
        writeVisits: true,
        attachFiles: true,
    };
    serverIncident = {
        number: 1,
        event: eventName,
        state: "on_scene",
        priority: 3,
        summary: "Dust storm",
        created: "2025-08-25T09:00:00Z",
        started: "2025-08-25T10:00:00Z",
        rangers: [{ handle: "Tool", role: "lead" }, { handle: "Hot Slots" }],
        incident_type_ids: [1],
        location: { name: "The Man", address: "12:00 & A", description: "by the fire" },
        report_entries: [
            { id: 1, created: "2025-08-25T10:00:00Z", author: "Hot Slots", text: "Changed state: on_scene", system_entry: true },
            { id: 2, created: "2025-08-25T10:05:00Z", author: "Hot Slots", text: "Dust storm at the Man", system_entry: false },
        ],
        linked_incidents: [{ event_id: eventId, event_name: eventName, number: 5, summary: "Related" }],
    };
    serverPersonnel = [
        { handle: "Hot Slots", status: "active", directory_id: 1234 },
        { handle: "Tool", status: "active", directory_id: null },
        { handle: "Tulsa", status: "active", directory_id: 5678 },
        { handle: "Bean Counter", status: "auditor", directory_id: 9 },
    ];
    serverTypes = [
        { id: 1, name: "Junk", hidden: false, description: "Erroneously-created incidents" },
        { id: 2, name: "Lost Child", hidden: false, description: "A lost child" },
        { id: 3, name: "Old Type", hidden: true, description: "" },
    ];
    serverEvents = [
        { id: eventId, name: eventName, map_url: "https://map.example.com/2025" },
        { id: 2, name: "2015" },
    ];
    serverPlaces = {
        camp: [{ name: "Camp Friendly", location_string: "5:00 & C" }],
        art: [{ name: "Trash Fence Gallery", location_string: "11:30 & K" }],
        mv: [{ name: "Disco Bus" }],
        other: [{ name: "First Camp", location_string: null }],
    };
    serverFieldReports = [
        {
            number: 7,
            incident: 1,
            summary: "FR about storm",
            report_entries: [
                { id: 21, created: "2025-08-25T10:10:00Z", author: "Tool", text: "from the field", system_entry: false },
            ],
        },
        { number: 8, incident: null, summary: "Unattached FR", report_entries: [] },
        { number: 9, incident: 3, summary: "Other incident's FR", report_entries: [] },
    ];
    serverVisits = [
        {
            number: 2,
            incident: 1,
            guest_preferred_name: "Sparkle",
            report_entries: [
                { id: 31, created: "2025-08-25T10:15:00Z", author: "Moon Dog", text: "visit note", system_entry: false },
            ],
        },
        { number: 3, incident: null, guest_preferred_name: "Wanderer", report_entries: [] },
    ];
});

// Increment a quoted-integer ETag, e.g. `"5"` -> `"6"`.
function bumpETag(etag: string): string {
    return `"${Number(etag.replaceAll(`"`, "")) + 1}"`;
}

function incidentRoutes(url: string, init?: RequestInit): Response | undefined {
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
    if (url === `/ims/api/personnel?event_id=${eventName}`) {
        return jsonResponse(serverPersonnel);
    }
    if (url === "/ims/api/incident_types") {
        return jsonResponse(serverTypes);
    }
    if (url === `/ims/api/events/${eventName}/places?exclude_external_data=true`) {
        return jsonResponse(serverPlaces);
    }
    if (url === `/ims/api/events/${eventName}/field_reports` && !hasBody) {
        return jsonResponse(serverFieldReports);
    }
    if (url === `/ims/api/events/${eventName}/visits` && !hasBody) {
        return jsonResponse(serverVisits);
    }
    if (url.startsWith(`/ims/api/events/${eventName}/incidents/1/rangers/`)) {
        serverETag = bumpETag(serverETag);
        return new Response(null, { status: 204, headers: { "ETag": serverETag } });
    }
    if (url.startsWith(`/ims/api/events/${eventName}/incidents/1/report_entries/`) && hasBody) {
        return new Response(null, { status: 204 });
    }
    if (url === `/ims/api/events/${eventName}/incidents/1/attachments` && hasBody) {
        return new Response(null, { status: 200 });
    }
    if (url === `/ims/api/events/${eventName}/incidents` && hasBody) {
        // Incident creation.
        serverIncident.number = 42;
        return new Response(null, { status: 201, headers: { "IMS-Incident-Number": "42" } });
    }
    if ((url === `/ims/api/events/${eventName}/incidents/1` || url === `/ims/api/events/${eventName}/incidents/42`) && !hasBody) {
        return jsonResponse(serverIncident, 200, { "ETag": serverETag });
    }
    if ((url === `/ims/api/events/${eventName}/incidents/1` || url === `/ims/api/events/${eventName}/incidents/42`) && hasBody) {
        serverETag = bumpETag(serverETag);
        return new Response(null, { status: 204, headers: { "ETag": serverETag } });
    }
    if (url === `/ims/api/events/${eventName}/field_reports/8` && !hasBody) {
        return jsonResponse(serverFieldReports.find((fr: ims.FieldReport): boolean => fr.number === 8));
    }
    if (url.startsWith(`/ims/api/events/${eventName}/field_reports/`) && hasBody) {
        return new Response(null, { status: 204 });
    }
    if (url.startsWith(`/ims/api/events/${eventName}/visits/`) && hasBody) {
        return new Response(null, { status: 204 });
    }
    return undefined;
}

// Import incident.ts with a fake server behind it, and wait for the page to
// finish initializing (the loading overlay goes away at the end of init).
async function initIncidentPage(handler: (url: string, init?: RequestInit) => Response | undefined = incidentRoutes) {
    const mock = mockFetch(handler);
    await import("../typescript/incident.ts");
    await vi.waitFor((): void => {
        expect(document.getElementById("loading-overlay")!.style.display).toBe("none");
    });
    return mock;
}

function inputValue(id: string): string {
    return (document.getElementById(id) as HTMLInputElement).value;
}

// The JSON bodies POSTed to the given URL, oldest first.
function postedBodies(mock: ReturnType<typeof mockFetch>, url: string): unknown[] {
    return mock.mock.calls
        .filter(([u, init]): boolean => u === url && init?.body != null)
        .map(([, init]): unknown => JSON.parse(init!.body as string));
}

// The If-Match header of each POST to the given URL, oldest first
// (null for a POST that carried no If-Match).
function postedIfMatches(mock: ReturnType<typeof mockFetch>, url: string): (string|null)[] {
    return mock.mock.calls
        .filter(([u, init]): boolean => u === url && init?.body != null)
        .map(([, init]): string|null => new Headers(init!.headers).get("If-Match"));
}

test("page init draws the incident fields from the API", async (): Promise<void> => {
    await initIncidentPage();

    expect(inputValue("incident_number")).toBe("1");
    expect((document.getElementById("incident_state") as HTMLSelectElement).value).toBe("on_scene");
    expect(inputValue("incident_summary")).toBe("Dust storm");
    expect(inputValue("incident_location_name")).toBe("The Man");
    expect(inputValue("incident_location_address")).toBe("12:00 & A");
    expect(inputValue("incident_location_description")).toBe("by the fire");
    expect(document.title).toBe("#1 Dust storm | 2025");
    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(true);

    // The started datetime went into the flatpickr, and the timezone label is set.
    const fp = MockFlatpickr.instances.find((f: MockFlatpickr): boolean => f.input.id === "started_datetime")!;
    expect(fp.selectedDates[0]!.toISOString()).toBe("2025-08-25T10:00:00.000Z");
    expect(fp.altInput.id).toBe("alt_started_datetime");
    expect(document.getElementById("started_datetime_tz")!.textContent).not.toBe("");

    // attachFiles access unhides the attach button; the map link points at the
    // event's map URL.
    expect(document.getElementById("attach_file")!.classList.contains("hidden")).toBe(false);
    const mapLink = document.getElementById("map-link") as HTMLAnchorElement;
    expect(mapLink.href).toBe("https://map.example.com/2025");
    expect(mapLink.classList.contains("d-none")).toBe(false);
});

test("rangers and incident types are drawn with their add datalists", async (): Promise<void> => {
    await initIncidentPage();

    // Rangers are sorted by handle. Hot Slots has a directory_id, so it links
    // to the Clubhouse; Tool keeps its role in the role input.
    const rangerItems = [...document.querySelectorAll<HTMLLIElement>("#incident_rangers_list li")];
    expect(rangerItems.map((li: HTMLLIElement): string | undefined => li.dataset["rangerHandle"])).toEqual(["Hot Slots", "Tool"]);
    const hotSlotsLink = rangerItems[0]!.querySelector("a")!;
    expect(hotSlotsLink.textContent).toBe("Hot Slots");
    expect(hotSlotsLink.href).toBe("https://ranger-clubhouse.burningman.org/person/1234");
    expect(rangerItems[1]!.querySelector("input")!.value).toBe("lead");

    // The Ranger add datalist holds all personnel except auditors.
    const handleOptions = [...document.querySelectorAll<HTMLOptionElement>("#ranger_handles option")]
        .map((o: HTMLOptionElement): string => o.value);
    expect(handleOptions).toEqual(["", "Hot Slots", "Tool", "Tulsa"]);

    // The incident's one type is listed; the add datalist skips hidden types.
    const typeItems = [...document.querySelectorAll<HTMLLIElement>("#incident_types_list li")];
    expect(typeItems.length).toBe(1);
    expect(typeItems[0]!.querySelector("span")!.textContent).toBe("Junk");
    expect(typeItems[0]!.dataset["incidentTypeId"]).toBe("1");
    const typeOptions = [...document.querySelectorAll<HTMLOptionElement>("#incident_types option")]
        .map((o: HTMLOptionElement): string => o.value);
    expect(typeOptions).toEqual(["", "Junk", "Lost Child"]);

    // The type info modal lists visible types with descriptions.
    const infoNames = [...document.querySelectorAll("#incident-type-info .type-name")]
        .map((d: Element): string | null => d.textContent);
    expect(infoNames).toEqual(["Junk", "Lost Child"]);
});

test("the places datalist is drawn sorted, with type-specific labels", async (): Promise<void> => {
    await initIncidentPage();

    const options = [...document.querySelectorAll<HTMLOptionElement>("#places-list option")];
    expect(options.map((o: HTMLOptionElement): string => o.value)).toEqual([
        "",
        "Camp Friendly (5:00 & C)",
        "Disco Bus (MV)",
        "First Camp (??)",
        "Trash Fence Gallery (Art) (11:30 & K)",
    ]);
    expect(options[4]!.dataset["type"]).toBe("Art");
    expect(options[4]!.dataset["address"]).toBe("11:30 & K");
});

test("attached field reports and visits are listed, with the attach dropdown", async (): Promise<void> => {
    await initIncidentPage();

    const attached = [...document.querySelectorAll<HTMLLIElement>("#attached_field_reports li")];
    expect(attached.length).toBe(2);
    expect(attached[0]!.dataset["frNumber"]).toBe("7");
    expect(attached[0]!.querySelector("a")!.textContent).toBe("FR #7 (Tool): FR about storm");
    expect(attached[0]!.querySelector("a")!.getAttribute("href")).toBe("/ims/app/events/2025/field_reports/7");
    expect(attached[1]!.dataset["visitNumber"]).toBe("2");
    expect(attached[1]!.querySelector("a")!.textContent).toBe("VS #2: Sparkle");
    expect(attached[1]!.querySelector("a")!.getAttribute("href")).toBe("/ims/app/events/2025/visits/2");

    // The attach dropdown offers the unattached FR#8 and VS#3, plus FR#9 that
    // belongs to another incident, but not the ones already attached here.
    const select = document.getElementById("attached_field_report_add") as HTMLSelectElement;
    const values = [...select.options].map((o: HTMLOptionElement): string => o.value);
    expect(values).toEqual(["", "FR#8", "VS#3", "FR#9"]);
});

test("report entries from the incident, field reports, and visits are merged in order", async (): Promise<void> => {
    await initIncidentPage();

    const entries = [...document.querySelectorAll<HTMLDivElement>("#report_entries .report_entry")];
    const texts = entries.map((e: HTMLDivElement): string | null => e.querySelector(".report_entry_text")!.textContent);
    expect(texts).toEqual(["Changed state: on_scene", "Dust storm at the Man", "from the field", "visit note"]);

    expect(entries[0]!.classList.contains("report_entry_system")).toBe(true);
    expect(entries[1]!.classList.contains("report_entry_user")).toBe(true);
    expect(entries[2]!.classList.contains("report_entry_merged")).toBe(true);
    expect(entries[2]!.querySelector("a")!.textContent).toBe("field report #7");
    expect(entries[3]!.querySelector("a")!.textContent).toBe("VS #2");

    // History (system entries) is hidden until the checkbox is checked.
    expect(document.getElementById("report_entries")!.classList.contains("hide-history")).toBe(true);
});

test("linked incidents are drawn with links and metadata", async (): Promise<void> => {
    await initIncidentPage();

    const linked = [...document.querySelectorAll<HTMLElement>("#linked_incidents .list-group-item")];
    expect(linked.length).toBe(1);
    expect(linked[0]!.dataset["eventName"]).toBe("2025");
    expect(linked[0]!.dataset["incidentNumber"]).toBe("5");
    const link = linked[0]!.querySelector("a")!;
    expect(link.textContent).toBe("IMS 2025 #5: Related");
    expect(link.getAttribute("href")).toBe("/ims/app/events/2025/incidents/5");
});

test("an unauthorized user sees an error and no incident", async (): Promise<void> => {
    serverEventAccess.readIncidents = false;
    await initIncidentPage();

    expect(document.getElementById("error_text")!.textContent).toContain(
        `not currently authorized to view Incidents in Event "2025"`);
    expect(inputValue("incident_number")).toBe("");
});

test("editing the summary sends the edit and reloads the incident", async (): Promise<void> => {
    const mock = await initIncidentPage();

    const summary = document.getElementById("incident_summary") as HTMLInputElement;
    summary.value = "Bigger dust storm";
    await window.editIncidentSummary();

    expect(postedBodies(mock, "/ims/api/events/2025/incidents/1")).toEqual([
        { summary: "Bigger dust storm", number: 1 },
    ]);
    expect(summary.classList.contains("is-valid")).toBe(true);
});

test("field edits carry If-Match from the loaded ETag; note appends do not", async (): Promise<void> => {
    const mock = await initIncidentPage();
    const incidentURL = "/ims/api/events/2025/incidents/1";

    // A field edit sends the ETag captured when the incident was loaded.
    const summary = document.getElementById("incident_summary") as HTMLInputElement;
    summary.value = "Bigger dust storm";
    await window.editIncidentSummary();
    expect(postedIfMatches(mock, incidentURL)).toEqual([`"5"`]);

    // After the edit round-trip, the stored ETag is the server's new one.
    mock.mockClear();
    summary.value = "Even bigger dust storm";
    await window.editIncidentSummary();
    expect(postedIfMatches(mock, incidentURL)).toEqual([`"6"`]);

    // A report-entry append is unconditional: it can't lose data, and it must
    // not fail just because someone else edited a field.
    mock.mockClear();
    const textarea = document.getElementById("report_entry_add") as HTMLTextAreaElement;
    textarea.value = "saw a thing";
    window.reportEntryEdited();
    await window.submitReportEntry();
    expect(postedBodies(mock, incidentURL)).toEqual([
        { report_entries: [{ text: "saw a thing", id: -1 }], number: 1 },
    ]);
    expect(postedIfMatches(mock, incidentURL)).toEqual([null]);
});

test("a 412 conflict shows the conflict banner and refetches the incident", async (): Promise<void> => {
    const mock = await initIncidentPage();
    const incidentURL = "/ims/api/events/2025/incidents/1";

    // Someone else edited the incident: the next edit is rejected with a 412.
    const conflictRoutes = (url: string, init?: RequestInit): Response | undefined => {
        if (url === incidentURL && init?.body != null) {
            return problemResponse("Someone else got here first", 412);
        }
        return incidentRoutes(url, init);
    };
    mock.mockImplementation(async (url: string, init?: RequestInit): Promise<Response> => {
        const response = conflictRoutes(url, init);
        if (response == null) {
            throw new Error(`no mocked fetch route for ${url}`);
        }
        return response;
    });

    serverIncident.summary = "Their conflicting edit";
    const summary = document.getElementById("incident_summary") as HTMLInputElement;
    summary.value = "My rejected edit";
    const { err } = await window.editIncidentSummary().then((): {err: string|null} => ({err: null}), (e: Error): {err: string} => ({err: e.message}));
    expect(err).toBeNull();

    // The user is told what happened, and the page refetched the other
    // person's version of the incident.
    expect(document.getElementById("error_text")!.textContent).toContain(
        "Someone else has edited this incident");
    expect(inputValue("incident_summary")).toBe("Their conflicting edit");
});

test("editState warns when closing an incident that has no incident types", async (): Promise<void> => {
    serverIncident.incident_type_ids = [];
    const mock = await initIncidentPage();
    // happy-dom doesn't implement window.alert.
    const alertSpy = vi.fn();
    vi.stubGlobal("alert", alertSpy);

    const state = document.getElementById("incident_state") as HTMLSelectElement;
    state.value = "closed";
    await window.editState();

    expect(alertSpy).toHaveBeenCalledOnce();
    expect(alertSpy.mock.calls[0]![0]).toContain("Please add an incident type");
    expect(postedBodies(mock, "/ims/api/events/2025/incidents/1")).toEqual([
        { state: "closed", number: 1 },
    ]);
});

test("picking a date in the started flatpickr sends the new time", async (): Promise<void> => {
    const mock = await initIncidentPage();

    const fp = MockFlatpickr.instances.find((f: MockFlatpickr): boolean => f.input.id === "started_datetime")!;
    fp.setDate(new Date("2025-08-26T12:00:00Z"), true);

    await vi.waitFor((): void => {
        expect(postedBodies(mock, "/ims/api/events/2025/incidents/1")).toEqual([
            { started: "2025-08-26T12:00:00.000Z", number: 1 },
        ]);
    });
});

test("choosing a known place fills the location name and address", async (): Promise<void> => {
    const mock = await initIncidentPage();

    const name = document.getElementById("incident_location_name") as HTMLInputElement;
    name.value = "Camp Friendly (5:00 & C)";
    await window.editLocationName();
    expect(postedBodies(mock, "/ims/api/events/2025/incidents/1")).toEqual([
        { location: { name: "Camp Friendly", address: "5:00 & C" }, number: 1 },
    ]);

    // Art places get an " (Art)" suffix on the name.
    mock.mockClear();
    name.value = "Trash Fence Gallery (Art) (11:30 & K)";
    await window.editLocationName();
    expect(postedBodies(mock, "/ims/api/events/2025/incidents/1")).toEqual([
        { location: { name: "Trash Fence Gallery (Art)", address: "11:30 & K" }, number: 1 },
    ]);

    // Free-form location names just edit location.name.
    mock.mockClear();
    name.value = "Somewhere odd";
    await window.editLocationName();
    expect(postedBodies(mock, "/ims/api/events/2025/incidents/1")).toEqual([
        { location: { name: "Somewhere odd" }, number: 1 },
    ]);

    mock.mockClear();
    const address = document.getElementById("incident_location_address") as HTMLInputElement;
    address.value = "9:00 & B";
    await window.editLocationAddress();
    expect(postedBodies(mock, "/ims/api/events/2025/incidents/1")).toEqual([
        { location: { address: "9:00 & B" }, number: 1 },
    ]);
});

test("addRanger fuzzy-matches handles and posts to the rangers endpoint", async (): Promise<void> => {
    const mock = await initIncidentPage();

    const rangerAdd = document.getElementById("ranger_add") as HTMLInputElement;
    rangerAdd.value = "  tULSa ";
    await window.addRanger();

    expect(postedBodies(mock, "/ims/api/events/2025/incidents/1/rangers/Tulsa")).toEqual([
        { handle: "Tulsa" },
    ]);
    expect(rangerAdd.value).toBe("");
    expect(rangerAdd.disabled).toBe(false);

    // A Ranger who is already on the incident isn't re-added.
    mock.mockClear();
    rangerAdd.value = "Hot Slots";
    await window.addRanger();
    expect(mock.mock.calls.length).toBe(0);
    expect(rangerAdd.value).toBe("");

    // Nor is a handle that doesn't exist.
    rangerAdd.value = "nobody";
    await window.addRanger();
    expect(mock.mock.calls.length).toBe(0);
});

test("removeRanger and setRangerRole hit the per-Ranger endpoint", async (): Promise<void> => {
    const mock = await initIncidentPage();

    const toolLi = document.querySelector<HTMLLIElement>('#incident_rangers_list li[data-ranger-handle="Tool"]')!;
    window.removeRanger(toolLi.querySelector("button")!);
    await vi.waitFor((): void => {
        const del = mock.mock.calls.find(
            ([url, init]): boolean =>
                url === "/ims/api/events/2025/incidents/1/rangers/Tool" && init?.method === "DELETE");
        expect(del).toBeDefined();
    });

    const roleInput = toolLi.querySelector("input")!;
    roleInput.value = "medic";
    await window.setRangerRole(roleInput);
    expect(postedBodies(mock, "/ims/api/events/2025/incidents/1/rangers/Tool")).toEqual([
        { handle: "Tool", role: "medic" },
    ]);
    expect(roleInput.classList.contains("is-valid")).toBe(true);
});

test("addIncidentType and removeIncidentType edit the incident's type list", async (): Promise<void> => {
    const mock = await initIncidentPage();

    const typeAdd = document.getElementById("incident_type_add") as HTMLInputElement;
    typeAdd.value = " lost child ";
    await window.addIncidentType();
    expect(postedBodies(mock, "/ims/api/events/2025/incidents/1")).toEqual([
        { incident_type_ids: [1, 2], number: 1 },
    ]);
    expect(typeAdd.value).toBe("");

    mock.mockClear();
    const junkLi = document.querySelector<HTMLLIElement>('#incident_types_list li[data-incident-type-id="1"]')!;
    await window.removeIncidentType(junkLi.querySelector("button")!);
    expect(postedBodies(mock, "/ims/api/events/2025/incidents/1")).toEqual([
        { incident_type_ids: [], number: 1 },
    ]);
});

test("submitReportEntry posts the new entry and clears the textarea", async (): Promise<void> => {
    const mock = await initIncidentPage();

    const textarea = document.getElementById("report_entry_add") as HTMLTextAreaElement;
    const submit = document.getElementById("report_entry_submit")!;
    textarea.value = "saw a thing";
    window.reportEntryEdited();
    expect(submit.classList.contains("disabled")).toBe(false);

    await window.submitReportEntry();
    expect(postedBodies(mock, "/ims/api/events/2025/incidents/1")).toEqual([
        { report_entries: [{ text: "saw a thing", id: -1 }], number: 1 },
    ]);
    expect(textarea.value).toBe("");
    expect(submit.classList.contains("disabled")).toBe(true);
});

test("the strike button strikes a report entry", async (): Promise<void> => {
    const mock = await initIncidentPage();

    // The non-system incident entry (id 2) gets a Strike button.
    const userEntry = document.querySelector<HTMLDivElement>("#report_entries .report_entry_user")!;
    const strike = userEntry.querySelector("button")!;
    expect(strike.textContent).toBe("Strike");
    strike.click();

    await vi.waitFor((): void => {
        expect(postedBodies(mock, "/ims/api/events/2025/incidents/1/report_entries/2")).toEqual([
            { stricken: true },
        ]);
    });
});

test("attachFieldReport and detachFieldReport use the field report action URLs", async (): Promise<void> => {
    const mock = await initIncidentPage();

    const select = document.getElementById("attached_field_report_add") as HTMLSelectElement;
    select.value = "FR#8";
    await window.attachFieldReport();
    const attach = mock.mock.calls.find(
        ([url]): boolean =>
            url === "/ims/api/events/2025/field_reports/8?action=attach&incident=1");
    expect(attach).toBeDefined();

    const frLi = document.querySelector<HTMLLIElement>('#attached_field_reports li[data-fr-number="7"]')!;
    await window.detachFieldReport(frLi.querySelector("button")!);
    const detach = mock.mock.calls.find(
        ([url]): boolean =>
            url === "/ims/api/events/2025/field_reports/7?action=detach&incident=1");
    expect(detach).toBeDefined();
});

test("visits attach and detach through the visits endpoint", async (): Promise<void> => {
    const mock = await initIncidentPage();

    const select = document.getElementById("attached_field_report_add") as HTMLSelectElement;
    select.value = "VS#3";
    await window.attachFieldReport();
    expect(postedBodies(mock, "/ims/api/events/2025/visits/3")).toEqual([
        { event: "2025", number: 3, incident: 1 },
    ]);

    mock.mockClear();
    const visitLi = document.querySelector<HTMLLIElement>('#attached_field_reports li[data-visit-number="2"]')!;
    await window.detachFieldReport(visitLi.querySelector("button")!);
    expect(postedBodies(mock, "/ims/api/events/2025/visits/2")).toEqual([
        { event: "2025", number: 2, incident: 0 },
    ]);
});

test("linkIncident links incidents in other events and rejects bad input", async (): Promise<void> => {
    const mock = await initIncidentPage();

    const input = document.getElementById("linked_incident_add") as HTMLInputElement;
    input.value = "2015#2";
    await window.linkIncident(input);
    expect(postedBodies(mock, "/ims/api/events/2025/incidents/1")).toEqual([
        {
            linked_incidents: [
                { event_id: 1, event_name: "2025", number: 5, summary: "Related" },
                { event_id: 2, number: 2 },
            ],
            number: 1,
        },
    ]);
    expect(input.value).toBe("");

    // An unknown event name is an error and sends nothing.
    mock.mockClear();
    input.value = "nowhere#3";
    await window.linkIncident(input);
    expect(mock.mock.calls.length).toBe(0);
    expect(document.getElementById("error_text")!.textContent).toContain("no Event for name 'nowhere'");

    // Linking the incident to itself is rejected.
    input.value = "1";
    await window.linkIncident(input);
    expect(mock.mock.calls.length).toBe(0);
    expect(document.getElementById("error_text")!.textContent).toContain("No valid other incidents");
});

test("unlinkIncident removes the linked incident", async (): Promise<void> => {
    const mock = await initIncidentPage();

    const linkedLi = document.querySelector<HTMLElement>('#linked_incidents [data-incident-number="5"]')!;
    await window.unlinkIncident(linkedLi.querySelector("button")!);
    expect(postedBodies(mock, "/ims/api/events/2025/incidents/1")).toEqual([
        { linked_incidents: [], number: 1 },
    ]);
});

test("a new incident is created on the first edit and adopts the server's number", async (): Promise<void> => {
    window.history.replaceState(null, "", "/ims/app/events/2025/incidents/new");
    const mock = await initIncidentPage();

    expect(inputValue("incident_number")).toBe("(new)");
    expect(document.title).toBe("New Incident | 2025");

    const summary = document.getElementById("incident_summary") as HTMLInputElement;
    summary.value = "Fresh incident";
    await window.editIncidentSummary();

    // Creation fills in the required state/priority fields.
    expect(postedBodies(mock, "/ims/api/events/2025/incidents")).toEqual([
        { summary: "Fresh incident", state: "new", priority: 3 },
    ]);
    await vi.waitFor((): void => {
        expect(inputValue("incident_number")).toBe("42");
    });
    expect(window.location.pathname).toBe("/ims/app/events/2025/incidents/42");
});

test("an incident broadcast for this incident reloads it", async (): Promise<void> => {
    await initIncidentPage();

    serverIncident.summary = "Updated by someone else";
    const channel = new BroadcastChannel("incident_update");
    channel.postMessage({ event_id: eventId, incident_number: 1 });

    await vi.waitFor((): void => {
        expect(inputValue("incident_summary")).toBe("Updated by someone else");
    });
    channel.close();
});

test("a broadcast redraw does not clobber a field the user is typing in", async (): Promise<void> => {
    await initIncidentPage();

    // The user is mid-typing in the location name field.
    const locationName = document.getElementById("incident_location_name") as HTMLInputElement;
    locationName.focus();
    locationName.value = "Camp Half-Typed";
    locationName.dispatchEvent(new Event("input", { bubbles: true }));

    // Meanwhile another client updates the incident, broadcasting a reload.
    serverIncident.summary = "Updated by someone else";
    const channel = new BroadcastChannel("incident_update");
    channel.postMessage({ event_id: eventId, incident_number: 1 });

    // The redraw applied the remote change to the unfocused summary field...
    await vi.waitFor((): void => {
        expect(inputValue("incident_summary")).toBe("Updated by someone else");
    });
    // ...but left the focused field's in-progress text alone.
    expect(locationName.value).toBe("Camp Half-Typed");
    channel.close();
});

test("a field report broadcast refreshes that one field report", async (): Promise<void> => {
    await initIncidentPage();

    // FR#8 got attached to this incident elsewhere.
    serverFieldReports[1] = { number: 8, incident: 1, summary: "Now attached", report_entries: [] };
    const channel = new BroadcastChannel("field_report_update");
    channel.postMessage({ event_id: eventId, field_report_number: 8 });

    await vi.waitFor((): void => {
        expect(document.querySelector('#attached_field_reports li[data-fr-number="8"]')).not.toBeNull();
    });
    channel.close();
});

test("keyboard shortcuts toggle history and jump to the entry box", async (): Promise<void> => {
    await initIncidentPage();

    const checkbox = document.getElementById("history_checkbox") as HTMLInputElement;
    expect(checkbox.checked).toBe(false);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "h", bubbles: true }));
    expect(checkbox.checked).toBe(true);
    // The checkbox's inline onchange doesn't run in happy-dom, so invoke it
    // the way the page would.
    window.toggleShowHistory();
    expect(document.getElementById("report_entries")!.classList.contains("hide-history")).toBe(false);

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "a", bubbles: true }));
    expect(document.activeElement!.id).toBe("report_entry_add");
});

test("printing swaps in a filesystem-safe document title", async (): Promise<void> => {
    await initIncidentPage();

    window.dispatchEvent(new Event("beforeprint"));
    expect(document.title).toBe("IMS-2025-1_Dust-storm");
    window.dispatchEvent(new Event("afterprint"));
    expect(document.title).toBe("#1 Dust storm | 2025");
});

test("attachFile posts the file form data to the attachments endpoint", async (): Promise<void> => {
    const mock = await initIncidentPage();

    await window.attachFile();
    const attach = mock.mock.calls.find(
        ([url, init]): boolean =>
            url === "/ims/api/events/2025/incidents/1/attachments" && init?.body instanceof FormData);
    expect(attach).toBeDefined();
});

test("attachFile shows an uploading state, posts the file, then confirms and reverts", async (): Promise<void> => {
    const mock = await initIncidentPage();
    const button = document.getElementById("attach_file") as HTMLInputElement;
    expect(button.value).toBe("Attach file");

    vi.useFakeTimers();
    try {
        // The synchronous prefix of attachFile disables the button and relabels
        // it before the upload fetch is awaited.
        const pending = window.attachFile();
        expect(button.disabled).toBe(true);
        expect(button.value).toBe("Uploading...");

        await pending;

        // The file form data went to the attachments endpoint.
        expect(mock.mock.calls.some(([url, init]) =>
            url === `/ims/api/events/${eventName}/incidents/1/attachments` && init?.body instanceof FormData)).toBe(true);

        // On success the button re-enables and briefly confirms.
        expect(button.disabled).toBe(false);
        expect(button.value).toBe("Uploaded ✓");

        // The confirmation reverts to the default label after a moment.
        vi.advanceTimersByTime(2000);
        expect(button.value).toBe("Attach file");
    } finally {
        vi.useRealTimers();
    }
});

test("a failed attachment re-enables the button and surfaces the error", async (): Promise<void> => {
    await initIncidentPage((url, init) => {
        if (url === `/ims/api/events/${eventName}/incidents/1/attachments` && init?.body != null) {
            return undefined;
        }
        return incidentRoutes(url, init);
    });
    const button = document.getElementById("attach_file") as HTMLInputElement;

    await window.attachFile();

    // The button is left usable, keeps its default label (no success), and the
    // failure is shown to the user.
    expect(button.disabled).toBe(false);
    expect(button.value).toBe("Attach file");
    expect(document.getElementById("error_text")!.textContent).toContain("Failed to attach file");
});
