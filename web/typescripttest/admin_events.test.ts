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
import { answerDialog, change, type FetchHandler, jsonResponse, loadFixture, mockFetch, problemResponse } from "./helpers.ts";

let page: typeof import("../typescript/admin_events.ts");

interface AccessLike {
    expression: string;
    validity: string;
    not_before?: string|null;
    not_after?: string|null;
    description?: string|null;
    pending?: boolean|null;
    expired?: boolean|null;
    debug_info?: {
        known_target?: boolean|null;
        matches_users?: string[]|null;
        matches_all_users?: boolean|null;
        matches_no_one?: boolean|null;
    }|null;
}

let serverEvents: ims.EventData[];
let serverACL: Record<string, Partial<Record<string, AccessLike[]>>>;
let serverAuth: object;
// Consulted before the fake server's normal routes, so that a test can fail
// one of them.
let routeOverride: FetchHandler;

beforeEach((): void => {
    vi.resetModules();
    loadFixture("admin_events.html");
    serverAuth = { authenticated: true, user: "Tester", admin: true };
    routeOverride = (): undefined => undefined;
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

// Install a fake server behind fetch, without loading the page.
function mockServer() {
    return mockFetch((url, init) => {
        const overridden = routeOverride(url, init);
        if (overridden != null) {
            return overridden;
        }
        if (url === url_logout) {
            return new Response(null, { status: 204 });
        }
        if (url === url_auth && init?.body == null) {
            return jsonResponse(serverAuth);
        }
        if (url === url_event.replace("<event_id>", "2025") && init?.method === "DELETE") {
            return new Response(null, { status: 204 });
        }
        if ((url === url_events || url === url_events + "?include_groups=true") && init?.body == null) {
            return jsonResponse(serverEvents);
        }
        if (url === url_events && init?.body != null) {
            return new Response(null, { status: 204 });
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
}

// Import admin_events.ts with a fake server behind it, and wait for the page
// to finish drawing the event access cards.
async function initAdminEventsPage() {
    const mock = mockServer();
    page = await import("../typescript/admin_events.ts");
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
    expect(!(card.querySelector(".access_rules_collapse") as HTMLElement).hidden).toBe(true);

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
    expect(!(card.querySelector(".access_rules_collapse") as HTMLElement).hidden).toBe(true);

    const chip = whoChips(grantBlocks(card)[0]!)[0]!;
    expect(chip.classList.contains("who-unknown")).toBe(true);
    expect(chip.querySelector(".who_fix")!.classList.contains("d-none")).toBe(false);
});

test("adding a who to a grant posts the grant's whos plus the new one", async (): Promise<void> => {
    serverACL["2025"]!["readers"] = [
        { expression: "person:Tool", validity: "always", expired: false, pending: false, debug_info: { known_target: true } },
    ];
    const mock = await initAdminEventsPage();

    const reader = grantBlocks(eventCards()[0]!)[0]!;
    const addInput = reader.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "team:Council";
    addInput.dispatchEvent(new Event("change"));

    const body = await vi.waitFor(() => lastACLPost(mock));
    const expressions = body["2025"]!["readers"]!.map(a => a.expression);
    expect(expressions).toEqual(["person:Tool", "team:Council"]);
    // The existing rule came back from the server with response-only fields,
    // which aren't sent back.
    for (const rule of body["2025"]!["readers"]!) {
        expect(rule).not.toHaveProperty("debug_info");
        expect(rule).not.toHaveProperty("expired");
        expect(rule).not.toHaveProperty("pending");
    }
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
    expect(new Set(descriptions)).toEqual(new Set(["Sanctuary leads", "Comms team"]));
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

// One expression may hold several access modes on an event at once (the server
// replaces rules one mode at a time, not one expression at a time), so the page
// has to show it under each mode and edit each mode independently.

test("one expression in two modes renders as two grants, one chip each", async (): Promise<void> => {
    serverACL["2025"] = {
        readers: [
            { expression: "person:Tool", validity: "always", debug_info: { known_target: true } },
        ],
        visit_writers: [
            { expression: "person:Tool", validity: "always", debug_info: { known_target: true } },
        ],
    };
    await initAdminEventsPage();

    const card = eventCards()[0]!;
    // Two grants, and person:Tool is counted once per mode it holds.
    expect(card.querySelector(".rule_count")!.textContent).toBe("2 grants · 2 expressions");
    // Holding two modes isn't an issue, so nothing is flagged.
    expect(card.querySelector(".issue_count")!.classList.contains("d-none")).toBe(true);

    const grants = grantBlocks(card);
    expect(grants.length).toBe(2);
    expect(grants.map(g => g.querySelector(".grant_level_badge")!.textContent))
        .toEqual(["Read all", "Write visits"]);
    for (const grant of grants) {
        const chips = whoChips(grant);
        expect(chips.length).toBe(1);
        expect(chips[0]!.querySelector(".who_expression")!.textContent).toBe("person:Tool");
    }
});

test("adding a who to one mode posts only that mode, leaving its other modes alone", async (): Promise<void> => {
    serverACL["2025"] = {
        readers: [
            { expression: "person:Tool", validity: "always", debug_info: { known_target: true } },
        ],
        visit_writers: [],
    };
    const mock = await initAdminEventsPage();

    // Make person:Tool a visit writer too, by adding it to a new grant at that
    // level. The reader rule for the same expression must not be touched.
    (eventCards()[0]!.querySelector(".new_grant_button") as HTMLButtonElement).click();
    const draft = eventCards()[0]!.querySelector(".grant-draft") as HTMLElement;
    const levelSelect = draft.querySelector(".grant_level") as HTMLSelectElement;
    levelSelect.value = "visit_writers";
    levelSelect.dispatchEvent(new Event("change"));
    const addInput = draft.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "person:Tool";
    addInput.dispatchEvent(new Event("change"));

    const body = await vi.waitFor(() => lastACLPost(mock));
    // Only visit_writers is in the body: readers isn't posted at all, so the
    // server leaves that mode's rules exactly as they are.
    expect(Object.keys(body["2025"]!)).toEqual(["visit_writers"]);
    expect(body["2025"]!["visit_writers"]!.map(a => a.expression)).toEqual(["person:Tool"]);
});

test("removing a who from one mode posts only that mode, keeping its other modes", async (): Promise<void> => {
    serverACL["2025"] = {
        readers: [
            { expression: "person:Tool", validity: "always", debug_info: { known_target: true } },
        ],
        visit_writers: [
            { expression: "person:Tool", validity: "always", debug_info: { known_target: true } },
        ],
    };
    const mock = await initAdminEventsPage();

    // Revoke only the visit-writer grant.
    const visitWriter = grantBlocks(eventCards()[0]!)[1]!;
    expect(visitWriter.querySelector(".grant_level_badge")!.textContent).toBe("Write visits");
    (whoChips(visitWriter)[0]!.querySelector(".who-remove") as HTMLButtonElement).click();

    const body = await vi.waitFor(() => lastACLPost(mock));
    expect(body).toEqual({ "2025": { visit_writers: [] } });
});

test("adding a who to a mode that expression already holds doesn't duplicate it", async (): Promise<void> => {
    serverACL["2025"] = {
        readers: [
            { expression: "person:Tool", validity: "always", debug_info: { known_target: true } },
        ],
    };
    const mock = await initAdminEventsPage();

    // Re-adding an expression to a mode it's already in replaces the old rule
    // rather than stacking a second one, since the server no longer dedupes.
    const reader = grantBlocks(eventCards()[0]!)[0]!;
    const addInput = reader.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "person:Tool";
    addInput.dispatchEvent(new Event("change"));

    const body = await vi.waitFor(() => lastACLPost(mock));
    expect(body["2025"]!["readers"]!.map(a => a.expression)).toEqual(["person:Tool"]);
});

test("the explain modal lists an expression under each mode it holds", async (): Promise<void> => {
    serverACL["2025"] = {
        readers: [
            {
                expression: "person:Tool",
                validity: "always",
                debug_info: { known_target: true, matches_users: ["Tool"] },
            },
        ],
        visit_writers: [
            {
                expression: "person:Tool",
                validity: "onsite",
                debug_info: { known_target: true, matches_users: ["Tool"] },
            },
        ],
    };
    await initAdminEventsPage();

    (eventCards()[0]!.querySelector(".explain_button") as HTMLButtonElement).click();

    const body = document.querySelector("#explainModal .modal-body")!.textContent!;
    expect(body).toContain("Read all:");
    expect(body).toContain("person:Tool (always)");
    expect(body).toContain("Write visits:");
    expect(body).toContain("person:Tool (onsite)");
});

test("clicking the card header toggles the grant list, except on its buttons", async (): Promise<void> => {
    await initAdminEventsPage();

    const card = eventCards()[0]!;
    const toggle = card.querySelector(".access_collapse_toggle") as HTMLButtonElement;
    const grantList = card.querySelector(".access_rules_collapse") as HTMLElement;
    // The event has rules, so it starts expanded.
    expect(grantList.hidden).toBe(false);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");

    (card.querySelector(".card-header") as HTMLElement).click();
    expect(grantList.hidden).toBe(true);
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    (card.querySelector(".rule_count") as HTMLElement).click();
    expect(grantList.hidden).toBe(false);
    toggle.click();
    expect(grantList.hidden).toBe(true);

    // Clicks on the header's other buttons are left alone.
    (card.querySelector(".show-edit-modal") as HTMLButtonElement).click();
    (card.querySelector(".explain_button") as HTMLButtonElement).click();
    expect(grantList.hidden).toBe(true);
});

test("the edit modal shows the event's address normalization setting, and toggling it posts the change", async (): Promise<void> => {
    serverEvents = [{ id: 1, name: "2025", normalize_addresses: true }];
    const mock = await initAdminEventsPage();

    (eventCards()[0]!.querySelector(".show-edit-modal") as HTMLButtonElement).click();

    expect(document.getElementById("edit_normalize_addresses_group")!.classList.contains("d-none")).toBe(false);
    const normalize = document.getElementById("edit_normalize_addresses") as HTMLInputElement;
    expect(normalize.checked).toBe(true);

    normalize.checked = false;
    change(normalize);

    const call = await vi.waitFor(() => {
        const call = mock.mock.calls.findLast(([url, init]) => url === url_events && init?.body != null);
        expect(call).toBeDefined();
        return call;
    });
    expect(JSON.parse(call![1]!.body as string)).toEqual({ id: 1, name: null, normalize_addresses: false });
});

test("the active event's card shows an Active badge", async (): Promise<void> => {
    serverEvents = [{ id: 1, name: "2025" }, { id: 2, name: "2026", is_active: true }];
    await initAdminEventsPage();

    const badges = new Map(eventCards().map((card) => [
        card.dataset["eventName"],
        !card.querySelector(".active_badge")!.classList.contains("d-none"),
    ]));
    expect(badges.get("2025")).toBe(false);
    expect(badges.get("2026")).toBe(true);
});

test("the edit modal shows whether the event is active, and toggling it posts the change", async (): Promise<void> => {
    serverEvents = [{ id: 1, name: "2025" }];
    const mock = await initAdminEventsPage();

    (eventCards()[0]!.querySelector(".show-edit-modal") as HTMLButtonElement).click();

    expect(document.getElementById("edit_is_active_group")!.classList.contains("d-none")).toBe(false);
    const isActive = document.getElementById("edit_is_active") as HTMLInputElement;
    expect(isActive.checked).toBe(false);

    isActive.checked = true;
    change(isActive);

    const call = await vi.waitFor(() => {
        const call = mock.mock.calls.findLast(([url, init]) => url === url_events && init?.body != null);
        expect(call).toBeDefined();
        return call;
    });
    expect(JSON.parse(call![1]!.body as string)).toEqual({ id: 1, name: null, is_active: true });
});

test("the edit modal hides the active setting for event groups", async (): Promise<void> => {
    serverEvents = [{ id: 1, name: "2025", is_group: true }];
    await initAdminEventsPage();

    (eventCards()[0]!.querySelector(".show-edit-modal") as HTMLButtonElement).click();

    expect(document.getElementById("edit_is_active_group")!.classList.contains("d-none")).toBe(true);
});

test("the edit modal hides address normalization for event groups", async (): Promise<void> => {
    serverEvents = [{ id: 1, name: "2025", is_group: true }];
    await initAdminEventsPage();

    (eventCards()[0]!.querySelector(".show-edit-modal") as HTMLButtonElement).click();

    expect(document.getElementById("edit_normalize_addresses_group")!.classList.contains("d-none")).toBe(true);
});

test("the edit modal shows the event's release times, blank where there's no embargo", async (): Promise<void> => {
    serverEvents = [{
        id: 1,
        name: "2025",
        camp_locations_release: new Date(2025, 7, 20, 9, 0).toISOString(),
        map_url_release: new Date(2025, 7, 22, 18, 30).toISOString(),
    }];
    await initAdminEventsPage();

    (eventCards()[0]!.querySelector(".show-edit-modal") as HTMLButtonElement).click();

    expect(document.getElementById("edit_release_times_group")!.classList.contains("d-none")).toBe(false);
    expect((document.getElementById("edit_camp_locations_release") as HTMLInputElement).value)
        .toBe("Wed 2025-08-20 @ 09:00");
    expect((document.getElementById("edit_map_url_release") as HTMLInputElement).value)
        .toBe("Fri 2025-08-22 @ 18:30");
    // The event embargoes no art locations, so that field stays empty.
    expect((document.getElementById("edit_art_locations_release") as HTMLInputElement).value).toBe("");
});

test("setting a release time posts the parsed time and reformats the field", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    (eventCards()[0]!.querySelector(".show-edit-modal") as HTMLButtonElement).click();

    const campRelease = document.getElementById("edit_camp_locations_release") as HTMLInputElement;
    campRelease.value = "2025-08-24 15:00";
    change(campRelease);

    const call = await vi.waitFor(() => {
        const call = mock.mock.calls.findLast(([url, init]) => url === url_events && init?.body != null);
        expect(call).toBeDefined();
        return call;
    });
    expect(JSON.parse(call![1]!.body as string)).toEqual({
        id: 1,
        name: null,
        camp_locations_release: new Date(2025, 7, 24, 15, 0).toISOString(),
    });
    // The admin gets to see how the server understood what they typed.
    await vi.waitFor((): void => {
        expect(campRelease.value).toBe("Sun 2025-08-24 @ 15:00");
    });
});

test("clearing a release time posts the zero time, which lifts the embargo", async (): Promise<void> => {
    serverEvents = [{
        id: 1,
        name: "2025",
        art_locations_release: new Date(2025, 7, 20, 9, 0).toISOString(),
    }];
    const mock = await initAdminEventsPage();

    (eventCards()[0]!.querySelector(".show-edit-modal") as HTMLButtonElement).click();

    const artRelease = document.getElementById("edit_art_locations_release") as HTMLInputElement;
    artRelease.value = "";
    change(artRelease);

    const call = await vi.waitFor(() => {
        const call = mock.mock.calls.findLast(([url, init]) => url === url_events && init?.body != null);
        expect(call).toBeDefined();
        return call;
    });
    expect(JSON.parse(call![1]!.body as string)).toEqual({
        id: 1,
        name: null,
        art_locations_release: "0001-01-01T00:00:00Z",
    });
});

test("an unparseable release time posts nothing", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    (eventCards()[0]!.querySelector(".show-edit-modal") as HTMLButtonElement).click();

    const mapRelease = document.getElementById("edit_map_url_release") as HTMLInputElement;
    mapRelease.value = "sometime next week";
    await page.setMapURLRelease(mapRelease);

    expect(mock.mock.calls.find(([url, init]) => url === url_events && init?.body != null)).toBeUndefined();
});

test("the edit modal hides release times for event groups", async (): Promise<void> => {
    serverEvents = [{ id: 1, name: "2025", is_group: true }];
    await initAdminEventsPage();

    (eventCards()[0]!.querySelector(".show-edit-modal") as HTMLButtonElement).click();

    expect(document.getElementById("edit_release_times_group")!.classList.contains("d-none")).toBe(true);
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
    deleteButton.click();
    expect(await answerDialog("cancel")).toContain('Delete the event "2025"');
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(mock.mock.calls.find(deleteCall)).toBeUndefined();

    // Accepting it deletes the event.
    deleteButton.click();
    await answerDialog("ok");
    await vi.waitFor((): void => {
        expect(mock.mock.calls.find(deleteCall)).toBeDefined();
    });
});

// Find the most recent POST to the events endpoint and return its parsed body.
async function lastEventsPost(mock: ReturnType<typeof mockFetch>): Promise<Record<string, unknown>> {
    const call = await vi.waitFor(() => {
        const call = mock.mock.calls.findLast(([url, init]) => url === url_events && init?.body != null);
        expect(call).toBeDefined();
        return call;
    });
    return JSON.parse(call![1]!.body as string);
}

function aclPosts(mock: ReturnType<typeof mockFetch>): unknown[] {
    return mock.mock.calls.filter(([url, init]) => url === url_acl && init?.body != null);
}

// Let any listener-started async work run, for tests that check nothing was sent.
async function settle(): Promise<void> {
    await new Promise((resolve) => setTimeout(resolve, 0));
}

function openEditModal(card: HTMLElement): void {
    (card.querySelector(".show-edit-modal") as HTMLButtonElement).click();
}

function cardNamed(name: string): HTMLElement {
    const card = eventCards().find(c => c.dataset["eventName"] === name);
    expect(card).toBeDefined();
    return card!;
}

test("an unauthenticated visitor is sent to the login page", async (): Promise<void> => {
    serverAuth = { authenticated: false };
    const replace = vi.spyOn(window.location, "replace").mockImplementation((): void => {});
    mockServer();
    await import("../typescript/admin_events.ts");

    await vi.waitFor((): void => {
        expect(replace).toHaveBeenCalledWith(expect.stringContaining(url_login));
    });
    expect(eventCards()).toEqual([]);
});

test("a failure to load the events is reported, and no cards are drawn", async (): Promise<void> => {
    vi.spyOn(console, "error").mockImplementation((): void => {});
    routeOverride = (url, init) => url === url_events + "?include_groups=true" && init?.body == null
        ? problemResponse("database is down", 500)
        : undefined;
    mockServer();
    await import("../typescript/admin_events.ts");

    expect(await answerDialog("ok")).toContain("Failed to load events: database is down");
    await settle();
    expect(eventCards()).toEqual([]);
});

test("a failure to load the access control list is reported, and no cards are drawn", async (): Promise<void> => {
    vi.spyOn(console, "error").mockImplementation((): void => {});
    routeOverride = (url, init) => url === url_acl && init?.body == null
        ? problemResponse("no ACL for you", 500)
        : undefined;
    mockServer();
    await import("../typescript/admin_events.ts");

    expect(await answerDialog("ok")).toContain("Failed to load access control list: no ACL for you");
    await settle();
    expect(eventCards()).toEqual([]);
});

test("the access targets fill the typeahead, with an onduty: entry per position", async (): Promise<void> => {
    await initAdminEventsPage();

    const options = [...document.querySelectorAll<HTMLOptionElement>("#access_target_list option")].map(o => o.value);
    expect(options).toEqual(["*", "person:Tool", "position:007", "onduty:007", "team:Council"]);
});

test("without access targets, the page still works, and unknown expressions go unquestioned", async (): Promise<void> => {
    vi.spyOn(console, "error").mockImplementation((): void => {});
    routeOverride = (url) => url === url_accessTargets ? problemResponse("nope", 500) : undefined;
    const mock = await initAdminEventsPage();

    expect(document.querySelectorAll("#access_target_list option").length).toBe(0);

    const addInput = grantBlocks(eventCards()[0]!)[0]!.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "person:Nobody";
    change(addInput);

    const body = await vi.waitFor(() => lastACLPost(mock));
    expect(body["2025"]!["readers"]!.map(a => a.expression)).toEqual(["person:Tool", "person:Nobody"]);
    expect(document.querySelector("dialog.ims-dialog[open]")).toBeNull();
});

test("adding an expression that matches no known target asks first, and declining sends nothing", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const addInput = grantBlocks(eventCards()[0]!)[0]!.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "person:Nobody";
    change(addInput);

    expect(await answerDialog("cancel")).toContain("'person:Nobody' does not match any known person");
    await settle();
    expect(aclPosts(mock)).toEqual([]);
    expect(addInput.value).toBe("");
});

test("adding an expression that matches no known target can be confirmed", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const addInput = grantBlocks(eventCards()[0]!)[0]!.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "person:Nobody";
    change(addInput);
    await answerDialog("ok");

    const body = await vi.waitFor(() => lastACLPost(mock));
    expect(body["2025"]!["readers"]!.map(a => a.expression)).toEqual(["person:Tool", "person:Nobody"]);
});

test("adding a double wildcard warns that it does nothing", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const addInput = grantBlocks(eventCards()[0]!)[0]!.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "**";
    change(addInput);

    expect(await answerDialog("cancel")).toContain("Double-wildcard '**' ACLs are no longer supported");
    await settle();
    expect(aclPosts(mock)).toEqual([]);
});

test("adding an expression with no known prefix warns that it looks invalid", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const addInput = grantBlocks(eventCards()[0]!)[0]!.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "Hubcap";
    change(addInput);

    expect(await answerDialog("cancel")).toContain("'Hubcap' does not look like a valid ACL expression");
    await settle();
    expect(aclPosts(mock)).toEqual([]);
});

test("a blank who is ignored", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const addInput = grantBlocks(eventCards()[0]!)[0]!.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "   ";
    change(addInput);

    await settle();
    expect(aclPosts(mock)).toEqual([]);
    expect(document.querySelector("dialog.ims-dialog[open]")).toBeNull();
});

test("a failed who addition is reported and marks the add field", async (): Promise<void> => {
    vi.spyOn(console, "log").mockImplementation((): void => {});
    routeOverride = (url, init) => url === url_acl && init?.body != null
        ? problemResponse("ACL write failed", 500)
        : undefined;
    await initAdminEventsPage();

    const addInput = grantBlocks(eventCards()[0]!)[0]!.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "team:Council";
    change(addInput);

    expect(await answerDialog("ok")).toContain("Failed to edit ACL");
    await vi.waitFor((): void => {
        expect(addInput.classList.contains("is-invalid")).toBe(true);
    });
});

test("adding a who puts focus back on that grant's add field", async (): Promise<void> => {
    await initAdminEventsPage();

    const addInput = grantBlocks(eventCards()[0]!)[1]!.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "team:Council";
    change(addInput);

    // The page redraws after the post, so the focused field is the writer
    // grant's fresh add field.
    await vi.waitFor((): void => {
        const writer = grantBlocks(eventCards()[0]!)[1]!;
        expect(document.activeElement).toBe(writer.querySelector(".who_add"));
    });
});

test("each interval status gets its own badge, and an invalid interval counts as an issue", async (): Promise<void> => {
    serverACL["2025"] = {
        readers: [
            { expression: "person:Tool", validity: "always", pending: true, debug_info: { known_target: true } },
        ],
        writers: [
            { expression: "person:Tool", validity: "always", expired: true, debug_info: { known_target: true } },
        ],
        reporters: [
            { expression: "person:Tool", validity: "always", pending: true, expired: true, debug_info: { known_target: true } },
        ],
        visit_writers: [
            { expression: "person:Tool", validity: "always", debug_info: { known_target: true } },
        ],
    };
    await initAdminEventsPage();

    const card = eventCards()[0]!;
    const badges = grantBlocks(card).map(g => g.querySelector(".grant_interval_badge") as HTMLElement);
    expect(badges.map(b => b.textContent)).toEqual(["Pending", "Expired", "Invalid interval", ""]);
    expect(badges.map(b => b.classList.contains("d-none"))).toEqual([false, false, false, true]);
    expect(card.querySelector(".rule_count")!.textContent).toBe("4 grants · 4 expressions");
    expect(card.querySelector(".issue_count")!.textContent).toBe("1 issue");
});

test("grants at one level are ordered always before on-site, then by date", async (): Promise<void> => {
    serverACL["2025"] = {
        readers: [
            { expression: "person:A", validity: "onsite", debug_info: { known_target: true } },
            { expression: "person:B", validity: "always", not_before: "2025-08-25T00:00:00Z", debug_info: { known_target: true } },
            { expression: "person:C", validity: "always", not_before: "2025-08-20T00:00:00Z", not_after: "2025-09-01T00:00:00Z", debug_info: { known_target: true } },
            { expression: "person:D", validity: "always", not_before: "2025-08-20T00:00:00Z", not_after: "2025-08-30T00:00:00Z", debug_info: { known_target: true } },
        ],
    };
    await initAdminEventsPage();

    const order = grantBlocks(eventCards()[0]!).map(g => g.querySelector(".who_expression")!.textContent);
    expect(order).toEqual(["person:D", "person:C", "person:B", "person:A"]);
});

test("an empty event's card starts collapsed and counts no grants", async (): Promise<void> => {
    serverEvents = [{ id: 1, name: "2025" }, { id: 2, name: "2026" }];
    await initAdminEventsPage();

    const card = cardNamed("2026");
    expect(card.querySelector(".rule_count")!.textContent).toBe("0 grants · 0 expressions");
    expect((card.querySelector(".access_rules_collapse") as HTMLElement).hidden).toBe(true);
    expect(cardNamed("2025").querySelector(".rule_count")!.textContent).toBe("2 grants · 2 expressions");
});

test("a card header is marked stuck while it's pinned to the top of the window", async (): Promise<void> => {
    const callbacks: IntersectionObserverCallback[] = [];
    vi.stubGlobal("IntersectionObserver", class {
        constructor(cb: IntersectionObserverCallback) {
            callbacks.push(cb);
        }
        observe(): void {}
    });
    await initAdminEventsPage();

    const header = eventCards()[0]!.querySelector(".card-header") as HTMLElement;
    const fire = (ratio: number, top: number): void => {
        callbacks.at(-1)!([{ intersectionRatio: ratio, boundingClientRect: { top } } as IntersectionObserverEntry],
            {} as IntersectionObserver);
    };
    fire(0.9, 0);
    expect(header.classList.contains("is-stuck")).toBe(true);
    fire(1, 0);
    expect(header.classList.contains("is-stuck")).toBe(false);
    // Partly hidden off the bottom of the window isn't stuck.
    fire(0.5, 500);
    expect(header.classList.contains("is-stuck")).toBe(false);
});

//
// Event groups
//

function setUpGroups(): void {
    serverEvents = [
        { id: 4, name: "Loose" },
        { id: 2, name: "2024", parent_group: 1 },
        { id: 1, name: "Group", is_group: true },
        { id: 3, name: "2025", parent_group: 1 },
    ];
    serverACL = {
        "Group": {
            readers: [
                { expression: "*", validity: "always", debug_info: { known_target: true, matches_all_users: true } },
            ],
        },
        "2025": {
            writers: [
                {
                    expression: "person:Tool",
                    validity: "onsite",
                    pending: true,
                    debug_info: { known_target: true, matches_users: ["Tool"] },
                },
                {
                    expression: "position:Nobody",
                    validity: "always",
                    expired: true,
                    debug_info: { known_target: false, matches_no_one: true },
                },
                {
                    expression: "team:Weird",
                    validity: "always",
                    pending: true,
                    expired: true,
                    debug_info: { known_target: true, matches_users: ["A", "B"] },
                },
                // A rule the server sent without debug info is left out of the explanation.
                { expression: "team:Council", validity: "always" },
            ],
        },
    };
}

test("a group's card comes before its children's, which name the group they extend", async (): Promise<void> => {
    setUpGroups();
    await initAdminEventsPage();

    expect(eventCards().map(c => c.querySelector(".event_name")!.textContent)).toEqual([
        "Group (Group)",
        "2025 (extends Group)",
        "2024 (extends Group)",
        "Loose",
    ]);
});

test("explaining a group lists its child events", async (): Promise<void> => {
    setUpGroups();
    await initAdminEventsPage();

    (cardNamed("Group").querySelector(".explain_button") as HTMLButtonElement).click();

    expect(document.querySelector("#explainModal .modal-title")!.textContent).toBe("Current permissions for Group");
    const body = document.querySelector("#explainModal .modal-body")!.textContent!;
    expect(body).toContain("Read all:\n    * (always)\n        ALL authenticated users");
    expect(body).toContain("This is an event group");
    expect(body).toContain("    2025\n");
    expect(body).toContain("    2024\n");
    expect(body).not.toContain("Loose");
});

test("explaining a child event describes each rule, plus what it inherits from its group", async (): Promise<void> => {
    setUpGroups();
    await initAdminEventsPage();

    (cardNamed("2025").querySelector(".explain_button") as HTMLButtonElement).click();

    const body = document.querySelector("#explainModal .modal-body")!.textContent!;
    expect(body).toContain("person:Tool (onsite) (pending)\n        Tool");
    expect(body).toContain("position:Nobody (always) (unknown target) (expired)\n        NO users");
    expect(body).toContain("team:Weird (always) (invalid interval)\n        A\n        B");
    expect(body).not.toContain("team:Council");
    expect(body).toContain('Inherited from parent group "Group":\nRead all:');
});

test("explaining an event with no rules says so", async (): Promise<void> => {
    setUpGroups();
    serverACL["Group"] = {};
    await initAdminEventsPage();

    (cardNamed("Loose").querySelector(".explain_button") as HTMLButtonElement).click();
    expect(document.querySelector("#explainModal .modal-body")!.textContent).toBe("No permissions");

    (cardNamed("2024").querySelector(".explain_button") as HTMLButtonElement).click();
    expect(document.querySelector("#explainModal .modal-body")!.textContent)
        .toBe('No permissions\n\nInherited from parent group "Group":\nNo permissions');
});

test("the edit modal shows a child event's parent group, and disables it for a group", async (): Promise<void> => {
    setUpGroups();
    await initAdminEventsPage();

    const parentInput = document.getElementById("edit_parent_group") as HTMLInputElement;

    openEditModal(cardNamed("2025"));
    expect(document.querySelector("#editEventModal .modal-title")!.textContent).toBe("2025");
    expect(parentInput.disabled).toBe(false);
    expect(parentInput.value).toBe("Group");

    openEditModal(cardNamed("Group"));
    expect(parentInput.disabled).toBe(true);
    expect((document.getElementById("edit_map_url") as HTMLInputElement).disabled).toBe(true);
});

test("setting a parent group posts the group's id", async (): Promise<void> => {
    setUpGroups();
    const mock = await initAdminEventsPage();

    openEditModal(cardNamed("Loose"));
    const parentInput = document.getElementById("edit_parent_group") as HTMLInputElement;
    parentInput.value = "Group";
    change(parentInput);

    expect(await lastEventsPost(mock)).toEqual({ id: 4, name: null, parent_group: 1 });
    await vi.waitFor((): void => {
        expect(parentInput.classList.contains("is-valid")).toBe(true);
    });
});

test("clearing a parent group posts a zero group id", async (): Promise<void> => {
    setUpGroups();
    const mock = await initAdminEventsPage();

    openEditModal(cardNamed("2025"));
    const parentInput = document.getElementById("edit_parent_group") as HTMLInputElement;
    parentInput.value = "";
    change(parentInput);

    expect(await lastEventsPost(mock)).toEqual({ id: 3, name: null, parent_group: 0 });
});

test("a parent group name that matches no event is refused without posting", async (): Promise<void> => {
    vi.spyOn(console, "log").mockImplementation((): void => {});
    setUpGroups();
    const mock = await initAdminEventsPage();

    openEditModal(cardNamed("Loose"));
    const parentInput = document.getElementById("edit_parent_group") as HTMLInputElement;
    parentInput.value = "Not A Group";
    change(parentInput);

    expect(await answerDialog("ok")).toBe("No group by that name");
    await vi.waitFor((): void => {
        expect(parentInput.classList.contains("is-invalid")).toBe(true);
    });
    expect(mock.mock.calls.find(([url, init]) => url === url_events && init?.body != null)).toBeUndefined();
});

test("a failed parent group edit is reported", async (): Promise<void> => {
    vi.spyOn(console, "log").mockImplementation((): void => {});
    setUpGroups();
    routeOverride = (url, init) => url === url_events && init?.body != null
        ? problemResponse("can't nest groups", 400)
        : undefined;
    await initAdminEventsPage();

    openEditModal(cardNamed("Loose"));
    const parentInput = document.getElementById("edit_parent_group") as HTMLInputElement;
    parentInput.value = "Group";
    change(parentInput);

    expect(await answerDialog("ok")).toBe("Failed to edit event: can't nest groups (HTTP 400)");
    await vi.waitFor((): void => {
        expect(parentInput.classList.contains("is-invalid")).toBe(true);
    });
});

//
// Fixing an unknown who
//

function setUpUnknownWho(): void {
    serverACL["2025"] = {
        readers: [
            { expression: "person:Tool", validity: "always", debug_info: { known_target: true } },
            { expression: "person:Tol", validity: "always", description: "typo'd", debug_info: { known_target: false } },
        ],
    };
}

function unknownChip(): HTMLElement {
    const chip = eventCards()[0]!.querySelector<HTMLElement>(".who-chip.who-unknown");
    expect(chip).not.toBeNull();
    return chip!;
}

test("fixing an unknown who replaces its expression, keeping the grant's terms", async (): Promise<void> => {
    setUpUnknownWho();
    const mock = await initAdminEventsPage();

    const chip = unknownChip();
    (chip.querySelector(".who_fix") as HTMLButtonElement).click();
    const fixInput = chip.querySelector(".who_fix_input") as HTMLInputElement;
    expect(fixInput.classList.contains("d-none")).toBe(false);
    expect(chip.querySelector(".who_expression")!.classList.contains("d-none")).toBe(true);
    expect(fixInput.value).toBe("person:Tol");

    fixInput.value = "person:Tool";
    change(fixInput);

    const body = await vi.waitFor(() => lastACLPost(mock));
    expect(body["2025"]!["readers"]).toEqual([
        { expression: "person:Tool", validity: "always", not_before: null, not_after: null, description: "typo'd" },
    ]);
});

test("leaving an unknown who's expression unchanged sends nothing", async (): Promise<void> => {
    setUpUnknownWho();
    const mock = await initAdminEventsPage();

    const chip = unknownChip();
    (chip.querySelector(".who_fix") as HTMLButtonElement).click();
    change(chip.querySelector(".who_fix_input") as HTMLInputElement);

    await settle();
    expect(aclPosts(mock)).toEqual([]);
    // The redraw puts the chip back the way it was.
    expect(unknownChip().querySelector(".who_fix_input")!.classList.contains("d-none")).toBe(true);
});

test("declining the warning about a fixed expression sends nothing", async (): Promise<void> => {
    setUpUnknownWho();
    const mock = await initAdminEventsPage();

    const chip = unknownChip();
    (chip.querySelector(".who_fix") as HTMLButtonElement).click();
    const fixInput = chip.querySelector(".who_fix_input") as HTMLInputElement;
    fixInput.value = "person:StillWrong";
    change(fixInput);

    expect(await answerDialog("cancel")).toContain("'person:StillWrong' does not match any known person");
    await settle();
    expect(aclPosts(mock)).toEqual([]);
});

test("a failed fix is reported and marks the fix field", async (): Promise<void> => {
    vi.spyOn(console, "log").mockImplementation((): void => {});
    setUpUnknownWho();
    routeOverride = (url, init) => url === url_acl && init?.body != null
        ? problemResponse("ACL write failed", 500)
        : undefined;
    await initAdminEventsPage();

    const chip = unknownChip();
    (chip.querySelector(".who_fix") as HTMLButtonElement).click();
    const fixInput = chip.querySelector(".who_fix_input") as HTMLInputElement;
    fixInput.value = "person:Tool";
    change(fixInput);

    expect(await answerDialog("ok")).toContain("Failed to edit ACL");
    await vi.waitFor((): void => {
        expect(fixInput.classList.contains("is-invalid")).toBe(true);
    });
});

//
// Editing grant terms
//

test("an unparseable not-before date is flagged without posting", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const reader = grantBlocks(eventCards()[0]!)[0]!;
    (reader.querySelector(".grant_edit_terms") as HTMLButtonElement).click();
    const notBefore = reader.querySelector(".grant_not_before") as HTMLInputElement;
    notBefore.value = "the day after tomorrow";
    (reader.querySelector(".grant_apply_terms") as HTMLButtonElement).click();

    await settle();
    expect(notBefore.classList.contains("is-invalid")).toBe(true);
    expect(aclPosts(mock)).toEqual([]);
});

test("an out-of-range not-after date is flagged without posting", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const reader = grantBlocks(eventCards()[0]!)[0]!;
    (reader.querySelector(".grant_edit_terms") as HTMLButtonElement).click();
    const notAfter = reader.querySelector(".grant_not_after") as HTMLInputElement;
    notAfter.value = "2025-13-01";
    (reader.querySelector(".grant_apply_terms") as HTMLButtonElement).click();

    await settle();
    expect(notAfter.classList.contains("is-invalid")).toBe(true);
    expect(aclPosts(mock)).toEqual([]);
});

test("applying unchanged terms just leaves edit mode", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const reader = grantBlocks(eventCards()[0]!)[0]!;
    (reader.querySelector(".grant_edit_terms") as HTMLButtonElement).click();
    expect(reader.querySelector(".grant_terms_edit")!.classList.contains("d-none")).toBe(false);
    (reader.querySelector(".grant_apply_terms") as HTMLButtonElement).click();

    await settle();
    expect(aclPosts(mock)).toEqual([]);
    const redrawn = grantBlocks(eventCards()[0]!)[0]!;
    expect(redrawn.querySelector(".grant_terms_edit")!.classList.contains("d-none")).toBe(true);
});

test("cancelling a terms edit puts the grant back as it was", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const reader = grantBlocks(eventCards()[0]!)[0]!;
    (reader.querySelector(".grant_edit_terms") as HTMLButtonElement).click();
    (reader.querySelector(".grant_level") as HTMLSelectElement).value = "writers";
    (reader.querySelector(".grant_cancel_terms") as HTMLButtonElement).click();

    await settle();
    expect(aclPosts(mock)).toEqual([]);
    const redrawn = grantBlocks(eventCards()[0]!)[0]!;
    expect(redrawn.querySelector(".grant_level_badge")!.textContent).toBe("Read all");
    expect(redrawn.querySelector(".grant_terms_edit")!.classList.contains("d-none")).toBe(true);
});

test("changing a grant's validity rewrites only that grant's whos within its level", async (): Promise<void> => {
    serverACL["2025"] = {
        readers: [
            { expression: "person:Tool", validity: "onsite", debug_info: { known_target: true } },
            { expression: "person:Hubcap", validity: "onsite", debug_info: { known_target: true } },
            { expression: "team:Council", validity: "always", debug_info: { known_target: true } },
        ],
    };
    const mock = await initAdminEventsPage();

    // The always grant sorts first, so the on-site one is second.
    const onsite = grantBlocks(eventCards()[0]!)[1]!;
    expect(whoChips(onsite).length).toBe(2);
    (onsite.querySelector(".grant_edit_terms") as HTMLButtonElement).click();
    (onsite.querySelector(".grant_validity") as HTMLSelectElement).value = "always";
    (onsite.querySelector(".grant_apply_terms") as HTMLButtonElement).click();

    const body = await vi.waitFor(() => lastACLPost(mock));
    expect(Object.keys(body["2025"]!)).toEqual(["readers"]);
    expect(body["2025"]!["readers"]!.map(a => [a.expression, a.validity])).toEqual([
        ["team:Council", "always"],
        ["person:Tool", "always"],
        ["person:Hubcap", "always"],
    ]);
});

//
// Draft grants
//

function newDraft(): HTMLElement {
    (eventCards()[0]!.querySelector(".new_grant_button") as HTMLButtonElement).click();
    const drafts = eventCards()[0]!.querySelectorAll<HTMLElement>(".grant-draft");
    return drafts[drafts.length - 1]!;
}

test("a new draft grant starts in edit mode, with focus on its add field", async (): Promise<void> => {
    await initAdminEventsPage();

    const draft = newDraft();
    expect(draft.querySelector(".grant_terms_edit")!.classList.contains("d-none")).toBe(false);
    expect(draft.querySelector(".grant_edit_terms")!.classList.contains("d-none")).toBe(true);
    expect(draft.querySelector(".grant_remove_draft")!.classList.contains("d-none")).toBe(false);
    expect(document.activeElement).toBe(draft.querySelector(".who_add"));
});

test("removing one draft grant keeps the others", async (): Promise<void> => {
    await initAdminEventsPage();

    newDraft();
    newDraft();
    expect(eventCards()[0]!.querySelectorAll(".grant-draft").length).toBe(2);

    (eventCards()[0]!.querySelector(".grant-draft .grant_remove_draft") as HTMLButtonElement).click();
    expect(eventCards()[0]!.querySelectorAll(".grant-draft").length).toBe(1);
    (eventCards()[0]!.querySelector(".grant-draft .grant_remove_draft") as HTMLButtonElement).click();
    expect(eventCards()[0]!.querySelectorAll(".grant-draft").length).toBe(0);
});

test("a draft grant's terms carry into the rule its first who creates", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const draft = newDraft();
    const validity = draft.querySelector(".grant_validity") as HTMLSelectElement;
    validity.value = "onsite";
    change(validity);
    const notBefore = draft.querySelector(".grant_not_before") as HTMLInputElement;
    notBefore.value = "2025-08-20";
    change(notBefore);
    const notAfter = draft.querySelector(".grant_not_after") as HTMLInputElement;
    notAfter.value = "Mon 2025-09-01 @ 18:00";
    change(notAfter);
    const description = draft.querySelector(".grant_description") as HTMLInputElement;
    description.value = "Temple crew";
    change(description);

    const addInput = draft.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "team:Council";
    change(addInput);

    const body = await vi.waitFor(() => lastACLPost(mock));
    expect(body["2025"]!["readers"]!.at(-1)).toEqual({
        expression: "team:Council",
        validity: "onsite",
        not_before: new Date(2025, 7, 20).toISOString(),
        not_after: new Date(2025, 8, 1, 18, 0).toISOString(),
        description: "Temple crew",
    });
    // The draft has become a real grant.
    await vi.waitFor((): void => {
        expect(eventCards()[0]!.querySelectorAll(".grant-draft").length).toBe(0);
    });
});

test("an unparseable date on a draft grant is flagged and left out of its terms", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const draft = newDraft();
    const notBefore = draft.querySelector(".grant_not_before") as HTMLInputElement;
    notBefore.value = "soon";
    change(notBefore);
    expect(notBefore.classList.contains("is-invalid")).toBe(true);
    const notAfter = draft.querySelector(".grant_not_after") as HTMLInputElement;
    notAfter.value = "2025-02-30 25:00";
    change(notAfter);
    expect(notAfter.classList.contains("is-invalid")).toBe(true);

    const addInput = draft.querySelector(".who_add") as HTMLInputElement;
    addInput.value = "team:Council";
    change(addInput);

    const body = await vi.waitFor(() => lastACLPost(mock));
    const added = body["2025"]!["readers"]!.at(-1)!;
    expect(added.not_before).toBe(null);
    expect(added.not_after).toBe(null);
});

//
// Adding events
//

test("adding an event posts it, clears the field, and draws its card expanded", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const addEvent = document.getElementById("event_add") as HTMLInputElement;
    addEvent.value = " 2026 ";
    serverEvents.push({ id: 2, name: "2026" });
    change(addEvent);

    expect(await lastEventsPost(mock)).toEqual({ id: 0, name: "2026", is_group: false });
    await vi.waitFor((): void => {
        expect(addEvent.value).toBe("");
    });
    expect((cardNamed("2026").querySelector(".access_rules_collapse") as HTMLElement).hidden).toBe(false);
});

test("adding an event group posts it as a group", async (): Promise<void> => {
    const mock = await initAdminEventsPage();

    const addGroup = document.getElementById("event_add_group") as HTMLInputElement;
    addGroup.value = "Burns";
    change(addGroup);

    expect(await lastEventsPost(mock)).toEqual({ id: 0, name: "Burns", is_group: true });
});

test("a failed event addition is reported and marks the field", async (): Promise<void> => {
    vi.spyOn(console, "log").mockImplementation((): void => {});
    routeOverride = (url, init) => url === url_events && init?.body != null
        ? problemResponse("event already exists", 409)
        : undefined;
    await initAdminEventsPage();

    const addEvent = document.getElementById("event_add") as HTMLInputElement;
    addEvent.value = "2025";
    change(addEvent);

    expect(await answerDialog("ok")).toBe("Failed to add event: event already exists (HTTP 409)");
    await vi.waitFor((): void => {
        expect(addEvent.classList.contains("is-invalid")).toBe(true);
    });
    expect(addEvent.value).toBe("2025");
});

//
// Edit modal settings
//

test("the edit modal shows the event's map URL, and editing it posts the change", async (): Promise<void> => {
    serverEvents = [{ id: 1, name: "2025", map_url: "https://example.com/old.pdf" }];
    const mock = await initAdminEventsPage();

    openEditModal(eventCards()[0]!);
    const mapURL = document.getElementById("edit_map_url") as HTMLInputElement;
    expect(mapURL.disabled).toBe(false);
    expect(mapURL.value).toBe("https://example.com/old.pdf");

    mapURL.value = "https://example.com/new.pdf";
    change(mapURL);

    expect(await lastEventsPost(mock)).toEqual({ id: 1, name: null, map_url: "https://example.com/new.pdf" });
    await vi.waitFor((): void => {
        expect(mapURL.classList.contains("is-valid")).toBe(true);
    });
});

test("a failed map URL edit is reported and marks the field", async (): Promise<void> => {
    vi.spyOn(console, "log").mockImplementation((): void => {});
    routeOverride = (url, init) => url === url_events && init?.body != null
        ? problemResponse("bad URL", 400)
        : undefined;
    await initAdminEventsPage();

    openEditModal(eventCards()[0]!);
    const mapURL = document.getElementById("edit_map_url") as HTMLInputElement;
    mapURL.value = "nonsense";
    change(mapURL);

    expect(await answerDialog("ok")).toBe("Failed to edit event: bad URL (HTTP 400)");
    await vi.waitFor((): void => {
        expect(mapURL.classList.contains("is-invalid")).toBe(true);
    });
});

test("a failed active setting edit is reported and marks the checkbox", async (): Promise<void> => {
    vi.spyOn(console, "log").mockImplementation((): void => {});
    routeOverride = (url, init) => url === url_events && init?.body != null
        ? problemResponse("nope", 500)
        : undefined;
    await initAdminEventsPage();

    openEditModal(eventCards()[0]!);
    const isActive = document.getElementById("edit_is_active") as HTMLInputElement;
    isActive.checked = true;
    change(isActive);

    expect(await answerDialog("ok")).toBe("Failed to edit event: nope (HTTP 500)");
    await vi.waitFor((): void => {
        expect(isActive.classList.contains("is-invalid")).toBe(true);
    });
});

test("a failed address normalization edit is reported and marks the checkbox", async (): Promise<void> => {
    vi.spyOn(console, "log").mockImplementation((): void => {});
    routeOverride = (url, init) => url === url_events && init?.body != null
        ? problemResponse("nope", 500)
        : undefined;
    await initAdminEventsPage();

    openEditModal(eventCards()[0]!);
    const normalize = document.getElementById("edit_normalize_addresses") as HTMLInputElement;
    normalize.checked = true;
    change(normalize);

    expect(await answerDialog("ok")).toBe("Failed to edit event: nope (HTTP 500)");
    await vi.waitFor((): void => {
        expect(normalize.classList.contains("is-invalid")).toBe(true);
    });
});

test("a failed release time edit is reported and leaves the field as typed", async (): Promise<void> => {
    vi.spyOn(console, "log").mockImplementation((): void => {});
    routeOverride = (url, init) => url === url_events && init?.body != null
        ? problemResponse("nope", 500)
        : undefined;
    await initAdminEventsPage();

    openEditModal(eventCards()[0]!);
    const artRelease = document.getElementById("edit_art_locations_release") as HTMLInputElement;
    artRelease.value = "2025-08-24 15:00";
    change(artRelease);

    expect(await answerDialog("ok")).toBe("Failed to edit event: nope (HTTP 500)");
    await vi.waitFor((): void => {
        expect(artRelease.classList.contains("is-invalid")).toBe(true);
    });
    expect(artRelease.value).toBe("2025-08-24 15:00");
});

test("a failed event deletion is reported, and the modal stays open", async (): Promise<void> => {
    vi.spyOn(console, "log").mockImplementation((): void => {});
    serverAuth = { authenticated: true, user: "Tester", admin: true, event_deletion_allowed: true };
    routeOverride = (_url, init) => init?.method === "DELETE" ? problemResponse("event in use", 409) : undefined;
    await initAdminEventsPage();

    openEditModal(eventCards()[0]!);
    const modal = document.getElementById("editEventModal") as HTMLDialogElement;
    expect(modal.open).toBe(true);
    (document.getElementById("event_delete") as HTMLButtonElement).click();
    await answerDialog("ok");

    expect(await answerDialog("ok")).toBe("Failed to delete event: event in use (HTTP 409)");
    expect(modal.open).toBe(true);
});
