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
// (adminplaces.templ). The page loads an event's places into JSON textareas,
// each of which has its own Save button that posts just that one place type.

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

const placesUrl = url_places.replace("<event_id>", "2025");
const importUrl = url_placesImport.replace("<event_id>", "2025");
const adminPlacesPath = "/ims/app/admin/places";

let serverEvents: ims.EventData[];
let serverAuth: ims.AuthInfo;
// Requests to the places import endpoint, which carries its arguments in the
// query string rather than a body.
let importHandler: (url: string) => Response | undefined;

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
    serverAuth = { authenticated: true, user: "Tester", admin: true, places_import_allowed: true };
    importHandler = (): Response | undefined => jsonResponse({ count: 0 });
});

async function initAdminPlacesPage(placesHandler: (init?: RequestInit) => Response | undefined = () => undefined) {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse(serverAuth);
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse(serverEvents);
        }
        if (url.startsWith(importUrl)) {
            return importHandler(url);
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

function clickSave(id: string): void {
    (document.getElementById(id) as HTMLButtonElement).click();
}

function postBodies(mock: ReturnType<typeof mockFetch>): any[] {
    return mock.mock.calls
        .filter(([url, init]) => url === placesUrl && init?.body != null)
        .map(([, init]) => JSON.parse(init!.body as string));
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

test("saving with no event selected surfaces an error and posts nothing", async (): Promise<void> => {
    const mock = await initAdminPlacesPage();
    await eventSelect();

    field("art-data").value = "[]";

    clickSave("art-save");

    await vi.waitFor((): void => {
        expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    });
    expect(document.getElementById("error_text")!.textContent).toContain("Select an event");
    expect(postBodies(mock)).toEqual([]);
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

test("saving the art field posts only art places, leaving the other types alone", async (): Promise<void> => {
    const mock = await initAdminPlacesPage((init) => {
        if (init?.body != null) {
            return new Response(null, { status: 204 });
        }
        return jsonResponse({ art: [], camp: [], mv: [], other: [] });
    });

    (await eventSelect()).value = "2025";
    field("art-data").value = JSON.stringify([{ name: "Temple", location_string: "9:00" }]);
    // Whatever is sitting in the other textareas stays out of the request, so
    // an unsaved edit elsewhere can't ride along.
    field("camp-data").value = JSON.stringify([{ name: "Unsaved Camp", location_string: "3:00" }]);

    clickSave("art-save");

    await vi.waitFor((): void => {
        expect(postBodies(mock)).toHaveLength(1);
    });
    const body = postBodies(mock)[0];
    expect(body.art).toEqual([
        { name: "Temple", location_string: "9:00", external_data: { name: "Temple", location_string: "9:00" } },
    ]);
    // Types absent from the body are the ones the server won't touch.
    expect(Object.keys(body)).toEqual(["art"]);
});

test("saving the mutant vehicle field posts only mv places", async (): Promise<void> => {
    const mock = await initAdminPlacesPage((init) => {
        if (init?.body != null) {
            return new Response(null, { status: 204 });
        }
        return jsonResponse({ art: [], camp: [], mv: [], other: [] });
    });

    (await eventSelect()).value = "2025";
    field("mv-data").value = JSON.stringify([{ name: "Art Car" }]);

    clickSave("mv-save");

    await vi.waitFor((): void => {
        expect(postBodies(mock)).toHaveLength(1);
    });
    const body = postBodies(mock)[0];
    // Mutant vehicles have no location of their own.
    expect(body.mv).toEqual([{ name: "Art Car", external_data: { name: "Art Car" } }]);
    expect(Object.keys(body)).toEqual(["mv"]);
});

test("a successful save reloads just that field from the server", async (): Promise<void> => {
    await initAdminPlacesPage((init) => {
        if (init?.body != null) {
            return new Response(null, { status: 204 });
        }
        return jsonResponse({
            art: [{ name: "Temple", location_string: "9:00", external_data: { name: "Temple", location_string: "9:00" } }],
            camp: [{ name: "Camp", location_string: "3:00", external_data: { name: "Camp", location_string: "3:00" } }],
            mv: [],
            other: [],
        });
    });

    (await eventSelect()).value = "2025";
    await window.loadPlaces();
    // A pending edit in another field must survive the art save.
    field("camp-data").value = "pending edit";
    field("art-data").value = JSON.stringify([{ name: "Whatever", location_string: "6:00" }]);

    clickSave("art-save");

    await vi.waitFor((): void => {
        // Redrawn from the GET that follows the POST, not from what was typed.
        expect(JSON.parse(field("art-data").value)).toEqual([{ name: "Temple", location_string: "9:00" }]);
    });
    expect(document.getElementById("art-data-label")!.textContent).toBe("Art JSON Data (1)");
    expect(field("camp-data").value).toBe("pending edit");
    expect(field("art-data").classList.contains("is-valid")).toBe(true);
});

test("invalid JSON in a textarea surfaces an error and posts nothing", async (): Promise<void> => {
    const mock = await initAdminPlacesPage();

    (await eventSelect()).value = "2025";
    field("art-data").value = "this is not json";

    clickSave("art-save");

    await vi.waitFor((): void => {
        expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    });
    expect(document.getElementById("error_text")!.textContent).toContain("Art JSON Data");
    expect(postBodies(mock)).toEqual([]);
    expect(field("art-data").classList.contains("is-invalid")).toBe(true);
});

test("a failed save marks only its own field and leaves the text as typed", async (): Promise<void> => {
    const mock = await initAdminPlacesPage((init) => {
        if (init?.body != null) {
            return new Response("nope", { status: 403 });
        }
        return jsonResponse({ art: [], camp: [], mv: [], other: [] });
    });

    (await eventSelect()).value = "2025";
    field("art-data").value = JSON.stringify([{ name: "Temple", location_string: "9:00" }]);

    clickSave("art-save");

    await vi.waitFor((): void => {
        expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    });
    expect(field("art-data").classList.contains("is-invalid")).toBe(true);
    expect(field("camp-data").classList.contains("is-invalid")).toBe(false);
    // The rejected edit isn't thrown away, so it can be fixed and retried.
    expect(JSON.parse(field("art-data").value)).toEqual([{ name: "Temple", location_string: "9:00" }]);
    expect(postBodies(mock)).toHaveLength(1);
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

// The "Set from API" buttons have the server pull a place type straight from
// the Burning Man API, for a year the admin gives separately (an IMS event
// isn't necessarily named for the year whose data it wants).

function importButton(placeType: string): HTMLButtonElement {
    return document.getElementById(`${placeType}-api-import`) as HTMLButtonElement;
}

function yearInput(placeType: string): HTMLInputElement {
    return document.getElementById(`${placeType}-api-year`) as HTMLInputElement;
}

function importUrls(mock: ReturnType<typeof mockFetch>): string[] {
    return mock.mock.calls.map(([url]) => url).filter(url => url.startsWith(importUrl));
}

test("the import buttons are disabled when the server has no Burning Man API key", async (): Promise<void> => {
    serverAuth = { authenticated: true, user: "Tester", admin: true, places_import_allowed: false };
    const mock = await initAdminPlacesPage(() => jsonResponse({ art: [], camp: [], mv: [], other: [] }));

    await vi.waitFor((): void => {
        expect(importButton("camp").disabled).toBe(true);
    });
    // The explanation goes on the wrapper, since a disabled button doesn't fire
    // the events a tooltip needs.
    expect(document.getElementById("camp-api-wrapper")!.title).toContain("no Burning Man API key");

    (await eventSelect()).value = "2025";
    vi.stubGlobal("confirm", vi.fn((): boolean => true));
    importButton("camp").click();

    expect(importUrls(mock)).toEqual([]);
});

test("only art, camp, and mv have import controls; other does not", async (): Promise<void> => {
    await initAdminPlacesPage();

    await vi.waitFor((): void => {
        expect(importButton("camp").disabled).toBe(false);
    });
    expect(importButton("art")).not.toBeNull();
    expect(importButton("mv")).not.toBeNull();
    // "Other" places are hand-written, with no upstream API to pull from.
    expect(importButton("other")).toBeNull();
    expect(yearInput("other")).toBeNull();
});

test("the year inputs are prefilled from the selected event name", async (): Promise<void> => {
    await initAdminPlacesPage(() => jsonResponse({ art: [], camp: [], mv: [], other: [] }));

    (await eventSelect()).value = "2024";
    await window.loadPlaces();

    expect(yearInput("camp").value).toBe("2024");
    expect(yearInput("art").value).toBe("2024");
    expect(yearInput("mv").value).toBe("2024");
});

test("an event name with no year in it prefills the current year", async (): Promise<void> => {
    serverEvents = [{ id: 1, name: "Training" }];
    await initAdminPlacesPage(() => jsonResponse({ art: [], camp: [], mv: [], other: [] }));

    (await eventSelect()).value = "Training";
    await window.loadPlaces();

    expect(yearInput("camp").value).toBe(new Date().getFullYear().toString());
});

test("importing camps posts the place type and year, then reloads just that field", async (): Promise<void> => {
    importHandler = (): Response => jsonResponse({ count: 2 });
    const mock = await initAdminPlacesPage(() => jsonResponse({
        art: [],
        camp: [
            { name: "Camp A", location_string: "3:00", external_data: { name: "Camp A", location_string: "3:00" } },
            { name: "Camp B", location_string: "4:00", external_data: { name: "Camp B", location_string: "4:00" } },
        ],
        mv: [],
        other: [],
    }));

    (await eventSelect()).value = "2025";
    await window.loadPlaces();
    // An unsaved edit in another field must survive the camp import.
    field("art-data").value = "pending edit";
    yearInput("camp").value = "2019";
    vi.stubGlobal("confirm", vi.fn((): boolean => true));

    importButton("camp").click();

    await vi.waitFor((): void => {
        expect(importUrls(mock)).toHaveLength(1);
    });
    const params = new URLSearchParams(importUrls(mock)[0]!.split("?")[1]);
    expect(params.get("place_type")).toBe("camp");
    // The year the admin typed, not the event name.
    expect(params.get("year")).toBe("2019");

    await vi.waitFor((): void => {
        expect(JSON.parse(field("camp-data").value)).toHaveLength(2);
    });
    expect(document.getElementById("camp-data-label")!.textContent).toBe("Camp JSON Data (2)");
    // The count comes from the server's response, not from the textarea.
    expect(document.getElementById("camp-api-status")!.textContent).toContain("Saved 2 camps");
    expect(field("camp-data").classList.contains("is-valid")).toBe(true);
    expect(field("art-data").value).toBe("pending edit");
});

test("the import confirmation names what it's about to delete, and declining sends nothing", async (): Promise<void> => {
    const mock = await initAdminPlacesPage(() => jsonResponse({
        art: [],
        camp: [{ name: "Camp A", location_string: "3:00", external_data: { name: "Camp A" } }],
        mv: [],
        other: [],
    }));

    (await eventSelect()).value = "2025";
    await window.loadPlaces();
    const confirmMock = vi.fn((_message: string): boolean => false);
    vi.stubGlobal("confirm", confirmMock);

    importButton("camp").click();

    await vi.waitFor((): void => {
        expect(confirmMock).toHaveBeenCalledTimes(1);
    });
    const prompt: string = confirmMock.mock.calls[0]![0];
    expect(prompt).toContain("2025");
    // The count the page last loaded, so the admin knows the size of the loss.
    expect(prompt).toContain("delete the 1 camps");
    expect(importUrls(mock)).toEqual([]);
});

test("importing with no event selected surfaces an error and sends nothing", async (): Promise<void> => {
    const mock = await initAdminPlacesPage();
    await eventSelect();
    vi.stubGlobal("confirm", vi.fn((): boolean => true));

    importButton("camp").click();

    await vi.waitFor((): void => {
        expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    });
    expect(document.getElementById("error_text")!.textContent).toContain("Select an event");
    expect(importUrls(mock)).toEqual([]);
});

test("importing with an empty year surfaces an error and sends nothing", async (): Promise<void> => {
    const mock = await initAdminPlacesPage(() => jsonResponse({ art: [], camp: [], mv: [], other: [] }));

    (await eventSelect()).value = "2025";
    await window.loadPlaces();
    yearInput("camp").value = "";
    vi.stubGlobal("confirm", vi.fn((): boolean => true));

    importButton("camp").click();

    await vi.waitFor((): void => {
        expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    });
    expect(document.getElementById("error_text")!.textContent).toContain("year");
    expect(yearInput("camp").classList.contains("is-invalid")).toBe(true);
    expect(importUrls(mock)).toEqual([]);
});

test("a failed import surfaces the server's message and re-enables the button", async (): Promise<void> => {
    importHandler = (): Response => new Response(
        JSON.stringify({ detail: "The Burning Man API has no camp data for 1999. Nothing was changed." }),
        { status: 502, headers: { "content-type": "application/problem+json" } },
    );
    await initAdminPlacesPage(() => jsonResponse({ art: [], camp: [], mv: [], other: [] }));

    (await eventSelect()).value = "2025";
    await window.loadPlaces();
    yearInput("camp").value = "1999";
    vi.stubGlobal("confirm", vi.fn((): boolean => true));

    importButton("camp").click();

    await vi.waitFor((): void => {
        expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    });
    expect(document.getElementById("error_text")!.textContent).toContain("no camp data for 1999");
    expect(document.getElementById("camp-api-status")!.textContent).toBe("");
    // The admin can fix the year and try again.
    expect(importButton("camp").disabled).toBe(false);
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
