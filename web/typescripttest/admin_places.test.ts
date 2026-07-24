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

// Tests for admin_places.ts against the real templ-rendered places admin page
// (adminplaces.templ). The page loads an event's places into JSON textareas
// and submits edited JSON back to the API.

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

const placesUrl = url_places.replace("<event_id>", "2025");
const adminPlacesPath = "/ims/app/admin/places";

let serverEvents: ims.EventData[];

beforeEach((): void => {
    vi.resetModules();
    loadFixture("admin_places.html");
    // Each test starts on the admin places page with no query string.
    window.history.replaceState(null, "", adminPlacesPath);
    serverEvents = [
        { id: 1, name: "2025" },
        { id: 2, name: "2024" },
        { id: 3, name: "Group", is_group: true },
    ];
});

async function initAdminPlacesPage(placesHandler: (init?: RequestInit) => Response | undefined = () => undefined) {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: true, user: "Tester", admin: true });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse(serverEvents);
        }
        if (url === placesUrl) {
            return placesHandler(init);
        }
        return undefined;
    });
    await import("../typescript/admin_places.ts");
    await vi.waitFor((): void => {
        expect(window.loadPlaces).toBeTypeOf("function");
    });
    return mock;
}

function field(id: string): HTMLTextAreaElement {
    return document.getElementById(id) as HTMLTextAreaElement;
}

// Waits for drawEventNames, which runs after init awaits the events fetch.
async function eventSelect(): Promise<HTMLSelectElement> {
    const select = document.getElementById("event-name") as HTMLSelectElement;
    await vi.waitFor((): void => {
        expect(select.options.length).toBeGreaterThan(1);
    });
    return select;
}

test("the event-name select is populated in reverse-alphabetical order, excluding groups", async (): Promise<void> => {
    await initAdminPlacesPage();

    const select = await eventSelect();
    const options = [...select.options].map(o => o.value);
    // The templ-rendered placeholder stays first, then events newest-first.
    expect(options).toEqual(["", "2025", "2024"]);
    // Groups hold no places of their own.
    expect(options).not.toContain("Group");
    // Options need visible text, not just a value, to be pickable in a select.
    expect([...select.options].map(o => o.textContent)).toEqual([
        "Select an event…", "2025", "2024",
    ]);
    // Nothing is selected until the user picks an event.
    expect(select.value).toBe("");
});

test("loading places with no event selected fetches nothing", async (): Promise<void> => {
    const mock = await initAdminPlacesPage();
    await eventSelect();

    await window.loadPlaces();

    expect(mock.mock.calls.some(([url]) => url === placesUrl)).toBe(false);
});

test("submitting with no event selected surfaces an error and posts nothing", async (): Promise<void> => {
    const mock = await initAdminPlacesPage();
    await eventSelect();

    field("art-data").value = "[]";
    field("camp-data").value = "[]";
    field("mv-data").value = "[]";
    field("other-data").value = "[]";

    document.getElementById("place-form")!.dispatchEvent(
        new Event("submit", { bubbles: true, cancelable: true }),
    );

    await vi.waitFor((): void => {
        expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    });
    expect(document.getElementById("error_text")!.textContent).toContain("Select an event");
    expect(mock.mock.calls.some(([url, init]) => url === placesUrl && init?.body != null)).toBe(false);
});

test("loading places fills each JSON textarea and its count label", async (): Promise<void> => {
    await initAdminPlacesPage(() => jsonResponse({
        art: [{ name: "Temple", location_string: "9:00", external_data: { name: "Temple", location_string: "9:00" } }],
        camp: [],
        mv: [{ name: "Art Car", external_data: { name: "Art Car" } }],
        other: [],
    }));

    (await eventSelect()).value = "2025";
    await window.loadPlaces();

    expect(JSON.parse(field("art-data").value)).toEqual([{ name: "Temple", location_string: "9:00" }]);
    expect(document.getElementById("art-data-label")!.textContent).toBe("Art JSON Data (1)");
    expect(JSON.parse(field("mv-data").value)).toEqual([{ name: "Art Car" }]);
    expect(document.getElementById("mv-data-label")!.textContent).toBe("Mutant vehicle JSON Data (1)");
    // An empty category still renders an empty array and a zero count.
    expect(JSON.parse(field("camp-data").value)).toEqual([]);
    expect(document.getElementById("camp-data-label")!.textContent).toBe("Camp JSON Data (0)");
});

test("submitting the form posts the parsed places to the event's places endpoint", async (): Promise<void> => {
    const mock = await initAdminPlacesPage((init) => {
        if (init?.body != null) {
            return new Response(null, { status: 204 });
        }
        return undefined;
    });

    (await eventSelect()).value = "2025";
    field("art-data").value = JSON.stringify([{ name: "Temple", location_string: "9:00" }]);
    field("camp-data").value = "[]";
    field("mv-data").value = "[]";
    field("other-data").value = "[]";

    document.getElementById("place-form")!.dispatchEvent(
        new Event("submit", { bubbles: true, cancelable: true }),
    );

    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url, init]) => url === placesUrl && init?.body != null)).toBe(true);
    });
    const postCall = mock.mock.calls.find(([url, init]) => url === placesUrl && init?.body != null)!;
    const body = JSON.parse(postCall[1]!.body as string);
    expect(body.art).toEqual([
        { name: "Temple", location_string: "9:00", external_data: { name: "Temple", location_string: "9:00" } },
    ]);
});

test("invalid JSON in a textarea surfaces an error and posts nothing", async (): Promise<void> => {
    const mock = await initAdminPlacesPage();

    (await eventSelect()).value = "2025";
    field("art-data").value = "this is not json";
    field("camp-data").value = "[]";
    field("mv-data").value = "[]";
    field("other-data").value = "[]";

    document.getElementById("place-form")!.dispatchEvent(
        new Event("submit", { bubbles: true, cancelable: true }),
    );

    await vi.waitFor((): void => {
        expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    });
    expect(document.getElementById("error_text")!.textContent).toContain("Error");
    expect(mock.mock.calls.some(([url, init]) => url === placesUrl && init?.body != null)).toBe(false);
});

// The page keeps an "event_id" query param (which holds an event name, as
// everywhere else in IMS) in sync with the event-name select, so that a linked
// or bookmarked URL lands on a loaded event.

test("an event_id query param preselects that event and loads its places", async (): Promise<void> => {
    window.history.replaceState(null, "", `${adminPlacesPath}?event_id=2025`);

    const mock = await initAdminPlacesPage(() => jsonResponse({
        art: [{ name: "Temple", location_string: "9:00", external_data: { name: "Temple", location_string: "9:00" } }],
        camp: [],
        mv: [],
        other: [],
    }));

    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url]) => url === placesUrl)).toBe(true);
    });
    expect((await eventSelect()).value).toBe("2025");
    expect(JSON.parse(field("art-data").value)).toEqual([{ name: "Temple", location_string: "9:00" }]);
    // The param that got us here survives the load.
    expect(window.location.search).toBe("?event_id=2025");
});

test("an event_id for an event the user can't see errors out and loads nothing", async (): Promise<void> => {
    window.history.replaceState(null, "", `${adminPlacesPath}?event_id=1999`);

    const mock = await initAdminPlacesPage();

    await vi.waitFor((): void => {
        expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    });
    expect(document.getElementById("error_text")!.textContent).toContain("No such event: 1999");
    // The select falls back to the placeholder rather than showing nothing at all.
    expect((await eventSelect()).value).toBe("");
    expect(mock.mock.calls.some(([url]) => url === url_places.replace("<event_id>", "1999"))).toBe(false);
});

test("selecting an event writes the event_id query param", async (): Promise<void> => {
    await initAdminPlacesPage(() => jsonResponse({ art: [], camp: [], mv: [], other: [] }));

    const select = await eventSelect();
    select.value = "2025";
    // What the select's onchange handler does.
    await window.loadPlaces();

    expect(window.location.pathname).toBe(adminPlacesPath);
    expect(window.location.search).toBe("?event_id=2025");
});

test("clearing the event selection drops the event_id query param", async (): Promise<void> => {
    window.history.replaceState(null, "", `${adminPlacesPath}?event_id=2025`);
    await initAdminPlacesPage(() => jsonResponse({ art: [], camp: [], mv: [], other: [] }));

    const select = await eventSelect();
    await vi.waitFor((): void => {
        expect(select.value).toBe("2025");
    });
    select.value = "";
    await window.loadPlaces();

    // No dangling "?" left on the URL.
    expect(window.location.search).toBe("");
    expect(window.location.pathname).toBe(adminPlacesPath);
});
