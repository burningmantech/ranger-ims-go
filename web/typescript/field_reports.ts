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
        frShowDays: (daysBackToShow: number | string, replaceState: boolean)=>void;
        frShowRows: (rowsToShow: string, replaceState: boolean)=>void;
        toggleMultisearchModal: (e?: MouseEvent)=>void;
    }
}

let fieldReportsTable: ims.DataTablesTable|null = null;

let _frShowModifiedAfter: Date|null = null;
let _frShowDaysBack: number|string|null = null;
const frDefaultDaysBack = "all";

const _frSearchDelayMs = 250;
let _frSearchDelayTimer: number|undefined = undefined;

let _frShowRows: string|null = null;
const frDefaultRows = "25";

//
// Initialize UI
//

initFieldReportsPage();

async function initFieldReportsPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    if (!ims.eventAccess!.readIncidents && !ims.eventAccess!.writeFieldReports) {
        ims.setErrorMessage(
            `You're not currently authorized to view Field Reports in Event "${ims.pathIds.eventName}".`
        );
        ims.hideLoadingOverlay();
        return;
    }

    window.frShowDays = frShowDays;
    window.frShowRows = frShowRows;

    ims.disableEditing();
    initFieldReportsTable();

    const helpModal = ims.bsModal(document.getElementById("helpModal")!);

    const multisearchModal = ims.bsModal(document.getElementById("multisearchModal")!);

    const eventDatas = ((await initResult.eventDatas)??[]).toReversed();

    window.toggleMultisearchModal = function (e?: MouseEvent): void {
        // Don't follow a href
        e?.preventDefault();

        multisearchModal.toggle();

        const list = document.getElementById("multisearch-events-list") as HTMLUListElement;
        list.querySelectorAll("li").forEach((el) => {el.remove()});

        const hashParams = ims.windowFragmentParams();
        const liTemplate = list.querySelector("template")!;
        for (const eventData of eventDatas) {
            const liFrag = liTemplate.content.cloneNode(true) as DocumentFragment;
            const eventLink = liFrag.querySelector("a")!;
            eventLink.textContent = eventData.name;
            eventLink.href = `${url_viewFieldReports.replace("<event_id>", eventData.name)}#${new URLSearchParams(hashParams).toString()}`;
            list.append(liFrag);
        }
    }

    // Keyboard shortcuts
    document.addEventListener("keydown", function(e: KeyboardEvent): void {
        // No shortcuts when an input field is active
        if (ims.blockKeyboardShortcutFieldActive()) {
            return;
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
            (document.getElementById("search_input") as HTMLInputElement).focus();
        }
        // n --> new incident
        if (e.key.toLowerCase() === "n") {
            (document.getElementById("new_field_report") as HTMLButtonElement).click();
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
// Dispatch queue table
//

function initFieldReportsTable() {
    frInitDataTables();
    frInitTableButtons();
    frInitSearchField();
    frInitSearch();
    ims.clearErrorMessage();

    if (ims.eventAccess?.writeFieldReports) {
        ims.enableEditing();
    }

    // Wait until the table is initialized before starting to listen for updates.
    // https://github.com/burningmantech/ranger-ims-go/issues/399
    fieldReportsTable!.on("init", function (): void {
        console.log("Table initialized. Requesting EventSource lock");
        ims.requestEventSourceLock();

        ims.newFieldReportChannel().onmessage = function (e: MessageEvent<ims.FieldReportBroadcast>): void {
            if (e.data.update_all) {
                console.log("Reloading the whole table to be cautious, as an SSE was missed");
                fieldReportsTable!.ajax.reload();
                ims.clearErrorMessage();
                return;
            }

            const number = e.data.field_report_number;
            const eventId = e.data.event_id;
            if (eventId !== ims.pathIds.eventId) {
                return;
            }
            console.log(`Got field report update: ${number}`);
            // TODO(issue/1498): this reloads the entire Field Report table on any
            //  update to any Field Report. That's not ideal. The thing of which
            //  to be mindful when GETting a particular single Field Report is that
            //  limited access users will receive errors when they try to access
            //  Field Reports for which they're not authorized, and those errors
            //  show up in the browser console. I'd like to find a way to avoid
            //  bringing those errors into the console constantly.
            fieldReportsTable!.ajax.reload(null, false);
            ims.clearErrorMessage();
        };
    });
}

declare let DataTable: any;

//
// Initialize DataTables
//

function frInitDataTables() {
    DataTable.ext.errMode = "none";
    fieldReportsTable = new DataTable("#field_reports_table", {
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
        // Responsive is too slow to resize when all FRs are shown.
        // Decide on this another day.
        // "responsive": {
        //     "details": false,
        // },
        // DataTables gets mad if you return a Promise from this function, so we use an inner
        // async function instead.
        // https://datatables.net/forums/discussion/47411/i-always-get-error-when-i-use-table-ajax-reload
        "ajax": function (_data: unknown, callback: (resp: {data: ims.FieldReport[]})=>void, _settings: unknown): void {
            async function doAjax(): Promise<void> {
                const {json, err} = await ims.fetchNoThrow<ims.FieldReport[]>(
                    // don't use exclude_system_entries here, since the field reports
                    // per-user authorization can exclude field reports entirely from
                    // someone who created a field report but then didn't add an
                    // entry to it.
                    ims.urlReplace(url_fieldReports), null,
                );
                if (err != null || json == null) {
                    ims.setErrorMessage(`Failed to load table: ${err}`);
                    return;
                }
                callback({data: json});
            }
            doAjax();
        },
        "columns": [
            {   // 0
                "name": "field_report_number",
                "className": "field_report_number text-right all",
                "data": "number",
                "defaultContent": null,
                "render": ims.renderFieldReportNumber,
                "cellType": "th",
            },
            {   // 1
                "name": "field_report_incident",
                "className": "field_report_incident text-center",
                "data": "incident",
                "defaultContent": "-",
                "render": ims.renderIncidentNumber,
                "responsivePriority": 3,
            },
            {   // 2
                "name": "field_report_created",
                "className": "field_report_created text-center",
                "data": "created",
                "defaultContent": null,
                "render": ims.renderDate,
                "responsivePriority": 4,
            },
            {   // 3
                "name": "field_report_summary",
                "className": "field_report_summary all",
                "data": "summary",
                "defaultContent": "",
                "render": renderSummary,
                "width": "70%",
            },
        ],
        "order": [
            // creation time descending
            [2, "dsc"],
        ],
        "createdRow": function (row: HTMLElement, fieldReport: ims.FieldReport, _index: number) {
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
                    window.location.href = `${ims.urlReplace(url_viewFieldReports)}/${fieldReport.number}`;
                }
                // Left click while holding modifier key or middle click: open in a new tab
                if (isMiddleClick || (isLeftClick && holdingModifier)) {
                    window.open(`${ims.urlReplace(url_viewFieldReports)}/${fieldReport.number}`);
                    return;
                }
            }
            row.addEventListener("click", openLink);
            row.addEventListener("auxclick", openLink);
        },
    });
}

function renderSummary(_data: string|null, type: string, fieldReport: ims.FieldReport): string|undefined {
    switch (type) {
        case "display":
            // XSS prevention
            return DataTable.render.text().display(ims.summarizeIncidentOrFR(fieldReport)) as string;
        case "sort":
            return ims.summarizeIncidentOrFR(fieldReport);
        case "filter":
            return ims.reportTextFromIncident(fieldReport);
        case "type":
            return "";
    }
    return undefined;
}

//
// Initialize table buttons
//

function frInitTableButtons() {
    const fragmentParams: URLSearchParams = ims.windowFragmentParams();

    // Set button defaults

    frShowDays(fragmentParams.get("days")??frDefaultDaysBack, false);

    frShowRows(fragmentParams.get("rows")??frDefaultRows, false);

    frShowRows(
        ims.coalesceRowsPerPage(
            fragmentParams.get("rows"),
            ims.getPreferredTableRowsPerPage(),
            frDefaultRows,
        ), false);
}


//
// Initialize search field
//

function frInitSearchField(): void {
    // Search field handling
    const searchInput = document.getElementById("search_input") as HTMLInputElement;

    function searchAndDraw(): void {
        frReplaceWindowState();
        let q = searchInput.value;
        let isRegex = false;
        let smartSearch = true;
        if (q.startsWith("/") && q.endsWith("/")) {
            isRegex = true;
            smartSearch = false;
            q = q.slice(1, q.length-1);
        }
        fieldReportsTable!.search(q, isRegex, smartSearch);
        fieldReportsTable!.draw();
    }

    const fragmentParams: URLSearchParams = ims.windowFragmentParams();
    const queryString: string|null = fragmentParams.get("q");
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
            clearTimeout(_frSearchDelayTimer);
            _frSearchDelayTimer = setTimeout(searchAndDraw, _frSearchDelayMs);
        }
    );
    searchInput.addEventListener("keydown",
        function (e: KeyboardEvent): void {
            // No shortcuts when ctrl, alt, or meta is being held down
            if (e.altKey || e.ctrlKey || e.metaKey) {
                return;
            }
            // "Jump to Field Report" functionality, triggered on hitting Enter
            if (e.key === "Enter") {
                // If the value in the search box is an integer, assume it's an FR number and go to it.
                // This will work regardless of whether that FR is visible with the current filters.
                const val = searchInput.value;
                if (ims.integerRegExp.test(val)) {
                    // Open the Field Report
                    window.location.href = `${ims.urlReplace(url_viewFieldReports)}/${val}`;
                    searchInput.value = "";
                    return;
                }
                // Otherwise, search immediately on Enter.
                clearTimeout(_frSearchDelayTimer);
                searchAndDraw();
            }
        }
    );
}


//
// Initialize search plug-in
//

function frInitSearch() {
    function modifiedAfter(fieldReport: ims.FieldReport, timestamp: Date) {
        if (timestamp < new Date(Date.parse(fieldReport.created!))) {
            return true;
        }
        // needs to use native comparison
        for (const entry of fieldReport.report_entries??[]) {
            if (timestamp < new Date(Date.parse(entry.created!))) {
                return true;
            }
        }
        return false;
    }

    fieldReportsTable!.search.fixed("modification_date",
        function(_searchStr: string, _rowData: object, rowIndex: number): boolean {
            const fieldReport = fieldReportsTable!.data()[rowIndex]!;
            return !(_frShowModifiedAfter != null &&
                !modifiedAfter(fieldReport, _frShowModifiedAfter));

        },
    );
}


//
// Show days button handling
//

function frShowDays(daysBackToShow: number|string, replaceState: boolean): void {
    const id: string = daysBackToShow.toString();
    _frShowDaysBack = daysBackToShow;

    const item = document.getElementById("show_days_" + id) as HTMLLIElement;

    // Get title from selected item
    const selection = item.getElementsByClassName("name")[0]!.textContent;

    // Update menu title to reflect selected item
    const menu = document.getElementById("show_days") as HTMLButtonElement;
    menu.getElementsByClassName("selection")[0]!.textContent = selection

    if (daysBackToShow === "all")  {
        _frShowModifiedAfter = null;
    } else {
        const after = new Date();
        after.setHours(0);
        after.setMinutes(0);
        after.setSeconds(0);
        after.setDate(after.getDate()-Number(daysBackToShow));
        _frShowModifiedAfter = after;
    }

    if (replaceState) {
        frReplaceWindowState();
    }

    fieldReportsTable!.draw();
}


//
// Show rows button handling
//

function frShowRows(rowsToShow: string, replaceState: boolean) {
    const id = rowsToShow.toString();
    _frShowRows = rowsToShow;

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
        frReplaceWindowState();
    }

    fieldReportsTable!.page.len(ims.parseInt10(rowsToShow));
    fieldReportsTable!.draw();
}


//
// Update the page URL based on the search input and other filters.
//

function frReplaceWindowState(): void {
    const newParams: [string, string][] = [];

    const searchVal = (document.getElementById("search_input") as HTMLInputElement).value;
    if (searchVal) {
        newParams.push(["q", searchVal]);
    }
    if (_frShowDaysBack != null && _frShowDaysBack !== frDefaultDaysBack) {
        newParams.push(["days", _frShowDaysBack.toString()]);
    }
    if (_frShowRows != null && _frShowRows !== frDefaultRows) {
        newParams.push(["rows", _frShowRows.toString()]);
    }

    // Next step is to create search params for the other filters too

    const newURL = `${ims.urlReplace(url_viewFieldReports)}#${new URLSearchParams(newParams).toString()}`;
    window.history.replaceState(null, "", newURL);
}
