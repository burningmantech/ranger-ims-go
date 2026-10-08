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
    showKind: ims.typedElement("show_kind", HTMLButtonElement),
    ulShowKind: ims.typedElement("ul_show_kind", HTMLUListElement),
    showKindToggleAll: ims.typedElement("show_kind_toggle_all", HTMLButtonElement),
    showEvent: ims.typedElement("show_event", HTMLButtonElement),
    ulShowEvent: ims.typedElement("ul_show_event", HTMLUListElement),
    showEventToggleAll: ims.typedElement("show_event_toggle_all", HTMLButtonElement),
    showEventTemplate: ims.typedElement("show_event_template", HTMLTemplateElement),
    resultsInfo: ims.typedElement("search_results_info", HTMLParagraphElement),
    resultsTable: ims.typedElement("search_results_table", HTMLTableElement),
    resultRowTemplate: ims.typedElement("search_result_row_template", HTMLTemplateElement),
};

// A dropdown of checkable items that narrows a selection down from
// everything, working like the Incidents page's Incident Type filter.
class CheckMenu {
    // The values the search uses. This lags the checkmarks while a fresh
    // selection is being started (see startingFresh).
    private selected: string[] = [];
    // Typing into the filter box while every item is checked starts a new
    // selection: the checkmarks clear, but the selection stays everything
    // until an item is actually picked.
    private startingFresh = false;

    constructor(
        private readonly toggle: HTMLButtonElement,
        private readonly menu: HTMLUListElement,
        toggleAll: HTMLButtonElement,
        private readonly noun: string,
        private readonly onChange: ()=>void,
    ) {
        for (const item of this.items()) {
            item.addEventListener("click", (e: MouseEvent): void => {
                e.preventDefault();
                setChecked(item, !isChecked(item));
                this.commit();
            });
        }
        toggleAll.addEventListener("click", (): void => {
            // Acts only on the items the filter box leaves showing.
            const shown = this.items().filter(item => !item.parentElement!.classList.contains("d-none"));
            const check = shown.some(item => !isChecked(item));
            for (const item of shown) {
                setChecked(item, check);
            }
            this.commit();
        });
        ims.addMenuFilter(toggle, menu, {
            placeholder: `Filter ${noun.toLowerCase()}...`,
            onFilter: (query: string): void => {
                toggleAll.textContent = query ? "Select/Deselect Matching" : "Select/Deselect All";
                if (query && !this.startingFresh && this.allSelected()) {
                    this.startingFresh = true;
                    this.checkOnly([]);
                } else if (!query && this.startingFresh) {
                    // Nothing got picked, so the selection is still everything.
                    this.startingFresh = false;
                    this.checkOnly(this.allValues());
                }
            },
        });
        this.selected = this.allValues();
    }

    private items(): HTMLElement[] {
        return [...this.menu.querySelectorAll<HTMLElement>(":scope > li > .dropdown-item-checkable")];
    }

    private allValues(): string[] {
        return this.items().map(item => item.dataset["value"]??"");
    }

    private checkOnly(values: string[]): void {
        for (const item of this.items()) {
            setChecked(item, values.includes(item.dataset["value"]??""));
        }
    }

    values(): string[] {
        return this.selected;
    }

    allSelected(): boolean {
        return this.selected.length === this.items().length;
    }

    // select sets the selection, ignoring unknown values, without counting
    // as a change to the form.
    select(values: string[]): void {
        this.checkOnly(values);
        this.read();
    }

    private commit(): void {
        this.read();
        this.onChange();
    }

    private read(): void {
        this.startingFresh = false;
        const checked = this.items().filter(isChecked);
        this.selected = checked.map(item => item.dataset["value"]??"");
        if (this.allSelected()) {
            this.toggle.textContent = `All ${this.noun}`;
        } else if (checked.length === 1) {
            this.toggle.textContent = checked[0]!.textContent;
        } else {
            this.toggle.textContent = `${this.noun} (${checked.length})`;
        }
    }
}

// The checkmark is drawn from the class; aria-pressed is what tells assistive
// tech the toggle's state.
function setChecked(item: Element, checked: boolean): void {
    item.classList.toggle("dropdown-item-checked", checked);
    item.setAttribute("aria-pressed", checked ? "true" : "false");
}

function isChecked(item: Element): boolean {
    return item.classList.contains("dropdown-item-checked");
}

let kindMenu: CheckMenu;
let eventMenu: CheckMenu;

initSearchPage();

async function initSearchPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }

    const eventNames = ((await initResult.eventDatas)??[])
        .filter(e => !e.is_group)
        .map(e => e.name)
        .sort((a, b) => b.localeCompare(a));
    for (const name of eventNames) {
        const li = el.showEventTemplate.content.cloneNode(true) as DocumentFragment;
        const item = li.querySelector("button")!;
        item.dataset["value"] = name;
        item.textContent = name;
        el.ulShowEvent.append(li);
    }

    // Searches only run when asked for, since each one is a real load on the
    // server. Edits to the form just keep the shareable URL current and note
    // that the results on screen no longer match the form.
    kindMenu = new CheckMenu(el.showKind, el.ulShowKind, el.showKindToggleAll, "Types", formChanged);
    eventMenu = new CheckMenu(el.showEvent, el.ulShowEvent, el.showEventToggleAll, "Events", formChanged);
    el.searchInput.addEventListener("input", formChanged);

    // Restore search parameters from the URL fragment, so that search links
    // can be shared and reloaded.
    const fragmentParams = ims.windowFragmentParams();
    el.searchInput.value = fragmentParams.get("q")??"";
    const kinds = fragmentParams.get("kinds");
    if (kinds != null) {
        kindMenu.select(kinds.split(","));
    }
    const events = fragmentParams.getAll("event");
    if (events.length > 0) {
        eventMenu.select(events);
    }

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
        el.resultsInfo.textContent = "Searching...";
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

function replaceWindowState(): void {
    const newParams: [string, string][] = [];
    if (el.searchInput.value) {
        newParams.push(["q", el.searchInput.value]);
    }
    if (!kindMenu.allSelected()) {
        newParams.push(["kinds", kindMenu.values().join(",")]);
    }
    if (!eventMenu.allSelected()) {
        for (const event of eventMenu.values()) {
            newParams.push(["event", event]);
        }
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
    if (kindMenu.values().length === 0) {
        return {problem: "Select at least one record type to search."};
    }
    // An empty Event menu (e.g. the Events failed to load) leaves the search
    // to cover whatever the server allows.
    if (!eventMenu.allSelected() && eventMenu.values().length === 0) {
        return {problem: "Select at least one Event to search."};
    }
    if (query.length < minQueryLength) {
        return {problem: `Enter at least ${minQueryLength} characters to search.`};
    }

    const params = new URLSearchParams([["q", query]]);
    if (isRegex) {
        params.set("regex", "true");
    }
    if (!kindMenu.allSelected()) {
        params.set("kinds", kindMenu.values().join(","));
    }
    // With every Event selected, send none, which also covers any Event
    // created since the page loaded.
    if (!eventMenu.allSelected()) {
        for (const event of eventMenu.values()) {
            params.append("event", event);
        }
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
        info += " (too many matches; not all are shown. Try a more specific search)";
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
