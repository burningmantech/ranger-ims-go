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

// Tests for settings.ts against the real templ-rendered settings page
// (settings.templ). The page reflects stored table preferences into its
// selects and persists changes back to localStorage.

import { beforeEach, expect, test, vi } from "vitest";
import { jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

beforeEach((): void => {
    // settings.ts looks up its elements and runs its init at import time, so
    // each test must load the fixture first, then dynamically import a fresh
    // copy of the module.
    vi.resetModules();
    loadFixture("settings.html");
});

// Import settings.ts behind a fake authenticated server and wait for its init
// to attach the page's window functions.
async function initSettingsPage() {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: true, user: "Tester" });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse([]);
        }
        return undefined;
    });
    await import("../typescript/settings.ts");
    await vi.waitFor((): void => {
        expect(window.setPreferredState).toBeTypeOf("function");
    });
    return mock;
}

function select(id: string): HTMLSelectElement {
    return document.getElementById(id) as HTMLSelectElement;
}

test("stored preferences are reflected into the selects on load", async (): Promise<void> => {
    localStorage.setItem("preferred_incidents_state", "open");
    localStorage.setItem("preferred_visits_status", "current");
    localStorage.setItem("preferred_table_rows_per_page", "50");

    await initSettingsPage();

    expect(select("preferred_state").value).toBe("open");
    expect(select("preferred_visits_status").value).toBe("current");
    expect(select("preferred_rows_per_page").value).toBe("50");
});

test("with no stored preferences the selects keep their default option", async (): Promise<void> => {
    await initSettingsPage();

    expect(select("preferred_state").value).toBe("none");
    expect(select("preferred_visits_status").value).toBe("none");
    expect(select("preferred_rows_per_page").value).toBe("none");
});

test("choosing a valid incidents state persists it to localStorage", async (): Promise<void> => {
    await initSettingsPage();

    const stateSelect = select("preferred_state");
    stateSelect.value = "active";
    await window.setPreferredState(stateSelect);

    expect(localStorage.getItem("preferred_incidents_state")).toBe("active");
});

test("choosing the default ('none') incidents state clears the stored preference", async (): Promise<void> => {
    localStorage.setItem("preferred_incidents_state", "open");
    await initSettingsPage();

    const stateSelect = select("preferred_state");
    stateSelect.value = "none";
    await window.setPreferredState(stateSelect);

    expect(localStorage.getItem("preferred_incidents_state")).toBeNull();
});

test("the visits status and rows-per-page selects persist their valid values", async (): Promise<void> => {
    await initSettingsPage();

    const visitsSelect = select("preferred_visits_status");
    visitsSelect.value = "all";
    await window.setPreferredVisitsStatus(visitsSelect);
    expect(localStorage.getItem("preferred_visits_status")).toBe("all");

    const rowsSelect = select("preferred_rows_per_page");
    rowsSelect.value = "100";
    await window.setPreferredRowsPerPage(rowsSelect);
    expect(localStorage.getItem("preferred_table_rows_per_page")).toBe("100");
});
