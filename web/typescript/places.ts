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
        destShowType: (typeToShow: string, replaceState: boolean)=>void;
    }
}

let placesTable: ims.DataTablesTable|null = null;

const _destSearchDelayMs = 250;
let _destSearchDelayTimer: number|undefined = undefined;

let _destShowRows: string|null = null;
const destDefaultRows = "25";

let _destShowType: string|null = null;
const destDefaultType = "all";

// The key of the place whose modal is currently open (or which a shared link
// asked for), as it appears in the URL fragment.
let _destShowPlace: string|null = null;

let placeInfoModal: ReturnType<typeof ims.bsModal>|null = null;

//
// Initialize UI
//

const el = {
    searchInput: ims.typedElement("search_input", HTMLInputElement),
    showRowsMenu: ims.typedElement("show_rows", HTMLButtonElement),
    showTypeMenu: ims.typedElement("show_type", HTMLButtonElement),
    placeInfoModal: ims.typedElement("placeInfoModal", HTMLElement),
    placeInfoModalLabel: ims.typedElement("placeInfoModalLabel", HTMLParagraphElement),
    placeBody: ims.typedElement("placeBody", HTMLElement),
    mapLink: ims.typedElement("map-link", HTMLAnchorElement),
    embargoNotice: ims.typedElement("embargo_notice", HTMLDivElement),
    helpModal: ims.typedElement("helpModal", HTMLDivElement),
};

initPlacesPage();



async function initPlacesPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    if (!ims.eventAccess!.readIncidents && !ims.eventAccess!.writeFieldReports) {
        ims.setErrorMessage(
            `You're not currently authorized to view Places in Event "${ims.pathIds.eventName}".`
        );
        ims.hideLoadingOverlay();
        return;
    }

    window.destShowRows = destShowRows;
    window.destShowType = destShowType;

    const eventDatas = await initResult.eventDatas;
    ims.setupMapLink(el.mapLink, eventDatas);
    renderEmbargoNotice(eventDatas, initResult.authInfo.admin);

    ims.disableEditing();
    initPlacesTable();

    const helpModal = ims.bsModal(el.helpModal);

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
            el.searchInput.focus();
        }
    });
    el.helpModal.addEventListener("keydown", function(e: KeyboardEvent): void {
        if (e.key === "?") {
            helpModal.toggle();
            // This is needed to prevent the document's listener for "?" to trigger the modal to
            // toggle back on immediately. This is fallout from the fix for
            // https://github.com/twbs/bootstrap/issues/41005#issuecomment-2497670835
            e.stopPropagation();
        }
    });
}


// Burning Man doesn't publish placement until shortly before the event, so the
// event can embargo camp locations, art locations, and the map link. Tell the
// reader when each of those will show up. Admins are never embargoed, so they're
// told when everyone else will see the data instead.
function renderEmbargoNotice(events: ims.EventData[]|null, isAdmin: boolean): void {
    const event = (events??[]).find(e => e.name === ims.pathIds.eventName);
    if (event == null) {
        return;
    }
    const releases: [string, string, string|null|undefined][] = [
        ["Camp addresses", "are", event.camp_locations_release],
        ["Art addresses", "are", event.art_locations_release],
        ["The event map", "is", event.map_url_release],
    ];
    let anyEmbargoed = false;
    for (const [subject, verb, release] of releases) {
        if (release == null) {
            continue;
        }
        const date = new Date(release);
        if (isNaN(date.getTime()) || date.getTime() <= Date.now()) {
            continue;
        }
        anyEmbargoed = true;
        const when = document.createElement("span");
        when.title = ims.longFormatDate(date);
        when.textContent = ims.formatDateShort(date);
        const line = document.createElement("p");
        line.classList.add("mb-0");
        line.append(
            isAdmin
                ? `${subject} ${verb} hidden from non-admins until `
                : `${subject} will be shown `,
            when,
            ".",
        );
        el.embargoNotice.append(line);
    }
    if (anyEmbargoed) {
        el.embargoNotice.classList.remove("d-none");
    }
}


//
// Dispatch queue table
//

function initPlacesTable() {
    placeInfoModal = ims.bsModal(el.placeInfoModal);

    // A shared link names the place to open. Read it before anything can rewrite
    // the fragment, and drop it again once the reader closes the modal.
    _destShowPlace = ims.windowFragmentParams().get("place");
    el.placeInfoModal.addEventListener("hidden.bs.modal", function(): void {
        _destShowPlace = null;
        destReplaceWindowState();
    });

    destInitDataTables();
    destInitTableButtons();
    destInitSearchField();
    destInitSearch();
    ims.clearErrorMessage();

    placesTable!.on("init", function (): void {
        ims.enableKeyboardSorting("places_table");
    });
}

declare let DataTable: any;

//
// Initialize DataTables
//

function destInitDataTables() {
    DataTable.ext.errMode = "none";
    placesTable = new DataTable("#places_table", {
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
        "ajax": function (_data: unknown, callback: (resp: {data: ims.Place[]})=>void, _settings: unknown): void {
            async function doAjax(): Promise<void> {
                const {json, err} = await ims.fetchNoThrow<ims.Places>(
                    ims.urlReplace(url_places), null,
                );
                if (err != null || json == null) {
                    ims.setErrorMessage(`Failed to load table: ${err}`);
                    return;
                }
                const places: ims.Place[] = [];
                for (const art of json.art??[]) {
                    art.type = "art";
                    art.description = (art.external_data as ims.BMArt).description;
                    places.push(art);
                }
                for (const camp of json.camp??[]) {
                    camp.type = "camp";
                    camp.description = (camp.external_data as ims.BMCamp).description;
                    places.push(camp);
                }
                for (const mv of json.mv??[]) {
                    mv.type = "mv";
                    mv.description = (mv.external_data as ims.BMMV).description;
                    places.push(mv);
                }
                for (const other of json.other??[]) {
                    other.type = "other";
                    other.description = (other.external_data as ims.OtherDest).description??"";
                    places.push(other);
                }
                callback({data: places});
                openLinkedPlace(places);
            }
            doAjax();
        },
        "columns": [
            {   // 0
                "name": "place_name",
                "className": "place_name text-left all",
                "data": "name",
                "cellType": "th",
            },
            {   // 1
                "name": "place_address",
                "className": "place_address text-left",
                "data": "location_string",
            },
            {   // 2
                "name": "place_type",
                "className": "place_type text-left",
                "data": "type",
            },
            {   // 3
                "name": "place_description",
                "className": "place_description text-left",
                "data":  "description",
                "render": renderWithMaxLength(200),
            },
        ],
        "order": [
            [0, "asc"],
        ],

        "createdRow": function (row: HTMLElement, place: ims.Place, _index: number) {
            const openLink = function(_e: MouseEvent): void {
                showPlace(place);
            }
            row.addEventListener("click", openLink);
            row.addEventListener("auxclick", openLink);
        },
    });
}


function showPlace(place: ims.Place): void {
    el.placeInfoModalLabel.textContent = place.name??"(unnamed place)";
    el.placeBody.replaceChildren(placeToHTML(place));
    _destShowPlace = placeKey(place);
    destReplaceWindowState();
    placeInfoModal!.show();
}


// Open the place named by a shared link, once the table data has arrived.
function openLinkedPlace(places: ims.Place[]): void {
    if (_destShowPlace == null) {
        return;
    }
    const place = places.find((p: ims.Place): boolean => placeKey(p) === _destShowPlace);
    if (place == null) {
        ims.setErrorMessage(
            `The linked place isn't in Event "${ims.pathIds.eventName}" anymore.`
        );
        _destShowPlace = null;
        destReplaceWindowState();
        return;
    }
    showPlace(place);
}


// The URL-shareable name for a place. Our own Place IDs churn, since places get
// deleted and reimported wholesale, so key off the Burning Man Salesforce UID
// where there is one and off the type and name where there isn't. Both are too
// long and too ugly to paste into a URL, so carry a hash of them instead: 8
// base36 digits is ~41 bits, which leaves collisions well below one in 100,000
// across an event's few thousand places.
function placeKey(place: ims.Place): string {
    const uid = (place.external_data as {uid?: string|null}|null)?.uid;
    const identity = uid || `${place.type}/${(place.name??"").trim().toLowerCase()}`;
    return hash53(identity).toString(36).padStart(11, "0").slice(-8);
}


// cyrb53, a small non-cryptographic 53-bit string hash. Deliberately not
// crypto.subtle.digest, which is both async and unavailable outside secure
// contexts, and we need neither property here.
function hash53(str: string): number {
    let h1 = 0xdeadbeef;
    let h2 = 0x41c6ce57;
    for (let i = 0; i < str.length; i++) {
        const ch = str.charCodeAt(i);
        h1 = Math.imul(h1 ^ ch, 2654435761);
        h2 = Math.imul(h2 ^ ch, 1597334677);
    }
    h1 = Math.imul(h1 ^ (h1 >>> 16), 2246822507) ^ Math.imul(h2 ^ (h2 >>> 13), 3266489909);
    h2 = Math.imul(h2 ^ (h2 >>> 16), 2246822507) ^ Math.imul(h1 ^ (h1 >>> 13), 3266489909);
    return 4294967296 * (2097151 & h2) + (h1 >>> 0);
}


function placeToHTML(place: ims.Place): Node {
    function setImageDetails(imageDd: HTMLElement, imageURL?: string|null) {
        if (imageURL) {
            if (imageURL.includes("?")) {
                imageURL = imageURL.substring(0, imageURL.indexOf("?"));
            }
            const imageLink = imageDd.querySelector("a")!;
            imageLink.href = imageURL;
        } else {
            imageDd.textContent = "None provided"
        }
    }

    switch (place.type) {
        case "other":
        case "camp": {
            const camp = place.external_data as ims.BMCamp;

            const campTemplate = document.getElementById("camp_template") as HTMLTemplateElement;

            // Clone the new row and insert it into the table
            const campEl = campTemplate.content.cloneNode(true) as DocumentFragment;

            campEl.getElementById("camp_name")!.textContent = camp.name;
            campEl.getElementById("location_label")!.textContent = (camp.location?.intersection_type)
                ? (` - frontage ${camp.location?.intersection_type} intersection`)
                : "";
            campEl.getElementById("location_string")!.textContent =
                `${camp.location_string ?? "Unknown"}\n` +
                `${camp.location?.exact_location ?? ""}\n` +
                `${camp.location?.dimensions ?? ""}`;
            campEl.getElementById("description")!.textContent = camp.description ?? "None provided";
            campEl.getElementById("landmark")!.textContent = camp.landmark ?? "None provided";
            let imageURL = camp.images?.find((value: object): boolean => {
                return "thumbnail_url" in value;
            })?.thumbnail_url;
            setImageDetails(campEl.getElementById("image_dd")!, imageURL);
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
            const art = place.external_data as ims.BMArt;

            const template = document.getElementById("art_template") as HTMLTemplateElement;

            // Clone the new row and insert it into the table
            const artEl = template.content.cloneNode(true) as DocumentFragment;

            artEl.getElementById("art_name")!.textContent = art.name;
            const lat = art.location?.gps_latitude?.toFixed(6) ?? "Unknown";
            const long = art.location?.gps_longitude?.toFixed(6) ?? "Unknown";
            artEl.getElementById("location_string")!.textContent =
                `${art.location_string ?? "Unknown"}\n` +
                `${lat},${long}`;
            artEl.getElementById("description")!.textContent = art.description ?? "None provided";
            artEl.getElementById("artist")!.textContent = art.artist ?? "None provided";
            let imageURL = art.images?.find((value: object): boolean => {
                return "thumbnail_url" in value;
            })?.thumbnail_url;
            setImageDetails(artEl.getElementById("image_dd")!, imageURL);
            if (art.contact_email) {
                const emailLink = artEl.getElementById("email_link") as HTMLAnchorElement;
                emailLink.href = `mailto:${art.contact_email}`;
                emailLink.textContent = art.contact_email;
            } else {
                artEl.getElementById("email_dd")!.textContent = "None provided";
            }
            if (art.url) {
                const websiteLink = artEl.getElementById("website_url") as HTMLAnchorElement;
                websiteLink.href = art.url;
                websiteLink.textContent = art.url;
            } else {
                artEl.getElementById("website_dd")!.textContent = "None provided";
            }
            artEl.getElementById("hometown")!.textContent = art.hometown ?? "None provided";
            artEl.getElementById("uid")!.textContent = art.uid ?? "None";
            return artEl;
        }
        case "mv": {
            const mv = place.external_data as ims.BMMV;

            const template = document.getElementById("mv_template") as HTMLTemplateElement;

            // Clone the new row and insert it into the table
            const mvEl = template.content.cloneNode(true) as DocumentFragment;

            mvEl.getElementById("mv_name")!.textContent = mv.name;
            mvEl.getElementById("description")!.textContent = mv.description ?? "None provided";
            mvEl.getElementById("artist")!.textContent = mv.artist ?? "None provided";
            let imageURL = mv.images?.find((value: object): boolean => {
                return "thumbnail_url" in value;
            })?.thumbnail_url;
            setImageDetails(mvEl.getElementById("image_dd")!, imageURL);
            if (mv.contact_email) {
                const emailLink = mvEl.getElementById("email_link") as HTMLAnchorElement;
                emailLink.href = `mailto:${mv.contact_email}`;
                emailLink.textContent = mv.contact_email;
            } else {
                mvEl.getElementById("email_dd")!.textContent = "None provided";
            }
            if (mv.url) {
                const websiteLink = mvEl.getElementById("website_url") as HTMLAnchorElement;
                websiteLink.href = mv.url;
                websiteLink.textContent = mv.url;
            } else {
                mvEl.getElementById("website_dd")!.textContent = "None provided";
            }
            mvEl.getElementById("hometown")!.textContent = mv.hometown ?? "None provided";
            mvEl.getElementById("uid")!.textContent = mv.uid ?? "None";
            mvEl.getElementById("tags")!.textContent = (mv.tags??[]).join(", ");
            return mvEl;
        }
        default:
            throw new Error("Found no place type");
    }
}

function renderWithMaxLength(maxLength: number): (data: (string | null), type: string, _dest: ims.Place) => (string | undefined) {
    return function (data: string|null, type: string, _dest: ims.Place): string|undefined {
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

    destShowType(fragmentParams.get("type") ?? destDefaultType, false);
}


//
// Initialize search field
//

function destInitSearchField(): void {
    // Search field handling
    function searchAndDraw(): void {
        destReplaceWindowState();
        let q = el.searchInput.value;
        let isRegex = false;
        let smartSearch = true;
        if (q.startsWith("/") && q.endsWith("/")) {
            isRegex = true;
            smartSearch = false;
            q = q.slice(1, q.length-1);
        }
        placesTable!.search(q, isRegex, smartSearch);
        placesTable!.draw();
    }

    const fragmentParams: URLSearchParams = ims.windowFragmentParams();
    const queryString: string|null = fragmentParams.get("q");
    if (queryString) {
        el.searchInput.value = queryString;
        searchAndDraw();
    }

    el.searchInput.addEventListener("input",
        function (_: Event): void {
            // Delay the search in case the user is still typing.
            // This reduces perceived lag, since searching can be
            // very slow, and it's super annoying for a user when
            // the page fully locks up before they're done typing.
            clearTimeout(_destSearchDelayTimer);
            _destSearchDelayTimer = window.setTimeout(searchAndDraw, _destSearchDelayMs);
        }
    );
    el.searchInput.addEventListener("keydown",
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
    placesTable!.search.fixed("type", function(_searchStr: string, _rowData: object, rowIndex: number): boolean {
        if (_destShowType == null || _destShowType === "all") {
            return true;
        }
        const place: ims.Place = placesTable!.data()[rowIndex]!;
        return place.type === _destShowType;
    });
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
    el.showRowsMenu.getElementsByClassName("selection")[0]!.textContent = selection

    if (rowsToShow === "all") {
        rowsToShow = "-1";
    }

    if (replaceState) {
        destReplaceWindowState();
    }

    placesTable!.page.len(ims.parseInt10(rowsToShow));
    placesTable!.draw();
}


//
// Show type button handling
//

function destShowType(typeToShow: string, replaceState: boolean) {
    _destShowType = typeToShow;

    const item = document.getElementById("show_type_" + typeToShow) as HTMLLIElement;

    // Get title from selected item
    const selection = item.getElementsByClassName("name")[0]!.textContent;

    // Update menu title to reflect selected item
    el.showTypeMenu.getElementsByClassName("selection")[0]!.textContent = selection;

    if (replaceState) {
        destReplaceWindowState();
    }

    placesTable!.draw();
}


//
// Update the page URL based on the search input and other filters.
//
function destReplaceWindowState(): void {
    const newParams: [string, string][] = [];

    const searchVal = el.searchInput.value;
    if (searchVal) {
        newParams.push(["q", searchVal]);
    }
    if (_destShowRows != null && _destShowRows !== destDefaultRows) {
        newParams.push(["rows", _destShowRows]);
    }
    if (_destShowType != null && _destShowType !== destDefaultType) {
        newParams.push(["type", _destShowType]);
    }
    if (_destShowPlace != null) {
        newParams.push(["place", _destShowPlace]);
    }
    const newURL = `${ims.urlReplace(url_viewPlaces)}#${new URLSearchParams(newParams).toString()}`;
    window.history.replaceState(null, "", newURL);
}
