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
        destShowRows: (rowsToShow: string, replaceState: boolean)=>void;
    }
}

let destinationsTable: ims.DataTablesTable|null = null;

const _destSearchDelayMs = 250;
let _destSearchDelayTimer: number|undefined = undefined;

let _destShowRows: string|null = null;
const destDefaultRows = "25";

//
// Initialize UI
//

initDestinationsPage();



async function initDestinationsPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    if (!ims.eventAccess!.readIncidents && !ims.eventAccess!.writeFieldReports) {
        ims.setErrorMessage(
            `You're not currently authorized to view Destinations in Event "${ims.pathIds.eventName}".`
        );
        ims.hideLoadingOverlay();
        return;
    }

    window.destShowRows = destShowRows;

    ims.disableEditing();
    initDestinationsTable();

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
        // / --> jump to search box
        if (e.key === "/") {
            // don't immediately input a "/" into the search box
            e.preventDefault();
            document.getElementById("search_input")!.focus();
        }
    });
}


//
// Dispatch queue table
//

function initDestinationsTable() {
    destInitDataTables();
    destInitTableButtons();
    destInitSearchField();
    destInitSearch();
    ims.clearErrorMessage();
}

declare let DataTable: any;

//
// Initialize DataTables
//

function destInitDataTables() {
    const destinationInfoModal = ims.bsModal(document.getElementById("destinationInfoModal")!);

    DataTable.ext.errMode = "none";
    destinationsTable = new DataTable("#destinations_table", {
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
        // DataTables gets mad if you return a Promise from this function, so we use an inner
        // async function instead.
        // https://datatables.net/forums/discussion/47411/i-always-get-error-when-i-use-table-ajax-reload
        "ajax": function (_data: unknown, callback: (resp: {data: ims.Destination[]})=>void, _settings: unknown): void {
            async function doAjax(): Promise<void> {
                const {json, err} = await ims.fetchNoThrow<ims.Destinations>(
                    ims.urlReplace(url_destinations), null,
                );
                if (err != null || json == null) {
                    ims.setErrorMessage(`Failed to load table: ${err}`);
                    return;
                }
                const destinations: ims.Destination[] = [];
                for (const art of json.art??[]) {
                    art.type = "art";
                    art.description = (art.external_data as ims.BMArt).description;
                    destinations.push(art);
                }
                for (const camp of json.camp??[]) {
                    camp.type = "camp";
                    camp.description = (camp.external_data as ims.BMCamp).description;
                    destinations.push(camp);
                }
                for (const other of json.other??[]) {
                    other.type = "other";
                    destinations.push(other);
                }
                callback({data: destinations});
            }
            doAjax();
        },
        "columns": [
            {   // 0
                "name": "destination_name",
                "className": "destination_name text-left all",
                "data": "name",
                "cellType": "th",
            },
            {   // 1
                "name": "destination_address",
                "className": "destination_address text-left",
                "data": "location_string",
            },
            {   // 2
                "name": "destination_type",
                "className": "destination_type text-left",
                "data": "type",
            },
            {   // 3
                "name": "destination_description",
                "className": "destination_description text-left",
                "data":  "description",
                "render": renderWithMaxLength(200),
            },
        ],
        "order": [
            [0, "asc"],
        ],

        "createdRow": function (row: HTMLElement, destination: ims.Destination, _index: number) {
            const openLink = function(_e: MouseEvent): void {
                (document.getElementById("destinationInfoModalLabel") as HTMLParagraphElement).textContent = destination.name??"(unnamed destination)";
                document.getElementById("destinationBody")!.replaceChildren(destinationToHTML(destination));
                destinationInfoModal.toggle();
            }
            row.addEventListener("click", openLink);
            row.addEventListener("auxclick", openLink);
        },
    });
}

function destinationToHTML(destination: ims.Destination): Node {
    switch (destination.type) {
        case "camp": {
            const camp = destination.external_data as ims.BMCamp;

            const campTemplate = document.getElementById("camp_template") as HTMLTemplateElement;

            // Clone the new row and insert it into the table
            const campEl = campTemplate.content.cloneNode(true) as DocumentFragment;

            campEl.getElementById("camp_name")!.textContent = camp.name;
            campEl.getElementById("location_label")!.textContent = `frontage ${camp.location?.intersection_type} intersection`;
            campEl.getElementById("location_string")!.textContent =
                `${camp.location_string ?? "Unknown"}\n` +
                `${camp.location?.exact_location ?? ""}\n` +
                `${camp.location?.dimensions ?? "Unknown"}`;
            campEl.getElementById("description")!.textContent = camp.description ?? "None provided";
            campEl.getElementById("landmark")!.textContent = camp.landmark ?? "None provided";
            let imageURL = camp.images?.find((value: object): boolean => {
                return "thumbnail_url" in value;
            })?.thumbnail_url;
            if (imageURL) {
                if (imageURL.includes("?")) {
                    imageURL = imageURL.substring(0, imageURL.indexOf("?"));
                }
                const imageLink = campEl.getElementById("image_url") as HTMLAnchorElement;
                imageLink.href = imageURL;
            } else {
                campEl.getElementById("image_dd")!.textContent = "None provided"
            }
            if (camp.contact_email) {
                const emailLink = campEl.getElementById("email_link") as HTMLAnchorElement;
                emailLink.href = `mailto:${camp.contact_email}`;
                emailLink.textContent = camp.contact_email;
            } else {
                campEl.getElementById("email_dd")!.textContent = "None provided";
            }
            if (camp.url) {
                const websiteLink = campEl.getElementById("website_url") as HTMLAnchorElement;
                websiteLink.href = camp.url;
                websiteLink.textContent = camp.url;
            } else {
                campEl.getElementById("website_dd")!.textContent = "None provided";
            }
            campEl.getElementById("hometown")!.textContent = camp.hometown ?? "None provided";
            campEl.getElementById("uid")!.textContent = camp.uid ?? "None";
            return campEl;
        }
        case "art": {
            const art = destination.external_data as ims.BMArt;

            const template = document.getElementById("art_template") as HTMLTemplateElement;

            // Clone the new row and insert it into the table
            const clone = template.content.cloneNode(true) as DocumentFragment;

            clone.getElementById("art_name")!.textContent = art.name;
            clone.getElementById("location_string")!.textContent =
                `${art.location_string ?? "Unknown"}\n` +
                // TODO: could link to Google Maps with the lat/long: https://www.google.com/maps/search/%s
                `${art.location?.gps_latitude ?? "Unknown"},${art.location?.gps_longitude ?? "Unknown"}`;
            clone.getElementById("description")!.textContent = art.description ?? "None provided";
            clone.getElementById("artist")!.textContent = art.artist ?? "None provided";
            let imageURL = art.images?.find((value: object): boolean => {
                return "thumbnail_url" in value;
            })?.thumbnail_url;
            if (imageURL) {
                if (imageURL.includes("?")) {
                    imageURL = imageURL.substring(0, imageURL.indexOf("?"));
                }
                const imageLink = clone.getElementById("image_url") as HTMLAnchorElement;
                imageLink.href = imageURL;
            } else {
                clone.getElementById("image_dd")!.textContent = "None provided"
            }
            if (art.contact_email) {
                const emailLink = clone.getElementById("email_link") as HTMLAnchorElement;
                emailLink.href = `mailto:${art.contact_email}`;
                emailLink.textContent = art.contact_email;
            } else {
                clone.getElementById("email_dd")!.textContent = "None provided";
            }
            if (art.url) {
                const websiteLink = clone.getElementById("website_url") as HTMLAnchorElement;
                websiteLink.href = art.url;
                websiteLink.textContent = art.url;
            } else {
                clone.getElementById("website_dd")!.textContent = "None provided";
            }
            clone.getElementById("hometown")!.textContent = art.hometown ?? "None provided";
            clone.getElementById("uid")!.textContent = art.uid ?? "None";
            return clone;
        }
        default:
            // TODO: implement something to present ad-hoc locations better
            const el = document.createElement("p");
            el.textContent = JSON.stringify(destination.external_data, null, 2);
            return el;
    }
}

function renderWithMaxLength(maxLength: number): (data: (string | null), type: string, _dest: ims.Destination) => (string | undefined) {
    return function (data: string|null, type: string, _dest: ims.Destination): string|undefined {
        switch (type) {
            case "display":
                if ((data?.length??0) > maxLength+3) {
                    data = data!.substring(0, maxLength) + "...";
                }
                // XSS prevention
                return DataTable.render.text().display(data) as string;
            case "sort":
            case "filter":
                return data??"";
            case "type":
                return "";
        }
        return undefined;
    }
}
//
// Initialize table buttons
//

function destInitTableButtons() {
    const fragmentParams: URLSearchParams = ims.windowFragmentParams();

    // Set button defaults

    destShowRows(
        ims.coalesceRowsPerPage(
            fragmentParams.get("rows"),
            ims.getPreferredTableRowsPerPage(),
            destDefaultRows,
        ), false);
}


//
// Initialize search field
//

function destInitSearchField(): void {
    // Search field handling
    const searchInput = document.getElementById("search_input") as HTMLInputElement;

    function searchAndDraw(): void {
        destReplaceWindowState();
        let q = searchInput.value;
        let isRegex = false;
        let smartSearch = true;
        if (q.startsWith("/") && q.endsWith("/")) {
            isRegex = true;
            smartSearch = false;
            q = q.slice(1, q.length-1);
        }
        destinationsTable!.search(q, isRegex, smartSearch);
        destinationsTable!.draw();
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
            clearTimeout(_destSearchDelayTimer);
            _destSearchDelayTimer = setTimeout(searchAndDraw, _destSearchDelayMs);
        }
    );
    searchInput.addEventListener("keydown",
        function (e: KeyboardEvent): void {
            // No shortcuts when ctrl, alt, or meta is being held down
            if (e.altKey || e.ctrlKey || e.metaKey) {
                return;
            }
        }
    );
}


//
// Initialize search plug-in
//

function destInitSearch() {
}


//
// Show rows button handling
//

function destShowRows(rowsToShow: string, replaceState: boolean) {
    const id = rowsToShow;
    _destShowRows = rowsToShow;

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
        destReplaceWindowState();
    }

    destinationsTable!.page.len(ims.parseInt10(rowsToShow));
    destinationsTable!.draw();
}


//
// Update the page URL based on the search input and other filters.
//
function destReplaceWindowState(): void {
    const newParams: [string, string][] = [];

    const searchVal = (document.getElementById("search_input") as HTMLInputElement).value;
    if (searchVal) {
        newParams.push(["q", searchVal]);
    }
    if (_destShowRows != null && _destShowRows !== destDefaultRows) {
        newParams.push(["rows", _destShowRows]);
    }
    const newURL = `${ims.urlReplace(url_viewDestinations)}#${new URLSearchParams(newParams).toString()}`;
    window.history.replaceState(null, "", newURL);
}
