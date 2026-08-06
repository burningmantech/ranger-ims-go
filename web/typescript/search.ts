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

"use strict";

import * as ims from "./ims.ts";

interface SearchResult {
    kind: string;
    event: string;
    event_id: number;
    number: number;
    created: string;
    state?: string;
    summary?: string;
    snippet?: string;
    incident?: number;
}

interface SearchResults {
    hits: SearchResult[];
    truncated: boolean;
}

const kindIncident = "incident";
const kindFieldReport = "field_report";
const kindVisit = "visit";

const kindLabels: Record<string, string> = {
    [kindIncident]: "Incident",
    [kindFieldReport]: "Field Report",
    [kindVisit]: "Visit",
};

const minQueryLength = 2;

// e.g. "2025-08-30 @ 18:00", matching the flatpickr fields' display format
// ("Y-m-d @ H:i"). Results span many years' Events, so unlike the per-event
// tables, the year matters here.
function formatCreated(d: Date): string {
    return `${ims.localDateISO(d)} @ ${ims.localTimeHHMM(d)}`;
}

// Distinguishes the newest search request from any stale in-flight ones.
let _searchSequence = 0;

// The in-flight search, if any. Searches can be expensive server-side, so
// starting a new one aborts the old request, which drops the connection and
// cancels the queries the server was still running for it.
let _searchAbort: AbortController|null = null;

// The info line for the results on screen, and the query parameters that
// produced them, so that later edits to the form can be flagged as unapplied.
let _resultsInfo = "";
let _resultsParams: string|null = null;

const el = {
    searchInput: ims.typedElement("search_input", HTMLInputElement),
    searchButton: ims.typedElement("search_button", HTMLButtonElement),
    searchSpinner: ims.typedElement("search_spinner", HTMLSpanElement),
    kindIncident: ims.typedElement("kind_incident", HTMLInputElement),
    kindFieldReport: ims.typedElement("kind_field_report", HTMLInputElement),
    kindVisit: ims.typedElement("kind_visit", HTMLInputElement),
    resultsInfo: ims.typedElement("search_results_info", HTMLParagraphElement),
    resultsTable: ims.typedElement("search_results_table", HTMLTableElement),
    resultRowTemplate: ims.typedElement("search_result_row_template", HTMLTemplateElement),
};

initSearchPage();

async function initSearchPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }

    // Restore search parameters from the URL fragment, so that search links
    // can be shared and reloaded.
    const fragmentParams = ims.windowFragmentParams();
    el.searchInput.value = fragmentParams.get("q")??"";
    const kinds = fragmentParams.get("kinds");
    if (kinds) {
        const kindSet = new Set(kinds.split(","));
        el.kindIncident.checked = kindSet.has(kindIncident);
        el.kindFieldReport.checked = kindSet.has(kindFieldReport);
        el.kindVisit.checked = kindSet.has(kindVisit);
    }
    // Searches only run when asked for, since each one is a real load on the
    // server. Edits to the form just keep the shareable URL current and note
    // that the results on screen no longer match the form.
    el.searchInput.addEventListener("input", formChanged);
    el.kindIncident.addEventListener("change", formChanged);
    el.kindFieldReport.addEventListener("change", formChanged);
    el.kindVisit.addEventListener("change", formChanged);

    el.searchButton.addEventListener("click", doSearch);
    el.searchInput.addEventListener("keydown", function(e: KeyboardEvent): void {
        if (e.key === "Enter") {
            e.preventDefault();
            doSearch();
        }
    });

    document.addEventListener("keydown", function(e: KeyboardEvent): void {
        if (ims.blockKeyboardShortcutFieldActive()) {
            return;
        }
        if (e.altKey || e.ctrlKey || e.metaKey) {
            return;
        }
        // / --> jump to search box
        if (e.key === "/") {
            // don't immediately input a "/" into the search box
            e.preventDefault();
            el.searchInput.focus();
        }
    });

    el.searchInput.focus();

    if (el.searchInput.value) {
        await doSearch();
    }
}

function formChanged(): void {
    replaceWindowState();
    refreshInfo();
}

// refreshInfo writes the results info line, which says what the results on
// screen are, whether a search is running, and whether the form has moved on
// from the results being shown.
function refreshInfo(): void {
    if (_searchAbort != null) {
        el.resultsInfo.textContent = "Searching…";
        return;
    }
    let info = _resultsInfo;
    const current = currentQuery();
    if (_resultsParams != null && "params" in current && current.params !== _resultsParams) {
        info += " — press Search to apply your changes";
    }
    el.resultsInfo.textContent = info;
}

function setSearching(searching: boolean): void {
    el.searchSpinner.classList.toggle("d-none", !searching);
    el.searchButton.setAttribute("aria-busy", searching ? "true" : "false");
}

function selectedKinds(): string[] {
    const kinds: string[] = [];
    if (el.kindIncident.checked) {
        kinds.push(kindIncident);
    }
    if (el.kindFieldReport.checked) {
        kinds.push(kindFieldReport);
    }
    if (el.kindVisit.checked) {
        kinds.push(kindVisit);
    }
    return kinds;
}

function replaceWindowState(): void {
    const newParams: [string, string][] = [];
    if (el.searchInput.value) {
        newParams.push(["q", el.searchInput.value]);
    }
    const kinds = selectedKinds();
    if (kinds.length < 3) {
        newParams.push(["kinds", kinds.join(",")]);
    }
    const fragment = new URLSearchParams(newParams).toString();
    history.replaceState(null, "", fragment ? "#" + fragment : window.location.pathname);
}

// currentQuery returns the query parameters the form currently describes, or
// a message explaining why there's nothing to search for yet.
function currentQuery(): {params: string}|{problem: string} {
    const rawQuery = el.searchInput.value.trim();
    // A query enclosed in slashes, like /ab?c/, is a regular expression.
    const isRegex = rawQuery.length > 2 && rawQuery.startsWith("/") && rawQuery.endsWith("/");
    const query = isRegex ? rawQuery.slice(1, -1) : rawQuery;
    const kinds = selectedKinds();

    if (kinds.length === 0) {
        return {problem: "Select at least one record type to search."};
    }
    if (query.length < minQueryLength) {
        return {problem: `Enter at least ${minQueryLength} characters to search.`};
    }

    const params = new URLSearchParams([["q", query]]);
    if (isRegex) {
        params.set("regex", "true");
    }
    if (kinds.length < 3) {
        params.set("kinds", kinds.join(","));
    }
    return {params: params.toString()};
}

async function doSearch(): Promise<void> {
    replaceWindowState();

    const current = currentQuery();
    const sequence = ++_searchSequence;
    // Whatever the previous search was still doing, it's obsolete now.
    _searchAbort?.abort();
    _searchAbort = null;

    if ("problem" in current) {
        renderResults([]);
        setSearching(false);
        _resultsInfo = current.problem;
        _resultsParams = null;
        refreshInfo();
        return;
    }

    const abort = new AbortController();
    _searchAbort = abort;
    setSearching(true);
    refreshInfo();

    const {resp, json, err} = await ims.fetchNoThrow<SearchResults>(
        `${url_search}?${current.params}`, {signal: abort.signal},
    );
    if (sequence !== _searchSequence) {
        // A newer search has been issued; it owns the page now.
        return;
    }
    _searchAbort = null;
    setSearching(false);

    if (err != null || json == null) {
        // A rejected query (e.g. an invalid regular expression) or one that
        // ran out of time is about the search itself, so report it where the
        // results would go rather than in the page-wide error banner.
        if (resp?.status === 400 || resp?.status === 503) {
            renderResults([]);
            _resultsInfo = err ?? "Search failed";
            _resultsParams = null;
            refreshInfo();
            ims.clearErrorMessage();
            return;
        }
        const message = `Search failed: ${err}`;
        console.error(message);
        ims.setErrorMessage(message);
        _resultsInfo = "";
        _resultsParams = null;
        refreshInfo();
        return;
    }
    ims.clearErrorMessage();

    renderResults(json.hits);
    let info = json.hits.length === 1 ? "1 result" : `${json.hits.length} results`;
    if (json.truncated) {
        info += " (too many matches; not all are shown — try a more specific search)";
    }
    _resultsInfo = info;
    _resultsParams = current.params;
    refreshInfo();
}

function resultURL(hit: SearchResult): string {
    switch (hit.kind) {
        case kindFieldReport:
            return url_viewFieldReportNumber
                .replace("<event_id>", hit.event)
                .replace("<number>", hit.number.toString());
        case kindVisit:
            return url_viewVisitNumber
                .replace("<event_id>", hit.event)
                .replace("<number>", hit.number.toString());
        default:
            return url_viewIncidentNumber
                .replace("<event_id>", hit.event)
                .replace("<number>", hit.number.toString());
    }
}

function renderResults(hits: SearchResult[]): void {
    const tbody = document.createElement("tbody");
    for (const hit of hits) {
        const rowFrag = el.resultRowTemplate.content.cloneNode(true) as DocumentFragment;
        const row = rowFrag.querySelector("tr")!;

        row.getElementsByClassName("result-event")[0]!.textContent = hit.event;
        row.getElementsByClassName("result-kind")[0]!.textContent = kindLabels[hit.kind]??hit.kind;

        const link: HTMLAnchorElement = row.querySelector(".result-number a")!;
        link.href = resultURL(hit);
        link.textContent = hit.number.toString();

        const created = new Date(hit.created);
        const createdCell: HTMLTableCellElement = row.querySelector(".result-created")!;
        createdCell.textContent = formatCreated(created);
        createdCell.title = ims.longFormatDate(created);

        row.getElementsByClassName("result-summary")[0]!.textContent = hit.summary??"";
        row.getElementsByClassName("result-snippet")[0]!.textContent = hit.snippet??"";

        row.addEventListener("click", function(e: MouseEvent): void {
            // Let clicks on the number link behave normally.
            if (e.target instanceof HTMLAnchorElement) {
                return;
            }
            window.location.href = resultURL(hit);
        });

        tbody.append(rowFrag);
    }
    el.resultsTable.querySelector("tbody")?.replaceWith(tbody);
}
