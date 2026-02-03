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
import {fetchPersonnel} from "./ims.ts";

declare global {
    interface Window {
        editState: ()=>Promise<void>;
        editIncidentSummary: ()=>Promise<void>;
        editLocationName: ()=>Promise<void>;
        editLocationAddress: ()=>Promise<void>;
        editLocationDescription: ()=>Promise<void>;
        removeRanger: (el: HTMLElement)=>void;
        setRangerRole: (el: HTMLInputElement)=>void;
        removeIncidentType: (el: HTMLElement)=>Promise<void>;
        detachFieldReport: (el: HTMLElement)=>Promise<void>;
        attachFieldReport: ()=>Promise<void>;
        unlinkIncident: (el: HTMLElement)=>Promise<void>;
        linkIncident: (el: HTMLInputElement)=>Promise<void>;
        addRanger: ()=>void;
        addIncidentType: ()=>Promise<void>;
        attachFile: ()=>void;
        drawMergedReportEntries: ()=>void;
        toggleShowHistory: ()=>void;
        reportEntryEdited: ()=>void;
        submitReportEntry: ()=>void;
    }
}

let incident: ims.Incident|null = null;

let allIncidentTypes: ims.IncidentType[] = [];

let allEvents: ims.EventData[]|null = null;

let destinations: ims.Destinations = {};

//
// Initialize UI
//

const el = {
    incidentNumber: ims.typedElement("incident_number", HTMLInputElement),
    incidentSummary: ims.typedElement("incident_summary", HTMLInputElement),
    incidentState: ims.typedElement("incident_state", HTMLSelectElement),
    startedDatetime: ims.typedElement("started_datetime", HTMLInputElement) as ims.FlatpickrHTMLInputElement,
    startedDatetimeTz: ims.typedElement("started_datetime_tz", HTMLSpanElement),

    locationName: ims.typedElement("incident_location_name", HTMLInputElement),
    locationAddress: ims.typedElement("incident_location_address", HTMLInputElement),
    locationDescription: ims.typedElement("incident_location_description", HTMLInputElement),

    rangerAdd: ims.typedElement("ranger_add", HTMLInputElement),
    rangerHandles: ims.typedElement("ranger_handles", HTMLDataListElement),
    rangersList: ims.typedElement("incident_rangers_list", HTMLElement),
    rangersLiTemplate: ims.typedElement("incident_rangers_li_template", HTMLTemplateElement),

    incidentTypeAdd: ims.typedElement("incident_type_add", HTMLInputElement),
    incidentTypes: ims.typedElement("incident_types", HTMLDataListElement),
    incidentTypesList: ims.typedElement("incident_types_list", HTMLElement),
    incidentTypesLiTemplate: ims.typedElement("incident_types_li_template", HTMLTemplateElement),
    incidentTypeInfo: ims.typedElement("incident-type-info", HTMLUListElement),
    incidentTypeInfoTemplate: ims.typedElement("incident-type-info-template", HTMLTemplateElement),
    showIncidentTypeInfo: ims.typedElement("show-incident-type-info", HTMLElement),

    destinationsList: ims.typedElement("destinations-list", HTMLDataListElement),

    attachedFieldReportAddContainer: ims.typedElement("attached_field_report_add_container", HTMLDivElement),
    attachedFieldReportAdd: ims.typedElement("attached_field_report_add", HTMLSelectElement),
    attachedFieldReports: ims.typedElement("attached_field_reports", HTMLElement),

    linkedIncidents: ims.typedElement("linked_incidents", HTMLElement),

    historyCheckbox: ims.typedElement("history_checkbox", HTMLInputElement),
    reportEntryAdd: ims.typedElement("report_entry_add", HTMLTextAreaElement),
    reportEntrySubmit: ims.typedElement("report_entry_submit", HTMLElement),
    attachFile: ims.typedElement("attach_file", HTMLInputElement),
    attachFileInput: ims.typedElement("attach_file_input", HTMLInputElement),

    helpModal: ims.typedElement("helpModal", HTMLDivElement),
};

initIncidentPage();

async function initIncidentPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    if (!ims.eventAccess!.readIncidents) {
        ims.setErrorMessage(
            `You're not currently authorized to view Incidents in Event "${ims.pathIds.eventName}".`
        );
        ims.hideLoadingOverlay();
        return;
    }

    window.editState = editState;
    window.editIncidentSummary = editIncidentSummary;
    window.editLocationName = editLocationName;
    window.editLocationAddress = editLocationAddress;
    window.editLocationDescription = editLocationDescription;
    window.removeRanger = removeRanger;
    window.setRangerRole = setRangerRole;
    window.removeIncidentType = removeIncidentType;
    window.detachFieldReport = detachFieldReport;
    window.attachFieldReport = attachFieldReport;
    window.unlinkIncident = unlinkIncident;
    window.linkIncident = linkIncident;
    window.addRanger = addRanger;
    window.addIncidentType = addIncidentType;
    window.attachFile = attachFile;
    window.drawMergedReportEntries = drawMergedReportEntries;
    window.toggleShowHistory = ims.toggleShowHistory;
    window.reportEntryEdited= ims.reportEntryEdited;
    window.submitReportEntry = ims.submitReportEntry;

    // load everything from the APIs concurrently
    await Promise.all([
        await ims.loadStreets(ims.pathIds.eventId),
        await loadIncident(),
        await loadPersonnel(),
        await ims.loadIncidentTypes().then(
            value=> {
                allIncidentTypes = value.types;
            },
        ),
        await loadDestinations(),
        await loadAllStays(),
        await loadAllFieldReports(),
    ]);

    allEvents = await initResult.eventDatas;

    ims.newFlatpickr("#started_datetime", "alt_started_datetime", setStartDatetime);

    ims.disableEditing();
    displayIncident();
    if (incident == null) {
        return;
    }
    drawRangers();
    drawRangersToAdd();
    drawIncidentTypesToAdd();
    drawIncidentTypeInfo();
    drawDestinationsList();
    renderFieldReportData();

    ims.hideLoadingOverlay();

    // for a new incident, jump to summary field
    if (incident!.number == null) {
        el.incidentSummary.focus();
    }

    // Warn the user if they're about to navigate away with unsaved text.
    window.addEventListener("beforeunload", function (e: BeforeUnloadEvent): void {
        if (el.reportEntryAdd.value !== "") {
            e.preventDefault();
        }
    });

    ims.requestEventSourceLock();

    ims.newIncidentChannel().onmessage = async function (e: MessageEvent<ims.IncidentBroadcast>): Promise<void> {
        const number = e.data.incident_number;
        const eventId = e.data.event_id;
        const updateAll = e.data.update_all??false;

        if (updateAll || (eventId === ims.pathIds.eventId && number === ims.pathIds.incidentNumber)) {
            console.log("Got incident update: " + number);
            await loadAndDisplayIncident();
            await loadAllStays();
            await loadAllFieldReports();
            renderFieldReportData();
        }
    };

    ims.newFieldReportChannel().onmessage = async function (e: MessageEvent<ims.FieldReportBroadcast>): Promise<void> {
        const updateAll = e.data.update_all??false;
        if (updateAll) {
            console.log("Updating all field reports");
            await loadAllFieldReports();
            renderFieldReportData();
            return;
        }

        const number = e.data.field_report_number;
        const eventId = e.data.event_id;
        if (eventId === ims.pathIds.eventId) {
            console.log("Got field report update: " + number);
            await loadOneFieldReport(number!);
            renderFieldReportData();
            return;
        }
    };

    ims.newStayChannel().onmessage = async function (e: MessageEvent<ims.StayBroadcast>): Promise<void> {
        const updateAll = e.data.update_all??false;
        if (updateAll) {
            console.log("Updating all stays");
            await loadAllStays();
            renderFieldReportData();
            return;
        }

        const number = e.data.stay_number;
        const eventId = e.data.event_id;
        if (eventId === ims.pathIds.eventId) {
            console.log("Got stay update: " + number);
            await loadOneStay(number!);
            renderFieldReportData();
            return;
        }
    }

    const helpModal = ims.bsModal(el.helpModal);

    const incidentTypeInfoModal = ims.bsModal(document.getElementById("incidentTypeInfoModal")!);

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
        // a --> jump to add a new report entry
        if (e.key === "a") {
            e.preventDefault();
            // Scroll to report_entry_add field
            el.reportEntryAdd.focus();
            el.reportEntryAdd.scrollIntoView(true);
        }
        // h --> toggle showing system entries
        if (e.key.toLowerCase() === "h") {
            el.historyCheckbox.click();
        }
        // n --> new incident
        if (e.key.toLowerCase() === "n") {
            (window.open("./new", '_blank') as Window).focus();
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
    el.reportEntryAdd.addEventListener("keydown", function (e: KeyboardEvent): void {
        const submitEnabled = !el.reportEntrySubmit.classList.contains("disabled");
        if (submitEnabled && (e.ctrlKey || e.altKey) && e.key === "Enter") {
            ims.submitReportEntry();
        }
    });
    el.showIncidentTypeInfo.addEventListener(
        "click",
        function (e: MouseEvent): void {
            e.preventDefault();
            incidentTypeInfoModal.show();
        },
    );

    window.addEventListener("beforeprint", (_event: Event): void => {
        drawIncidentTitle("for_print_to_pdf");
    });
    window.addEventListener("afterprint", (_event: Event): void => {
        drawIncidentTitle("for_display");
    });
}


//
// Load incident
//

async function loadIncident(): Promise<{err: string|null}> {
    let number: number|null;
    if (incident == null) {
        // First time here.  Use page JavaScript initial value.
        number = ims.pathIds.incidentNumber??null;
    } else {
        // We have an incident already.  Use that number.
        number = incident.number!;
    }

    if (number == null) {
        incident = {
            "number": null,
            "state": "new",
            "priority": 3,
            "summary": "",
        };
    } else {
        const {json, err} = await ims.fetchNoThrow<ims.Incident>(
            `${ims.urlReplace(url_incidents)}/${number}`, null);
        if (err != null) {
            ims.disableEditing();
            const message = `Failed to load Incident ${number}: ${err}`;
            console.error(message);
            ims.setErrorMessage(message);
            return {err: message};
        }
        incident = json;
    }
    return {err: null};
}

async function loadAndDisplayIncident(): Promise<void> {
    await loadIncident();
    displayIncident();
}

function displayIncident(): void {
    if (incident == null) {
        const message = "Incident failed to load";
        console.log(message);
        ims.setErrorMessage(message);
        return;
    }

    drawIncidentFields();
    ims.clearErrorMessage();

    if (ims.eventAccess?.writeIncidents) {
        ims.enableEditing();
    }

    if (ims.eventAccess?.attachFiles) {
        el.attachFile.classList.remove("hidden");
    }
}

// Do all the client-side rendering based on the state of allFieldReports.
function renderFieldReportData(): void {
    loadAttachedFieldReports();
    loadAttachedStays();
    drawFieldReportsToAttach();
    drawMergedReportEntries();
    drawAttachedFieldReports();
    drawLinkedIncidents();
}


//
// Load personnel
//

let personnel: ims.PersonnelMap|null = null;

async function loadPersonnel(): Promise<void> {
    const res = await fetchPersonnel();
    if (res.err != null || res.personnel == null) {
        ims.setErrorMessage(res.err??"");
    }
    personnel = res.personnel;
}

//
// Load all field reports and stays
//

let allFieldReports: ims.FieldReport[]|null|undefined = null;

async function loadAllFieldReports(): Promise<{err: string|null}> {
    if (allFieldReports === undefined) {
        return {err: null};
    }

    const {resp, json, err} = await ims.fetchNoThrow<ims.FieldReport[]>(ims.urlReplace(url_fieldReports), null);
    if (err != null) {
        if (resp != null && resp.status === 403) {
            // We're not allowed to look these up.
            allFieldReports = undefined;
            console.error("Got a 403 looking up field reports");
            return {err: null};
        } else {
            const message = `Failed to load field reports: ${err}`;
            console.error(message);
            ims.setErrorMessage(message);
            return {err: message};
        }
    }
    const _allFieldReports: ims.FieldReport[] = [];
    for (const d of json!) {
        _allFieldReports.push(d);
    }
    // apply a descending sort based on the field report number,
    // being cautious about field report number being null
    _allFieldReports.sort(function (a, b) {
        return (b.number ?? -1) - (a.number ?? -1);
    });
    allFieldReports = _allFieldReports;
    return {err: null};
}

async function loadOneFieldReport(fieldReportNumber: number): Promise<{err: string|null}> {
    if (allFieldReports === undefined) {
        return {err: null};
    }

    const {resp, json, err} = await ims.fetchNoThrow<ims.FieldReport>(
        ims.urlReplace(url_fieldReport).replace("<field_report_number>", fieldReportNumber.toString()), null);
    if (err != null) {
        if (resp == null || resp.status !== 403) {
            const message = `Failed to load field report ${fieldReportNumber} ${err}`;
            console.error(message);
            ims.setErrorMessage(message);
            return {err: message};
        }
    }

    let found = false;
    for (const i in allFieldReports!) {
        if (allFieldReports[i]!.number === json!.number) {
            allFieldReports[i] = json!;
            found = true;
        }
    }
    if (!found) {
        if (allFieldReports == null) {
            allFieldReports = [];
        }
        allFieldReports.push(json!);
        // apply a descending sort based on the field report number,
        // being cautious about field report number being null
        allFieldReports.sort(function (a, b) {
            return (b.number ?? -1) - (a.number ?? -1);
        });
    }

    return {err: null};
}

let allStays: ims.Stay[]|null|undefined = null;

async function loadAllStays(): Promise<{err: string|null}> {
    if (allStays === undefined) {
        return {err: null};
    }

    const {resp, json, err} = await ims.fetchNoThrow<ims.Stay[]>(ims.urlReplace(url_stays), null);
    if (err != null) {
        if (resp != null && resp.status === 403) {
            // We're not allowed to look these up.
            allFieldReports = undefined;
            console.error("Got a 403 looking up stays");
            return {err: null};
        } else {
            const message = `Failed to load stays: ${err}`;
            console.error(message);
            ims.setErrorMessage(message);
            return {err: message};
        }
    }
    const stays: ims.Stay[] = [];
    for (const d of json!) {
        stays.push(d);
    }
    // apply a descending sort based on the stay number,
    // being cautious about field report number being null
    stays.sort(function (a, b) {
        return (b.number ?? -1) - (a.number ?? -1);
    });
    allStays = stays;
    return {err: null};
}

async function loadOneStay(stayNumber: number): Promise<{err: string|null}> {
    if (allStays === undefined) {
        return {err: null};
    }

    const {resp, json, err} = await ims.fetchNoThrow<ims.Stay>(
        ims.urlReplace(url_stayNumber).replace("<stay_number>", stayNumber.toString()), null);
    if (err != null) {
        if (resp == null || resp.status !== 403) {
            const message = `Failed to load stay ${stayNumber} ${err}`;
            console.error(message);
            ims.setErrorMessage(message);
            return {err: message};
        }
    }

    let found = false;
    for (const i in allStays!) {
        if (allStays[i]!.number === json!.number) {
            allStays[i] = json!;
            found = true;
        }
    }
    if (!found) {
        if (allStays == null) {
            allStays = [];
        }
        allStays.push(json!);
        // apply a descending sort based on the stay number,
        // being cautious about field report number being null
        allStays.sort(function (a, b) {
            return (b.number ?? -1) - (a.number ?? -1);
        });
    }

    return {err: null};
}



//
// Load attached field reports and stays
//

let attachedFieldReports: ims.FieldReport[]|null = null;

function loadAttachedFieldReports() {
    if (ims.pathIds.incidentNumber == null) {
        return;
    }
    const _attachedFieldReports: ims.FieldReport[] = [];
    for (const fr of allFieldReports??[]) {
        if (fr.incident === ims.pathIds.incidentNumber) {
            _attachedFieldReports.push(fr);
        }
    }
    attachedFieldReports = _attachedFieldReports;
}

let attachedStays: ims.Stay[]|null = null;

function loadAttachedStays() {
    if (ims.pathIds.incidentNumber == null) {
        return;
    }
    const newAttachedStays: ims.Stay[] = [];
    for (const s of allStays??[]) {
        if (s.incident === ims.pathIds.incidentNumber) {
            newAttachedStays.push(s);
        }
    }
    attachedStays = newAttachedStays;
}


//
// Draw all fields
//

function drawIncidentFields() {
    drawIncidentTitle("for_display");
    drawIncidentNumber();
    drawState();
    drawStarted();
    drawPriority();
    drawIncidentSummary();
    drawRangers();
    drawIncidentTypes();
    drawLocationName();
    drawLocationAddress();
    drawLocationDescription();
    ims.toggleShowHistory();
    drawMergedReportEntries();

    el.reportEntryAdd.addEventListener("input", ims.reportEntryEdited);
}


//
// Populate page title
//

function drawIncidentTitle(mode: "for_display"|"for_print_to_pdf"): void {
    let newTitle: string = "";
    if (mode === "for_print_to_pdf" && incident?.number) {
        const fsSafeDescription: string = ims.summarizeIncidentOrFR(incident)
            .replaceAll("#", "-")
            .replaceAll("\n", "-")
            .replaceAll(" ", "-")
            .replaceAll(":", "-")
            .replaceAll(";", "-")
            .replaceAll("!", "-")
            .replaceAll("$", "")
            .replace(/^-+/, "")
            .replace(/-+$/, "");
        newTitle = `IMS-${ims.pathIds.eventName}-${incident.number}_${fsSafeDescription}`
    } else {
        const eventSuffix: string = ims.pathIds.eventName != null ? ` | ${ims.pathIds.eventName}` : "";
        newTitle = `${ims.incidentAsString(incident!)}${eventSuffix}`;
    }
    document.title = newTitle;
}


//
// Populate incident number
//

function drawIncidentNumber(): void {
    const number: number|string = incident!.number??"(new)";
    el.incidentNumber.value = number.toString();
}


//
// Populate incident state
//

function drawState(): void {
    ims.selectOptionWithValue(
        el.incidentState,
        ims.stateForIncident(incident!)
    );
}


//
// Populate started datetime
//

function drawStarted(): void {
    const date: string|null = incident!.started??null;
    if (date == null) {
        return;
    }
    const dateNum: number = Date.parse(date);
    const dateDate: Date = new Date(dateNum);
    el.startedDatetime._flatpickr.setDate(date, false, "Z");

    el.startedDatetimeTz.textContent = ims.localTzShortName(dateDate);
    el.startedDatetimeTz.title = `${Intl.DateTimeFormat().resolvedOptions().timeZone}\n\n` +
        `All date and time fields in IMS use your computer's time zone, not necessarily Gerlach time.`;
}

//
// Populate incident priority
//

function drawPriority(): void {
    const priorityElement = document.getElementById("incident_priority");
    // priority is currently hidden from the incident page, so we should expect this early return
    if (priorityElement == null) {
        return;
    }
    ims.selectOptionWithValue(
        priorityElement as HTMLSelectElement,
        (incident!.priority??"").toString(),
    )
}


//
// Populate incident summary
//

function drawIncidentSummary(): void {
    el.incidentSummary.placeholder = "One-line summary of incident";
    if (incident!.summary) {
        el.incidentSummary.value = incident!.summary;
        el.incidentSummary.placeholder = "";
        return;
    }

    el.incidentSummary.value = ims.summarizeIncidentOrFR(incident!);
}


//
// Populate Rangers list
//

function drawRangers() {
    const rangers: ims.IncidentRanger[] = incident?.rangers??[];
    rangers.sort((a: ims.IncidentRanger, b: ims.IncidentRanger) => (a.handle??"").localeCompare(b.handle??""));

    el.rangersList.querySelectorAll("li").forEach((li: HTMLElement) => {li.remove()});

    for (const ranger of rangers) {
        if (!ranger.handle) {
            continue;
        }
        const handle = ranger.handle;

        const rangerFragment = el.rangersLiTemplate.content.cloneNode(true) as DocumentFragment;
        const rangerLi = rangerFragment.querySelector("li")!;
        rangerLi.classList.remove("hidden");
        rangerLi.dataset["rangerHandle"] = handle;

        const rangerName =  rangerLi.querySelector("span")!
        if (personnel?.[handle] == null) {
            rangerName.textContent = handle;
        } else {
            const person = personnel[handle];
            const rangerLink = rangerName.querySelector("a")!;
            rangerLink.textContent = person.handle;
            if (person.directory_id != null) {
                rangerLink.href = `${ims.clubhousePersonURL}/${person.directory_id}`;
                rangerLink.target = "_blank";
            }
        }
        const roleInput = rangerLi.querySelector("input")!;
        roleInput.ariaLabel = `Ranger role for ${handle}`;
        if (ranger.role) {
            rangerLi.querySelector("input")!.value = ranger.role;
        }

        el.rangersList.append(rangerFragment);
    }
}


function drawRangersToAdd(): void {
    const handles: string[] = [];
    for (const handle in personnel) {
        handles.push(handle);
    }
    handles.sort((a: string, b: string) => a.localeCompare(b));

    el.rangerHandles.replaceChildren();
    el.rangerHandles.append(document.createElement("option"));

    if (personnel != null) {
        for (const handle of handles) {
            const ranger = personnel[handle];
            if (ranger === undefined) {
                console.error(`no record for personnel with handle ${handle}`);
                continue;
            }

            const option: HTMLOptionElement = document.createElement("option");
            option.value = handle;
            option.text = ranger.handle;

            el.rangerHandles.append(option);
        }
    }
}


//
// Populate incident types list
//

function drawIncidentTypes() {
    el.incidentTypesList.querySelectorAll("li").forEach((li: HTMLElement) => {li.remove()});

    for (const validType of allIncidentTypes) {
        if ((incident!.incident_type_ids??[]).includes(validType.id??-1)) {
            const fragment = el.incidentTypesLiTemplate.content.cloneNode(true) as DocumentFragment;
            const item = fragment.querySelector("li")!;
            item.classList.remove("hidden");
            const typeSpan = document.createElement("span");
            typeSpan.textContent = validType.name??"";
            item.append(typeSpan);
            item.dataset["incidentTypeId"] = (validType.id??-1).toString();
            el.incidentTypesList.append(fragment);
        }
    }
}


function drawIncidentTypesToAdd() {
    el.incidentTypes.replaceChildren();
    el.incidentTypes.append(document.createElement("option"));
    for (const incidentType of allIncidentTypes) {
        if (incidentType.hidden || !incidentType.name) {
            continue;
        }
        const option: HTMLOptionElement = document.createElement("option");
        option.value = incidentType.name;
        el.incidentTypes.append(option);
    }
}

function drawIncidentTypeInfo(): void {
    for (const incidentType of allIncidentTypes) {
        if (incidentType.hidden) {
            continue;
        }
        const frag = el.incidentTypeInfoTemplate.content.cloneNode(true) as DocumentFragment;
        frag.querySelector(".type-name")!.textContent = incidentType.name??"";
        frag.querySelector(".type-description")!.textContent = incidentType.description??"";
        el.incidentTypeInfo.append(frag);
    }
}


//
// Populate location
//

function drawLocationName() {
    if (incident?.location?.name) {
        el.locationName.value = incident.location.name;
    }
}

async function loadDestinations(): Promise<void> {
    const {json, err} = await ims.fetchNoThrow<ims.Destinations>(
       `${ims.urlReplace(url_destinations)}?exclude_external_data=true`,
        null,
    );
    if (err != null || json == null) {
        const message = `Failed to load destinations: ${err}`;
        console.error(message);
        ims.setErrorMessage(message);
        return;
    }
    destinations = json;
}

function drawDestinationsList(): void {
    el.destinationsList.replaceChildren();
    el.destinationsList.append(document.createElement("option"));

    const newOptions: HTMLOptionElement[] = [];
    for (const d of destinations.art??[]) {
        const option: HTMLOptionElement = document.createElement("option");
        option.value = `${d.name} (Art) (${d.location_string})`;
        option.dataset["name"] = d.name??"";
        option.dataset["address"] = d.location_string??"";
        option.dataset["type"] = "Art";
        newOptions.push(option);
    }
    for (const d of destinations.camp??[]) {
        const option: HTMLOptionElement = document.createElement("option");
        option.value = `${d.name} (${d.location_string})`;
        option.dataset["name"] = d.name??"";
        option.dataset["address"] = d.location_string??"";
        option.dataset["type"] = "Camp";
        newOptions.push(option);
    }
    for (const d of destinations.other??[]) {
        const option: HTMLOptionElement = document.createElement("option");
        option.value = `${d.name} (${d.location_string})`;
        option.dataset["name"] = d.name??"";
        option.dataset["address"] = d.location_string??"";
        option.dataset["type"] = "Other";
        newOptions.push(option);
    }
    newOptions.sort((a: HTMLOptionElement, b: HTMLOptionElement): number => a.value.localeCompare(b.value));
    el.destinationsList.append(...newOptions);
}

function drawLocationAddress() {
    if (!incident || !incident.location) {
        el.locationAddress.value = "";
        return;
    }
    el.locationAddress.value = incident.location.address??"";
}

function drawLocationDescription() {
    if (incident!.location?.description) {
        el.locationDescription.value = incident!.location.description;
    }
}


//
// Draw report entries
//

function drawMergedReportEntries(): void {
    const entries: ims.ReportEntry[] = (incident!.report_entries??[]).slice()

    for (const report of (attachedFieldReports??[])) {
        for (const entry of report.report_entries??[]) {
            entry.frNum = report.number??null;
            entries.push(entry);
        }
    }

    for (const stay of (attachedStays??[])) {
        for (const entry of stay.report_entries??[]) {
            entry.stayNum = stay.number??null;
            entries.push(entry);
        }
    }

    entries.sort(ims.compareReportEntries);

    ims.drawReportEntries(entries);
}


let _reportsItem: HTMLElement|null = null;

function drawAttachedFieldReports() {
    if (_reportsItem == null) {
         const elements = el.attachedFieldReports.getElementsByClassName("list-group-item");
        if (elements.length === 0) {
            console.error("found no reportsItem");
            return;
        }
        _reportsItem = elements[0] as HTMLElement;
    }

    const reports = attachedFieldReports??[];
    reports.sort();

    el.attachedFieldReports.replaceChildren();

    for (const report of reports) {
        const link: HTMLAnchorElement = document.createElement("a");
        link.href = `${ims.urlReplace(url_viewFieldReports)}/${report.number}`;
        link.innerText = ims.fieldReportAsString(report);

        const item = _reportsItem.cloneNode(true) as HTMLElement;
        item.classList.remove("hidden");
        item.append(link);
        item.dataset["frNumber"] = report.number!.toString();

        el.attachedFieldReports.append(item);
    }
}

let _linkedIncidentsItem: HTMLElement|null = null;

function drawLinkedIncidents(): void {
    if (_linkedIncidentsItem == null) {
        const elements = el.linkedIncidents.getElementsByClassName("list-group-item");
        if (elements.length === 0) {
            console.error("found no linkedIncidents");
            return;
        }
        _linkedIncidentsItem = elements[0] as HTMLElement;
    }

    const linkedIncidents = incident!.linked_incidents??[];
    linkedIncidents.sort(function (a: ims.LinkedIncident, b: ims.LinkedIncident): number {
        if ((b.event_name??"") === (a.event_name??"")) {
            return (a.number || 0) - (b.number || 0);
        }
        return (a.event_name??"").localeCompare(b.event_name??"");
    });

    el.linkedIncidents.replaceChildren();

    for (const linked of linkedIncidents) {
        const link: HTMLAnchorElement = document.createElement("a");

        link.href = url_viewIncidentNumber
            .replace("<event_id>", linked.event_name??"")
            .replace("<number>", linked.number?.toString()??"");

        let summary: string = ""
        if (linked.summary) {
            summary = `: ${linked.summary}`;
        }

        link.innerText = `IMS ${linked.event_name??""} #${linked.number}${summary}`;

        const item = _linkedIncidentsItem.cloneNode(true) as HTMLElement;
        item.classList.remove("hidden");
        item.append(link);
        item.dataset["eventId"] = linked.event_id?.toString();
        item.dataset["eventName"] = linked.event_name?.toString();
        item.dataset["incidentNumber"] = linked.number?.toString();

        el.linkedIncidents.append(item);
    }
}


function drawFieldReportsToAttach() {
    el.attachedFieldReportAdd.replaceChildren();
    el.attachedFieldReportAdd.append(document.createElement("option"));

    if (!allFieldReports) {
        el.attachedFieldReportAddContainer.classList.add("hidden");
    } else {
        const unattachedGroup: HTMLOptGroupElement = document.createElement("optgroup");
        unattachedGroup.label = "Unattached to any incident";
        el.attachedFieldReportAdd.append(unattachedGroup);
        for (const report of allFieldReports) {
            // Skip field reports that *are* attached to an incident
            if (report.incident != null) {
                continue;
            }
            const option: HTMLOptionElement = document.createElement("option");
            option.value = report.number!.toString();
            option.text = ims.fieldReportAsString(report);
            el.attachedFieldReportAdd.append(option);
        }
        const attachedGroup: HTMLOptGroupElement = document.createElement("optgroup");
        attachedGroup.label = "Attached to another incident";
        el.attachedFieldReportAdd.append(attachedGroup);
        for (const report of allFieldReports) {
            // Skip field reports that *are not* attached to an incident
            if (report.incident == null) {
                continue;
            }
            // Skip field reports that are already attached this incident
            if (report.incident === ims.pathIds.incidentNumber) {
                continue;
            }
            const option: HTMLOptionElement = document.createElement("option");
            option.value = report.number!.toString();
            option.text = ims.fieldReportAsString(report);
            el.attachedFieldReportAdd.append(option);
        }
        el.attachedFieldReportAdd.append(document.createElement("optgroup"));

        el.attachedFieldReportAddContainer.classList.remove("hidden");
    }
}


//
// Editing
//

async function sendEdits(edits: ims.Incident): Promise<{err:string|null}> {
    const number = incident!.number;
    let url = ims.urlReplace(url_incidents);

    if (number == null) {
        // We're creating a new incident.
        // required fields are ["state", "priority"];
        if (edits.state == null) {
            edits.state = incident!.state??null;
        }
        if (edits.priority == null) {
            edits.priority = incident!.priority??null;
        }
    } else {
        // We're editing an existing incident.
        edits.number = number;
        url += `/${number}`;
    }

    const {resp, err} = await ims.fetchNoThrow(url, {
        body: JSON.stringify(edits),
    });

    if (err != null) {
        const message = `Failed to apply edit: ${err}`;
        await loadAndDisplayIncident();
        ims.setErrorMessage(message);
        return {err: message};
    }

    if (number == null && resp != null) {
        // We created a new incident.
        // We need to find out the created incident number so that future
        // edits don't keep creating new resources.

        let newNumber: string|number|null = resp.headers.get("IMS-Incident-Number");
        // Check that we got a value back
        if (newNumber == null) {
            const msg = "No IMS-Incident-Number header provided.";
            ims.setErrorMessage(msg);
            return {err: msg};
        }

        newNumber = ims.parseInt10(newNumber);
        // Check that the value we got back is valid
        if (newNumber == null) {
            const msg = "Non-integer IMS-Incident-Number header provided:" + newNumber;
            ims.setErrorMessage(msg);
            return {err: msg};
        }

        // Store the new number in our incident object
        ims.pathIds.incidentNumber = incident!.number = newNumber;

        // Update browser history to update URL
        drawIncidentTitle("for_display");
        window.history.pushState(
            null, document.title, `${ims.urlReplace(url_viewIncidents)}/${newNumber}`
        );

        // Fetch auth info again with the newly updated URL, just to update
        // the action log.
        await ims.getAuthInfo();
    }

    await loadAndDisplayIncident();
    return {err: null};
}
ims.setSendEdits(sendEdits);

async function editState(): Promise<void> {
    if (el.incidentState.value === "closed" && (incident!.incident_type_ids??[]).length === 0) {
        window.alert(
            "Closing out this incident?\n"+
            "Please add an incident type!\n\n" +
            "Special cases:\n" +
            "    Junk: for erroneously-created Incidents\n" +
            "    Admin: for administrative information, i.e. not Incidents at all\n\n" +
            "See the Incident Types help link for more details.\n"
        );
    }

    await ims.editFromElement(el.incidentState, "state");
}

async function setStartDatetime(selectedDates: Date[], _dateStr: string, sender: ims.Flatpickr): Promise<void> {
    const prevDate = new Date(incident?.started??0);
    const newDate = selectedDates[0];
    if (!newDate || newDate.getTime() === prevDate.getTime()) {
        // nothing to do
        return;
    }

    await ims.editFromElement(sender.altInput!, "started", (_: string|null):string=> {
        return newDate.toISOString();
    });
}

async function editIncidentSummary(): Promise<void> {
    await ims.editFromElement(el.incidentSummary, "summary");
}


async function editLocationName(): Promise<void> {
    const destination = document.querySelector(`option[value='${CSS.escape(el.locationName.value)}']`) as HTMLOptionElement|null;
    if (destination) {
        return await setLocationFromDestination(destination);
    }
    await ims.editFromElement(el.locationName, "location.name");
}

async function setLocationFromDestination(knownLoc: HTMLOptionElement): Promise<void> {
    const nameSuffix: string = knownLoc.dataset["type"] === "Art" ? ` (${knownLoc.dataset["type"]})` : "";
    const edits: ims.Incident = {
        location: {
            name: ((knownLoc.dataset["name"]??"") + nameSuffix).trim(),
            address: (knownLoc.dataset["address"]??"").trim(),
        },
    }
    const {err} = await sendEdits!(edits);
    if (err != null) {
        ims.controlHasError(el.locationName);
    } else {
        ims.controlHasSuccess(el.locationName);
        ims.controlHasSuccess(el.locationAddress);
    }
}

async function editLocationAddress(): Promise<void> {
    await ims.editFromElement(el.locationAddress, "location.address");
}

async function editLocationDescription(): Promise<void> {
    await ims.editFromElement(el.locationDescription, "location.description");
}

async function removeRanger(sender: HTMLElement): Promise<void> {
    const parent = sender.parentElement as HTMLElement;
    const rangerHandle = parent.dataset["rangerHandle"];
    if (!rangerHandle) {
        return;
    }

    const url = (
        ims.urlReplace(url_incidentRanger)
            .replace("<incident_number>", ims.pathIds.incidentNumber!.toString())
            .replace("<ranger_name>", encodeURIComponent(rangerHandle))
    );
    await ims.fetchNoThrow(url, {
        method: "DELETE",
    });
}

async function setRangerRole(sender: HTMLInputElement): Promise<void> {
    const handle = sender.closest("li")?.dataset["rangerHandle"];
    if (!handle) {
        console.log("no Ranger handle for element");
        return;
    }

    const url = (
        ims.urlReplace(url_incidentRanger)
            .replace("<incident_number>", ims.pathIds.incidentNumber!.toString())
            .replace("<ranger_name>", encodeURIComponent(handle))
    );
    const {err} = await ims.fetchNoThrow(url, {
        body: JSON.stringify({
            handle: handle,
            role: sender.value,
        }),
    });
    if (err !== null) {
        ims.controlHasError(sender);
        return;
    }
    ims.controlHasSuccess(sender);

    return;
}


async function removeIncidentType(sender: HTMLElement): Promise<void> {
    const parent = sender.parentElement as HTMLElement;
    const incidentType = ims.parseInt10(parent.dataset["incidentTypeId"]);
    await sendEdits({
        "incident_type_ids": (incident!.incident_type_ids??[]).filter(
            function(t) { return t !== incidentType; }
        ),
    });
}

function normalize(str: string): string {
    return str.toLowerCase().trim();
}

async function addRanger(): Promise<void> {
    let handle: string = el.rangerAdd.value;

    // make a copy of the rangers
    const rangers = (incident!.rangers??[]).slice();
    const handles = rangers.map(r=>r.handle).filter(handle => handle != null);

    // fuzzy-match on handle, to allow case insensitivity and
    // leading/trailing whitespace.
    if (!(handle in (personnel??[]))) {
        const normalized = normalize(handle);
        for (const validHandle in personnel) {
            if (normalized === normalize(validHandle)) {
                handle = validHandle;
                break;
            }
        }
    }
    if (!(handle in (personnel??[]))) {
        // Not a valid handle
        el.rangerAdd.value = "";
        return;
    }

    if (handles.indexOf(handle) !== -1) {
        // Already in the list, so… move along.
        el.rangerAdd.value = "";
        return;
    }

    rangers.push({handle: handle});

    el.rangerAdd.disabled = true;

    const url = (
        ims.urlReplace(url_incidentRanger)
            .replace("<incident_number>", ims.pathIds.incidentNumber!.toString())
            .replace("<ranger_name>", encodeURIComponent(handle))
    );
    const {err} = await ims.fetchNoThrow(url, {
        body: JSON.stringify({
            handle: handle,
        }),
    });
    if (err !== null) {
        ims.controlHasError(el.rangerAdd);
        el.rangerAdd.value = "";
        el.rangerAdd.disabled = false;
        return;
    }
    el.rangerAdd.value = "";
    el.rangerAdd.disabled = false;
    ims.controlHasSuccess(el.rangerAdd);
    el.rangerAdd.focus();
}


async function addIncidentType(): Promise<void> {
    let typeInput = el.incidentTypeAdd.value;

    // make a copy of the incident types
    const currentIncidentTypes = (incident!.incident_type_ids??[]).slice();

    // fuzzy-match on incidentType, to allow case insensitivity and
    // leading/trailing whitespace.
    const normalizedTypeInput = normalize(typeInput);
    // let validTypeInput: string = "";
    let validTypeInputId: number|null = null;
    for (const validType of allIncidentTypes) {
        if (!validType.hidden && validType.name && normalizedTypeInput === normalize(validType.name)) {
            validTypeInputId = validType.id??null;
            break;
        }
    }
    if (validTypeInputId == null) {
        // Not a valid incident type
        el.incidentTypeAdd.value = "";
        return;
    }

    if (currentIncidentTypes.indexOf(validTypeInputId) !== -1) {
        // Already in the list, so… move along.
        el.incidentTypeAdd.value = "";
        return;
    }

    currentIncidentTypes.push(validTypeInputId);

    el.incidentTypeAdd.disabled = true;
    const {err} = await sendEdits({"incident_type_ids": currentIncidentTypes});
    if (err != null) {
        ims.controlHasError(el.incidentTypeAdd);
        el.incidentTypeAdd.value = "";
        el.incidentTypeAdd.disabled = false;
        return;
    }
    el.incidentTypeAdd.value = "";
    el.incidentTypeAdd.disabled = false;
    ims.controlHasSuccess(el.incidentTypeAdd);
    el.incidentTypeAdd.focus();
}


async function detachFieldReport(sender: HTMLElement): Promise<void> {
    const parent: HTMLElement = sender.parentElement!;
    const frNumber = parent.dataset["frNumber"]!;

    const url = (
        `${ims.urlReplace(url_fieldReports)}/${frNumber}` +
        `?action=detach&incident=${ims.pathIds.incidentNumber}`
    );
    const {err} = await ims.fetchNoThrow(url, {
        body: JSON.stringify({}),
    });
    if (err != null) {
        const message = `Failed to detach field report ${err}`;
        console.log(message);
        await loadAllStays();
        await loadAllFieldReports();
        renderFieldReportData();
        ims.setErrorMessage(message);
        return;
    }
    await loadAllStays();
    await loadAllFieldReports();
    renderFieldReportData();
}


async function attachFieldReport(): Promise<void> {
    if (ims.pathIds.incidentNumber == null) {
        // Incident doesn't exist yet. Create it first.
        const {err} = await sendEdits({});
        if (err != null) {
            return;
        }
    }

    const fieldReportNumber = el.attachedFieldReportAdd.value;

    const url = (
        `${ims.urlReplace(url_fieldReports)}/${fieldReportNumber}` +
        `?action=attach&incident=${ims.pathIds.incidentNumber}`
    );
    const {err} = await ims.fetchNoThrow(url, {
        body: JSON.stringify({}),
    });
    if (err != null) {
        const message = `Failed to attach field report: ${err}`;
        console.log(message);
        await loadAllStays();
        await loadAllFieldReports();
        renderFieldReportData();
        ims.setErrorMessage(message);
        ims.controlHasError(el.attachedFieldReportAdd);
        return;
    }
    await loadAllStays();
    await loadAllFieldReports();
    renderFieldReportData();
    ims.controlHasSuccess(el.attachedFieldReportAdd);
}

async function unlinkIncident(sender: HTMLElement): Promise<void> {
    const parent = sender.parentElement as HTMLElement;
    const linkedEventId = ims.parseInt10(parent.dataset["eventId"]);
    const linkedIncidentNumber = ims.parseInt10(parent.dataset["incidentNumber"]);
    await sendEdits({
        "linked_incidents": (incident!.linked_incidents??[]).filter(
            function(t: ims.LinkedIncident): boolean {
                return ! (t.event_id === linkedEventId && t.number === linkedIncidentNumber);
            }
        ),
    });
}

async function linkIncident(input: HTMLInputElement): Promise<void> {
    if (ims.pathIds.incidentNumber == null) {
        // Incident doesn't exist yet. Create it first.
        const {err} = await sendEdits({});
        if (err != null) {
            return;
        }
    }

    const currentEventId = (allEvents??[]).find(value => value.name === ims.pathIds.eventName)!.id;
    const currentLinkedIncidents: ims.LinkedIncident[] = (incident!.linked_incidents??[]).slice();
    let wouldMakeAChange: boolean = false;

    for (let eventAndIncident of input.value.trim().split(",")) {
        // Assume the current event unless another is specified
        let eventID: number|null = currentEventId;
        let incidentNumber: number|null = null;

        eventAndIncident = eventAndIncident.trim();
        // Remove any "#" prefix, since "#123" means the same as "123" (current event, IMS #123).
        if (eventAndIncident.indexOf("#") === 0) {
            eventAndIncident = eventAndIncident.substring(1);
        }

        if (eventAndIncident.indexOf("#") === -1) {
            incidentNumber = ims.parseInt10(eventAndIncident.trim());
        }
        if (eventAndIncident.indexOf("#") > 0) {
            let eventAndIncidentPair: string[] = eventAndIncident.split("#", 2);
            const eventName: string = (eventAndIncidentPair[0]??"").trim();
            if (!eventName) {
                ims.controlHasError(input);
                ims.setErrorMessage(`Invalid format for linked incident. Got '${eventAndIncident}'`);
                input.value = "";
                input.disabled = false;
                return;
            }
            eventID = (allEvents??[]).find(value => value.name === eventName)?.id||null;
            if (!eventID) {
                ims.controlHasError(input);
                ims.setErrorMessage(`There is no Event for name '${eventName}' or you're not may not be authorized to access it`);
                input.value = "";
                input.disabled = false;
                return;
            }
            incidentNumber = ims.parseInt10((eventAndIncidentPair[1]??"").trim());
        }
        const linkedIncident: ims.LinkedIncident = {
            event_id: eventID,
            number: incidentNumber,
        };

        const selfLink: boolean = linkedIncident.event_id === currentEventId && linkedIncident.number === incident?.number;
        if (!selfLink) {
            currentLinkedIncidents.push(linkedIncident!);
            wouldMakeAChange = true;
        }
    }

    if (!wouldMakeAChange) {
        ims.controlHasError(input);
        ims.setErrorMessage("No valid other incidents were provided for linking");
        input.value = "";
        input.disabled = false;
        return;
    }

    input.disabled = true;
    const {err} = await sendEdits({"linked_incidents": currentLinkedIncidents});
    if (err != null) {
        ims.controlHasError(input);
        input.value = "";
        input.disabled = false;
        return;
    }
    input.value = "";
    input.disabled = false;
    ims.controlHasSuccess(input);
    input.focus();
}


// The success callback for a report entry strike call.
async function onStrikeSuccess(): Promise<void> {
    await loadAndDisplayIncident();
    await loadAllStays();
    await loadAllFieldReports();
    renderFieldReportData();
    ims.clearErrorMessage();
}
ims.setOnStrikeSuccess(onStrikeSuccess);

async function attachFile(): Promise<void> {
    if (ims.pathIds.incidentNumber == null) {
        // Incident doesn't exist yet.  Create it first.
        const {err} = await sendEdits({});
        if (err != null) {
            return;
        }
    }
    const formData = new FormData();

    for (const f of el.attachFileInput.files??[]) {
        // this must match the key sought by the server
        formData.append("imsAttachment", f);
    }

    const attachURL = ims.urlReplace(url_incidentAttachments)
        .replace("<incident_number>", (ims.pathIds.incidentNumber??"").toString());
    const {err} = await ims.fetchNoThrow(attachURL, {
        body: formData
    });
    if (err != null) {
        const message = `Failed to attach file: ${err}`;
        ims.setErrorMessage(message);
        return;
    }
    ims.clearErrorMessage();
    el.attachFileInput.value = "";
    await loadAndDisplayIncident();
}
