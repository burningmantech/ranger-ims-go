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

// Tests for admin_events.ts against the real templ-rendered events
// admin page (adminevents.templ).

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

interface AccessLike {
    expression: string;
    validity: string;
    not_before?: string|null;
    not_after?: string|null;
    pending?: boolean|null;
    expired?: boolean|null;
    debug_info?: {known_target?: boolean|null}|null;
}

let serverEvents: ims.EventData[];
let serverACL: Record<string, Partial<Record<string, AccessLike[]>>>;
let serverAuth: object;

beforeEach((): void => {
    vi.resetModules();
    loadFixture("admin_events.html");
    serverAuth = { authenticated: true, user: "Tester", admin: true };
    serverEvents = [
        { id: 1, name: "2025" },
    ];
    serverACL = {
        "2025": {
            readers: [
                { expression: "person:Tool", validity: "always", debug_info: { known_target: true } },
            ],
            writers: [
                {
                    expression: "position:007",
                    validity: "onsite",
                    not_before: "2025-08-20T00:00:00Z",
                    not_after: "2025-09-10T00:00:00Z",
                    debug_info: { known_target: true },
                },
            ],
        },
    };
});

// Import admin_events.ts with a fake server behind it, and wait for the page
// to finish drawing the event access cards.
async function initAdminEventsPage() {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse(serverAuth);
        }
        if (url === url_event.replace("<event_id>", "2025") && init?.method === "DELETE") {
            return new Response(null, { status: 204 });
        }
        if ((url === url_events || url === url_events + "?include_groups=true") && init?.body == null) {
            return jsonResponse(serverEvents);
        }
        if (url === url_acl && init?.body == null) {
            return jsonResponse(serverACL);
        }
        if (url === url_acl && init?.body != null) {
            return new Response(null, { status: 204 });
        }
        if (url === url_accessTargets && init?.body == null) {
            return jsonResponse({ persons: ["Tool"], positions: ["007"], teams: ["Council"] });
        }
        return undefined;
    });
    await import("../typescript/admin_events.ts");
    await vi.waitFor((): void => {
        expect(eventCards().length).toBeGreaterThan(0);
    });
    return mock;
}

function eventCards(): HTMLElement[] {
    return [...document.querySelectorAll<HTMLElement>("#event_access_container .event_access")];
}

function ruleRows(card: HTMLElement): HTMLTableRowElement[] {
    return [...card.querySelectorAll<HTMLTableRowElement>("tr.access_rule")];
}

test("event cards are rendered with their access rules", async (): Promise<void> => {
    await initAdminEventsPage();

    const cards = eventCards();
    expect(cards.length).toBe(1);
    const card = cards[0]!;
    expect(card.querySelector(".event_name")!.textContent).toBe("2025");
    expect(card.querySelector(".rule_count")!.textContent).toBe("2 rules");
    // No rules have issues, so the issue badge stays hidden. The event has
    // rules, though, so its rule table auto-expands.
    expect(card.querySelector(".issue_count")!.classList.contains("d-none")).toBe(true);
    expect(card.querySelector(".access_rules_collapse")!.classList.contains("show")).toBe(true);

    // Rows are drawn in access mode order: readers before writers.
    const rows = ruleRows(card);
    expect(rows.length).toBe(2);

    const reader = rows[0]!;
    expect((reader.querySelector(".access_level") as HTMLSelectElement).value).toBe("readers");
    expect(reader.querySelector(".access_expression")!.textContent).toBe("person:Tool");
    expect((reader.querySelector(".access_validity") as HTMLSelectElement).value).toBe("always");

    const writer = rows[1]!;
    expect((writer.querySelector(".access_level") as HTMLSelectElement).value).toBe("writers");
    expect(writer.querySelector(".access_expression")!.textContent).toBe("position:007");
    expect((writer.querySelector(".access_validity") as HTMLSelectElement).value).toBe("onsite");
});

test("a rule with an unknown target is flagged and its event auto-expands", async (): Promise<void> => {
    serverACL["2025"]!["readers"] = [
        { expression: "person:Typo", validity: "always", debug_info: { known_target: false } },
    ];
    await initAdminEventsPage();

    const card = eventCards()[0]!;
    expect(card.querySelector(".issue_count")!.classList.contains("d-none")).toBe(false);
    expect(card.querySelector(".issue_count")!.textContent).toBe("1 issue");
    expect(card.querySelector(".access_rules_collapse")!.classList.contains("show")).toBe(true);

    const row = ruleRows(card)[0]!;
    expect(row.classList.contains("table-danger")).toBe(true);
    expect(row.querySelector(".unknown_target_text")!.classList.contains("d-none")).toBe(false);
    expect(row.querySelector(".fix_button")!.classList.contains("d-none")).toBe(false);
});

test("date fields are always shown, with a rule's dates displayed in them", async (): Promise<void> => {
    await initAdminEventsPage();

    const rows = ruleRows(eventCards()[0]!);

    // Both rows show their date editors inline, with no toggle or badge.
    const reader = rows[0]!;
    expect(reader.querySelector(".access_dates")!.classList.contains("d-none")).toBe(false);
    // The dateless reader rule has empty date inputs.
    expect((reader.querySelector(".access_not_before") as HTMLInputElement).value).toBe("");
    expect((reader.querySelector(".access_not_after") as HTMLInputElement).value).toBe("");

    // The writer rule has dates, shown formatted like "Sun 2025-08-24 @ 12:00"
    // in the browser's time zone (so an exact match here would be TZ-dependent).
    const writer = rows[1]!;
    expect(writer.querySelector(".access_dates")!.classList.contains("d-none")).toBe(false);
    const dateFormat = /^[A-Z][a-z]{2} \d{4}-\d{2}-\d{2} @ \d{2}:\d{2}$/;
    expect((writer.querySelector(".access_not_before") as HTMLInputElement).value).toMatch(dateFormat);
    expect((writer.querySelector(".access_not_after") as HTMLInputElement).value).toMatch(dateFormat);
});

test("typing a date into a rule's field posts the parsed not-before time", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const reader = ruleRows(eventCards()[0]!)[0]!;
    const notBefore = reader.querySelector(".access_not_before") as HTMLInputElement;
    notBefore.value = "Sun 2025-08-24 @ 12:00";
    notBefore.dispatchEvent(new Event("change"));

    const postCall = await vi.waitFor(() => {
        const call = mock.mock.calls.find(
            ([url, init]) => url === url_acl && init?.body != null,
        );
        expect(call).toBeDefined();
        return call!;
    });
    const body = JSON.parse(postCall[1]!.body as string);
    const rule = body["2025"].readers[0];
    expect(rule.expression).toBe("person:Tool");
    // The typed local time is serialized as a UTC ISO instant.
    expect(rule.not_before).toBe(new Date(2025, 7, 24, 12, 0).toISOString());
});

test("clearing a rule's date field posts a null not-before time", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const writer = ruleRows(eventCards()[0]!)[1]!;
    const notBefore = writer.querySelector(".access_not_before") as HTMLInputElement;
    notBefore.value = "";
    notBefore.dispatchEvent(new Event("change"));

    const postCall = await vi.waitFor(() => {
        const call = mock.mock.calls.find(
            ([url, init]) => url === url_acl && init?.body != null,
        );
        expect(call).toBeDefined();
        return call!;
    });
    const body = JSON.parse(postCall[1]!.body as string);
    expect(body["2025"].writers[0].not_before).toBe(null);
});

test("clicking the card header toggles the collapse, except on its buttons", async (): Promise<void> => {
    await initAdminEventsPage();

    const card = eventCards()[0]!;
    // Bootstrap's collapse plugin isn't loaded in these tests, so observe the
    // delegated header click through the toggle button it forwards to.
    const toggleClicks = vi.fn();
    card.querySelector(".access_collapse_toggle")!.addEventListener("click", toggleClicks);

    (card.querySelector(".card-header") as HTMLElement).click();
    expect(toggleClicks).toHaveBeenCalledTimes(1);
    (card.querySelector(".rule_count") as HTMLElement).click();
    expect(toggleClicks).toHaveBeenCalledTimes(2);

    // Clicks on the header's other buttons are left alone.
    (card.querySelector(".show-edit-modal") as HTMLButtonElement).click();
    (card.querySelector(".explain_button") as HTMLButtonElement).click();
    expect(toggleClicks).toHaveBeenCalledTimes(2);
});

test("addAccess posts the new rule for the selected access level", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const card = eventCards()[0]!;
    // adminevents.templ wires the add input with onchange="addAccess(this)".
    const addInput = card.querySelector(".access_add") as HTMLInputElement;
    expect(addInput.getAttribute("onchange")).toBe("addAccess(this)");

    (card.querySelector(".access_add_level") as HTMLSelectElement).value = "reporters";
    addInput.value = "team:Council";
    await window.addAccess(addInput);

    const postCall = mock.mock.calls.find(
        ([url, init]) => url === url_acl && init?.body != null,
    )!;
    expect(JSON.parse(postCall[1]!.body as string)).toEqual({
        "2025": {
            reporters: [{ expression: "team:Council", validity: "always" }],
        },
    });
});

test("setLevel moves a rule to the other access mode", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const reader = ruleRows(eventCards()[0]!)[0]!;
    const levelSelect = reader.querySelector(".access_level") as HTMLSelectElement;
    levelSelect.value = "writers";
    await window.setLevel(levelSelect);

    const postCall = mock.mock.calls.find(
        ([url, init]) => url === url_acl && init?.body != null,
    )!;
    const edits = JSON.parse(postCall[1]!.body as string);
    // The rule leaves the readers list and joins the existing writers list.
    expect(edits["2025"].readers).toEqual([]);
    const writerExpressions = edits["2025"].writers.map((a: AccessLike) => a.expression);
    expect(writerExpressions).toEqual(["position:007", "person:Tool"]);
});

test("the delete button is disabled when the server disallows event deletion", async (): Promise<void> => {
    await initAdminEventsPage();

    const deleteButton = document.getElementById("event_delete") as HTMLButtonElement;
    expect(deleteButton.disabled).toBe(true);
    expect(document.getElementById("event_delete_wrapper")!.title).toContain("disabled");
});

test("the delete button DELETEs the event after confirmation", async (): Promise<void> => {
    serverAuth = { authenticated: true, user: "Tester", admin: true, event_deletion_allowed: true };
    const mock = await initAdminEventsPage();

    const deleteButton = document.getElementById("event_delete") as HTMLButtonElement;
    expect(deleteButton.disabled).toBe(false);

    // Open the edit modal for the event, which is what tells the delete
    // button which event it applies to.
    (eventCards()[0]!.querySelector(".show-edit-modal") as HTMLButtonElement).click();

    const deleteCall = ([url, init]: [url: string, init?: RequestInit | undefined]): boolean =>
        url === url_event.replace("<event_id>", "2025") && init?.method === "DELETE";

    // Declining the confirmation sends nothing.
    const confirmMock = vi.fn().mockReturnValueOnce(false);
    vi.stubGlobal("confirm", confirmMock);
    deleteButton.click();
    await vi.waitFor((): void => {
        expect(confirmMock).toHaveBeenCalledTimes(1);
    });
    expect(mock.mock.calls.find(deleteCall)).toBeUndefined();

    // Accepting it deletes the event.
    confirmMock.mockReturnValueOnce(true);
    deleteButton.click();
    await vi.waitFor((): void => {
        expect(mock.mock.calls.find(deleteCall)).toBeDefined();
    });
});

test("removeAccess posts the ACL without the removed rule", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const reader = ruleRows(eventCards()[0]!)[0]!;
    await window.removeAccess(reader.querySelector("[aria-label='Remove rule']") as HTMLButtonElement);

    const postCall = mock.mock.calls.find(
        ([url, init]) => url === url_acl && init?.body != null,
    )!;
    expect(JSON.parse(postCall[1]!.body as string)).toEqual({
        "2025": { readers: [] },
    });
});
