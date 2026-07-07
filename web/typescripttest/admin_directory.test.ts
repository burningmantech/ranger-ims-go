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
import { jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

interface ServerDirectory {
    persons: object[];
    teams: object[];
    positions: object[];
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

// Import admin_directory.ts with a fake server behind it, and wait for the
// page to finish drawing the persons table.
async function initAdminDirectoryPage() {
    const mock = mockFetch((url, init) => {
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
            const edits = JSON.parse(init.body as string) as { id?: number, handle?: string };
            if (edits.id == null) {
                serverDirectory.persons.push({
                    id: 100, handle: edits.handle ?? "", email: null,
                    active: true, onsite: false, team_ids: [], position_ids: [],
                });
            }
            return new Response(null, { status: 204 });
        }
        if ((url === url_directoryTeams || url === url_directoryPositions) && init?.body != null) {
            return new Response(null, { status: 204 });
        }
        return undefined;
    });
    await import("../typescript/admin_directory.ts");
    await vi.waitFor((): void => {
        expect(personRows().length).toBeGreaterThan(0);
    });
    return mock;
}

function personRows(): HTMLTableRowElement[] {
    return [...document.querySelectorAll<HTMLTableRowElement>("#persons_tbody tr")];
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
