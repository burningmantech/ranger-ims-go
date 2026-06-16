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

// Tests for admin_streets.ts against the real templ-rendered streets admin
// page (adminstreets.templ). The page draws one card of concentric streets
// per event and supports adding a street via the API.

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

let serverEvents: ims.EventData[];
let serverStreets: Record<number, Record<string, string>>;

beforeEach((): void => {
    vi.resetModules();
    loadFixture("admin_streets.html");
    serverEvents = [{ id: 1, name: "2025" }];
    serverStreets = {
        1: { "100": "Esplanade", "200": "Anchovy" },
    };
});

async function initAdminStreetsPage() {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: true, user: "Tester", admin: true });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse(serverEvents);
        }
        if (url === url_streets && init?.body == null) {
            return jsonResponse(serverStreets);
        }
        if (url === url_streets && init?.body != null) {
            return new Response(null, { status: 204 });
        }
        return undefined;
    });
    await import("../typescript/admin_streets.ts");
    await vi.waitFor((): void => {
        expect(document.getElementById("event_streets_1")).not.toBeNull();
    });
    return mock;
}

test("each event gets a card listing its streets by id and name", async (): Promise<void> => {
    await initAdminStreetsPage();

    const card = document.getElementById("event_streets_1")!;
    expect(card.querySelector(".event_name")!.textContent).toBe("2025");

    const items = card.querySelectorAll(".list-group .list-group-item");
    expect(items.length).toBe(2);
    expect(items[0]!.textContent).toContain("100: Esplanade");
    expect(items[1]!.textContent).toContain("200: Anchovy");
});

test("adding a street posts the parsed id and name, then redraws the list", async (): Promise<void> => {
    const mock = await initAdminStreetsPage();

    const card = document.getElementById("event_streets_1")!;
    const input = card.querySelector("#street_add") as HTMLInputElement;
    input.value = "300: Bacon";

    // The server now also knows the new street for the post-save reload.
    serverStreets[1]!["300"] = "Bacon";
    await window.addStreet(input);

    const postCall = mock.mock.calls.find(([url, init]) => url === url_streets && init?.body != null)!;
    expect(JSON.parse(postCall[1]!.body as string)).toEqual({ 1: { "300": "Bacon" } });

    // The input is cleared and the new street shows up in the redrawn list.
    expect(input.value).toBe("");
    const items = [...card.querySelectorAll(".list-group .list-group-item")].map(i => i.textContent);
    expect(items.some(t => t!.includes("300: Bacon"))).toBe(true);
});

test("adding a street without a ':' separator is rejected before any request", async (): Promise<void> => {
    const mock = await initAdminStreetsPage();
    const alert = vi.fn();
    vi.stubGlobal("alert", alert);

    const input = document.querySelector("#event_streets_1 #street_add") as HTMLInputElement;
    input.value = "no separator here";
    await window.addStreet(input);

    expect(alert).toHaveBeenCalled();
    expect(mock.mock.calls.some(([url, init]) => url === url_streets && init?.body != null)).toBe(false);
});
