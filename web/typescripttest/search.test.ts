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

// Tests for search.ts against the real templ-rendered cross-event search page
// (search.templ). The page queries the /ims/api/search endpoint and renders
// the merged results table itself, with no DataTables involved.

import { beforeEach, expect, test, vi } from "vitest";
import { jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

interface ServerSearchResults {
    hits: object[];
    truncated: boolean;
}

let serverResults: ServerSearchResults;

beforeEach((): void => {
    vi.resetModules();
    loadFixture("search.html");
    window.history.replaceState(null, "", "/ims/app/search");

    serverResults = {
        hits: [
            {
                kind: "incident",
                event: "2025",
                event_id: 2,
                number: 15,
                created: "2025-08-25T10:00:00Z",
                state: "closed",
                summary: "Dusty bike crash",
            },
            {
                kind: "field_report",
                event: "2024",
                event_id: 1,
                number: 3,
                created: "2024-08-27T18:30:00Z",
                summary: "Found wallet",
                snippet: "…the wallet was extremely dusty…",
                incident: 7,
            },
            {
                kind: "visit",
                event: "2024",
                event_id: 1,
                number: 9,
                created: "2024-08-26T02:15:00Z",
                summary: "Guesty McGuest",
            },
        ],
        truncated: false,
    };
});

// Import search.ts behind a fake authenticated server and wait for its init
// to settle, which ends with the page focusing the search input.
async function initSearchPage() {
    const searchCalls: string[] = [];
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: true, user: "Tester" });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse([]);
        }
        if (url.startsWith(`${url_search}?`) && init?.body == null) {
            searchCalls.push(url);
            return jsonResponse(serverResults);
        }
        return undefined;
    });
    await import("../typescript/search.ts");
    await vi.waitFor((): void => {
        expect(document.activeElement?.id).toBe("search_input");
    });
    return { mock, searchCalls };
}

function searchInput(): HTMLInputElement {
    return document.getElementById("search_input") as HTMLInputElement;
}

function resultRows(): HTMLTableRowElement[] {
    return Array.from(document.querySelectorAll("#search_results_table tbody tr"));
}

test("typing a query fetches results and renders one row per hit", async (): Promise<void> => {
    const { searchCalls } = await initSearchPage();

    searchInput().value = "dusty";
    searchInput().dispatchEvent(new Event("input"));

    await vi.waitFor((): void => {
        expect(resultRows().length).toBe(3);
    });
    expect(searchCalls[0]).toContain("q=dusty");

    // The incident row shows its event, kind, and summary, and its number
    // links to the incident page.
    const incidentRow = resultRows()[0]!;
    const cells = Array.from(incidentRow.cells).map((c) => c.textContent);
    expect(cells[0]).toBe("2025");
    expect(cells[1]).toBe("Incident");
    expect(cells[2]).toBe("15");
    expect(cells[4]).toBe("Dusty bike crash");
    expect(incidentRow.querySelector("a")!.href).toContain("/ims/app/events/2025/incidents/15");

    // The field report row links to the field report page and shows the
    // matched report entry excerpt.
    const frRow = resultRows()[1]!;
    expect(frRow.querySelector("a")!.href).toContain("/ims/app/events/2024/field_reports/3");
    expect(frRow.cells[5]!.textContent).toContain("extremely dusty");

    // The visit row links to the visit page.
    const visitRow = resultRows()[2]!;
    expect(visitRow.querySelector("a")!.href).toContain("/ims/app/events/2024/visits/9");

    expect(document.getElementById("search_results_info")!.textContent).toBe("3 results");
});

test("search parameters are restored from the URL fragment on load", async (): Promise<void> => {
    window.history.replaceState(null, "", "/ims/app/search#q=dusty&kinds=incident");

    const { searchCalls } = await initSearchPage();

    expect(searchInput().value).toBe("dusty");
    expect((document.getElementById("kind_incident") as HTMLInputElement).checked).toBe(true);
    expect((document.getElementById("kind_field_report") as HTMLInputElement).checked).toBe(false);
    expect((document.getElementById("kind_visit") as HTMLInputElement).checked).toBe(false);

    await vi.waitFor((): void => {
        expect(searchCalls.length).toBe(1);
    });
    expect(searchCalls[0]).toContain("q=dusty");
    expect(searchCalls[0]).toContain("kinds=incident");
});

test("a too-short query does not hit the server and prompts for more input", async (): Promise<void> => {
    const { searchCalls } = await initSearchPage();

    searchInput().value = "d";
    searchInput().dispatchEvent(new Event("input"));

    await vi.waitFor((): void => {
        expect(document.getElementById("search_results_info")!.textContent).toContain("at least 2 characters");
    });
    expect(searchCalls.length).toBe(0);
    expect(resultRows().length).toBe(0);
});

test("unchecking every record type prompts to select one instead of searching", async (): Promise<void> => {
    const { searchCalls } = await initSearchPage();

    searchInput().value = "dusty";
    for (const id of ["kind_incident", "kind_field_report", "kind_visit"]) {
        const checkbox = document.getElementById(id) as HTMLInputElement;
        checkbox.checked = false;
    }
    (document.getElementById("kind_visit") as HTMLInputElement).dispatchEvent(new Event("change"));

    await vi.waitFor((): void => {
        expect(document.getElementById("search_results_info")!.textContent).toContain("at least one record type");
    });
    expect(searchCalls.length).toBe(0);
});

test("truncated results include a warning in the results info line", async (): Promise<void> => {
    serverResults.truncated = true;

    await initSearchPage();

    searchInput().value = "dusty";
    searchInput().dispatchEvent(new Event("input"));

    await vi.waitFor((): void => {
        expect(document.getElementById("search_results_info")!.textContent).toContain("too many matches");
    });
});
