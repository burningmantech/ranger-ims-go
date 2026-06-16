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
