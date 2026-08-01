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
import { type FetchHandler, jsonResponse, loadFixture, mockFetch, problemResponse } from "./helpers.ts";

let serverTypes: ims.IncidentType[];

beforeEach((): void => {
    vi.resetModules();
    loadFixture("admin_types.html");
    serverTypes = [
        { id: 1, name: "Junk", hidden: false, description: "Found junk" },
        { id: 2, name: "Old Type", hidden: true, description: "" },
    ];
});

// A fake server that applies edits to serverTypes, so a redraw after a mutation
// reflects what was sent, the way the real API would.
function typesRoutes(url: string, init?: RequestInit): Response | undefined {
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
            return new Response(null, { status: 204 });
        }
        const existing = serverTypes.find((t): boolean => t.id === edits.id);
        if (existing == null) {
            return problemResponse("No such incident type", 404);
        }
        if (edits.name != null) {
            existing.name = edits.name;
        }
        if (edits.description != null) {
            existing.description = edits.description;
        }
        if (edits.hidden != null) {
            existing.hidden = edits.hidden;
        }
        return new Response(null, { status: 204 });
    }
    return undefined;
}

// Import admin_types.ts with a fake server behind it, and wait for the page to
// finish drawing the incident types list.
async function initAdminTypesPage(handler: FetchHandler = typesRoutes) {
    const mock = mockFetch(handler);
    await import("../typescript/admin_types.ts");
    await vi.waitFor((): void => {
        expect(typeList().length).toBeGreaterThan(0);
    });
    return mock;
}

function typeList(): HTMLLIElement[] {
    return [...document.querySelectorAll<HTMLLIElement>("#incident_types ul li")];
}

// The edits POST bodies sent to the incident types endpoint, in order.
function editsSent(mock: ReturnType<typeof mockFetch>): ims.IncidentType[] {
    return mock.mock.calls
        .filter(([url, init]) => url === url_incidentTypes && init?.body != null)
        .map(([, init]) => JSON.parse(init!.body as string) as ims.IncidentType);
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

test("hiding a visible type posts hidden=true and redraws it as hidden", async (): Promise<void> => {
    const mock = await initAdminTypesPage();

    // The "Active" badge in type_li_template is wired to hideIncidentType(this).
    const activeBadge = typeList()[0]!.querySelector<HTMLButtonElement>(".badge-visible")!;
    expect(activeBadge.getAttribute("onclick")).toBe("hideIncidentType(this)");

    mock.mockClear();
    await window.hideIncidentType(activeBadge);

    expect(editsSent(mock)).toContainEqual({ id: 1, hidden: true });
    await vi.waitFor((): void => {
        expect(typeList()[0]!.classList.contains("item-hidden")).toBe(true);
    });
});

test("showing a hidden type posts hidden=false and redraws it as visible", async (): Promise<void> => {
    const mock = await initAdminTypesPage();

    const hiddenBadge = typeList()[1]!.querySelector<HTMLButtonElement>(".badge-hidden")!;
    expect(hiddenBadge.getAttribute("onclick")).toBe("showIncidentType(this)");

    mock.mockClear();
    await window.showIncidentType(hiddenBadge);

    expect(editsSent(mock)).toContainEqual({ id: 2, hidden: false });
    await vi.waitFor((): void => {
        expect(typeList()[1]!.classList.contains("item-visible")).toBe(true);
    });
});

// The handlers find the type by walking up to the <li>, so a detached button
// has nothing to act on and must not post a malformed edit.
test("show and hide do nothing for a button outside any type row", async (): Promise<void> => {
    const mock = await initAdminTypesPage();

    const orphan = document.createElement("button");
    mock.mockClear();
    await window.hideIncidentType(orphan);
    await window.showIncidentType(orphan);

    expect(editsSent(mock)).toEqual([]);
});

test("the Edit button loads that type into the modal", async (): Promise<void> => {
    await initAdminTypesPage();

    typeList()[0]!.querySelector<HTMLButtonElement>(".show-edit-modal")!.click();

    const modal = document.getElementById("editIncidentTypeModal")!;
    expect(modal.dataset["incidentTypeId"]).toBe("1");
    expect((document.getElementById("edit_incident_type_name") as HTMLInputElement).value)
        .toBe("Junk");
    expect((document.getElementById("edit_incident_type_description") as HTMLTextAreaElement).value)
        .toBe("Found junk");
});

test("renaming a type from the modal posts the new name and redraws", async (): Promise<void> => {
    const mock = await initAdminTypesPage();

    typeList()[0]!.querySelector<HTMLButtonElement>(".show-edit-modal")!.click();
    const nameField = document.getElementById("edit_incident_type_name") as HTMLInputElement;
    expect(nameField.getAttribute("onchange")).toBe("setIncidentTypeName(this);");

    mock.mockClear();
    nameField.value = "Abandoned Property";
    await window.setIncidentTypeName(nameField);

    expect(editsSent(mock)).toContainEqual({ id: 1, name: "Abandoned Property" });
    expect(nameField.classList.contains("is-valid")).toBe(true);
    await vi.waitFor((): void => {
        const names = typeList().map((li) => li.querySelector(".type-name")!.textContent);
        expect(names).toContain("Abandoned Property");
    });
});

test("editing a type's description from the modal posts it and redraws", async (): Promise<void> => {
    const mock = await initAdminTypesPage();

    typeList()[0]!.querySelector<HTMLButtonElement>(".show-edit-modal")!.click();
    const descriptionField =
        document.getElementById("edit_incident_type_description") as HTMLTextAreaElement;
    expect(descriptionField.getAttribute("onchange")).toBe("setIncidentTypeDescription(this);");

    mock.mockClear();
    descriptionField.value = "Property left unattended";
    await window.setIncidentTypeDescription(descriptionField);

    expect(editsSent(mock)).toContainEqual({ id: 1, description: "Property left unattended" });
    expect(descriptionField.classList.contains("is-valid")).toBe(true);
    await vi.waitFor((): void => {
        const descriptions = typeList()
            .map((li) => li.querySelector(".type-description")!.textContent);
        expect(descriptions).toContain("Property left unattended");
    });
});

// A blank name would be rejected by the server, so the page doesn't send it.
test("a blank name or description in the modal sends nothing", async (): Promise<void> => {
    const mock = await initAdminTypesPage();

    typeList()[0]!.querySelector<HTMLButtonElement>(".show-edit-modal")!.click();

    const nameField = document.getElementById("edit_incident_type_name") as HTMLInputElement;
    const descriptionField =
        document.getElementById("edit_incident_type_description") as HTMLTextAreaElement;

    mock.mockClear();
    nameField.value = "   ";
    await window.setIncidentTypeName(nameField);
    descriptionField.value = "";
    await window.setIncidentTypeDescription(descriptionField);

    expect(editsSent(mock)).toEqual([]);
});

// Without an id the page can't tell the server which type to change, so both
// modal handlers bail out rather than creating a new type by accident.
test("modal edits send nothing before a type has been loaded into it", async (): Promise<void> => {
    const mock = await initAdminTypesPage();

    const nameField = document.getElementById("edit_incident_type_name") as HTMLInputElement;
    mock.mockClear();
    nameField.value = "Orphaned Rename";
    await window.setIncidentTypeName(nameField);

    expect(editsSent(mock)).toEqual([]);
});

test("a rejected rename marks the field invalid and leaves the list alone", async (): Promise<void> => {
    const alertSpy = vi.fn();
    // happy-dom doesn't implement window.alert.
    vi.stubGlobal("alert", alertSpy);

    await initAdminTypesPage((url, init) => {
        if (url === url_incidentTypes && init?.body != null) {
            return problemResponse("That name is already taken", 400);
        }
        return typesRoutes(url, init);
    });

    typeList()[0]!.querySelector<HTMLButtonElement>(".show-edit-modal")!.click();
    const nameField = document.getElementById("edit_incident_type_name") as HTMLInputElement;
    nameField.value = "Junk";
    await window.setIncidentTypeName(nameField);

    expect(nameField.classList.contains("is-invalid")).toBe(true);
    expect(nameField.getAttribute("aria-invalid")).toBe("true");
    expect(alertSpy).toHaveBeenCalledOnce();
    // The list still shows the original name.
    expect(typeList()[0]!.querySelector(".type-name")!.textContent).toBe("Junk");
});

test("creating a type from a blank input sends nothing", async (): Promise<void> => {
    const mock = await initAdminTypesPage();

    const addInput = document.querySelector<HTMLInputElement>("#incident_types .card-footer input")!;
    mock.mockClear();
    addInput.value = "   ";
    await window.createIncidentType(addInput);

    expect(editsSent(mock)).toEqual([]);
});

// Deletion isn't built yet; the button says so rather than silently doing nothing.
test("deleting a type reports that it is unimplemented", async (): Promise<void> => {
    const alertSpy = vi.fn();
    vi.stubGlobal("alert", alertSpy);
    const mock = await initAdminTypesPage();

    mock.mockClear();
    window.deleteIncidentType(typeList()[0]!);

    expect(alertSpy).toHaveBeenCalledOnce();
    expect(alertSpy.mock.calls[0]![0]).toContain("unimplemented");
    expect(editsSent(mock)).toEqual([]);
});
