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

declare global {
    interface Window {
        showState: (stateToShow: ims.IncidentsTableState, replaceState: boolean)=>void;
        showDays: (daysBackToShow: number | string, replaceState: boolean)=>void;
        showRows: (rowsToShow: string, replaceState: boolean)=>void;
        toggleCheckAllTypes: ()=>void;
        toggleMultisearchModal: (e?: MouseEvent)=>void;
    }
}

// The DataTables object
let incidentsTable: ims.DataTablesTable|null = null;

const _searchDelayMs = 250;
let _searchDelayTimer: number|undefined = undefined;

let _showState: ims.IncidentsTableState|null = null;
const defaultState: ims.IncidentsTableState = "open";

let _showModifiedAfter: Date|null = null;
let _showDaysBack: number|string|null = null;
const defaultDaysBack = "all";

// list of Incident Types to show, in text form
let _showTypes: string[] = [];
let _showBlankType = true;
let _showOtherType = true;
// these must match values in incidents_template/template.xhtml
const _blankPlaceholder = "(blank)";
const _otherPlaceholder = "(other)";

let _showRows: string|null = null;
const defaultRows = "25";

let allIncidentTypes: ims.IncidentType[] = [];
let allIncidentTypeIds: number[] = [];
let visibleIncidentTypes: ims.IncidentType[] = [];
let visibleIncidentTypeIds: number[] = [];

//
// Initialize UI
//

initIncidentsPage();

async function initIncidentsPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    if (!ims.eventAccess!.readIncidents) {
        // This is a janky way of recreating the old server-side redirect to the Field Reports page.
        // The idea is that if the user is coming from the IMS home page and they don't have incidents
        // access, we should try to send them to FRs instead. If they're already within the scope of
        // the event, we should send them to the viewIncidents page and let them see the auth error.
        if (ims.eventAccess!.writeFieldReports && document.referrer.indexOf(ims.urlReplace(url_viewEvent)) < 0) {
            console.log("redirecting to Field Reports");
            window.location.replace(ims.urlReplace(url_viewFieldReports));
            return;
        }
        ims.setErrorMessage(
            "You're not currently authorized to access Incidents for this event. " +
            "You may be able to write Field Reports though. If you need access to " +
            "IMS Incidents while on-site, please get in touch with an on-duty " +
            "Operator. For post-event access, reach out to the tech cadre, at " +
            "ranger-tech-" + "" + "cadre" + "@burningman.org"
        );
        return;
    }

    window.showState = showState;
    window.showDays = showDays;
    window.showRows = showRows;
    window.toggleCheckAllTypes = toggleCheckAllTypes;

    await initIncidentsTable();

    const helpModal = ims.bsModal(document.getElementById("helpModal")!);

    const multisearchModal = ims.bsModal(document.getElementById("multisearchModal")!);

    const eventDatas = ((await initResult.eventDatas)??[]).toReversed();

    window.toggleMultisearchModal = function (e?: MouseEvent): void {
        // Don't follow a href
        e?.preventDefault();

        multisearchModal.toggle();

        const list = document.getElementById("multisearch-events-list") as HTMLUListElement;
        list.replaceChildren();

        for (const eventData of eventDatas) {
            const hashParams = ims.windowFragmentParams();

            const newLink = document.createElement("a");
            newLink.textContent = eventData.name;
            newLink.href = `${url_viewIncidents.replace("<event_id>", eventData.name)}#${new URLSearchParams(hashParams).toString()}`;

            const newLi = document.createElement("li");
            newLi.append(newLink);
            list.append(newLi);
        }
    }

    // Keyboard shortcuts
    document.addEventListener("keydown", function(e: KeyboardEvent): void {
        // No shortcuts when an input field is active
        if (ims.blockKeyboardShortcutFieldActive()) {
            return
        }
        // No shortcuts when ctrl, alt, or meta is being held down
        if (e.altKey || e.ctrlKey || e.metaKey) {
            return;
        }
        // ? --> show help modal
        if (e.key === "?") {
            helpModal.toggle();
        }
        // / --> jump to search box
        if (e.key === "/") {
            // don't immediately input a "/" into the search box
            e.preventDefault();
            document.getElementById("search_input")!.focus();
        }
        // n --> new incident
        if (e.key.toLowerCase() === "n") {
            document.getElementById("new_incident")!.click();
        }
        // m -> multi-search
        if (e.key.toLowerCase() === "m") {
            window.toggleMultisearchModal();
        }
    });

    document.getElementById("helpModal")!.addEventListener("keydown", function(e: KeyboardEvent): void {
        if (e.key === "?") {
            helpModal.toggle();
            // This is needed to prevent the document's listener for "?" to trigger the modal to
            // toggle back on immediately. This is fallout from the fix for
            // https://github.com/twbs/bootstrap/issues/41005#issuecomment-2497670835
            e.stopPropagation();
        }
    });

}


//
// Load event field reports
//
// Note that nothing from these data is displayed in the incidents table.
// We do this fetch in order to make incidents searchable by text in their
// attached field reports.

let eventFieldReports: ims.FieldReportsByNumber|undefined = undefined;

async function loadEventFieldReports(): Promise<{err: string|null}> {
    const {json, err} = await ims.fetchNoThrow<ims.FieldReport[]>(
        ims.urlReplace(url_fieldReports + "?exclude_system_entries=true"), null,
    );
    if (err != null) {
        const message = `Failed to load event field reports: ${err}`;
        console.error(message);
        ims.setErrorMessage(message);
        return {err: message};
    }
    const reports: ims.FieldReportsByNumber = {};

    for (const report of json!) {
        reports[report.number!] = report;
    }

    eventFieldReports = reports;

    console.log("Loaded event field reports");
    return {err: null};
}

//
// Dispatch queue table
//

async function initIncidentsTable(): Promise<void> {
    // Fetch Incident Types and Streets asynchronously, so that the DataTables ajax
    // handler can start its own API calls before these need to complete.
    const tablePrereqs: Promise<void> = Promise.all([
        await ims.loadIncidentTypes().then(
            value=>{
                allIncidentTypes=value.types;
                allIncidentTypeIds=value.types.map(it=>it.id).filter(id=>id != null);
                visibleIncidentTypes=value.types.filter(it=>!it.hidden);
                visibleIncidentTypeIds=visibleIncidentTypes.map(it=>it.id).filter(id=>id != null);
            },
        ),
        ims.loadStreets(ims.pathIds.eventId),
    ]).then(_=>{});

    initDataTables(tablePrereqs);
    initTableButtons();
    initSearchField();
    initSearch();
    ims.clearErrorMessage();

    if (ims.eventAccess?.writeIncidents) {
        ims.enableEditing();
    } else {
        ims.disableEditing();
    }

    // Wait until the table is initialized before starting to listen for updates.
    // https://github.com/burningmantech/ranger-ims-go/issues/399
    incidentsTable!.on("init", function (): void {
        console.log("Table initialized. Requesting EventSource lock");
        ims.requestEventSourceLock();

        ims.newIncidentChannel().onmessage = async function (e: MessageEvent<ims.IncidentBroadcast>): Promise<void> {
            if (e.data.update_all) {
                console.log("Reloading the whole table to be cautious, as an SSE was missed");
                incidentsTable!.ajax.reload();
                ims.clearErrorMessage();
                return;
            }

            const number = e.data.incident_number!;
            const eventId = e.data.event_id;
            if (eventId !== ims.pathIds.eventId) {
                return;
            }

            const {json, err} = await ims.fetchNoThrow(
                ims.urlReplace(url_incidentNumber).replace("<incident_number>", number.toString()),
                null,
            );
            if (err != null) {
                const message = `Failed to update Incident ${number}: ${err}`;
                console.error(message);
                ims.setErrorMessage(message);
                return;
            }
            // Now update/create the relevant row. This is a change from pre-2025, in that
            // we no longer reload all incidents here on any single incident update.
            let done = false;
            incidentsTable!.rows().every(function () {
                // @ts-expect-error use of "this" for DataTables
                const existingIncident = this.data();
                if (existingIncident.number === number) {
                    console.log("Updating Incident " + number);
                    // @ts-expect-error use of "this" for DataTables
                    this.data(json);
                    done = true;
                }
            });
            if (!done) {
                console.log("Loading new Incident " + number);
                incidentsTable!.row.add(json);
            }
            ims.clearErrorMessage();
            incidentsTable!.processing(false);
            // maintain page location if user is not on page 1
            incidentsTable!.draw("full-hold");
        };
    });
}

declare let DataTable: any;

//
// Initialize DataTables
//

function initDataTables(tablePrereqs: Promise<void>): void {
    DataTable.ext.errMode = "none";
    incidentsTable = new DataTable("#queue_table", {
        // Save table state to SessionStorage (-1). This tells DataTables to save state
        // on any update to the sorting/filtering, and to load that table state again
        // when the browsing context comes back to this page.
        "stateSave": true,
        "stateDuration": -1,
        "stateLoadParams": function(_settings: any, _data: any): boolean|void {
            // We only want to restore the table state if the user got here using back or forward buttons.
            // If the user arrived via reload or navigation through the site, we want to start fresh.
            const navType = window.performance.getEntries()[0];
            if (navType instanceof PerformanceNavigationTiming && navType?.type !== "back_forward") {
                return false;
            }
        },
        "deferRender": true,
        "paging": true,
        "lengthChange": false,
        "searching": true,
        "processing": true,
        "scrollX": false, "scrollY": false,
        "layout": {
            "topStart": null,
            "topEnd": null,
            "bottomStart": "info",
            "bottomEnd": "paging",
        },
        // Responsive is too slow to resize when all Incidents are shown.
        // Decide on this another day.
        // "responsive": {
        //     "details": false,
        // },

        // DataTables gets mad if you return a Promise from this function, so we use an inner
        // async function instead.
        // https://datatables.net/forums/discussion/47411/i-always-get-error-when-i-use-table-ajax-reload
        "ajax": function (_data: any, callback: (resp: {data: ims.Incident[]})=>void, _settings: any): void {
            async function doAjax(): Promise<void> {
                let json: ims.Incident[] = [];
                // concurrently fetch the data needed for the table
                await Promise.all([
                    tablePrereqs,
                    loadEventFieldReports(),
                    ims.fetchNoThrow<ims.Incident[]>(
                        ims.urlReplace(url_incidents + "?exclude_system_entries=true"), null,
                    ).then(res => {
                        if (res.err != null || res.json == null) {
                            ims.setErrorMessage(`Failed to load table: ${res.err}`);
                            return;
                        }
                        json = res.json;
                    }),
                ]);
                // then call the callback, only once all data sources have returned
                callback({data: json});
            }
            doAjax();
        },
        "columns": [
            {   // 0
                "name": "incident_number",
                "className": "incident_number text-right all",
                "data": "number",
                "defaultContent": null,
                "render": ims.renderIncidentNumber,
                "cellType": "th",
                // "all" class --> very high responsivePriority
            },
            {   // 1
                "name": "incident_started",
                "className": "incident_started text-center",
                "data": "started",
                "defaultContent": null,
                "render": ims.renderDate,
                "responsivePriority": 7,
            },
            {   // 2
                "name": "incident_state",
                "className": "incident_state text-center",
                "data": "state",
                "defaultContent": null,
                "render": ims.renderState,
                "responsivePriority": 3,
            },
            {   // 3
                "name": "incident_summary",
                "className": "incident_summary all",
                "data": "summary",
                "defaultContent": "",
                "render": renderSummary,
                "width": "40%",
                // "all" class --> very high responsivePriority
            },
            {   // 4
                "name": "incident_types",
                "className": "incident_types",
                "data": "incident_type_ids",
                "defaultContent": "",
                "render": function(ids: number[], _type: string, _incident: ims.Incident) {
                    const vals: string[] = [];
                    // render hidden incident types too here
                    for (const it of allIncidentTypes) {
                        if (ids.includes(it.id??-1) && it.name) {
                            vals.push(it.name);
                        }
                    }
                    return ims.renderSafeSorted(vals);
                },
                "responsivePriority": 4,
            },
            {   // 5
                "name": "incident_location",
                "className": "incident_location",
                "data": "location",
                "defaultContent": "",
                "render": ims.renderLocation,
                "responsivePriority": 5,
            },
            {   // 6
                "name": "incident_ranger_handles",
                "className": "incident_ranger_handles",
                "data": "ranger_handles",
                "defaultContent": "",
                "render": ims.renderSafeSorted,
                "responsivePriority": 6,
            },
            {   // 7
                "name": "incident_last_modified",
                "className": "incident_last_modified text-center",
                "data": "last_modified",
                "defaultContent": null,
                "render": ims.renderDate,
                "responsivePriority": 8,
            },
        ],
        "order": [
            // 1 --> "Started" time
            [1, "dsc"],
        ],
        "createdRow": function (row: HTMLElement, incident: ims.Incident, _index: number) {
            const openLink = function(e: MouseEvent): void {
                // If the user clicked on a link, then let them access that link without the JS below.
                if (e.target?.constructor?.name === "HTMLAnchorElement") {
                    return;
                }

                const isLeftClick = e.type === "click";
                const isMiddleClick = e.type === "auxclick" && e.button === 1;
                const holdingModifier = e.altKey || e.ctrlKey || e.metaKey;

                // Left click while not holding a modifier key: open in the same tab
                if (isLeftClick && !holdingModifier) {
                    window.location.href = `${ims.urlReplace(url_viewIncidents)}/${incident.number}`;
                }
                // Left click while holding modifier key or middle click: open in a new tab
                if (isMiddleClick || (isLeftClick && holdingModifier)) {
                    window.open(`${ims.urlReplace(url_viewIncidents)}/${incident.number}`);
                    return;
                }
            }
            row.addEventListener("click", openLink);
            row.addEventListener("auxclick", openLink);
        },
    });
}

function renderSummary(_data: string|null, type: string, incident: ims.Incident): string|undefined {
    switch (type) {
        case "display": {
            const maxDisplayLength = 250;
            let summarized = ims.summarizeIncidentOrFR(incident);
            if (summarized.length > maxDisplayLength) {
                summarized = summarized.substring(0, maxDisplayLength - 3) + "...";
            }
            // XSS prevention
            return DataTable.render.text().display(summarized) as string;
        }
        case "sort":
            return ims.summarizeIncidentOrFR(incident);
        case "filter":
            return ims.reportTextFromIncident(incident, eventFieldReports);
        case "type":
            return "";
    }
    return undefined;
}


//
// Initialize table buttons
//

function initTableButtons(): void {

    const typeFilter = document.getElementById("ul_show_type") as HTMLUListElement;
    for (const type of visibleIncidentTypes) {
        const template = document.getElementById("show_type_template") as HTMLTemplateElement;
        const newLi = template.content.cloneNode(true) as DocumentFragment;

        const newLink = newLi.querySelector("a")!;
        newLink.dataset["incidentTypeId"] = type.id?.toString();
        newLink.textContent = type.name??"";
        typeFilter.append(newLi);
    }

    for (const el of document.getElementsByClassName("dropdown-item-checkable")) {
        const htmlEl = el as HTMLElement;
        htmlEl.addEventListener("click", function (e: MouseEvent): void {
            e.preventDefault();
            htmlEl.classList.toggle("dropdown-item-checked");
            showCheckedTypes(true);
        })
    }

    const fragmentParams: URLSearchParams = ims.windowFragmentParams();

    // Set button defaults

    const types: string[] = fragmentParams.getAll("type");
    if (types.length > 0) {
        const validTypes: number[] = [];
        let includeBlanks = false;
        let includeOthers = false;
        for (const t of types) {
            const typeId = ims.parseInt10(t);
            if (typeId && visibleIncidentTypeIds.indexOf(typeId) !== -1) {
                validTypes.push(typeId);
            } else if (t === _blankPlaceholder) {
                includeBlanks = true;
            } else if (t === _otherPlaceholder) {
                includeOthers = true;
            }
        }
        setCheckedTypes(validTypes, includeBlanks, includeOthers);
    }
    showCheckedTypes(false);

    // For state, we look first at the default,
    // then override with preferred,
    // then override with fragment value
    let state: ims.IncidentsTableState = defaultState;
    const preferredState = ims.getIncidentsPreferredState();
    if (preferredState) {
        state = preferredState;
    }
    const stateStr = fragmentParams.get("state");
    if (ims.isValidIncidentsTableState(stateStr)) {
        state = stateStr;
    }
    showState(state, false);

    showDays(fragmentParams.get("days")??defaultDaysBack, false);

    showRows(
        ims.coalesceRowsPerPage(
        fragmentParams.get("rows"),
        ims.getPreferredTableRowsPerPage(),
        defaultRows,
    ), false);
}


//
// Initialize search field
//


function initSearchField() {
    // Search field handling
    const searchInput = document.getElementById("search_input") as HTMLInputElement;

    function searchAndDraw(): void {
        replaceWindowState();
        let q = searchInput.value;
        let isRegex = false;
        let smartSearch = true;
        if (q.startsWith("/") && q.endsWith("/")) {
            isRegex = true;
            smartSearch = false;
            q = q.slice(1, q.length-1);
        }
        incidentsTable!.search(q, isRegex, smartSearch);
        incidentsTable!.draw();
    }

    const fragmentParams: URLSearchParams = ims.windowFragmentParams();
    const queryString = fragmentParams.get("q");
    if (queryString) {
        searchInput.value = queryString;
        searchAndDraw();
    }

    searchInput.addEventListener("input",
        function (_: Event): void {
            // Delay the search in case the user is still typing.
            // This reduces perceived lag, since searching can be
            // very slow, and it's super annoying for a user when
            // the page fully locks up before they're done typing.
            clearTimeout(_searchDelayTimer);
            _searchDelayTimer = setTimeout(searchAndDraw, _searchDelayMs);
        }
    );
    searchInput.addEventListener("keydown",
        function (e: KeyboardEvent): void {
            // No shortcuts when ctrl, alt, or meta is being held down
            if (e.altKey || e.ctrlKey || e.metaKey) {
                return;
            }
            // "Jump to Incident" functionality, triggered on hitting Enter
            if (e.key === "Enter") {
                // If the value in the search box is an integer, assume it's an IMS number and go to it.
                // This will work regardless of whether that incident is visible with the current filters.
                const val = searchInput.value;
                if (ims.integerRegExp.test(val)) {
                    // Open the Incident
                    window.location.href = `${ims.urlReplace(url_viewIncidents)}/${val}`;
                    searchInput.value = "";
                    return;
                }
                // Otherwise, search immediately on Enter.
                clearTimeout(_searchDelayTimer);
                searchAndDraw();
            }
        }
    );
}


//
// Initialize search plug-in
//

function initSearch(): void {
    incidentsTable!.search.fixed("modification_date",
        function(_searchStr: string, _rowData: object, rowIndex: number): boolean {
            const incident: ims.Incident = incidentsTable!.data()[rowIndex]!;
            return !(_showModifiedAfter != null &&
                new Date(Date.parse(incident.last_modified!)) < _showModifiedAfter);
        },
    );

    incidentsTable!.search.fixed("state", function(_searchStr: string, _rowData: object, rowIndex: number): boolean {
        const incident: ims.Incident = incidentsTable!.data()[rowIndex]!;
        let state;
        if (_showState != null) {
            switch (_showState) {
                case "all":
                    break;
                case "active":
                    state = ims.stateForIncident(incident);
                    if (state === "on_hold" || state === "closed") {
                        return false;
                    }
                    break;
                case "open":
                    state = ims.stateForIncident(incident);
                    if (state === "closed") {
                        return false;
                    }
                    break;
            }
        }
        return true;
    });

    incidentsTable!.search.fixed("type", function (_searchStr: string, _rowData: object, rowIndex: number): boolean {
        const incident: ims.Incident = incidentsTable!.data()[rowIndex]!;
        // don't bother with filtering if all types are selected
        if (!allTypesChecked()) {
            const rowTypes: number[] = Object.values(incident.incident_type_ids??[]);
            // the selected types include one of this row's types
            const intersect = rowTypes.filter(t => _showTypes.includes(t.toString())).length > 0;
            // "blank" is selected, and this row has no types
            const blankShow = _showBlankType && rowTypes.length === 0;
            // "other" is selected, and this row has a type that is hidden
            const otherShow = _showOtherType && rowTypes.some(t => !(visibleIncidentTypeIds.includes(t)));
            if (!intersect && !blankShow && !otherShow) {
                return false;
            }
        }

        return true;
    });
}


//
// Show state button handling
//

function showState(stateToShow: ims.IncidentsTableState, replaceState: boolean) {
    const item = document.getElementById("show_state_" + stateToShow) as HTMLLIElement;

    // Get title from selected item
    const selection = item.getElementsByClassName("name")[0]!.textContent;

    // Update menu title to reflect selected item
    const menu = document.getElementById("show_state") as HTMLButtonElement;
    menu.getElementsByClassName("selection")[0]!.textContent = selection;

    _showState = stateToShow;

    if (replaceState) {
        replaceWindowState();
    }

    incidentsTable!.draw();
}


//
// Show days button handling
//

function showDays(daysBackToShow: number|string, replaceState: boolean): void {
    const id: string = daysBackToShow.toString();
    _showDaysBack = daysBackToShow;

    const item = document.getElementById("show_days_" + id) as HTMLLIElement;

    // Get title from selected item
    const selection = item.getElementsByClassName("name")[0]!.textContent;

    // Update menu title to reflect selected item
    const menu = document.getElementById("show_days") as HTMLButtonElement;
    menu.getElementsByClassName("selection")[0]!.textContent = selection

    if (daysBackToShow === "all") {
        _showModifiedAfter = null;
    } else {
        const after = new Date();
        after.setHours(0);
        after.setMinutes(0);
        after.setSeconds(0);
        after.setDate(after.getDate()-Number(daysBackToShow));
        _showModifiedAfter = after;
    }

    if (replaceState) {
        replaceWindowState();
    }

    incidentsTable!.draw();
}

//
// Show type button handling
//

function setCheckedTypes(types: number[], includeBlanks: boolean, includeOthers: boolean): void {
    for (const type of document.querySelectorAll('#ul_show_type > li > a')) {
        const typeIdStr = (type as HTMLElement).dataset["incidentTypeId"];
        const typeIdNum = ims.parseInt10(typeIdStr);
        if (types.includes(typeIdNum!) ||
            (includeBlanks && type.id === "show_blank_type") ||
            (includeOthers && type.id === "show_other_type")
        ) {
            type.classList.add("dropdown-item-checked");
        } else {
            type.classList.remove("dropdown-item-checked");
        }
    }
}

function toggleCheckAllTypes(): void {
    if (_showTypes.length === 0 || _showTypes.length < visibleIncidentTypes.length) {
        setCheckedTypes(allIncidentTypeIds, true, true);
    } else {
        setCheckedTypes([], false, false);
    }
    showCheckedTypes(true);
}

function readCheckedTypes(): void {
    _showTypes = [];

    for (const type of document.querySelectorAll('#ul_show_type > li > a')) {
        if (type.id === "show_blank_type") {
            _showBlankType = type.classList.contains("dropdown-item-checked");
        } else if (type.id === "show_other_type") {
            _showOtherType = type.classList.contains("dropdown-item-checked");
        } else if (type.classList.contains("dropdown-item-checked")) {
            const typeId = (type as HTMLElement).dataset["incidentTypeId"];
            _showTypes.push(typeId??"");
        }
    }
}

function allTypesChecked(): boolean {
    return _showTypes.length === visibleIncidentTypes.length && _showBlankType && _showOtherType;
}

function showCheckedTypes(replaceState: boolean): void {
    readCheckedTypes();

    const numTypesShown = _showTypes.length + (_showBlankType ? 1 : 0) + (_showOtherType ? 1 : 0);
    let showTypeText: string;
    if (numTypesShown === 1) {
        if (_showBlankType) {
            showTypeText = "(blank)";
        } else if (_showOtherType) {
            showTypeText = "(other)";
        } else {
            showTypeText = allIncidentTypes.find(it=>it.id?.toString()===_showTypes[0])?.name??"Unknown";
        }
    } else {
        if (allTypesChecked()) {
            showTypeText = "All Types";
        } else {
            showTypeText = `Types (${numTypesShown})`;
        }
    }

    document.getElementById("show_type")!.textContent = showTypeText;

    if (replaceState) {
        replaceWindowState();
    }

    incidentsTable!.draw();
}

//
// Show rows button handling
//

function showRows(rowsToShow: string, replaceState: boolean): void {
    const id = rowsToShow;
    _showRows = rowsToShow;

    const item = document.getElementById("show_rows_" + id) as HTMLLIElement;

    // Get title from selected item
    const selection = item.getElementsByClassName("name")[0]!.textContent;

    // Update menu title to reflect selected item
    const menu = document.getElementById("show_rows") as HTMLButtonElement;
    menu.getElementsByClassName("selection")[0]!.textContent = selection

    if (rowsToShow === "all") {
        rowsToShow = "-1";
    }

    if (replaceState) {
        replaceWindowState();
    }

    incidentsTable!.page.len(ims.parseInt10(rowsToShow));
    incidentsTable!.draw();
}

//
// Update the page URL based on the search input and other filters.
//

function replaceWindowState(): void {
    const newParams: [string, string][] = [];

    const searchVal = (document.getElementById("search_input") as HTMLInputElement).value;
    if (searchVal) {
        newParams.push(["q", searchVal]);
    }
    if (!allTypesChecked()) {
        for (const t of _showTypes) {
            newParams.push(["type", t]);
        }
        if (_showBlankType) {
            newParams.push(["type", _blankPlaceholder]);
        }
        if (_showOtherType) {
            newParams.push(["type", _otherPlaceholder]);
        }
    }
    if (_showState != null && _showState !== defaultState) {
        newParams.push(["state", _showState]);
    }
    if (_showDaysBack != null && _showDaysBack !== defaultDaysBack) {
        newParams.push(["days", _showDaysBack.toString()]);
    }
    if (_showRows != null && _showRows !== defaultRows) {
        newParams.push(["rows", _showRows]);
    }
    const newURL = `${ims.urlReplace(url_viewIncidents)}#${new URLSearchParams(newParams).toString()}`;
    window.history.replaceState(null, "", newURL);
}
