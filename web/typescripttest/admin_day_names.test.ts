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

// Tests for admin_day_names.ts against the real templ-rendered page
// (admindaynames.templ). The page edits an event's day names as CSV lines in
// a textarea, then saves the whole list at once.

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { change, jsonResponse, loadFixture, mockFetch, waitForPageInit } from "./helpers.ts";

const dayNamesUrl = url_dayNames.replace("<event_id>", "2025");
const adminDayNamesPath = "/ims/app/admin/daynames";

let serverEvents: ims.EventData[];
let serverDayNames: ims.EventDayName[];

beforeEach((): void => {
    vi.resetModules();
    loadFixture("admin_day_names.html");
    window.history.replaceState(null, "", `${adminDayNamesPath}?event_id=2025`);
    serverEvents = [
        { id: 1, name: "2025" },
        { id: 2, name: "2024" },
        { id: 3, name: "Group", is_group: true },
    ];
    serverDayNames = [
        { date: "2025-08-24", name: "Gate Opens" },
        { date: "2025-08-30", name: "Man Burn" },
    ];
});

async function initPage() {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: true, user: "Tester", admin: true });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse(serverEvents);
        }
        if (url === dayNamesUrl && init?.body == null) {
            return jsonResponse(serverDayNames);
        }
        if (url === dayNamesUrl) {
            serverDayNames = JSON.parse(init!.body as string);
            return new Response(null, { status: 204 });
        }
        return undefined;
    });
    await import("../typescript/admin_day_names.ts");
    await waitForPageInit();
    return mock;
}

function textarea(): HTMLTextAreaElement {
    return document.getElementById("day-names") as HTMLTextAreaElement;
}

function postBodies(mock: ReturnType<typeof mockFetch>): unknown[] {
    return mock.mock.calls
        .filter(([url, init]) => url === dayNamesUrl && init?.body != null)
        .map(([, init]) => JSON.parse(init!.body as string));
}

function click(id: string): void {
    (document.getElementById(id) as HTMLButtonElement).click();
}

function errorText(): string {
    return document.getElementById("error_text")!.textContent ?? "";
}

async function initPageLoaded() {
    const mock = await initPage();
    await vi.waitFor((): void => {
        expect(textarea().disabled).toBe(false);
    });
    return mock;
}

test("the event in the URL has its day names drawn as CSV", async (): Promise<void> => {
    await initPageLoaded();

    expect(textarea().value).toBe("2025-08-24,Gate Opens\n2025-08-30,Man Burn\n");
    // Groups have no days of their own.
    const options = [...(document.getElementById("event-name") as HTMLSelectElement).options].map(o => o.value);
    expect(options).toEqual(["", "2025", "2024"]);
    expect((document.getElementById("day-names-save") as HTMLButtonElement).disabled).toBe(false);
});

test("with no event selected, there's nothing to edit or save", async (): Promise<void> => {
    window.history.replaceState(null, "", adminDayNamesPath);
    await initPage();

    expect(textarea().value).toBe("");
    expect(textarea().disabled).toBe(true);
    expect((document.getElementById("day-names-save") as HTMLButtonElement).disabled).toBe(true);
});

test("saving posts every line, trimmed, skipping blank lines", async (): Promise<void> => {
    const mock = await initPageLoaded();

    textarea().value = "2025-08-30,Man Burn\r\n\n  2025-08-31 ,  Temple Burn \n\n";
    click("day-names-save");

    await vi.waitFor((): void => {
        expect(postBodies(mock)).toHaveLength(1);
    });
    expect(postBodies(mock)[0]).toEqual([
        { date: "2025-08-30", name: "Man Burn" },
        { date: "2025-08-31", name: "Temple Burn" },
    ]);
    // The page redraws from what the server stored.
    await vi.waitFor((): void => {
        expect(textarea().value).toBe("2025-08-30,Man Burn\n2025-08-31,Temple Burn\n");
    });
});

test("a name may contain commas, quoted or not", async (): Promise<void> => {
    const mock = await initPageLoaded();

    textarea().value = '2025-08-30,Burn, Man\n2025-08-31,"Temple, ""the"" Burn"\n';
    click("day-names-save");

    await vi.waitFor((): void => {
        expect(postBodies(mock)).toHaveLength(1);
    });
    expect(postBodies(mock)[0]).toEqual([
        { date: "2025-08-30", name: "Burn, Man" },
        { date: "2025-08-31", name: 'Temple, "the" Burn' },
    ]);
});

test("a name starting with a quote is drawn quoted, so it round-trips", async (): Promise<void> => {
    serverDayNames = [{ date: "2025-08-30", name: '"Big" Burn' }];
    const mock = await initPageLoaded();

    expect(textarea().value).toBe('2025-08-30,"""Big"" Burn"\n');
    click("day-names-save");

    await vi.waitFor((): void => {
        expect(postBodies(mock)).toHaveLength(1);
    });
    expect(postBodies(mock)[0]).toEqual([{ date: "2025-08-30", name: '"Big" Burn' }]);
});

test("an empty textarea saves no day names", async (): Promise<void> => {
    const mock = await initPageLoaded();

    textarea().value = "\n  \n";
    click("day-names-save");

    await vi.waitFor((): void => {
        expect(postBodies(mock)).toHaveLength(1);
    });
    expect(postBodies(mock)[0]).toEqual([]);
});

test("a line with no comma isn't saved", async (): Promise<void> => {
    const mock = await initPageLoaded();

    textarea().value = "2025-08-30,Man Burn\n2025-08-31 Temple Burn\n";
    click("day-names-save");

    expect(errorText()).toContain("Line 2");
    expect(errorText()).toContain('expected "date,name"');
    expect(postBodies(mock)).toEqual([]);
});

test("a date not in YYYY-MM-DD form isn't saved", async (): Promise<void> => {
    const mock = await initPageLoaded();

    textarea().value = "8/30/2025,Man Burn\n";
    click("day-names-save");

    expect(errorText()).toContain("Line 1");
    expect(errorText()).toContain("YYYY-MM-DD");
    expect(postBodies(mock)).toEqual([]);
});

test("a date that doesn't exist isn't saved", async (): Promise<void> => {
    const mock = await initPageLoaded();

    textarea().value = "2025-02-30,Not A Day\n";
    click("day-names-save");

    expect(errorText()).toContain('"2025-02-30" isn\'t a date');
    expect(postBodies(mock)).toEqual([]);
});

test("a blank name isn't saved", async (): Promise<void> => {
    const mock = await initPageLoaded();

    textarea().value = "2025-08-24,Gate Opens\n2025-08-30,   \n";
    click("day-names-save");

    expect(errorText()).toContain("Line 2: the name for 2025-08-30 is empty");
    expect(postBodies(mock)).toEqual([]);
});

test("the same date twice isn't saved", async (): Promise<void> => {
    const mock = await initPageLoaded();

    textarea().value = "2025-08-30,Man Burn\n2025-08-30,Burn Night\n";
    click("day-names-save");

    expect(errorText()).toContain("Line 2: 2025-08-30 is listed more than once");
    expect(postBodies(mock)).toEqual([]);
});

test("picking a different event loads its day names", async (): Promise<void> => {
    const mock = await initPageLoaded();

    const select = document.getElementById("event-name") as HTMLSelectElement;
    select.value = "2024";
    change(select);

    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url]) => url === url_dayNames.replace("<event_id>", "2024"))).toBe(true);
    });
    expect(new URLSearchParams(window.location.search).get("event_id")).toBe("2024");
});

test("a slow response for a previously picked event doesn't fill in the textarea", async (): Promise<void> => {
    window.history.replaceState(null, "", adminDayNamesPath);
    let release2025!: () => void;
    const slow2025 = new Promise<void>((resolve): void => {
        release2025 = resolve;
    });
    vi.stubGlobal("fetch", vi.fn(async (url: string): Promise<Response> => {
        if (url === url_auth) {
            return jsonResponse({ authenticated: true, user: "Tester", admin: true });
        }
        if (url === url_events) {
            return jsonResponse(serverEvents);
        }
        if (url === dayNamesUrl) {
            await slow2025;
            return jsonResponse(serverDayNames);
        }
        if (url === url_dayNames.replace("<event_id>", "2024")) {
            return jsonResponse([{ date: "2024-08-31", name: "Man Burn" }]);
        }
        throw new Error(`no mocked fetch route for ${url}`);
    }));
    await import("../typescript/admin_day_names.ts");
    await waitForPageInit();

    const select = document.getElementById("event-name") as HTMLSelectElement;
    select.value = "2025";
    change(select);
    select.value = "2024";
    change(select);
    await vi.waitFor((): void => {
        expect(textarea().value).toBe("2024-08-31,Man Burn\n");
    });

    release2025();
    await new Promise((resolve): void => {
        setTimeout(resolve, 0);
    });
    expect(textarea().value).toBe("2024-08-31,Man Burn\n");
});
