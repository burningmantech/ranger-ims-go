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
    description?: string|null;
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

function grantBlocks(card: HTMLElement): HTMLElement[] {
    return [...card.querySelectorAll<HTMLElement>(".grant")];
}

function whoChips(grant: HTMLElement): HTMLElement[] {
    return [...grant.querySelectorAll<HTMLElement>(".who-chip")];
}

// Find the most recent POST to the ACL endpoint and return its parsed body.
function lastACLPost(mock: ReturnType<typeof mockFetch>): Record<string, Record<string, AccessLike[]>> {
    const call = mock.mock.calls.findLast(
        ([url, init]) => url === url_acl && init?.body != null,
    );
    expect(call).toBeDefined();
    return JSON.parse(call![1]!.body as string);
}

test("event cards render their rules grouped into grant blocks", async (): Promise<void> => {
    await initAdminEventsPage();

    const cards = eventCards();
    expect(cards.length).toBe(1);
    const card = cards[0]!;
    expect(card.querySelector(".event_name")!.textContent).toBe("2025");
    // Two rules with different terms form two grants, one expression in each.
    expect(card.querySelector(".rule_count")!.textContent).toBe("2 grants · 2 expressions");
    // No rules have issues, so the issue badge stays hidden. The event has
    // rules, though, so its grant list auto-expands.
    expect(card.querySelector(".issue_count")!.classList.contains("d-none")).toBe(true);
    expect(card.querySelector(".access_rules_collapse")!.classList.contains("show")).toBe(true);

    // Grants are drawn in access mode order: readers before writers.
    const grants = grantBlocks(card);
    expect(grants.length).toBe(2);

    const reader = grants[0]!;
    expect(reader.querySelector(".grant_level_badge")!.textContent).toBe("Read all");
    expect(reader.querySelector(".grant_when_badge")!.textContent).toBe("Always");
    const readerWhos = whoChips(reader);
    expect(readerWhos.length).toBe(1);
    expect(readerWhos[0]!.querySelector(".who_expression")!.textContent).toBe("person:Tool");

    const writer = grants[1]!;
    expect(writer.querySelector(".grant_level_badge")!.textContent).toBe("Write all");
    expect(writer.querySelector(".grant_when_badge")!.textContent).toBe("On-Site");
    const writerWhos = whoChips(writer);
    expect(writerWhos.length).toBe(1);
    expect(writerWhos[0]!.querySelector(".who_expression")!.textContent).toBe("position:007");
});

test("a grant with a dateless rule hides its date badges; a dated rule shows them", async (): Promise<void> => {
    await initAdminEventsPage();

    const grants = grantBlocks(eventCards()[0]!);

    // The dateless reader grant shows no date badges.
    const reader = grants[0]!;
    expect(reader.querySelector(".grant_not_before_badge")!.classList.contains("d-none")).toBe(true);
    expect(reader.querySelector(".grant_not_after_badge")!.classList.contains("d-none")).toBe(true);

    // The writer grant has dates, shown as a bare ISO date like "2025-08-24" in
    // the browser's time zone (so an exact match here would be TZ-dependent).
    const writer = grants[1]!;
    const dateFormat = /^not-(before|after) \d{4}-\d{2}-\d{2}$/;
    expect(writer.querySelector(".grant_not_before_badge")!.classList.contains("d-none")).toBe(false);
    expect(writer.querySelector(".grant_not_before_badge")!.textContent).toMatch(dateFormat);
    expect(writer.querySelector(".grant_not_after_badge")!.classList.contains("d-none")).toBe(false);
    expect(writer.querySelector(".grant_not_after_badge")!.textContent).toMatch(dateFormat);

    // The time of day the badges omit stays available on hover.
    const titleFormat = /^Not (before|after): [A-Z][a-z]{2}, \d{4}-\d{2}-\d{2} at \d{2}:\d{2}:\d{2} /;
    expect((writer.querySelector(".grant_not_before_badge") as HTMLElement).title).toMatch(titleFormat);
    expect((writer.querySelector(".grant_not_after_badge") as HTMLElement).title).toMatch(titleFormat);
});

test("a wildcard chip is glossed as granting all authenticated users", async (): Promise<void> => {
    serverACL["2025"]!["readers"] = [
        { expression: "*", validity: "always", debug_info: { known_target: true } },
    ];
    await initAdminEventsPage();

    const chips = whoChips(grantBlocks(eventCards()[0]!)[0]!);
    expect(chips.length).toBe(1);
    // The expression itself stays the bare wildcard; the gloss is a sibling.
    expect(chips[0]!.querySelector(".who_expression")!.textContent).toBe("*");
    const gloss = chips[0]!.querySelector(".who-gloss")!;
    expect(gloss.classList.contains("d-none")).toBe(false);
    expect(gloss.textContent).toBe("(ALL authenticated users)");
});

test("a non-wildcard chip shows no gloss", async (): Promise<void> => {
    await initAdminEventsPage();

    const chips = whoChips(grantBlocks(eventCards()[0]!)[0]!);
    expect(chips[0]!.querySelector(".who_expression")!.textContent).toBe("person:Tool");
    expect(chips[0]!.querySelector(".who-gloss")!.classList.contains("d-none")).toBe(true);
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

    const chip = whoChips(grantBlocks(card)[0]!)[0]!;
    expect(chip.classList.contains("who-unknown")).toBe(true);
    expect(chip.querySelector(".who_fix")!.classList.contains("d-none")).toBe(false);
});

test("adding a who to a grant posts the grant's whos plus the new one", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const reader = grantBlocks(eventCards()[0]!)[0]!;
    const addInput = reader.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "team:Council";
    addInput.dispatchEvent(new Event("change"));

    const body = await vi.waitFor(() => lastACLPost(mock));
    const expressions = body["2025"]!["readers"]!.map(a => a.expression);
    expect(expressions).toEqual(["person:Tool", "team:Council"]);
    // The new rule inherits the grant's terms (Read all, Always, no dates).
    const added = body["2025"]!["readers"]!.find(a => a.expression === "team:Council")!;
    expect(added.validity).toBe("always");
    expect(added.not_before).toBe(null);
    expect(added.not_after).toBe(null);
});

test("a new grant posts a rule at the chosen access level", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    (eventCards()[0]!.querySelector(".new_grant_button") as HTMLButtonElement).click();

    // newGrant redraws the container, so re-query for the fresh card and draft.
    const draft = eventCards()[0]!.querySelector(".grant-draft") as HTMLElement;
    const levelSelect = draft.querySelector(".grant_level") as HTMLSelectElement;
    levelSelect.value = "reporters";
    levelSelect.dispatchEvent(new Event("change"));

    const addInput = draft.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "team:Council";
    addInput.dispatchEvent(new Event("change"));

    const body = await vi.waitFor(() => lastACLPost(mock));
    expect(body).toEqual({
        "2025": {
            reporters: [{ expression: "team:Council", validity: "always", not_before: null, not_after: null, description: "" }],
        },
    });
});

test("editing a grant's terms to a new level moves its whos to that mode", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const reader = grantBlocks(eventCards()[0]!)[0]!;
    (reader.querySelector(".grant_edit_terms") as HTMLButtonElement).click();
    const levelSelect = reader.querySelector(".grant_level") as HTMLSelectElement;
    levelSelect.value = "writers";
    (reader.querySelector(".grant_apply_terms") as HTMLButtonElement).click();

    const body = await vi.waitFor(() => lastACLPost(mock));
    // The rule leaves the readers list and joins the existing writers list.
    expect(body["2025"]!["readers"]).toEqual([]);
    const writerExpressions = body["2025"]!["writers"]!.map(a => a.expression);
    expect(writerExpressions).toEqual(["position:007", "person:Tool"]);
});

test("editing a grant's not-before date posts the parsed time for its whos", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const reader = grantBlocks(eventCards()[0]!)[0]!;
    (reader.querySelector(".grant_edit_terms") as HTMLButtonElement).click();
    const notBefore = reader.querySelector(".grant_not_before") as HTMLInputElement;
    notBefore.value = "Sun 2025-08-24 @ 12:00";
    (reader.querySelector(".grant_apply_terms") as HTMLButtonElement).click();

    const body = await vi.waitFor(() => lastACLPost(mock));
    const rule = body["2025"]!["readers"]![0]!;
    expect(rule.expression).toBe("person:Tool");
    // The typed local time is serialized as a UTC ISO instant.
    expect(rule.not_before).toBe(new Date(2025, 7, 24, 12, 0).toISOString());
});

test("clearing a grant's date field posts a null time for its whos", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const writer = grantBlocks(eventCards()[0]!)[1]!;
    (writer.querySelector(".grant_edit_terms") as HTMLButtonElement).click();
    const notBefore = writer.querySelector(".grant_not_before") as HTMLInputElement;
    notBefore.value = "";
    (writer.querySelector(".grant_apply_terms") as HTMLButtonElement).click();

    const body = await vi.waitFor(() => lastACLPost(mock));
    expect(body["2025"]!["writers"]![0]!.not_before).toBe(null);
});

test("rules with the same terms but different descriptions form separate grants, each showing its description", async (): Promise<void> => {
    serverACL["2025"]!["readers"] = [
        { expression: "person:Tool", validity: "always", description: "Sanctuary leads", debug_info: { known_target: true } },
        { expression: "person:Hubcap", validity: "always", description: "Comms team", debug_info: { known_target: true } },
    ];
    delete serverACL["2025"]!["writers"];
    await initAdminEventsPage();

    const grants = grantBlocks(eventCards()[0]!);
    // Same level/validity/dates but different descriptions => two distinct grants.
    expect(grants.length).toBe(2);
    for (const grant of grants) {
        expect(whoChips(grant).length).toBe(1);
    }
    const descriptions = grants.map(g => g.querySelector(".grant_description_display")!.textContent);
    expect(new Set(descriptions)).toEqual(new Set(["\"Sanctuary leads\"", "\"Comms team\""]));
});

test("a grant with no description hides its description display", async (): Promise<void> => {
    await initAdminEventsPage();

    const reader = grantBlocks(eventCards()[0]!)[0]!;
    expect(reader.querySelector(".grant_description_display")!.classList.contains("d-none")).toBe(true);
});

test("adding a who to a grant inherits the grant's description", async (): Promise<void> => {
    serverACL["2025"]!["readers"] = [
        { expression: "person:Tool", validity: "always", description: "Sanctuary leads", debug_info: { known_target: true } },
    ];
    const mock = await initAdminEventsPage();

    const reader = grantBlocks(eventCards()[0]!)[0]!;
    const addInput = reader.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "team:Council";
    addInput.dispatchEvent(new Event("change"));

    const body = await vi.waitFor(() => lastACLPost(mock));
    const added = body["2025"]!["readers"]!.find(a => a.expression === "team:Council")!;
    expect(added.description).toBe("Sanctuary leads");
});

test("editing a grant's description posts the new description for all its whos", async (): Promise<void> => {
    serverACL["2025"]!["readers"] = [
        { expression: "person:Tool", validity: "always", description: "old reason", debug_info: { known_target: true } },
        { expression: "person:Hubcap", validity: "always", description: "old reason", debug_info: { known_target: true } },
    ];
    delete serverACL["2025"]!["writers"];
    const mock = await initAdminEventsPage();

    // Both rules share terms and description, so they're one grant with two whos.
    const reader = grantBlocks(eventCards()[0]!)[0]!;
    expect(whoChips(reader).length).toBe(2);
    (reader.querySelector(".grant_edit_terms") as HTMLButtonElement).click();
    const descInput = reader.querySelector(".grant_description") as HTMLInputElement;
    // The edit control is pre-populated with the grant's current description.
    expect(descInput.value).toBe("old reason");
    descInput.value = "new reason";
    (reader.querySelector(".grant_apply_terms") as HTMLButtonElement).click();

    const body = await vi.waitFor(() => lastACLPost(mock));
    const readers = body["2025"]!["readers"]!;
    expect(readers.map(a => a.expression).sort()).toEqual(["person:Hubcap", "person:Tool"]);
    expect(readers.every(a => a.description === "new reason")).toBe(true);
});

test("removing a who posts the grant's mode without that rule", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const reader = grantBlocks(eventCards()[0]!)[0]!;
    (whoChips(reader)[0]!.querySelector(".who-remove") as HTMLButtonElement).click();

    const body = await vi.waitFor(() => lastACLPost(mock));
    expect(body).toEqual({ "2025": { readers: [] } });
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
