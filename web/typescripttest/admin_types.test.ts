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

// Tests for admin_types.ts against the real templ-rendered incident types
// admin page (admintypes.templ).

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

let serverTypes: ims.IncidentType[];

beforeEach((): void => {
    vi.resetModules();
    loadFixture("admin_types.html");
    serverTypes = [
        { id: 1, name: "Junk", hidden: false, description: "Found junk" },
        { id: 2, name: "Old Type", hidden: true, description: "" },
    ];
});

// Import admin_types.ts with a fake server behind it, and wait for the page to
// finish drawing the incident types list.
async function initAdminTypesPage() {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: true, user: "Tester", admin: true });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse([]);
        }
        if (url === url_incidentTypes && init?.body == null) {
            return jsonResponse(serverTypes);
        }
        if (url === url_incidentTypes && init?.body != null) {
            const edits = JSON.parse(init.body as string) as ims.IncidentType;
            if (edits.id == null) {
                serverTypes.push({ id: 100, name: edits.name ?? null, hidden: false, description: "" });
            }
            return new Response(null, { status: 204 });
        }
        return undefined;
    });
    await import("../typescript/admin_types.ts");
    await vi.waitFor((): void => {
        expect(typeList().length).toBeGreaterThan(0);
    });
    return mock;
}

function typeList(): HTMLLIElement[] {
    return [...document.querySelectorAll<HTMLLIElement>("#incident_types ul li")];
}

test("incident types are rendered from the type_li_template template element", async (): Promise<void> => {
    await initAdminTypesPage();

    const items = typeList();
    expect(items.length).toBe(2);

    // Types are drawn by cloning the <template id="type_li_template"> from
    // admintypes.templ and filling in the type-name/type-description nodes.
    expect(items[0]!.querySelector(".type-name")!.textContent).toBe("Junk");
    expect(items[0]!.querySelector(".type-description")!.textContent).toBe("Found junk");
    expect(items[0]!.classList.contains("item-visible")).toBe(true);
    expect(items[0]!.dataset["incidentTypeId"]).toBe("1");

    expect(items[1]!.querySelector(".type-name")!.textContent).toBe("Old Type");
    expect(items[1]!.classList.contains("item-hidden")).toBe(true);
});

test("the page shows the logged-in admin user and enables editing", async (): Promise<void> => {
    await initAdminTypesPage();

    const user = document.querySelector(".logged-in-user")!;
    expect(user.textContent).toBe("Tester");

    // The "Add" input from admintypes.templ starts disabled until
    // enableEditing() runs at the end of page init.
    const addInput = document.querySelector<HTMLInputElement>("#incident_types .card-footer input")!;
    await vi.waitFor((): void => {
        expect(addInput.disabled).toBe(false);
    });
});

test("createIncidentType posts the new type and redraws the list", async (): Promise<void> => {
    const mock = await initAdminTypesPage();

    // admintypes.templ wires the Add input with onchange="createIncidentType(this)".
    const addInput = document.querySelector<HTMLInputElement>("#incident_types .card-footer input")!;
    expect(addInput.getAttribute("onchange")).toBe("createIncidentType(this)");

    addInput.value = "Stuck Vehicle";
    await window.createIncidentType(addInput);

    const postCall = mock.mock.calls.find(
        ([url, init]) => url === url_incidentTypes && init?.body != null,
    )!;
    expect(JSON.parse(postCall[1]!.body as string)).toEqual({ name: "Stuck Vehicle" });

    // The input is cleared and the list now includes the new type.
    expect(addInput.value).toBe("");
    await vi.waitFor((): void => {
        const names = typeList().map((li) => li.querySelector(".type-name")!.textContent);
        expect(names).toContain("Stuck Vehicle");
    });
});
