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
import { jsonResponse, loadFixture, mockFetch, problemResponse } from "./helpers.ts";

const eventName = "2025";
const eventId = 1;
const visitsUrl = `/ims/api/events/${eventName}/visits`;

let serverEventAccess: ims.AuthInfoEventAccess;
let serverVisit: ims.Visit;
let serverPersonnel: ims.Personnel[];
let serverEvents: ims.EventData[];
// The visit's current ETag; edits through visitRoutes bump it, the way the
// server's version counter would.
let serverETag: string;

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
    serverETag = `"4"`;
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
        return jsonResponse(serverVisit, 200, { "ETag": serverETag });
    }
    if (url === `${visitsUrl}/2` && hasBody) {
        serverETag = `"${Number(serverETag.replaceAll(`"`, "")) + 1}"`;
        return new Response(null, { status: 204, headers: { "ETag": serverETag } });
    }
    if (url.startsWith(`${visitsUrl}/2/rangers/`) && hasBody) {
        serverETag = `"${Number(serverETag.replaceAll(`"`, "")) + 1}"`;
        return new Response(null, { status: 204, headers: { "ETag": serverETag } });
    }
    if (url === `${visitsUrl}/2/attachments` && hasBody) {
        return new Response(null, { status: 200 });
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

test("page init draws the visit's number, names, and arrival state from the API", async (): Promise<void> => {
    await initVisitPage();

    expect(inputValue("visit_number")).toBe("2");
    expect(inputValue("guest_preferred_name")).toBe("Sparkle");
    expect(inputValue("guest_legal_name")).toBe("Pat Doe");
    expect(inputValue("arrival_state")).toBe("calm");
    expect(document.title).toContain("2025");
    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(true);
});

test("editing the guest's preferred name posts the change to the visit", async (): Promise<void> => {
    const mock = await initVisitPage();

    const nameField = document.getElementById("guest_preferred_name") as HTMLInputElement;
    nameField.value = "Stardust";
    await window.editGuestPreferredName();

    const editCall = mock.mock.calls.find(([url, init]) => url === `${visitsUrl}/2` && init?.body != null)!;
    expect(JSON.parse(editCall[1]!.body as string)).toMatchObject({ guest_preferred_name: "Stardust" });
});

test("field edits carry If-Match from the loaded ETag and refresh it from the response", async (): Promise<void> => {
    const mock = await initVisitPage();

    const nameField = document.getElementById("guest_preferred_name") as HTMLInputElement;
    nameField.value = "Stardust";
    await window.editGuestPreferredName();

    let posts = mock.mock.calls.filter(([u, init]) => u === `${visitsUrl}/2` && init?.body != null);
    expect(posts).toHaveLength(1);
    expect(new Headers(posts[0]![1]!.headers).get("If-Match")).toBe(`"4"`);

    // The next edit carries the ETag produced by the first one.
    mock.mockClear();
    nameField.value = "Moonbeam";
    await window.editGuestPreferredName();
    posts = mock.mock.calls.filter(([u, init]) => u === `${visitsUrl}/2` && init?.body != null);
    expect(posts).toHaveLength(1);
    expect(new Headers(posts[0]![1]!.headers).get("If-Match")).toBe(`"5"`);
});

test("a 412 conflict shows the conflict banner and refetches the visit", async (): Promise<void> => {
    const mock = await initVisitPage();

    const conflictRoutes = (url: string, init?: RequestInit): Response | undefined => {
        if (url === `${visitsUrl}/2` && init?.body != null) {
            return problemResponse("Someone else got here first", 412);
        }
        return visitRoutes(url, init);
    };
    mock.mockImplementation(async (url: string, init?: RequestInit): Promise<Response> => {
        const response = conflictRoutes(url, init);
        if (response == null) {
            throw new Error(`no mocked fetch route for ${url}`);
        }
        return response;
    });

    serverVisit.guest_preferred_name = "Their Conflicting Name";
    const nameField = document.getElementById("guest_preferred_name") as HTMLInputElement;
    nameField.value = "My Rejected Name";
    await window.editGuestPreferredName();

    expect(document.getElementById("error_text")!.textContent).toContain(
        "Someone else has edited this visit");
    expect(inputValue("guest_preferred_name")).toBe("Their Conflicting Name");
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

test("attachFile shows an uploading state, posts the file, then confirms and reverts", async (): Promise<void> => {
    const mock = await initVisitPage();
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
            url === `${visitsUrl}/2/attachments` && init?.body instanceof FormData)).toBe(true);

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
    await initVisitPage((url, init) => {
        if (url === `${visitsUrl}/2/attachments` && init?.body != null) {
            return undefined;
        }
        return visitRoutes(url, init);
    });
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
