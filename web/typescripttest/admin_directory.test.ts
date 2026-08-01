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

// Tests for admin_directory.ts against the real templ-rendered user directory
// admin page (admindirectory.templ).

import { beforeEach, expect, test, vi } from "vitest";
import { type FetchHandler, jsonResponse, loadFixture, mockFetch, problemResponse } from "./helpers.ts";

interface ServerPerson {
    id?: number;
    handle?: string | null;
    email?: string | null;
    active?: boolean | null;
    onsite?: boolean | null;
    team_ids?: number[] | null;
    position_ids?: number[] | null;
}
interface ServerGroup {
    id?: number;
    title?: string | null;
    active?: boolean | null;
}
interface ServerDirectory {
    persons: ServerPerson[];
    teams: ServerGroup[];
    positions: ServerGroup[];
}

let serverDirectory: ServerDirectory;

beforeEach((): void => {
    vi.resetModules();
    loadFixture("admin_directory.html");
    serverDirectory = {
        persons: [
            {
                id: 1, handle: "Defect", email: "defect@example.com",
                active: true, onsite: true, team_ids: [10], position_ids: [20],
            },
            {
                id: 2, handle: "Slacker", email: null,
                active: false, onsite: false, team_ids: [], position_ids: [],
            },
        ],
        teams: [{ id: 10, title: "Green Dot", active: true }],
        positions: [{ id: 20, title: "Khaki", active: false }],
    };
});

// A fake server that applies edits to serverDirectory, so a redraw after a
// mutation reflects what was sent, the way the real API would.
function directoryRoutes(url: string, init?: RequestInit): Response | undefined {
    if (url === url_auth && init?.body == null) {
        return jsonResponse({ authenticated: true, user: "Tester", admin: true });
    }
    if (url === url_events && init?.body == null) {
        return jsonResponse([]);
    }
    if (url === url_directory && init?.body == null) {
        return jsonResponse(serverDirectory);
    }
    if (url === url_directoryPersons && init?.body != null) {
        const edits = JSON.parse(init.body as string) as ServerPerson;
        if (edits.id == null) {
            serverDirectory.persons.push({
                id: 100, handle: edits.handle ?? "", email: null,
                active: true, onsite: false, team_ids: [], position_ids: [],
            });
            return new Response(null, { status: 204 });
        }
        const person = serverDirectory.persons.find((p): boolean => p.id === edits.id);
        if (person == null) {
            return problemResponse("No such person", 404);
        }
        Object.assign(person, edits);
        return new Response(null, { status: 204 });
    }
    for (const [collection, single, list] of [
        [url_directoryTeams, url_directoryTeam, serverDirectory.teams],
        [url_directoryPositions, url_directoryPosition, serverDirectory.positions],
    ] as [string, string, ServerGroup[]][]) {
        if (url === collection && init?.body != null) {
            const edits = JSON.parse(init.body as string) as ServerGroup;
            if (edits.id == null) {
                list.push({ id: 200, title: edits.title ?? "", active: true });
                return new Response(null, { status: 204 });
            }
            const group = list.find((g): boolean => g.id === edits.id);
            if (group == null) {
                return problemResponse("No such group", 404);
            }
            Object.assign(group, edits);
            return new Response(null, { status: 204 });
        }
        const idMatch = single.replace(/<\w+>/, "(\\d+)");
        const deleted = new RegExp(`^${idMatch}$`).exec(url);
        if (deleted != null && init?.method === "DELETE") {
            const id = Number(deleted[1]);
            const index = list.findIndex((g): boolean => g.id === id);
            if (index < 0) {
                return problemResponse("No such group", 404);
            }
            list.splice(index, 1);
            return new Response(null, { status: 204 });
        }
    }
    const personDeleted = new RegExp(`^${url_directoryPerson.replace("<person_id>", "(\\d+)")}$`).exec(url);
    if (personDeleted != null && init?.method === "DELETE") {
        const id = Number(personDeleted[1]);
        const index = serverDirectory.persons.findIndex((p): boolean => p.id === id);
        if (index < 0) {
            return problemResponse("No such person", 404);
        }
        serverDirectory.persons.splice(index, 1);
        return new Response(null, { status: 204 });
    }
    if (/\/password$/.test(url) && init?.body != null) {
        return new Response(null, { status: 204 });
    }
    return undefined;
}

// Import admin_directory.ts with a fake server behind it, and wait for the
// page to finish drawing the persons table.
async function initAdminDirectoryPage(handler: FetchHandler = directoryRoutes) {
    const mock = mockFetch(handler);
    await import("../typescript/admin_directory.ts");
    await vi.waitFor((): void => {
        expect(personRows().length).toBeGreaterThan(0);
    });
    return mock;
}

function personRows(): HTMLTableRowElement[] {
    return [...document.querySelectorAll<HTMLTableRowElement>("#persons_tbody tr")];
}

// The person edits POSTed to the directory persons endpoint, in order.
function personEdits(mock: ReturnType<typeof mockFetch>): ServerPerson[] {
    return mock.mock.calls
        .filter(([url, init]) => url === url_directoryPersons && init?.body != null)
        .map(([, init]) => JSON.parse(init!.body as string) as ServerPerson);
}

// Open the edit modal for the person in the given table row.
function openPersonModal(rowIndex: number): void {
    personRows()[rowIndex]!.querySelector<HTMLButtonElement>(".show-edit-modal")!.click();
}

function modalInput(id: string): HTMLInputElement {
    return document.getElementById(id) as HTMLInputElement;
}

test("persons are rendered from the person_row_template template element", async (): Promise<void> => {
    await initAdminDirectoryPage();

    const rows = personRows();
    expect(rows.length).toBe(2);

    expect(rows[0]!.querySelector(".person-handle")!.textContent).toBe("Defect");
    expect(rows[0]!.querySelector(".person-email")!.textContent).toBe("defect@example.com");
    expect(rows[0]!.querySelector(".person-active")!.textContent).toBe("Active");
    expect(rows[0]!.querySelector(".person-onsite")!.textContent).toBe("Onsite");
    expect(rows[0]!.querySelector(".person-teams")!.textContent).toBe("Green Dot");
    expect(rows[0]!.querySelector(".person-positions")!.textContent).toBe("Khaki");
    expect(rows[0]!.dataset["personId"]).toBe("1");

    expect(rows[1]!.querySelector(".person-handle")!.textContent).toBe("Slacker");
    expect(rows[1]!.querySelector(".person-active")!.textContent).toBe("Deactivated");
    expect(rows[1]!.querySelector(".person-onsite")!.textContent).toBe("");
});

test("teams and positions are rendered from the group_li_template element", async (): Promise<void> => {
    await initAdminDirectoryPage();

    const teamItems = [...document.querySelectorAll<HTMLLIElement>("#teams_list li")];
    expect(teamItems.length).toBe(1);
    expect(teamItems[0]!.querySelector(".group-title")!.textContent).toBe("Green Dot");
    expect(teamItems[0]!.classList.contains("item-visible")).toBe(true);

    const positionItems = [...document.querySelectorAll<HTMLLIElement>("#positions_list li")];
    expect(positionItems.length).toBe(1);
    expect(positionItems[0]!.querySelector(".group-title")!.textContent).toBe("Khaki");
    expect(positionItems[0]!.classList.contains("item-hidden")).toBe(true);
});

test("createPerson posts the new person and redraws the table", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();

    const addInput = document.querySelector<HTMLInputElement>("#directory_persons .card-footer input")!;
    expect(addInput.getAttribute("onchange")).toBe("createPerson(this)");

    addInput.value = "Newbie";
    await window.createPerson(addInput);

    const postCall = mock.mock.calls.find(
        ([url, init]) => url === url_directoryPersons && init?.body != null,
    )!;
    expect(JSON.parse(postCall[1]!.body as string)).toEqual({ handle: "Newbie" });

    expect(addInput.value).toBe("");
    await vi.waitFor((): void => {
        const handles = personRows().map((row) => row.querySelector(".person-handle")!.textContent);
        expect(handles).toContain("Newbie");
    });
});

test("the edit modal is populated from the person row's Edit button", async (): Promise<void> => {
    await initAdminDirectoryPage();

    const editButton = personRows()[0]!.querySelector<HTMLButtonElement>(".show-edit-modal")!;
    editButton.click();

    const modal = document.getElementById("editPersonModal")!;
    expect(modal.dataset["personId"]).toBe("1");
    expect(document.querySelector<HTMLInputElement>("#edit_person_handle")!.value).toBe("Defect");
    expect(document.querySelector<HTMLInputElement>("#edit_person_email")!.value).toBe("defect@example.com");
    expect(document.querySelector<HTMLInputElement>("#edit_person_active")!.checked).toBe(true);
    expect(document.querySelector<HTMLInputElement>("#edit_person_onsite")!.checked).toBe(true);

    // Membership checkboxes reflect the person's teams and positions.
    const teamBox = document.querySelector<HTMLInputElement>("#edit_person_teams input[type=checkbox]")!;
    expect(teamBox.checked).toBe(true);
    expect(teamBox.dataset["groupId"]).toBe("10");
    const positionBox = document.querySelector<HTMLInputElement>("#edit_person_positions input[type=checkbox]")!;
    expect(positionBox.checked).toBe(true);
});

test("createTeam posts the new team", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();

    const addInput = document.querySelector<HTMLInputElement>("#directory_teams .card-footer input")!;
    expect(addInput.getAttribute("onchange")).toBe("createTeam(this)");

    addInput.value = "Night Shift";
    await window.createTeam(addInput);

    const postCall = mock.mock.calls.find(
        ([url, init]) => url === url_directoryTeams && init?.body != null,
    )!;
    expect(JSON.parse(postCall[1]!.body as string)).toEqual({ title: "Night Shift" });
    expect(addInput.value).toBe("");
});

test("renaming a person from the modal posts the handle and redraws", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();
    openPersonModal(0);

    const handleField = modalInput("edit_person_handle");
    expect(handleField.getAttribute("onchange")).toBe("setPersonHandle(this);");

    mock.mockClear();
    handleField.value = "Defector";
    await window.setPersonHandle(handleField);

    expect(personEdits(mock)).toContainEqual({ id: 1, handle: "Defector" });
    expect(handleField.classList.contains("is-valid")).toBe(true);
    await vi.waitFor((): void => {
        const handles = personRows().map((r) => r.querySelector(".person-handle")!.textContent);
        expect(handles).toContain("Defector");
    });
});

// A person with no handle can't be referred to anywhere, so a blank one isn't sent.
test("a blank handle sends nothing", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();
    openPersonModal(0);

    const handleField = modalInput("edit_person_handle");
    mock.mockClear();
    handleField.value = "";
    await window.setPersonHandle(handleField);

    expect(personEdits(mock)).toEqual([]);
});

test("changing a person's email posts it", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();
    openPersonModal(0);

    const emailField = modalInput("edit_person_email");
    mock.mockClear();
    emailField.value = "defector@example.com";
    await window.setPersonEmail(emailField);

    expect(personEdits(mock)).toContainEqual({ id: 1, email: "defector@example.com" });
});

// Unlike the handle, an empty email is a legitimate value: it clears the field.
test("clearing a person's email posts an empty string", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();
    openPersonModal(0);

    const emailField = modalInput("edit_person_email");
    mock.mockClear();
    emailField.value = "";
    await window.setPersonEmail(emailField);

    expect(personEdits(mock)).toContainEqual({ id: 1, email: "" });
});

test("deactivating a person posts active=false and redraws the row", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();
    openPersonModal(0);

    const activeBox = modalInput("edit_person_active");
    expect(activeBox.checked).toBe(true);

    mock.mockClear();
    activeBox.checked = false;
    await window.setPersonActive(activeBox);

    expect(personEdits(mock)).toContainEqual({ id: 1, active: false });
    await vi.waitFor((): void => {
        expect(personRows()[0]!.querySelector(".person-active")!.textContent).toBe("Deactivated");
    });
});

test("marking a person offsite posts onsite=false and redraws the row", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();
    openPersonModal(0);

    const onsiteBox = modalInput("edit_person_onsite");
    mock.mockClear();
    onsiteBox.checked = false;
    await window.setPersonOnsite(onsiteBox);

    expect(personEdits(mock)).toContainEqual({ id: 1, onsite: false });
    await vi.waitFor((): void => {
        expect(personRows()[0]!.querySelector(".person-onsite")!.textContent).toBe("");
    });
});

// The membership checkboxes post the whole resulting id list, not a delta.
test("unchecking a team posts the remaining team ids", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();
    openPersonModal(0);

    const teamBox = document.querySelector<HTMLInputElement>(
        "#edit_person_teams input[type=checkbox]")!;
    expect(teamBox.checked).toBe(true);

    mock.mockClear();
    teamBox.checked = false;
    teamBox.dispatchEvent(new Event("change"));

    await vi.waitFor((): void => {
        expect(personEdits(mock)).toContainEqual({ id: 1, team_ids: [] });
    });
});

test("checking a position posts the resulting position ids", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();
    openPersonModal(1);

    const positionBox = document.querySelector<HTMLInputElement>(
        "#edit_person_positions input[type=checkbox]")!;
    expect(positionBox.checked).toBe(false);

    mock.mockClear();
    positionBox.checked = true;
    positionBox.dispatchEvent(new Event("change"));

    await vi.waitFor((): void => {
        expect(personEdits(mock)).toContainEqual({ id: 2, position_ids: [20] });
    });
});

test("setting a password posts it to that person's password endpoint and clears the field",
    async (): Promise<void> => {
        const mock = await initAdminDirectoryPage();
        openPersonModal(0);

        const passwordField = modalInput("edit_person_password");
        const saveButton = document.getElementById("edit_person_password_save")!;
        expect(saveButton.getAttribute("onclick")).toBe("setPersonPassword(this);");

        mock.mockClear();
        passwordField.value = "correct horse battery staple";
        await window.setPersonPassword(saveButton);

        const call = mock.mock.calls.find(
            ([url, init]) => url === "/ims/api/directory/persons/1/password" && init?.body != null)!;
        expect(call).toBeDefined();
        expect(JSON.parse(call[1]!.body as string))
            .toEqual({ password: "correct horse battery staple" });

        // The password isn't left sitting in the DOM after a successful save.
        expect(passwordField.value).toBe("");
        expect(saveButton.classList.contains("is-valid")).toBe(true);
    });

test("an empty password sends nothing", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();
    openPersonModal(0);

    const saveButton = document.getElementById("edit_person_password_save")!;
    mock.mockClear();
    modalInput("edit_person_password").value = "";
    await window.setPersonPassword(saveButton);

    expect(mock.mock.calls.some(([url]) => (url as string).endsWith("/password"))).toBe(false);
});

test("a rejected password keeps the field and reports the failure", async (): Promise<void> => {
    const alertSpy = vi.fn();
    // happy-dom doesn't implement window.alert.
    vi.stubGlobal("alert", alertSpy);

    await initAdminDirectoryPage((url, init) => {
        if (/\/password$/.test(url)) {
            return problemResponse("Password is too weak", 400);
        }
        return directoryRoutes(url, init);
    });
    openPersonModal(0);

    const passwordField = modalInput("edit_person_password");
    const saveButton = document.getElementById("edit_person_password_save")!;
    passwordField.value = "hunter2";
    await window.setPersonPassword(saveButton);

    expect(alertSpy).toHaveBeenCalledOnce();
    expect(saveButton.classList.contains("is-invalid")).toBe(true);
    // The typed password is left in place so it can be corrected rather than retyped.
    expect(passwordField.value).toBe("hunter2");
});

test("deleting a person asks first, then DELETEs and redraws", async (): Promise<void> => {
    const confirmSpy = vi.fn((): boolean => true);
    vi.stubGlobal("confirm", confirmSpy);
    const mock = await initAdminDirectoryPage();
    openPersonModal(0);

    mock.mockClear();
    await window.deletePerson(document.getElementById("edit_person_delete")!);

    expect(confirmSpy).toHaveBeenCalledOnce();
    expect(mock.mock.calls.some(([url, init]) =>
        url === "/ims/api/directory/persons/1" && init?.method === "DELETE")).toBe(true);
    await vi.waitFor((): void => {
        const handles = personRows().map((r) => r.querySelector(".person-handle")!.textContent);
        expect(handles).not.toContain("Defect");
    });
});

test("declining the delete confirmation leaves the person alone", async (): Promise<void> => {
    vi.stubGlobal("confirm", vi.fn((): boolean => false));
    const mock = await initAdminDirectoryPage();
    openPersonModal(0);

    mock.mockClear();
    await window.deletePerson(document.getElementById("edit_person_delete")!);

    expect(mock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);
    expect(personRows()[0]!.querySelector(".person-handle")!.textContent).toBe("Defect");
});

// Every modal handler keys off the person id stashed on the modal, so before a
// person is loaded none of them may send anything.
test("modal handlers send nothing before a person has been loaded", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();

    mock.mockClear();
    await window.setPersonHandle(modalInput("edit_person_handle"));
    await window.setPersonEmail(modalInput("edit_person_email"));
    await window.setPersonActive(modalInput("edit_person_active"));
    await window.setPersonOnsite(modalInput("edit_person_onsite"));
    await window.setPersonPassword(document.getElementById("edit_person_password_save")!);
    await window.deletePerson(document.getElementById("edit_person_delete")!);

    expect(mock.mock.calls.filter(([, init]) => init?.body != null || init?.method === "DELETE"))
        .toEqual([]);
});

test("createPosition posts the new position", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();

    const addInput = document.querySelector<HTMLInputElement>(
        "#directory_positions .card-footer input")!;
    expect(addInput.getAttribute("onchange")).toBe("createPosition(this)");

    addInput.value = "Shiny Penny";
    await window.createPosition(addInput);

    const call = mock.mock.calls.find(
        ([url, init]) => url === url_directoryPositions && init?.body != null)!;
    expect(JSON.parse(call[1]!.body as string)).toEqual({ title: "Shiny Penny" });
    expect(addInput.value).toBe("");
});

test("deactivating a team posts active=false and redraws it as inactive", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();

    const teamItem = document.querySelector<HTMLLIElement>("#teams_list li")!;
    mock.mockClear();
    teamItem.querySelector<HTMLButtonElement>(".group-active")!.click();

    await vi.waitFor((): void => {
        const call = mock.mock.calls.find(
            ([url, init]) => url === url_directoryTeams && init?.body != null)!;
        expect(call).toBeDefined();
        expect(JSON.parse(call[1]!.body as string)).toEqual({ id: 10, active: false });
    });
    await vi.waitFor((): void => {
        expect(document.querySelector("#teams_list li")!.classList.contains("item-hidden")).toBe(true);
    });
});

test("reactivating a position posts active=true", async (): Promise<void> => {
    const mock = await initAdminDirectoryPage();

    const positionItem = document.querySelector<HTMLLIElement>("#positions_list li")!;
    mock.mockClear();
    positionItem.querySelector<HTMLButtonElement>(".group-inactive")!.click();

    await vi.waitFor((): void => {
        const call = mock.mock.calls.find(
            ([url, init]) => url === url_directoryPositions && init?.body != null)!;
        expect(call).toBeDefined();
        expect(JSON.parse(call[1]!.body as string)).toEqual({ id: 20, active: true });
    });
});

test("renaming a team posts the title typed into the prompt", async (): Promise<void> => {
    vi.stubGlobal("prompt", vi.fn((): string => "Green Dots"));
    const mock = await initAdminDirectoryPage();

    mock.mockClear();
    document.querySelector<HTMLButtonElement>("#teams_list li .group-rename")!.click();

    await vi.waitFor((): void => {
        const call = mock.mock.calls.find(
            ([url, init]) => url === url_directoryTeams && init?.body != null)!;
        expect(call).toBeDefined();
        expect(JSON.parse(call[1]!.body as string)).toEqual({ id: 10, title: "Green Dots" });
    });
});

// Renaming changes which event access rules match the group's members, so a
// cancelled or unchanged prompt must not send anything.
test("cancelling or not changing the rename prompt sends nothing", async (): Promise<void> => {
    const cancelled = vi.fn((): string | null => null);
    vi.stubGlobal("prompt", cancelled);
    const mock = await initAdminDirectoryPage();

    mock.mockClear();
    document.querySelector<HTMLButtonElement>("#teams_list li .group-rename")!.click();
    // The prompt call proves the handler ran and reached its decision, so the
    // absence of a request below isn't just a race.
    expect(cancelled).toHaveBeenCalledOnce();
    expect(mock.mock.calls.some(([, init]) => init?.body != null)).toBe(false);

    const unchanged = vi.fn((): string => "Green Dot");
    vi.stubGlobal("prompt", unchanged);
    document.querySelector<HTMLButtonElement>("#teams_list li .group-rename")!.click();
    expect(unchanged).toHaveBeenCalledOnce();
    expect(mock.mock.calls.some(([, init]) => init?.body != null)).toBe(false);
});

test("deleting a team asks first, then DELETEs it", async (): Promise<void> => {
    const confirmSpy = vi.fn((): boolean => true);
    vi.stubGlobal("confirm", confirmSpy);
    const mock = await initAdminDirectoryPage();

    mock.mockClear();
    document.querySelector<HTMLButtonElement>("#teams_list li .group-delete")!.click();

    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url, init]) =>
            url === "/ims/api/directory/teams/10" && init?.method === "DELETE")).toBe(true);
    });
    expect(confirmSpy).toHaveBeenCalledOnce();
    await vi.waitFor((): void => {
        expect(document.querySelectorAll("#teams_list li").length).toBe(0);
    });
});

test("declining the team delete confirmation leaves it alone", async (): Promise<void> => {
    const confirmSpy = vi.fn((): boolean => false);
    vi.stubGlobal("confirm", confirmSpy);
    const mock = await initAdminDirectoryPage();

    mock.mockClear();
    document.querySelector<HTMLButtonElement>("#teams_list li .group-delete")!.click();

    // The confirm call proves the handler ran and stopped there.
    expect(confirmSpy).toHaveBeenCalledOnce();
    expect(mock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);
    expect(document.querySelectorAll("#teams_list li").length).toBe(1);
});

test("a failed person edit reports the failure and marks the control invalid", async (): Promise<void> => {
    const alertSpy = vi.fn();
    vi.stubGlobal("alert", alertSpy);

    await initAdminDirectoryPage((url, init) => {
        if (url === url_directoryPersons && init?.body != null) {
            return problemResponse("That handle is already taken", 400);
        }
        return directoryRoutes(url, init);
    });
    openPersonModal(0);

    const handleField = modalInput("edit_person_handle");
    handleField.value = "Slacker";
    await window.setPersonHandle(handleField);

    expect(alertSpy).toHaveBeenCalledOnce();
    expect(handleField.classList.contains("is-invalid")).toBe(true);
    expect(personRows()[0]!.querySelector(".person-handle")!.textContent).toBe("Defect");
});
