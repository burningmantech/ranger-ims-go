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

const _searchDelayMs = 250;
let _searchDelayTimer: number|undefined = undefined;

// Distinguishes the newest search request from any stale in-flight ones.
let _searchSequence = 0;

const el = {
    searchInput: ims.typedElement("search_input", HTMLInputElement),
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
    el.searchInput.addEventListener("input", search);
    el.kindIncident.addEventListener("change", search);
    el.kindFieldReport.addEventListener("change", search);
    el.kindVisit.addEventListener("change", search);

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

function search(): void {
    clearTimeout(_searchDelayTimer);
    _searchDelayTimer = window.setTimeout(doSearch, _searchDelayMs);
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

async function doSearch(): Promise<void> {
    replaceWindowState();

    const query = el.searchInput.value.trim();
    const kinds = selectedKinds();
    const sequence = ++_searchSequence;

    if (query.length < minQueryLength || kinds.length === 0) {
        renderResults([]);
        el.resultsInfo.textContent =
            kinds.length === 0
                ? "Select at least one record type to search."
                : `Enter at least ${minQueryLength} characters to search.`;
        return;
    }

    const params = new URLSearchParams([["q", query]]);
    if (kinds.length < 3) {
        params.set("kinds", kinds.join(","));
    }

    const {json, err} = await ims.fetchNoThrow<SearchResults>(
        `${url_search}?${params.toString()}`, null,
    );
    if (sequence !== _searchSequence) {
        // A newer search has been issued; discard this result.
        return;
    }
    if (err != null || json == null) {
        const message = `Search failed: ${err}`;
        console.error(message);
        ims.setErrorMessage(message);
        return;
    }
    ims.clearErrorMessage();

    renderResults(json.hits);
    let info = json.hits.length === 1 ? "1 result" : `${json.hits.length} results`;
    if (json.truncated) {
        info += " (too many matches; not all are shown — try a more specific search)";
    }
    el.resultsInfo.textContent = info;
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
