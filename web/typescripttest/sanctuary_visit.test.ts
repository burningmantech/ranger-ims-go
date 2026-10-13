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

// Tests for sanctuary_visit.ts against the real templ-rendered sanctuary visit
// page (sanctuary_visit.templ).

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { change, jsonResponse, loadFixture, MockFlatpickr, mockFetch, mockXHR, problemResponse } from "./helpers.ts";

let page: typeof import("../typescript/sanctuary_visit.ts");

const eventName = "2025";
const eventId = 1;
const visitsUrl = `/ims/api/events/${eventName}/visits`;

let serverEventAccess: ims.AuthInfoEventAccess;
let serverVisit: ims.Visit;
let serverPersonnel: ims.Personnel[];
let serverEvents: ims.EventData[];

beforeEach((): void => {
    vi.resetModules();
    loadFixture("sanctuary_visit.html");
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/visits/2`);

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
    serverVisit = {
        number: 2,
        guest_preferred_name: "Sparkle",
        guest_legal_name: "Pat Doe",
        arrival_state: "calm",
        rangers: [{ handle: "Hot Slots" }],
        report_entries: [
            { id: 1, created: "2025-08-25T10:00:00Z", author: "Moon Dog", text: "checked in", system_entry: false },
        ],
    };
    serverPersonnel = [
        { handle: "Hot Slots", status: "active", directory_id: 1 },
        { handle: "Tool", status: "active", directory_id: 2 },
    ];
    serverEvents = [{ id: eventId, name: eventName }];
});

function visitRoutes(url: string, init?: RequestInit): Response | undefined {
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
    if (url === `${visitsUrl}/2` && !hasBody) {
        return jsonResponse(serverVisit, 200);
    }
    if (url === `${visitsUrl}/2` && hasBody) {
        return new Response(null, { status: 204 });
    }
    if (url.startsWith(`${visitsUrl}/2/rangers/`) && hasBody) {
        return new Response(null, { status: 204 });
    }
    if (url.startsWith(`${visitsUrl}/2/rangers/`) && init?.method === "DELETE") {
        return new Response(null, { status: 204 });
    }
    if (url === `${visitsUrl}/2/attachments` && hasBody) {
        return new Response(null, { status: 200 });
    }
    return undefined;
}

async function initVisitPage(handler: (url: string, init?: RequestInit) => Response | undefined = visitRoutes) {
    const mock = mockFetch(handler);
    page = await import("../typescript/sanctuary_visit.ts");
    await vi.waitFor((): void => {
        expect(document.getElementById("loading-overlay")!.style.display).toBe("none");
    });
    return mock;
}

function inputValue(id: string): string {
    return (document.getElementById(id) as HTMLInputElement).value;
}

// Types the given value into a field, fires its change event, and returns the
// JSON body of the resulting edit POST. Each field's listener maps one control
// to one key in the visit payload, and that mapping is what these tests check.
async function editField(
    mock: ReturnType<typeof mockFetch>,
    id: string,
    value: string,
): Promise<Record<string, unknown>> {
    mock.mockClear();
    const field = document.getElementById(id) as HTMLInputElement;
    field.value = value;
    change(field);
    const call = await vi.waitFor(() => {
        const call = mock.mock.calls.find(
            ([url, init]) => url === `${visitsUrl}/2` && init?.body != null);
        if (call == null) {
            throw new Error(`editing ${id} sent no edit request`);
        }
        return call;
    });
    return JSON.parse(call[1]!.body as string) as Record<string, unknown>;
}

test("page init draws the visit's number, names, and arrival state from the API", async (): Promise<void> => {
    await initVisitPage();

    expect(inputValue("visit_number")).toBe("2");
    expect(inputValue("guest_preferred_name")).toBe("Sparkle");
    expect(inputValue("guest_legal_name")).toBe("Pat Doe");
    expect(inputValue("arrival_state")).toBe("calm");
    expect(document.title).toContain("2025");
    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(true);
});

function expanded(sectionId: string): boolean {
    return (document.getElementById(sectionId) as HTMLDetailsElement).open;
}

test("sections holding data start open, empty ones stay closed", async (): Promise<void> => {
    serverVisit.resource_bed_id = "B4";
    await initVisitPage();

    // Arrival state, a bed, and a Ranger are all set on this visit.
    expect(expanded("collapse-arrival")).toBe(true);
    expect(expanded("collapse-resources")).toBe(true);
    expect(expanded("collapse-rangers")).toBe(true);
    // No camp details were recorded.
    expect(expanded("collapse-camp")).toBe(false);
    // Instructions is prose, not data, so it stays shut regardless.
    expect(expanded("collapse-instructions")).toBe(false);
});

test("a visit with nothing recorded leaves every collapsed section closed", async (): Promise<void> => {
    serverVisit = { number: 2 };
    await initVisitPage();

    expect(expanded("collapse-arrival")).toBe(false);
    expect(expanded("collapse-camp")).toBe(false);
    expect(expanded("collapse-resources")).toBe(false);
    expect(expanded("collapse-rangers")).toBe(false);
});

test("editing the guest's preferred name posts the change to the visit", async (): Promise<void> => {
    const mock = await initVisitPage();

    const nameField = document.getElementById("guest_preferred_name") as HTMLInputElement;
    nameField.value = "Stardust";
    await page.editGuestPreferredName();

    const editCall = mock.mock.calls.find(([url, init]) => url === `${visitsUrl}/2` && init?.body != null)!;
    expect(JSON.parse(editCall[1]!.body as string)).toMatchObject({ guest_preferred_name: "Stardust" });
});

test("adding a known ranger posts to that visit's ranger endpoint", async (): Promise<void> => {
    const mock = await initVisitPage();

    const addField = document.getElementById("ranger_add") as HTMLInputElement;
    addField.value = "Tool";
    await page.addRanger();

    expect(mock.mock.calls.some(([url, init]) =>
        url === `${visitsUrl}/2/rangers/Tool` && init?.body != null)).toBe(true);
});

test("adding an unknown ranger makes no request and clears the field", async (): Promise<void> => {
    const mock = await initVisitPage();

    const addField = document.getElementById("ranger_add") as HTMLInputElement;
    addField.value = "Nonexistent Person";
    await page.addRanger();

    expect(addField.value).toBe("");
    expect(mock.mock.calls.some(([url]) => (url as string).includes("/rangers/"))).toBe(false);
});

test("a viewer without visit read access sees an authorization error", async (): Promise<void> => {
    serverEventAccess.readVisits = false;

    await initVisitPage();

    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    expect(document.getElementById("error_text")!.textContent).toContain("not currently authorized");
});

test("an entry's attachment links to the visit's own endpoint", async (): Promise<void> => {
    serverVisit.report_entries![0]!.attachment = { name: "checkin.jpg", previewable: true };
    await initVisitPage();

    const entry = document.querySelector<HTMLDivElement>("#report_entries .report_entry")!;
    const links = [...entry.querySelectorAll("a")];
    const preview = links.find((a: HTMLAnchorElement): boolean => (a.textContent ?? "").includes("Preview"))!;
    const download = links.find((a: HTMLAnchorElement): boolean => (a.textContent ?? "").includes("Download"))!;
    expect(preview.getAttribute("href")).toBe(`${visitsUrl}/2/attachments/1`);
    expect(preview.target).toBe("_blank");
    expect(download.getAttribute("href")).toBe(`${visitsUrl}/2/attachments/1?download=true`);
});

test("attachFile shows an uploading state, posts the file, then confirms and reverts", async (): Promise<void> => {
    await initVisitPage();
    const button = document.getElementById("attach_file") as HTMLInputElement;
    expect(button.value).toBe("Attach file");

    const labels: string[] = [];
    const uploads = mockXHR(
        (url) => url === `${visitsUrl}/2/attachments` ? new Response(null, { status: 204 }) : undefined,
        { progress: [0.25, 1], onProgress: (): void => { labels.push(button.value); } },
    );

    vi.useFakeTimers();
    try {
        // The synchronous prefix of attachFile disables the button and relabels
        // it before the upload is awaited.
        const pending = page.attachFile();
        expect(button.disabled).toBe(true);
        expect(button.value).toBe("Uploading ...");

        await pending;

        // The file form data went to the attachments endpoint.
        expect(uploads.length).toBe(1);
        expect(uploads[0]!.url).toBe(`${visitsUrl}/2/attachments`);
        expect(uploads[0]!.body).toBeInstanceOf(FormData);

        // The button tracked the upload, then waited on the server to store it.
        expect(labels).toEqual(["Uploading 25%", "Uploading 100%", "Uploading ..."]);

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
    await initVisitPage();
    mockXHR(() => undefined);
    const button = document.getElementById("attach_file") as HTMLInputElement;

    await page.attachFile();

    // The button is left usable, keeps its default label (no success), and the
    // failure is shown to the user.
    expect(button.disabled).toBe(false);
    expect(button.value).toBe("Attach file");
    expect(document.getElementById("error_text")!.textContent).toContain("Failed to attach file");
});

test("a broadcast redraw does not clobber a field the user is typing in", async (): Promise<void> => {
    await initVisitPage();

    // The user is mid-typing in the camp name field.
    const campName = document.getElementById("guest_camp_name") as HTMLInputElement;
    campName.focus();
    campName.value = "Camp Half-Typed";
    campName.dispatchEvent(new Event("input", { bubbles: true }));

    // Meanwhile another client updates the visit, which broadcasts a reload.
    serverVisit.guest_legal_name = "Updated Elsewhere";
    const channel = new BroadcastChannel("visit_update");
    channel.postMessage({ event_id: eventId, visit_number: 2 });

    // The redraw applied the remote change to the unfocused field...
    await vi.waitFor((): void => {
        expect(inputValue("guest_legal_name")).toBe("Updated Elsewhere");
    });
    // ...but left the focused field's in-progress text alone.
    expect(campName.value).toBe("Camp Half-Typed");

    // Once the field is blurred, a later redraw applies server state again.
    campName.blur();
    serverVisit.guest_camp_name = "Camp Committed Elsewhere";
    channel.postMessage({ event_id: eventId, visit_number: 2 });
    await vi.waitFor((): void => {
        expect(campName.value).toBe("Camp Committed Elsewhere");
    });
    channel.close();
});

test("a broadcast redraw does not clobber a datetime the user is typing in", async (): Promise<void> => {
    await initVisitPage();

    // The user is mid-typing in the arrival time's flatpickr text field.
    const arrivalTime = document.getElementById("alt_arrival_time") as HTMLInputElement;
    arrivalTime.focus();
    arrivalTime.value = "2025-08-25 13:0";
    arrivalTime.dispatchEvent(new Event("input", { bubbles: true }));

    // Meanwhile another client sets the arrival time, broadcasting a reload.
    serverVisit.arrival_time = "2025-08-25T12:00:00Z";
    serverVisit.guest_legal_name = "Updated Elsewhere";
    const channel = new BroadcastChannel("visit_update");
    channel.postMessage({ event_id: eventId, visit_number: 2 });

    // The redraw applied the remote change to the unfocused field...
    await vi.waitFor((): void => {
        expect(inputValue("guest_legal_name")).toBe("Updated Elsewhere");
    });
    // ...but left the focused datetime's in-progress text alone.
    expect(arrivalTime.value).toBe("2025-08-25 13:0");

    // Once the field is blurred, a later redraw applies server state again.
    arrivalTime.blur();
    channel.postMessage({ event_id: eventId, visit_number: 2 });
    await vi.waitFor((): void => {
        expect(arrivalTime.value).toBe("2025-08-25T12:00:00.000Z");
    });
    channel.close();
});

test("a broadcast redraw still updates a focused field the user hasn't typed in", async (): Promise<void> => {
    await initVisitPage();

    // The user has focused the camp name field, but hasn't typed anything.
    const campName = document.getElementById("guest_camp_name") as HTMLInputElement;
    campName.focus();

    // Another client updates that same field, which broadcasts a reload.
    serverVisit.guest_camp_name = "Camp Updated Elsewhere";
    const channel = new BroadcastChannel("visit_update");
    channel.postMessage({ event_id: eventId, visit_number: 2 });

    // The remote change lands even though the field is focused.
    await vi.waitFor((): void => {
        expect(campName.value).toBe("Camp Updated Elsewhere");
    });
    expect(document.activeElement).toBe(campName);
    channel.close();
});

test("editing each guest identity field posts its own key", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "guest_legal_name", "Pat Q. Doe"))
        .toMatchObject({ guest_legal_name: "Pat Q. Doe" });
    expect(await editField(mock, "guest_description", "tall, red hat"))
        .toMatchObject({ guest_description: "tall, red hat" });
    expect(await editField(mock, "guest_action_plan", "rest, then campmates collect them"))
        .toMatchObject({ guest_action_plan: "rest, then campmates collect them" });
});

test("editing each camp field posts its own key", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "guest_camp_name", "Camp Questionable"))
        .toMatchObject({ guest_camp_name: "Camp Questionable" });
    expect(await editField(mock, "guest_camp_address", "7:30 & E"))
        .toMatchObject({ guest_camp_address: "7:30 & E" });
    expect(await editField(mock, "guest_camp_description", "big purple dome"))
        .toMatchObject({ guest_camp_description: "big purple dome" });
    expect(await editField(mock, "guest_camp_contacts", "Moon Dog, tent 3"))
        .toMatchObject({ guest_camp_contacts: "Moon Dog, tent 3" });
});

test("the camp address input is rewritten with the server's normalized address", async (): Promise<void> => {
    await initVisitPage((url: string, init?: RequestInit): Response|undefined => {
        const body = init?.body != null ? JSON.parse(init.body as string) : null;
        if (body?.guest_camp_address != null) {
            // Stand in for the server's address normalization.
            serverVisit.guest_camp_address = "7:00 & E";
        }
        return visitRoutes(url, init);
    });

    // Type into the field and save it without leaving it, as pressing Enter does.
    const address = document.getElementById("guest_camp_address") as HTMLInputElement;
    address.focus();
    address.value = "7+e";
    address.dispatchEvent(new Event("input", { bubbles: true }));
    await page.editGuestCampAddress();

    expect(address.value).toBe("7:00 & E");
});

test("editing each arrival field posts its own key", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "arrival_method", "walked in"))
        .toMatchObject({ arrival_method: "walked in" });
    expect(await editField(mock, "arrival_state", "agitated"))
        .toMatchObject({ arrival_state: "agitated" });
    expect(await editField(mock, "arrival_reason", "too much sun"))
        .toMatchObject({ arrival_reason: "too much sun" });
    expect(await editField(mock, "arrival_belongings", "backpack, water bottle"))
        .toMatchObject({ arrival_belongings: "backpack, water bottle" });
});

test("editing each departure field posts its own key", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "departure_method", "left with campmate"))
        .toMatchObject({ departure_method: "left with campmate" });
    expect(await editField(mock, "departure_state", "calm and oriented"))
        .toMatchObject({ departure_state: "calm and oriented" });
});

test("editing each resource field posts its own key", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "resource_sitter", "Hot Slots"))
        .toMatchObject({ resource_sitter: "Hot Slots" });
    expect(await editField(mock, "resource_bed_id", "B4"))
        .toMatchObject({ resource_bed_id: "B4" });
    expect(await editField(mock, "resource_rest", "two blankets"))
        .toMatchObject({ resource_rest: "two blankets" });
    expect(await editField(mock, "resource_clothes", "a jacket"))
        .toMatchObject({ resource_clothes: "a jacket" });
    expect(await editField(mock, "resource_pogs", "1 meal pog"))
        .toMatchObject({ resource_pogs: "1 meal pog" });
    expect(await editField(mock, "resource_food_bev", "electrolytes"))
        .toMatchObject({ resource_food_bev: "electrolytes" });
    expect(await editField(mock, "resource_other", "shower pog"))
        .toMatchObject({ resource_other: "shower pog" });
});

test("editing the parent incident sends the number as an integer", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "parent_incident", "17"))
        .toMatchObject({ incident: 17 });
});

// Clearing the field detaches the visit, which the API models as incident 0
// rather than a missing key.
test("clearing the parent incident sends zero", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "parent_incident", ""))
        .toMatchObject({ incident: 0 });
});

// parseInt10 returns null for unparseable text, and editFromElement turns a null
// into "" rather than dropping the key. The server rejects that, so the visit is
// left unchanged.
test("a non-numeric parent incident sends an empty value, not a bad number", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "parent_incident", "not a number"))
        .toMatchObject({ incident: "" });
});

test("removing a ranger DELETEs that ranger's endpoint", async (): Promise<void> => {
    const mock = await initVisitPage();

    const removeButton = document
        .getElementById("visit_rangers_list")!
        .querySelector("li button") as HTMLButtonElement;
    expect(removeButton.ariaLabel).toBe("Remove Ranger Hot Slots");

    mock.mockClear();
    await page.removeRanger(removeButton);

    expect(mock.mock.calls.some(([url, init]) =>
        url === `${visitsUrl}/2/rangers/Hot%20Slots` && init?.method === "DELETE")).toBe(true);
});

test("setting a ranger's role posts the handle and the role", async (): Promise<void> => {
    const mock = await initVisitPage();

    const roleInput = document
        .getElementById("visit_rangers_list")!
        .querySelector("li input") as HTMLInputElement;

    mock.mockClear();
    roleInput.value = "sitter";
    await page.setRangerRole(roleInput);

    const call = mock.mock.calls.find(([url, init]) =>
        url === `${visitsUrl}/2/rangers/Hot%20Slots` && init?.body != null)!;
    expect(call).toBeDefined();
    expect(JSON.parse(call[1]!.body as string)).toMatchObject({
        handle: "Hot Slots",
        role: "sitter",
    });
});

// The arrival/departure pickers post through flatpickr's onChange rather than a
// change listener, so these drive the picker instance directly.
function flatpickrFor(id: string): MockFlatpickr {
    const instance = MockFlatpickr.instances.find((fp): boolean => fp.input.id === id);
    if (instance == null) {
        throw new Error(`no flatpickr was created for ${id}`);
    }
    return instance;
}

test("picking an arrival time posts it as an ISO timestamp", async (): Promise<void> => {
    const mock = await initVisitPage();

    mock.mockClear();
    flatpickrFor("arrival_time").setDate(new Date("2025-08-25T12:00:00Z"), true);
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([u, init]) =>
            u === `${visitsUrl}/2` && init?.body != null)).toBe(true);
    });

    const post = mock.mock.calls.find(([u, init]) => u === `${visitsUrl}/2` && init?.body != null)!;
    expect(JSON.parse(post[1]!.body as string)).toMatchObject({
        arrival_time: "2025-08-25T12:00:00.000Z",
    });
});

// Re-selecting the time that's already stored would otherwise burn a version bump
// and an action log entry for no change.
test("re-picking the arrival time already stored sends nothing", async (): Promise<void> => {
    serverVisit.arrival_time = "2025-08-25T12:00:00Z";
    const mock = await initVisitPage();

    mock.mockClear();
    flatpickrFor("arrival_time").setDate(new Date("2025-08-25T12:00:00Z"), true);

    expect(mock.mock.calls.some(([u, init]) =>
        u === `${visitsUrl}/2` && init?.body != null)).toBe(false);
});

// Clearing sends Go's zero time, since omitting the key would mean "unchanged".
test("clearing the arrival time posts the zero time value", async (): Promise<void> => {
    serverVisit.arrival_time = "2025-08-25T12:00:00Z";
    const mock = await initVisitPage();

    mock.mockClear();
    flatpickrFor("arrival_time").clear(true);
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([u, init]) =>
            u === `${visitsUrl}/2` && init?.body != null)).toBe(true);
    });

    const post = mock.mock.calls.find(([u, init]) => u === `${visitsUrl}/2` && init?.body != null)!;
    expect(JSON.parse(post[1]!.body as string)).toMatchObject({
        arrival_time: "0001-01-01T00:00:00Z",
    });
});

test("picking a departure time posts it as an ISO timestamp", async (): Promise<void> => {
    const mock = await initVisitPage();

    mock.mockClear();
    flatpickrFor("departure_time").setDate(new Date("2025-08-25T18:30:00Z"), true);
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([u, init]) =>
            u === `${visitsUrl}/2` && init?.body != null)).toBe(true);
    });

    const post = mock.mock.calls.find(([u, init]) => u === `${visitsUrl}/2` && init?.body != null)!;
    expect(JSON.parse(post[1]!.body as string)).toMatchObject({
        departure_time: "2025-08-25T18:30:00.000Z",
    });
});

test("clearing the departure time posts the zero time value", async (): Promise<void> => {
    serverVisit.departure_time = "2025-08-25T18:30:00Z";
    const mock = await initVisitPage();

    mock.mockClear();
    flatpickrFor("departure_time").clear(true);
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([u, init]) =>
            u === `${visitsUrl}/2` && init?.body != null)).toBe(true);
    });

    const post = mock.mock.calls.find(([u, init]) => u === `${visitsUrl}/2` && init?.body != null)!;
    expect(JSON.parse(post[1]!.body as string)).toMatchObject({
        departure_time: "0001-01-01T00:00:00Z",
    });
});

function clickButton(id: string): void {
    const button = document.getElementById(id);
    if (!(button instanceof HTMLButtonElement)) {
        throw new Error(`no button element with id ${id}`);
    }
    button.click();
}

test("the departure time's Now button posts the current time", async (): Promise<void> => {
    const mock = await initVisitPage();

    mock.mockClear();
    const before = Date.now();
    clickButton("departure_time_now");
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([u, init]) =>
            u === `${visitsUrl}/2` && init?.body != null)).toBe(true);
    });

    const post = mock.mock.calls.find(([u, init]) => u === `${visitsUrl}/2` && init?.body != null)!;
    const posted = new Date(JSON.parse(post[1]!.body as string).departure_time).getTime();
    expect(posted).toBeGreaterThanOrEqual(before);
    expect(posted).toBeLessThanOrEqual(Date.now());
});

// Overwriting an already-set time is the whole point for corrections, so Now
// must not quietly do nothing when the field is populated.
test("the arrival time's Now button overwrites a time that's already set", async (): Promise<void> => {
    serverVisit.arrival_time = "2025-08-25T12:00:00Z";
    const mock = await initVisitPage();

    mock.mockClear();
    const before = Date.now();
    clickButton("arrival_time_now");
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([u, init]) =>
            u === `${visitsUrl}/2` && init?.body != null)).toBe(true);
    });

    const post = mock.mock.calls.find(([u, init]) => u === `${visitsUrl}/2` && init?.body != null)!;
    const posted = new Date(JSON.parse(post[1]!.body as string).arrival_time).getTime();
    expect(posted).toBeGreaterThanOrEqual(before);
    expect(posted).toBeLessThanOrEqual(Date.now());
});

test("the arrival time's clear button posts the zero time value", async (): Promise<void> => {
    serverVisit.arrival_time = "2025-08-25T12:00:00Z";
    const mock = await initVisitPage();

    mock.mockClear();
    clickButton("arrival_time_clear");
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([u, init]) =>
            u === `${visitsUrl}/2` && init?.body != null)).toBe(true);
    });

    const post = mock.mock.calls.find(([u, init]) => u === `${visitsUrl}/2` && init?.body != null)!;
    expect(JSON.parse(post[1]!.body as string)).toMatchObject({
        arrival_time: "0001-01-01T00:00:00Z",
    });
});

// An empty picker should open on now rather than on today-at-noon, so that
// picking a day nearby doesn't also require fixing the hour.
test("opening an unset picker points its time spinners at now", async (): Promise<void> => {
    await initVisitPage();

    const fp = flatpickrFor("departure_time");
    const before = new Date();
    fp.open();
    const after = new Date();

    // The clock could tick between the two samples, so either reading is right.
    const pad = (n: number): string => n.toString().padStart(2, "0");
    expect([pad(before.getHours()), pad(after.getHours())]).toContain(fp.hourElement.value);
    expect([pad(before.getMinutes()), pad(after.getMinutes())]).toContain(fp.minuteElement.value);
    expect([before.getHours(), after.getHours()]).toContain(fp.config.defaultHour);
    expect([before.getMinutes(), after.getMinutes()]).toContain(fp.config.defaultMinute);
    // Nothing is selected yet, so nothing should have been saved.
    expect(fp.selectedDates).toHaveLength(0);
});

// Opening a picker that already holds a time must leave it alone, or merely
// looking at an arrival time would rewrite it.
test("opening a picker that already has a time leaves it alone", async (): Promise<void> => {
    serverVisit.arrival_time = "2025-08-25T12:00:00Z";
    const mock = await initVisitPage();

    const fp = flatpickrFor("arrival_time");
    mock.mockClear();
    fp.open();

    expect(fp.hourElement.value).toBe("");
    expect(fp.minuteElement.value).toBe("");
    expect(fp.selectedDates[0]!.toISOString()).toBe("2025-08-25T12:00:00.000Z");
    expect(mock.mock.calls.some(([u, init]) =>
        u === `${visitsUrl}/2` && init?.body != null)).toBe(false);
});

test("the Go to... modal opens on this page's Visit type", async (): Promise<void> => {
    await initVisitPage();

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, bubbles: true, cancelable: true }));

    expect((document.getElementById("goto-resource-visit") as HTMLInputElement).checked).toBe(true);
    expect((document.getElementById("goto-number") as HTMLInputElement).getAttribute("aria-label")).toBe("VS#");
});

function errorText(): string {
    return document.getElementById("error_text")!.textContent ?? "";
}

function postedBodies(mock: ReturnType<typeof mockFetch>, url: string): unknown[] {
    return mock.mock.calls
        .filter(([u, init]) => u === url && init?.body != null)
        .map(([, init]) => JSON.parse(init!.body as string));
}

// Routes for a page opened on visits/new, where the first edit creates Visit #2.
function newVisitRoutes(createResponse: () => Response) {
    return (url: string, init?: RequestInit): Response | undefined => {
        if (url === `/ims/api/auth?event_id=${eventName}`) {
            return visitRoutes(url, init);
        }
        if (url === visitsUrl && init?.body != null) {
            return createResponse();
        }
        return visitRoutes(url, init);
    };
}

test("an unauthenticated visitor is sent to the login page", async (): Promise<void> => {
    const replace = vi.spyOn(window.location, "replace").mockImplementation((): void => {});
    mockFetch((url, init) => {
        if (url === `/ims/api/auth?event_id=${eventName}`) {
            return jsonResponse({ authenticated: false });
        }
        if (url === url_logout) {
            return new Response(null, { status: 204 });
        }
        return visitRoutes(url, init);
    });
    await import("../typescript/sanctuary_visit.ts");

    await vi.waitFor((): void => {
        expect(replace).toHaveBeenCalledWith(expect.stringContaining(url_login));
    });
});

test("a visit that fails to load shows the error, and the page stays read-only", async (): Promise<void> => {
    vi.spyOn(console, "error").mockImplementation((): void => {});
    vi.spyOn(console, "log").mockImplementation((): void => {});
    mockFetch((url, init) => url === `${visitsUrl}/2` && init?.body == null
        ? problemResponse("no such visit", 404)
        : visitRoutes(url, init));
    await import("../typescript/sanctuary_visit.ts");

    await vi.waitFor((): void => {
        expect(errorText()).toContain("Visit failed to load");
    });
    expect((document.getElementById("guest_preferred_name") as HTMLInputElement).disabled).toBe(true);
});

test("a viewer who can read but not write visits gets a read-only page", async (): Promise<void> => {
    serverEventAccess.writeVisits = false;
    serverEventAccess.attachFiles = false;
    await initVisitPage();

    expect((document.getElementById("guest_preferred_name") as HTMLInputElement).disabled).toBe(true);
    expect(document.getElementById("attach_file")!.classList.contains("hidden")).toBe(true);
});

test("a personnel load failure is shown, and the roster lists handles without links", async (): Promise<void> => {
    await initVisitPage((url, init) => url === `/ims/api/personnel?event_id=${eventName}`
        ? problemResponse("directory down", 503)
        : visitRoutes(url, init));

    // Drawing the visit clears the error, so check what the roster fell back to.
    const ranger = document.querySelector<HTMLElement>("#visit_rangers_list li")!;
    expect(ranger.dataset["rangerHandle"]).toBe("Hot Slots");
    expect(ranger.querySelector("span")!.textContent).toBe("Hot Slots");
    expect(ranger.querySelector("a")).toBeNull();
    // The add field offers no one but the blank option.
    expect(document.querySelectorAll("#ranger_handles option").length).toBe(1);
});

test("the roster links known Rangers to the Clubhouse, sorted, with their roles", async (): Promise<void> => {
    serverVisit.rangers = [
        { handle: "Tool", role: "sitter" },
        { handle: "Hot Slots" },
        // An entry without a handle is skipped.
        { role: "ghost" },
        { handle: "Former Ranger" },
    ];
    await initVisitPage();

    const items = [...document.querySelectorAll<HTMLElement>("#visit_rangers_list li")];
    expect(items.map(li => li.dataset["rangerHandle"])).toEqual(["Former Ranger", "Hot Slots", "Tool"]);
    expect(items[0]!.querySelector("span")!.textContent).toBe("Former Ranger");
    const toolLink = items[2]!.querySelector("a")!;
    expect(toolLink.textContent).toBe("Tool");
    expect(toolLink.href).toMatch(/\/2$/);
    expect(toolLink.target).toBe("_blank");
    expect(items[2]!.querySelector("input")!.value).toBe("sitter");
    expect(items[2]!.querySelector("input")!.ariaLabel).toBe("Ranger role for Tool");
    expect(items[2]!.querySelector("button")!.ariaLabel).toBe("Remove Ranger Tool");

    const options = [...document.querySelectorAll<HTMLOptionElement>("#ranger_handles option")].map(o => o.value);
    expect(options).toEqual(["", "Hot Slots", "Tool"]);
});

test("the document title says whether the visit is current or past, and whose it is", async (): Promise<void> => {
    await initVisitPage();
    expect(document.getElementById("doc-title")!.textContent).toBe("Current Sanctuary Visit (Sparkle)");

    const channel = new BroadcastChannel("visit_update");
    serverVisit.guest_preferred_name = null;
    serverVisit.departure_time = "2025-08-26T08:00:00Z";
    channel.postMessage({ event_id: eventId, visit_number: 2 });
    await vi.waitFor((): void => {
        expect(document.getElementById("doc-title")!.textContent).toBe("Past Sanctuary Visit (Pat Doe)");
    });
    // The departure time's full date is there on hover.
    expect((document.getElementById("alt_departure_time") as HTMLInputElement).title).toMatch(/2025-08-2\d/);

    serverVisit.guest_legal_name = null;
    channel.postMessage({ event_id: eventId, visit_number: 2 });
    await vi.waitFor((): void => {
        expect(document.getElementById("doc-title")!.textContent).toBe("Past Sanctuary Visit (no name)");
    });
    channel.close();
});

test("a broadcast about another visit, or another event, is ignored; an update-all isn't", async (): Promise<void> => {
    const mock = await initVisitPage();
    const visitLoads = (): number => mock.mock.calls.filter(([url, init]) => url === `${visitsUrl}/2` && init?.body == null).length;
    const before = visitLoads();

    const channel = new BroadcastChannel("visit_update");
    channel.postMessage({ event_id: eventId, visit_number: 3 });
    channel.postMessage({ event_id: eventId + 1, visit_number: 2 });
    await new Promise((resolve) => setTimeout(resolve, 10));
    expect(visitLoads()).toBe(before);

    // Pages left over from earlier tests are listening too, so an update-all
    // reloads more than once; only check that this page refreshed.
    serverVisit.guest_preferred_name = "Refreshed";
    channel.postMessage({ update_all: true });
    await vi.waitFor((): void => {
        expect(inputValue("guest_preferred_name")).toBe("Refreshed");
    });
    channel.close();
});

test("the parent incident field links to that incident", async (): Promise<void> => {
    serverVisit.incident = 17;
    await initVisitPage();

    expect(inputValue("parent_incident")).toBe("17");
    expect((document.getElementById("parent_incident_link") as HTMLAnchorElement).getAttribute("href"))
        .toBe("/ims/app/events/2025/incidents/17");
});

test("printing retitles the page for a PDF filename, then puts the title back", async (): Promise<void> => {
    await initVisitPage();
    const displayTitle = document.title;

    window.dispatchEvent(new Event("beforeprint"));
    expect(document.title).toBe("Visit-2025-2_Sparkle");
    window.dispatchEvent(new Event("afterprint"));
    expect(document.title).toBe(displayTitle);
});

test("leaving with an unsent report entry asks the browser to confirm", async (): Promise<void> => {
    await initVisitPage();

    const clean = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(clean);
    expect(clean.defaultPrevented).toBe(false);

    (document.getElementById("report_entry_add") as HTMLTextAreaElement).value = "half a thought";
    const dirty = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(dirty);
    expect(dirty.defaultPrevented).toBe(true);
});

test("keyboard shortcuts toggle history, jump to the entry box, and open a new visit", async (): Promise<void> => {
    await initVisitPage();
    const opened = { focus: vi.fn() };
    const open = vi.spyOn(window, "open").mockReturnValue(opened as unknown as Window);

    const checkbox = document.getElementById("history_checkbox") as HTMLInputElement;
    expect(checkbox.checked).toBe(false);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "h", bubbles: true }));
    expect(checkbox.checked).toBe(true);

    // A modifier makes it someone else's shortcut.
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "n", ctrlKey: true, bubbles: true }));
    expect(open).not.toHaveBeenCalled();

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "n", bubbles: true }));
    expect(open).toHaveBeenCalledWith("./new", "_blank");
    expect(opened.focus).toHaveBeenCalled();

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "a", bubbles: true }));
    expect(document.activeElement!.id).toBe("report_entry_add");

    // Now that a field has focus, shortcuts are off.
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "h", bubbles: true }));
    expect(checkbox.checked).toBe(true);
});

test("Alt+Enter in the entry box submits the entry", async (): Promise<void> => {
    const mock = await initVisitPage();
    const textarea = document.getElementById("report_entry_add") as HTMLTextAreaElement;

    // With nothing typed, the submit button is disabled and Enter does nothing.
    textarea.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", altKey: true, bubbles: true }));
    expect(postedBodies(mock, `${visitsUrl}/2`)).toEqual([]);

    textarea.value = "guest is resting";
    textarea.dispatchEvent(new Event("input", { bubbles: true }));
    textarea.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", altKey: true, bubbles: true }));

    await vi.waitFor((): void => {
        expect(postedBodies(mock, `${visitsUrl}/2`)).toEqual([
            { report_entries: [{ text: "guest is resting", id: -1 }], number: 2 },
        ]);
    });
});

test("a failed edit is reported, and the field goes back to the server's value", async (): Promise<void> => {
    const mock = await initVisitPage((url, init) => url === `${visitsUrl}/2` && init?.body != null
        ? problemResponse("visit is locked", 409)
        : visitRoutes(url, init));

    const field = document.getElementById("guest_legal_name") as HTMLInputElement;
    field.value = "Someone Else";
    change(field);

    await vi.waitFor((): void => {
        expect(errorText()).toContain("Failed to apply edit: visit is locked (HTTP 409)");
    });
    expect(field.value).toBe("Pat Doe");
    expect(postedBodies(mock, `${visitsUrl}/2`)).toEqual([{ guest_legal_name: "Someone Else", number: 2 }]);
});

//
// A new visit
//

test("a new visit focuses the name field, and its first edit creates it, assuming arrival now", async (): Promise<void> => {
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/visits/new`);
    const mock = await initVisitPage(newVisitRoutes(() =>
        new Response(null, { status: 201, headers: { "IMS-Visit-Number": "2" } })));

    expect(inputValue("visit_number")).toBe("(new)");
    expect(document.getElementById("doc-title")!.textContent).toBe("New Sanctuary Visit");
    expect(document.activeElement!.id).toBe("guest_preferred_name");

    const before = Date.now();
    const name = document.getElementById("guest_preferred_name") as HTMLInputElement;
    name.value = "Sparkle";
    name.blur();
    change(name);

    await vi.waitFor((): void => {
        expect(inputValue("visit_number")).toBe("2");
    });
    const [created] = postedBodies(mock, visitsUrl) as { guest_preferred_name: string; arrival_time: string }[];
    expect(created!.guest_preferred_name).toBe("Sparkle");
    expect(Date.parse(created!.arrival_time)).toBeGreaterThanOrEqual(before);
    expect(window.location.pathname).toBe(`/ims/app/events/${eventName}/visits/2`);
});

test("creating a visit without a number in the response shows an error", async (): Promise<void> => {
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/visits/new`);
    await initVisitPage(newVisitRoutes(() => new Response(null, { status: 201 })));

    const name = document.getElementById("guest_preferred_name") as HTMLInputElement;
    name.value = "Sparkle";
    await page.editGuestPreferredName();

    expect(errorText()).toContain("No IMS-Visit-Number header provided");
    expect(inputValue("visit_number")).toBe("(new)");
});

test("creating a visit with a non-integer number in the response shows an error", async (): Promise<void> => {
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/visits/new`);
    await initVisitPage(newVisitRoutes(() =>
        new Response(null, { status: 201, headers: { "IMS-Visit-Number": "two" } })));

    const name = document.getElementById("guest_preferred_name") as HTMLInputElement;
    name.value = "Sparkle";
    await page.editGuestPreferredName();

    expect(errorText()).toContain("Non-integer IMS-Visit-Number header provided");
    expect(inputValue("visit_number")).toBe("(new)");
});

test("adding a Ranger to a new visit creates the visit first", async (): Promise<void> => {
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/visits/new`);
    const mock = await initVisitPage(newVisitRoutes(() =>
        new Response(null, { status: 201, headers: { "IMS-Visit-Number": "2" } })));

    const addField = document.getElementById("ranger_add") as HTMLInputElement;
    addField.value = "Tool";
    await page.addRanger();

    expect(postedBodies(mock, visitsUrl).length).toBe(1);
    expect(postedBodies(mock, `${visitsUrl}/2/rangers/Tool`)).toEqual([{ handle: "Tool" }]);
});

test("a Ranger added to a new visit that can't be created isn't posted", async (): Promise<void> => {
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/visits/new`);
    const mock = await initVisitPage(newVisitRoutes(() => problemResponse("nope", 500)));

    const addField = document.getElementById("ranger_add") as HTMLInputElement;
    addField.value = "Tool";
    await page.addRanger();

    expect(errorText()).toContain("Failed to apply edit");
    expect(mock.mock.calls.some(([url]) => (url as string).includes("/rangers/"))).toBe(false);
});

test("attaching a file to a new visit creates the visit first", async (): Promise<void> => {
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/visits/new`);
    const mock = await initVisitPage(newVisitRoutes(() =>
        new Response(null, { status: 201, headers: { "IMS-Visit-Number": "2" } })));
    const uploads = mockXHR((url) => url === `${visitsUrl}/2/attachments` ? new Response(null, { status: 204 }) : undefined);

    await page.attachFile();

    expect(postedBodies(mock, visitsUrl).length).toBe(1);
    expect(uploads.map(u => u.url)).toEqual([`${visitsUrl}/2/attachments`]);
});

test("a file attached to a new visit that can't be created isn't uploaded", async (): Promise<void> => {
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/visits/new`);
    await initVisitPage(newVisitRoutes(() => problemResponse("nope", 500)));
    const uploads = mockXHR(() => new Response(null, { status: 204 }));

    await page.attachFile();

    expect(uploads).toEqual([]);
    expect(errorText()).toContain("Failed to apply edit");
});

//
// The Ranger roster
//

test("a Ranger's handle is matched regardless of case and surrounding space", async (): Promise<void> => {
    const mock = await initVisitPage();

    const addField = document.getElementById("ranger_add") as HTMLInputElement;
    addField.value = "  tOOL ";
    await page.addRanger();

    expect(postedBodies(mock, `${visitsUrl}/2/rangers/Tool`)).toEqual([{ handle: "Tool" }]);
    expect(addField.value).toBe("");
    expect(addField.classList.contains("is-valid")).toBe(true);
});

test("adding a Ranger who's already on the visit sends nothing", async (): Promise<void> => {
    const mock = await initVisitPage();

    const addField = document.getElementById("ranger_add") as HTMLInputElement;
    addField.value = "Hot Slots";
    await page.addRanger();

    expect(addField.value).toBe("");
    expect(mock.mock.calls.some(([url]) => (url as string).includes("/rangers/"))).toBe(false);
});

test("a failed Ranger addition marks the add field and re-enables it", async (): Promise<void> => {
    await initVisitPage((url, init) => url.startsWith(`${visitsUrl}/2/rangers/`) && init?.body != null
        ? problemResponse("nope", 500)
        : visitRoutes(url, init));

    const addField = document.getElementById("ranger_add") as HTMLInputElement;
    addField.value = "Tool";
    await page.addRanger();

    expect(addField.classList.contains("is-invalid")).toBe(true);
    expect(addField.disabled).toBe(false);
    expect(addField.value).toBe("");
});

test("a failed role edit marks the role field", async (): Promise<void> => {
    await initVisitPage((url, init) => url.startsWith(`${visitsUrl}/2/rangers/`) && init?.body != null
        ? problemResponse("nope", 500)
        : visitRoutes(url, init));

    const roleInput = document.querySelector<HTMLInputElement>("#visit_rangers_list li input")!;
    roleInput.value = "sitter";
    await page.setRangerRole(roleInput);

    expect(roleInput.classList.contains("is-invalid")).toBe(true);
});

test("a role field outside any Ranger's row sends nothing", async (): Promise<void> => {
    vi.spyOn(console, "log").mockImplementation((): void => {});
    const mock = await initVisitPage();

    const stray = document.createElement("input");
    document.body.append(stray);
    await page.setRangerRole(stray);

    expect(mock.mock.calls.some(([url]) => (url as string).includes("/rangers/"))).toBe(false);
});

test("a remove button outside any Ranger's row sends nothing", async (): Promise<void> => {
    const mock = await initVisitPage();

    const wrapper = document.createElement("div");
    const stray = document.createElement("button");
    wrapper.append(stray);
    await page.removeRanger(stray);

    expect(mock.mock.calls.some(([url]) => (url as string).includes("/rangers/"))).toBe(false);
});
