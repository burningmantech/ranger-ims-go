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
import { jsonResponse, loadFixture, problemResponse } from "./helpers.ts";

interface ServerSearchResults {
    hits: object[];
    truncated: boolean;
}

let serverResults: ServerSearchResults;
// When set, the search route responds with this problem instead of results.
let serverProblem: { detail: string; status: number } | null;
// Answers the search route. Tests that care about a search being in flight
// replace this with something they can resolve by hand.
let searchResponder: (url: string, init?: RequestInit) => Promise<Response>;

beforeEach((): void => {
    vi.resetModules();
    loadFixture("search.html");
    window.history.replaceState(null, "", "/ims/app/search");
    serverProblem = null;
    searchResponder = async (): Promise<Response> => {
        if (serverProblem != null) {
            return problemResponse(serverProblem.detail, serverProblem.status);
        }
        return jsonResponse(serverResults);
    };

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
    // Not the shared mockFetch helper, since the search route has to be able
    // to answer asynchronously.
    const mock = vi.fn(async (url: string, init?: RequestInit): Promise<Response> => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: true, user: "Tester" });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse([]);
        }
        if (url.startsWith(`${url_search}?`) && init?.body == null) {
            searchCalls.push(url);
            return await searchResponder(url, init);
        }
        throw new Error(`no mocked fetch route for ${url}`);
    });
    vi.stubGlobal("fetch", mock);
    await import("../typescript/search.ts");
    await vi.waitFor((): void => {
        expect(document.activeElement?.id).toBe("search_input");
    });
    return { mock, searchCalls };
}

function searchInput(): HTMLInputElement {
    return document.getElementById("search_input") as HTMLInputElement;
}

function clickSearch(): void {
    (document.getElementById("search_button") as HTMLButtonElement).click();
}

function resultsInfo(): string {
    return document.getElementById("search_results_info")!.textContent ?? "";
}

function spinnerShown(): boolean {
    return !document.getElementById("search_spinner")!.classList.contains("d-none");
}

function resultRows(): HTMLTableRowElement[] {
    return Array.from(document.querySelectorAll("#search_results_table tbody tr"));
}

test("pressing Search fetches results and renders one row per hit", async (): Promise<void> => {
    const { searchCalls } = await initSearchPage();

    searchInput().value = "dusty";
    clickSearch();

    await vi.waitFor((): void => {
        expect(resultRows().length).toBe(3);
    });
    expect(searchCalls[0]).toContain("q=dusty");
    // A plain query is a literal search, not a regexp one.
    expect(searchCalls[0]).not.toContain("regex=");

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

    expect(resultsInfo()).toBe("3 results");
});

test("typing a query doesn't search until Search is pressed", async (): Promise<void> => {
    const { searchCalls } = await initSearchPage();

    searchInput().value = "dusty";
    searchInput().dispatchEvent(new Event("input"));

    // Give a debounced search the chance to fire, if there were one.
    await new Promise((resolve): void => void setTimeout(resolve, 50));
    expect(searchCalls.length).toBe(0);
    expect(resultRows().length).toBe(0);
    // The query still lands in the URL fragment, so the page stays shareable.
    expect(window.location.hash).toContain("q=dusty");

    clickSearch();
    await vi.waitFor((): void => {
        expect(searchCalls.length).toBe(1);
    });
});

test("pressing Enter in the search box runs the search", async (): Promise<void> => {
    const { searchCalls } = await initSearchPage();

    searchInput().value = "dusty";
    searchInput().dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));

    await vi.waitFor((): void => {
        expect(searchCalls.length).toBe(1);
    });
    expect(searchCalls[0]).toContain("q=dusty");
});

test("editing the form after a search notes that the results are out of date", async (): Promise<void> => {
    await initSearchPage();

    searchInput().value = "dusty";
    clickSearch();
    await vi.waitFor((): void => {
        expect(resultsInfo()).toBe("3 results");
    });

    (document.getElementById("kind_visit") as HTMLInputElement).checked = false;
    (document.getElementById("kind_visit") as HTMLInputElement).dispatchEvent(new Event("change"));
    expect(resultsInfo()).toContain("press Search");
    // The stale results themselves stay put.
    expect(resultRows().length).toBe(3);
});

test("starting a search abandons the one still running", async (): Promise<void> => {
    const pending: { resolve: (r: Response) => void; signal: AbortSignal | undefined }[] = [];
    searchResponder = (_url, init): Promise<Response> =>
        new Promise<Response>((resolve, reject): void => {
            const signal = init?.signal ?? undefined;
            pending.push({ resolve: resolve, signal: signal });
            signal?.addEventListener("abort", (): void => reject(new Error("aborted")));
        });

    await initSearchPage();

    searchInput().value = "dusty";
    clickSearch();
    await vi.waitFor((): void => {
        expect(pending.length).toBe(1);
    });
    expect(resultsInfo()).toBe("Searching…");
    expect(spinnerShown()).toBe(true);

    // A second search is allowed while the first is in flight, and it cuts the
    // first one off so the server stops working on it.
    searchInput().value = "dustier";
    clickSearch();
    await vi.waitFor((): void => {
        expect(pending.length).toBe(2);
    });
    expect(pending[0]!.signal!.aborted).toBe(true);
    expect(pending[1]!.signal!.aborted).toBe(false);

    pending[1]!.resolve(jsonResponse(serverResults));
    await vi.waitFor((): void => {
        expect(resultRows().length).toBe(3);
    });
    expect(resultsInfo()).toBe("3 results");
    expect(spinnerShown()).toBe(false);
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
    clickSearch();

    await vi.waitFor((): void => {
        expect(resultsInfo()).toContain("at least 2 characters");
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
    clickSearch();

    await vi.waitFor((): void => {
        expect(resultsInfo()).toContain("at least one record type");
    });
    expect(searchCalls.length).toBe(0);
});

test("a slash-enclosed query is sent as a regular expression", async (): Promise<void> => {
    const { searchCalls } = await initSearchPage();

    searchInput().value = "/ab?c/";
    clickSearch();

    await vi.waitFor((): void => {
        expect(searchCalls.length).toBe(1);
    });
    // The slashes are stripped and the pattern travels in "q", with the
    // regexp mode flagged separately.
    expect(searchCalls[0]).toContain("q=ab%3Fc");
    expect(searchCalls[0]).toContain("regex=true");
});

test("an invalid regular expression shows the server's message in the results info", async (): Promise<void> => {
    serverProblem = { detail: "Invalid regular expression", status: 400 };

    await initSearchPage();

    searchInput().value = "/ab(c/";
    clickSearch();

    await vi.waitFor((): void => {
        expect(resultsInfo()).toContain("Invalid regular expression");
    });
    expect(resultRows().length).toBe(0);
});

test("a search that times out reports so where the results would go", async (): Promise<void> => {
    serverProblem = { detail: "Search took too long — try a more specific query", status: 503 };

    await initSearchPage();

    searchInput().value = "dusty";
    clickSearch();

    await vi.waitFor((): void => {
        expect(resultsInfo()).toContain("took too long");
    });
    expect(spinnerShown()).toBe(false);
    // ...and not in the page-wide error banner.
    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(true);
});

test("truncated results include a warning in the results info line", async (): Promise<void> => {
    serverResults.truncated = true;

    await initSearchPage();

    searchInput().value = "dusty";
    clickSearch();

    await vi.waitFor((): void => {
        expect(resultsInfo()).toContain("too many matches");
    });
});
