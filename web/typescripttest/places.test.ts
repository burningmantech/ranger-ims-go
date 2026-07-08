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

// Tests for places.ts against the real templ-rendered places list page
// (places.templ). The ajax source flattens the categorized Places payload into
// one tagged row list; a stand-in DataTable (MockDataTable) captures those rows.

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { jsonResponse, loadFixture, MockDataTable, mockFetch } from "./helpers.ts";

const eventName = "2025";
const eventId = 1;
const placesUrl = `/ims/api/events/${eventName}/places`;

let serverEventAccess: ims.AuthInfoEventAccess;
let serverPlaces: ims.Places;
let serverEvents: ims.EventData[];

beforeEach((): void => {
    vi.resetModules();
    loadFixture("places.html");
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/places`);

    vi.stubGlobal("DataTable", MockDataTable);

    serverEventAccess = {
        event_id: eventId,
        readIncidents: true,
        writeIncidents: true,
        writeFieldReports: true,
        readVisits: true,
        writeVisits: true,
        attachFiles: true,
    };
    // The external_data payloads only need the fields places.ts reads (name,
    // description), so cast minimal stand-ins for the full BM* shapes.
    serverPlaces = {
        art: [{ name: "Temple", location_string: "9:00", external_data: { name: "Temple", description: "A big temple" } as ims.BMArt }],
        camp: [{ name: "Camp Friendly", location_string: "5:00 & C", external_data: { name: "Camp Friendly", description: "Friendly folks" } as ims.BMCamp }],
        mv: [{ name: "Disco Bus", external_data: { name: "Disco Bus", description: "Boogie" } as ims.BMMV }],
        other: [{ name: "First Camp", location_string: null, external_data: { name: "First Camp", location_string: null } as ims.OtherDest }],
    };
    serverEvents = [{ id: eventId, name: eventName }];
});

function placesRoutes(url: string, init?: RequestInit): Response | undefined {
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
    if (url === placesUrl && init?.body == null) {
        return jsonResponse(serverPlaces);
    }
    return undefined;
}

async function initPlacesPage(handler: (url: string, init?: RequestInit) => Response | undefined = placesRoutes) {
    MockDataTable.lastInstance = null;
    const mock = mockFetch(handler);
    await import("../typescript/places.ts");
    await vi.waitFor((): void => {
        const errored = !document.getElementById("error_info")!.classList.contains("hidden");
        expect(errored || MockDataTable.lastInstance != null).toBe(true);
    });
    return mock;
}

test("page init flattens the categorized places into one tagged row list", async (): Promise<void> => {
    await initPlacesPage();

    await vi.waitFor((): void => {
        expect(MockDataTable.lastInstance?.data().length).toBe(4);
    });
    const rows = MockDataTable.lastInstance!.data() as ims.Place[];
    // Each row is tagged with its category type.
    expect(rows.map(r => r.type).sort()).toEqual(["art", "camp", "mv", "other"]);
    // Non-mv categories copy the description out of the external data.
    const temple = rows.find(r => r.name === "Temple")!;
    expect(temple.type).toBe("art");
    expect(temple.description).toBe("A big temple");
    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(true);
});

test("the show-rows control is wired and reflects the default selection", async (): Promise<void> => {
    await initPlacesPage();

    expect(window.destShowRows).toBeTypeOf("function");
    expect(document.getElementById("show_rows")!.querySelector(".selection")!.textContent).not.toBe("");
});

test("a viewer without incident-read or field-report-write access sees an authorization error", async (): Promise<void> => {
    serverEventAccess.readIncidents = false;
    serverEventAccess.writeFieldReports = false;

    await initPlacesPage();

    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    expect(document.getElementById("error_text")!.textContent).toContain("not currently authorized");
});

// Pull a column's render function off the table the page configured.
function renderColumn(name: string): (value: any, type: string, row: ims.Place) => unknown {
    const column = MockDataTable.lastInstance!.column(name)!;
    return column.render!;
}

// Run the table's createdRow hook against a fresh row, then click it to open the
// place-info modal. Returns the populated modal body for inspection.
function openPlace(place: ims.Place): HTMLElement {
    const row = document.createElement("tr");
    MockDataTable.lastInstance!.options.createdRow!(row, place, 0);
    row.dispatchEvent(new MouseEvent("click"));
    return document.getElementById("placeBody")!;
}

test("the description column truncates for display and passes raw text for sort/filter", async (): Promise<void> => {
    await initPlacesPage();
    const render = renderColumn("place_description");
    const place = serverPlaces.art![0]!;

    expect(render("short", "display", place)).toBe("short");
    expect(render("short", "sort", place)).toBe("short");
    expect(render("short", "filter", place)).toBe("short");
    expect(render(null, "filter", place)).toBe("");
    expect(render("anything", "type", place)).toBe("");
    expect(render("anything", "bogus", place)).toBeUndefined();

    // Over the 200-char limit (plus a 3-char grace) gets clipped with an ellipsis.
    const long = "z".repeat(300);
    const display = render(long, "display", place) as string;
    expect(display.length).toBe(203);
    expect(display.endsWith("...")).toBe(true);
});

test("clicking a camp row fills the modal with its details", async (): Promise<void> => {
    const camp: ims.Place = {
        name: "Camp Friendly", type: "camp",
        external_data: {
            name: "Camp Friendly", location_string: "5:00 & C", description: "Friendly folks",
            contact_email: "hi@camp.example", hometown: "Reno", landmark: "big flag",
            url: "https://camp.example", uid: "camp-1",
            images: [{ thumbnail_url: "https://img.example/c.jpg?sig=abc" }],
            location: { frontage: null, intersection: null, intersection_type: "&", dimensions: "100x100", exact_location: "corner" },
        } as ims.BMCamp,
    };
    await initPlacesPage();
    const body = openPlace(camp);

    expect(document.getElementById("placeInfoModalLabel")!.textContent).toBe("Camp Friendly");
    expect(body.querySelector("#camp_name")!.textContent).toBe("Camp Friendly");
    expect(body.querySelector("#location_label")!.textContent).toContain("intersection");
    expect(body.querySelector("#description")!.textContent).toBe("Friendly folks");
    expect(body.querySelector("#landmark")!.textContent).toBe("big flag");
    // The image link drops any query string from the thumbnail URL.
    expect(body.querySelector("#image_dd a")!.getAttribute("href")).toBe("https://img.example/c.jpg");
    expect((body.querySelector("#email_link") as HTMLAnchorElement).href).toBe("mailto:hi@camp.example");
    expect((body.querySelector("#website_url") as HTMLAnchorElement).href).toBe("https://camp.example/");
    expect(body.querySelector("#hometown")!.textContent).toBe("Reno");
    expect(body.querySelector("#uid")!.textContent).toBe("camp-1");
});

test("clicking an art row with full details renders GPS coordinates and links", async (): Promise<void> => {
    const art: ims.Place = {
        name: "Temple", type: "art",
        external_data: {
            name: "Temple", location_string: "9:00", description: "A big temple", artist: "Jane",
            contact_email: "art@example", hometown: "Oakland", url: "https://art.example", uid: "art-1",
            images: [{ thumbnail_url: "https://img.example/a.jpg" }],
            location: { hour: 9, minute: 0, distance: null, category: null, gps_latitude: 40.781, gps_longitude: -119.234 },
        } as ims.BMArt,
    };
    await initPlacesPage();
    const body = openPlace(art);

    expect(body.querySelector("#art_name")!.textContent).toBe("Temple");
    expect(body.querySelector("#location_string")!.textContent).toContain("40.781000,-119.234000");
    expect(body.querySelector("#artist")!.textContent).toBe("Jane");
    expect((body.querySelector("#email_link") as HTMLAnchorElement).href).toBe("mailto:art@example");
});

test("clicking a mutant-vehicle row joins its tags and renders details", async (): Promise<void> => {
    const mv: ims.Place = {
        name: "Disco Bus", type: "mv",
        external_data: {
            name: "Disco Bus", description: "Boogie", artist: "DJ", contact_email: "mv@example",
            hometown: "SF", url: "https://mv.example", uid: "mv-1", tags: ["loud", "shiny"],
            images: [{ thumbnail_url: "https://img.example/mv.jpg" }],
        } as ims.BMMV,
    };
    await initPlacesPage();
    const body = openPlace(mv);

    expect(body.querySelector("#mv_name")!.textContent).toBe("Disco Bus");
    expect(body.querySelector("#tags")!.textContent).toBe("loud, shiny");
    expect(body.querySelector("#artist")!.textContent).toBe("DJ");
});

test("clicking an 'other' place with missing details shows 'None provided' fallbacks", async (): Promise<void> => {
    // "other" places reuse the camp template; with no external detail the optional
    // fields fall back to their "None provided" placeholders.
    const other: ims.Place = {
        name: "First Camp", type: "other",
        external_data: { name: "First Camp", location_string: null } as ims.OtherDest,
    };
    await initPlacesPage();
    const body = openPlace(other);

    expect(body.querySelector("#description")!.textContent).toBe("None provided");
    expect(body.querySelector("#landmark")!.textContent).toBe("None provided");
    expect(body.querySelector("#email_dd")!.textContent).toBe("None provided");
    expect(body.querySelector("#website_dd")!.textContent).toBe("None provided");
    expect(body.querySelector("#image_dd")!.textContent).toContain("None provided");
});

test("destShowRows updates the dropdown label, page length, and URL hash", async (): Promise<void> => {
    await initPlacesPage();

    window.destShowRows("50", true);
    expect(document.getElementById("show_rows")!.querySelector(".selection")!.textContent).not.toBe("");
    expect(MockDataTable.lastInstance!.pageLen).toBe(50);
    expect(window.location.hash).toContain("rows=50");
});

test("destShowRows('all') sets the page length to unlimited", async (): Promise<void> => {
    await initPlacesPage();
    window.destShowRows("all", false);
    expect(MockDataTable.lastInstance!.pageLen).toBe(-1);
});

test("the show-type control is wired and reflects the default selection", async (): Promise<void> => {
    await initPlacesPage();

    expect(window.destShowType).toBeTypeOf("function");
    expect(document.getElementById("show_type")!.querySelector(".selection")!.textContent).toBe("All Types");
});

test("destShowType filters the table to the chosen type and updates the label and URL hash", async (): Promise<void> => {
    await initPlacesPage();
    await vi.waitFor((): void => {
        expect(MockDataTable.lastInstance!.data().length).toBe(4);
    });
    // The ajax source pushes rows in category order: art, camp, mv, other.
    const rows = MockDataTable.lastInstance!.data() as ims.Place[];
    expect(rows.map(r => r.type)).toEqual(["art", "camp", "mv", "other"]);

    window.destShowType("camp", true);

    expect(document.getElementById("show_type")!.querySelector(".selection")!.textContent).toBe("Camp");
    expect(window.location.hash).toContain("type=camp");
    // Only the camp row passes the fixed "type" predicate.
    expect(MockDataTable.lastInstance!.fixedSearch("type", 0)).toBe(false);
    expect(MockDataTable.lastInstance!.fixedSearch("type", 1)).toBe(true);
    expect(MockDataTable.lastInstance!.fixedSearch("type", 2)).toBe(false);
    expect(MockDataTable.lastInstance!.fixedSearch("type", 3)).toBe(false);
});

test("destShowType('all') passes every row and drops the type from the URL hash", async (): Promise<void> => {
    await initPlacesPage();
    await vi.waitFor((): void => {
        expect(MockDataTable.lastInstance!.data().length).toBe(4);
    });

    window.destShowType("mv", true);
    expect(window.location.hash).toContain("type=mv");

    window.destShowType("all", true);
    expect(document.getElementById("show_type")!.querySelector(".selection")!.textContent).toBe("All Types");
    expect(window.location.hash).not.toContain("type=");
    for (let i = 0; i < 4; i++) {
        expect(MockDataTable.lastInstance!.fixedSearch("type", i)).toBe(true);
    }
});

test("a type fragment applies the type filter on load", async (): Promise<void> => {
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/places#type=art`);
    await initPlacesPage();
    await vi.waitFor((): void => {
        expect(MockDataTable.lastInstance!.data().length).toBe(4);
    });

    expect(document.getElementById("show_type")!.querySelector(".selection")!.textContent).toBe("Art");
    // Only the art row (index 0) survives the fixed "type" predicate.
    expect(MockDataTable.lastInstance!.fixedSearch("type", 0)).toBe(true);
    expect(MockDataTable.lastInstance!.fixedSearch("type", 1)).toBe(false);
});

test("a q fragment applies a /regex/ search on load", async (): Promise<void> => {
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/places#q=${encodeURIComponent("/temple/")}`);
    await initPlacesPage();

    expect((document.getElementById("search_input") as HTMLInputElement).value).toBe("/temple/");
    expect(MockDataTable.lastInstance!.lastSearch).toEqual(["temple", true, false]);
});

test("typing in the search box runs a debounced smart search", async (): Promise<void> => {
    await initPlacesPage();
    const input = document.getElementById("search_input") as HTMLInputElement;
    input.value = "temple";
    input.dispatchEvent(new Event("input"));

    await vi.waitFor((): void => {
        expect(MockDataTable.lastInstance!.lastSearch).toEqual(["temple", false, true]);
    });
    expect(window.location.hash).toContain("q=temple");
});

test("the map link is revealed when the current event has a map URL", async (): Promise<void> => {
    serverEvents = [{ id: eventId, name: eventName, map_url: "https://map.example/2025" }];
    await initPlacesPage();

    const mapLink = document.getElementById("map-link") as HTMLAnchorElement;
    expect(mapLink.href).toBe("https://map.example/2025");
    expect(mapLink.classList.contains("d-none")).toBe(false);
});

test("the slash key focuses the search box", async (): Promise<void> => {
    await initPlacesPage();

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "/" }));
    expect(document.activeElement).toBe(document.getElementById("search_input"));
});
