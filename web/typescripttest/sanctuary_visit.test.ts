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
import { captureLinkClicks, jsonResponse, loadFixture, MockFlatpickr, mockFetch, mockXHR } from "./helpers.ts";

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
    if (/\/attachments\/\d+$/.test(url) && !hasBody) {
        return new Response("file contents", { status: 200 });
    }
    return undefined;
}

async function initVisitPage(handler: (url: string, init?: RequestInit) => Response | undefined = visitRoutes) {
    const mock = mockFetch(handler);
    await import("../typescript/sanctuary_visit.ts");
    await vi.waitFor((): void => {
        expect(document.getElementById("loading-overlay")!.style.display).toBe("none");
    });
    return mock;
}

function inputValue(id: string): string {
    return (document.getElementById(id) as HTMLInputElement).value;
}

// Types the given value into a field, runs its onchange handler, and returns the
// JSON body of the resulting edit POST. Each field's handler maps one control to
// one key in the visit payload, and that mapping is what these tests check.
// The edit handlers are declared on Window as () => void, though each one is
// async and returns a promise that awaiting here does wait on.
async function editField(
    mock: ReturnType<typeof mockFetch>,
    id: string,
    value: string,
    edit: () => void,
): Promise<Record<string, unknown>> {
    mock.mockClear();
    (document.getElementById(id) as HTMLInputElement).value = value;
    await edit();
    const call = mock.mock.calls.find(
        ([url, init]) => url === `${visitsUrl}/2` && init?.body != null);
    if (call == null) {
        throw new Error(`editing ${id} sent no edit request`);
    }
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

function expanded(collapseId: string): boolean {
    return document.getElementById(collapseId)!.classList.contains("show");
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

    // An opened section's header button reflects the state too.
    const button = document.querySelector("[data-bs-target=\"#collapse-arrival\"]")!;
    expect(button.classList.contains("collapsed")).toBe(false);
    expect(button.getAttribute("aria-expanded")).toBe("true");
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
    await window.editGuestPreferredName();

    const editCall = mock.mock.calls.find(([url, init]) => url === `${visitsUrl}/2` && init?.body != null)!;
    expect(JSON.parse(editCall[1]!.body as string)).toMatchObject({ guest_preferred_name: "Stardust" });
});

test("adding a known ranger posts to that visit's ranger endpoint", async (): Promise<void> => {
    const mock = await initVisitPage();

    const addField = document.getElementById("ranger_add") as HTMLInputElement;
    addField.value = "Tool";
    await window.addRanger();

    expect(mock.mock.calls.some(([url, init]) =>
        url === `${visitsUrl}/2/rangers/Tool` && init?.body != null)).toBe(true);
});

test("adding an unknown ranger makes no request and clears the field", async (): Promise<void> => {
    const mock = await initVisitPage();

    const addField = document.getElementById("ranger_add") as HTMLInputElement;
    addField.value = "Nonexistent Person";
    await window.addRanger();

    expect(addField.value).toBe("");
    expect(mock.mock.calls.some(([url]) => (url as string).includes("/rangers/"))).toBe(false);
});

test("a viewer without visit read access sees an authorization error", async (): Promise<void> => {
    serverEventAccess.readVisits = false;

    await initVisitPage();

    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    expect(document.getElementById("error_text")!.textContent).toContain("not currently authorized");
});

test("an entry's attachment is fetched from the visit's own endpoint", async (): Promise<void> => {
    serverVisit.report_entries![0]!.attachment = { name: "checkin.jpg", previewable: true };
    const mock = await initVisitPage();
    const links = captureLinkClicks();

    const entry = document.querySelector<HTMLDivElement>("#report_entries .report_entry")!;
    const download = [...entry.querySelectorAll("button")]
        .find((b: HTMLButtonElement): boolean => (b.textContent ?? "").includes("Download"))!;
    mock.mockClear();
    download.click();

    await vi.waitFor((): void => {
        expect(mock.mock.calls.map(([url]): string => url)).toContain(`${visitsUrl}/2/attachments/1`);
        expect(links.length).toBe(1);
    });
    expect(links[0]!.download).toBe("checkin.jpg");
    expect(document.getElementById("error_text")!.textContent).toBe("");
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
        const pending = window.attachFile();
        expect(button.disabled).toBe(true);
        expect(button.value).toBe("Uploading …");

        await pending;

        // The file form data went to the attachments endpoint.
        expect(uploads.length).toBe(1);
        expect(uploads[0]!.url).toBe(`${visitsUrl}/2/attachments`);
        expect(uploads[0]!.body).toBeInstanceOf(FormData);

        // The button tracked the upload, then waited on the server to store it.
        expect(labels).toEqual(["Uploading 25%", "Uploading 100%", "Uploading …"]);

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

    await window.attachFile();

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

    expect(await editField(mock, "guest_legal_name", "Pat Q. Doe", window.editGuestLegalName))
        .toMatchObject({ guest_legal_name: "Pat Q. Doe" });
    expect(await editField(mock, "guest_description", "tall, red hat", window.editGuestDescription))
        .toMatchObject({ guest_description: "tall, red hat" });
    expect(await editField(mock, "guest_action_plan", "rest, then campmates collect them",
        window.editGuestActionPlan))
        .toMatchObject({ guest_action_plan: "rest, then campmates collect them" });
});

test("editing each camp field posts its own key", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "guest_camp_name", "Camp Questionable", window.editGuestCampName))
        .toMatchObject({ guest_camp_name: "Camp Questionable" });
    expect(await editField(mock, "guest_camp_address", "7:30 & E", window.editGuestCampAddress))
        .toMatchObject({ guest_camp_address: "7:30 & E" });
    expect(await editField(mock, "guest_camp_description", "big purple dome",
        window.editGuestCampDescription))
        .toMatchObject({ guest_camp_description: "big purple dome" });
    expect(await editField(mock, "guest_camp_contacts", "Moon Dog, tent 3",
        window.editGuestCampContacts))
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
    await window.editGuestCampAddress();

    expect(address.value).toBe("7:00 & E");
});

test("editing each arrival field posts its own key", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "arrival_method", "walked in", window.editArrivalMethod))
        .toMatchObject({ arrival_method: "walked in" });
    expect(await editField(mock, "arrival_state", "agitated", window.editArrivalState))
        .toMatchObject({ arrival_state: "agitated" });
    expect(await editField(mock, "arrival_reason", "too much sun", window.editArrivalReason))
        .toMatchObject({ arrival_reason: "too much sun" });
    expect(await editField(mock, "arrival_belongings", "backpack, water bottle",
        window.editArrivalBelongings))
        .toMatchObject({ arrival_belongings: "backpack, water bottle" });
});

test("editing each departure field posts its own key", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "departure_method", "left with campmate",
        window.editDepartureMethod))
        .toMatchObject({ departure_method: "left with campmate" });
    expect(await editField(mock, "departure_state", "calm and oriented",
        window.editDepartureState))
        .toMatchObject({ departure_state: "calm and oriented" });
});

test("editing each resource field posts its own key", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "resource_sitter", "Hot Slots", window.editResourceSitter))
        .toMatchObject({ resource_sitter: "Hot Slots" });
    expect(await editField(mock, "resource_bed_id", "B4", window.editResourceBedID))
        .toMatchObject({ resource_bed_id: "B4" });
    expect(await editField(mock, "resource_rest", "two blankets", window.editResourceRest))
        .toMatchObject({ resource_rest: "two blankets" });
    expect(await editField(mock, "resource_clothes", "a jacket", window.editResourceClothes))
        .toMatchObject({ resource_clothes: "a jacket" });
    expect(await editField(mock, "resource_pogs", "1 meal pog", window.editResourcePogs))
        .toMatchObject({ resource_pogs: "1 meal pog" });
    expect(await editField(mock, "resource_food_bev", "electrolytes", window.editResourceFoodBev))
        .toMatchObject({ resource_food_bev: "electrolytes" });
    expect(await editField(mock, "resource_other", "shower pog", window.editResourceOther))
        .toMatchObject({ resource_other: "shower pog" });
});

test("editing the parent incident sends the number as an integer", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "parent_incident", "17", window.editParentIncident))
        .toMatchObject({ incident: 17 });
});

// Clearing the field detaches the visit, which the API models as incident 0
// rather than a missing key.
test("clearing the parent incident sends zero", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "parent_incident", "", window.editParentIncident))
        .toMatchObject({ incident: 0 });
});

// parseInt10 returns null for unparseable text, and editFromElement turns a null
// into "" rather than dropping the key. The server rejects that, so the visit is
// left unchanged.
test("a non-numeric parent incident sends an empty value, not a bad number", async (): Promise<void> => {
    const mock = await initVisitPage();

    expect(await editField(mock, "parent_incident", "not a number", window.editParentIncident))
        .toMatchObject({ incident: "" });
});

test("removing a ranger DELETEs that ranger's endpoint", async (): Promise<void> => {
    const mock = await initVisitPage();

    const removeButton = document
        .getElementById("visit_rangers_list")!
        .querySelector("li button") as HTMLButtonElement;
    expect(removeButton.ariaLabel).toBe("Remove Ranger Hot Slots");

    mock.mockClear();
    await window.removeRanger(removeButton);

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
    await window.setRangerRole(roleInput);

    const call = mock.mock.calls.find(([url, init]) =>
        url === `${visitsUrl}/2/rangers/Hot%20Slots` && init?.body != null)!;
    expect(call).toBeDefined();
    expect(JSON.parse(call[1]!.body as string)).toMatchObject({
        handle: "Hot Slots",
        role: "sitter",
    });
});

// The arrival/departure pickers post through flatpickr's onChange rather than an
// inline onchange attribute, so these drive the picker instance directly.
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

// Fixture pages load with JavaScript evaluation disabled, so a real click never
// runs the inline onclick attribute. Call the function the attribute names,
// which also checks that the template and the module agree on that name.
function clickButton(id: string): void {
    const button = document.getElementById(id);
    if (!(button instanceof HTMLButtonElement)) {
        throw new Error(`no button element with id ${id}`);
    }
    const handlerName = button.getAttribute("onclick")!.replace(/\(\)$/, "");
    const handler = (window as unknown as Record<string, unknown>)[handlerName];
    if (typeof handler !== "function") {
        throw new Error(`button ${id} calls ${handlerName}(), which isn't defined on window`);
    }
    (handler as () => void)();
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
