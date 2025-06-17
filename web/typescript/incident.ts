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
        editState: ()=>Promise<void>;
        editIncidentSummary: ()=>Promise<void>;
        editLocationName: ()=>Promise<void>;
        editLocationAddressRadialHour: ()=>Promise<void>;
        editLocationAddressRadialMinute: ()=>Promise<void>;
        editLocationAddressConcentric: ()=>Promise<void>;
        editLocationDescription: ()=>Promise<void>;
        removeRanger: (el: HTMLElement)=>Promise<void>;
        removeIncidentType: (el: HTMLElement)=>Promise<void>;
        detachFieldReport: (el: HTMLElement)=>Promise<void>;
        attachFieldReport: ()=>Promise<void>;
        addRanger: ()=>Promise<void>;
        addIncidentType: ()=>Promise<void>;
        attachFile: ()=>Promise<void>;
        drawMergedReportEntries: ()=>void;
        toggleShowHistory: ()=>void;
        reportEntryEdited: ()=>void;
        submitReportEntry: ()=>Promise<void>;
        overrideStartDate: ()=>Promise<void>;
        overrideStartTime: ()=>Promise<void>;
    }
}

const clubhousePersonURL = "https://ranger-clubhouse.burningman.org/person";

let incident: ims.Incident|null = null;

let incidentTypes: string[] = [];

//
// Initialize UI
//

initIncidentPage();

async function initIncidentPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }

    window.editState = editState;
    window.editIncidentSummary = editIncidentSummary;
    window.editLocationName = editLocationName;
    window.editLocationAddressRadialHour = editLocationAddressRadialHour;
    window.editLocationAddressRadialMinute = editLocationAddressRadialMinute;
    window.editLocationAddressConcentric = editLocationAddressConcentric;
    window.editLocationDescription = editLocationDescription;
    window.removeRanger = removeRanger;
    window.removeIncidentType = removeIncidentType;
    window.detachFieldReport = detachFieldReport;
    window.attachFieldReport = attachFieldReport;
    window.addRanger = addRanger;
    window.addIncidentType = addIncidentType;
    window.attachFile = attachFile;
    window.drawMergedReportEntries = drawMergedReportEntries;
    window.toggleShowHistory = ims.toggleShowHistory;
    window.reportEntryEdited= ims.reportEntryEdited;
    window.submitReportEntry = ims.submitReportEntry;
    window.overrideStartDate = overrideStartDate;
    window.overrideStartTime = overrideStartTime;

    // load everything from the APIs concurrently
    await Promise.all([
        await ims.loadStreets(ims.pathIds.eventID),
        await loadIncident(),
        await loadPersonnel(),
        await ims.loadIncidentTypes().then(
            value=> {incidentTypes = value.types;},
        ),
        await loadAllFieldReports(),
    ]);

    addLocationAddressOptions();
    ims.disableEditing();
    displayIncident();
    if (incident == null) {
        return;
    }
    drawRangers();
    drawRangersToAdd();
    drawIncidentTypesToAdd();
    renderFieldReportData();

    ims.hideLoadingOverlay();

    // for a new incident, jump to summary field
    if (incident!.number == null) {
        document.getElementById("incident_summary")!.focus();
    }

    // Warn the user if they're about to navigate away with unsaved text.
    window.addEventListener("beforeunload", function (e: BeforeUnloadEvent): void {
        if ((document.getElementById("report_entry_add") as HTMLTextAreaElement).value !== "") {
            e.preventDefault();
        }
    });

    ims.requestEventSourceLock();

    ims.newIncidentChannel().onmessage = async function (e: MessageEvent<ims.IncidentBroadcast>): Promise<void> {
        const number = e.data.incident_number;
        const event = e.data.event_name;
        const updateAll = e.data.update_all??false;

        if (updateAll || (event === ims.pathIds.eventID && number === ims.pathIds.incidentNumber)) {
            console.log("Got incident update: " + number);
            await loadAndDisplayIncident();
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
        const event = e.data.event_name;
        if (event === ims.pathIds.eventID) {
            console.log("Got field report update: " + number);
            await loadOneFieldReport(number!);
            renderFieldReportData();
            return;
        }
    };

    const helpModal = ims.bsModal(document.getElementById("helpModal")!);

    const startTimeModal = ims.bsModal(document.getElementById("startTimeModal")!);

    // Keyboard shortcuts
    document.addEventListener("keydown", function(e: KeyboardEvent): void {
        // No shortcuts when an input field is active
        if (document.activeElement !== document.body) {
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
        // a --> jump to add a new report entry
        if (e.key === "a") {
            e.preventDefault();
            // Scroll to report_entry_add field
            document.getElementById("report_entry_add")!.focus();
            document.getElementById("report_entry_add")!.scrollIntoView(true);
        }
        // h --> toggle showing system entries
        if (e.key.toLowerCase() === "h") {
            (document.getElementById("history_checkbox") as HTMLInputElement).click();
        }
        // n --> new incident
        if (e.key.toLowerCase() === "n") {
            (window.open("./new", '_blank') as Window).focus();
        }
    });
    (document.getElementById("helpModal") as HTMLDivElement).addEventListener("keydown", function(e: KeyboardEvent): void {
        if (e.key === "?") {
            helpModal.toggle();
            // This is needed to prevent the document's listener for "?" to trigger the modal to
            // toggle back on immediately. This is fallout from the fix for
            // https://github.com/twbs/bootstrap/issues/41005#issuecomment-2497670835
            e.stopPropagation();
        }
    });
    document.getElementById("report_entry_add")!.addEventListener("keydown", function (e: KeyboardEvent): void {
        const submitEnabled = !document.getElementById("report_entry_submit")!.classList.contains("disabled");
        if (submitEnabled && (e.ctrlKey || e.altKey) && e.key === "Enter") {
            ims.submitReportEntry();
        }
    });
    (document.getElementById("override_started_button") as HTMLElement).addEventListener(
        "click",
        function (_: MouseEvent): void {
            startTimeModal.show();
        },
    );

    // Incident fields generally trigger an update call automatically when the user
    // sets a new value in the field. This behavior is tricky to get right for date
    // and time inputs. Users might type directly into the date/time input, or they
    // might use a browser-supplied date/time picker. With typing, we only want the
    // update sent to the server when focus is lost from the input field (i.e. on blur).
    // With a picker, we want the update sent when the user has finished picking a
    // date or time (i.e. on input). In order to make both of those things work, while
    // not updating on every keypress if the user is typing, we do the below, which
    // avoids input updates for typing.
    //
    // The weird thing for date/time inputs is that an input and change event get fired
    // after every basically keypress if the user is typing. A normal text input field
    // only sees input and change events when the user blurs or hits enter/tab on the field.
    function addDateTimeInputListeners(dti: HTMLElement, update: ()=>void): void {
        let keydown = false;
        dti.addEventListener("keydown", (_: KeyboardEvent): void => {
            keydown = true;
            setTimeout(() => { keydown = false; }, 10);
        });
        dti.addEventListener("input", (_: Event): void => {
            if (!keydown) update();
        });
        dti.addEventListener("blur", (_: FocusEvent): void => {
            update();
        });
    }

    addDateTimeInputListeners(
        document.getElementById("override_start_date")!,
        overrideStartDate,
    )
    addDateTimeInputListeners(
        document.getElementById("override_start_time")!,
        overrideStartTime,
    )
}


//
// Load incident
//

async function loadIncident(): Promise<{err: string|null}> {
    let number: number|null = null;
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
        const {json, err} = await ims.fetchJsonNoThrow<ims.Incident>(
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
        (document.getElementById("attach_file") as HTMLInputElement).classList.remove("hidden");
    }
}

// Do all the client-side rendering based on the state of allFieldReports.
function renderFieldReportData(): void {
    loadAttachedFieldReports();
    drawFieldReportsToAttach();
    drawMergedReportEntries();
    drawAttachedFieldReports();
}


//
// Load personnel
//

let personnel: PersonnelMap|null = null;

interface Personnel {
    handle: string;
    directory_id?: number|null;
    // These are only the statuses that IMS actually reads from Clubhouse.
    // See https://github.com/burningmantech/ranger-ims-go/blob/master/directory/queries.sql
    status: "active"|"alpha"|"auditor"|"inactive extension"|"inactive"|"prospective";
}

// key is Ranger handle
type PersonnelMap = Record<string, Personnel>;

async function loadPersonnel(): Promise<{err: string|null}> {
    const {json, err} = await ims.fetchJsonNoThrow<Personnel[]>(ims.urlReplace(url_personnel + "?event_id=<event_id>"), null);
    if (err != null) {
        const message = `Failed to load personnel: ${err}`;
        console.error(message);
        ims.setErrorMessage(message);
        return {err: message};
    }
    const _personnel: PersonnelMap = {};
    for (const record of json!) {
        switch (record.status) {
            case "active":
            case "alpha":
            case "prospective":
                _personnel[record.handle] = record;
                break
            default:
                // Don't add this person to the personnel list.
                break;
        }
    }
    personnel = _personnel;
    return {err: null};
}

//
// Load all field reports
//

let allFieldReports: ims.FieldReport[]|null|undefined = null;

async function loadAllFieldReports(): Promise<{err: string|null}> {
    if (allFieldReports === undefined) {
        return {err: null};
    }

    const {resp, json, err} = await ims.fetchJsonNoThrow<ims.FieldReport[]>(ims.urlReplace(url_fieldReports), null);
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

    const {resp, json, err} = await ims.fetchJsonNoThrow<ims.FieldReport>(
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


//
// Load attached field reports
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


//
// Draw all fields
//

function drawIncidentFields() {
    drawIncidentTitle();
    drawIncidentNumber();
    drawState();
    drawStarted();
    drawPriority();
    drawIncidentSummary();
    drawRangers();
    drawIncidentTypes();
    drawLocationName();
    drawLocationAddressRadialHour();
    drawLocationAddressRadialMinute();
    drawLocationAddressConcentric();
    drawLocationDescription();
    ims.toggleShowHistory();
    drawMergedReportEntries();

    document.getElementById("report_entry_add")!.addEventListener("input", ims.reportEntryEdited);
}


//
// Add option elements to location address select elements
//

function addLocationAddressOptions(): void {
    const hours: number[] = ims.range(1, 13);
    const hourElement: HTMLElement = document.getElementById("incident_location_address_radial_hour")!;
    for (const hour of hours) {
        const hourStr: string = ims.padTwo(hour);
        const newOption: HTMLOptionElement = document.createElement("option");
        newOption.value = hourStr;
        newOption.textContent = hourStr;
        hourElement.append(newOption);
    }

    const minutes: number[] = ims.range(0, 12, 5);
    const minuteElement: HTMLElement = document.getElementById("incident_location_address_radial_minute")!;
    for (const minute of minutes) {
        const minuteStr = ims.padTwo(minute);
        const newOption: HTMLOptionElement = document.createElement("option");
        newOption.value = minuteStr;
        newOption.textContent = minuteStr;
        minuteElement.append(newOption);
    }

    const concentricElement: HTMLElement = document.getElementById("incident_location_address_concentric")!;
    for (const id in ims.concentricStreetNameByID!) {
        const newOption: HTMLOptionElement = document.createElement("option");
        newOption.value = id;
        newOption.textContent = ims.concentricStreetNameByID[id]??"null";
        concentricElement.append(newOption);
    }
}


//
// Populate page title
//

function drawIncidentTitle(): void {
    const eventSuffix = ims.pathIds.eventID != null ? ` | ${ims.pathIds.eventID}` : "";
    document.title = `${ims.incidentAsString(incident!)}${eventSuffix}`;
}


//
// Populate incident number
//

function drawIncidentNumber(): void {
    let number: number|string|null = incident!.number??null;
    if (number == null) {
        number = "(new)";
    }
    document.getElementById("incident_number")!.textContent = number.toString();
}


//
// Populate incident state
//

function drawState(): void {
    ims.selectOptionWithValue(
        document.getElementById("incident_state") as HTMLSelectElement,
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
    const startedElement: HTMLElement = document.getElementById("started_datetime")!;
    startedElement.textContent = `${ims.longDate.format(dateNum)}, ${ims.shortTimeTZ.format(dateNum)}`;
    startedElement.title = ims.fullDateTime.format(dateNum);

    const dateInput = document.getElementById("override_start_date") as HTMLInputElement;
    const newDateISO = ims.localDateISO(dateDate);
    if (dateInput.value !== newDateISO) {
        dateInput.value = newDateISO;
    }

    const timeInput = document.getElementById("override_start_time") as HTMLInputElement;
    const localTime =  localTimeHHMM(dateDate);
    if (timeInput.value !== localTime) {
        timeInput.value = localTime;
    }

    const tzInput = document.getElementById("override_start_tz") as HTMLSpanElement;
    tzInput.textContent = ims.localTzShortName(dateDate);
}

function localTimeHHMM(date: Date): string {
    const hours = date.getHours().toString().padStart(2, "0");
    const minutes = date.getMinutes().toString().padStart(2, "0");
    return `${hours}:${minutes}`;
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
    const summaryElement = document.getElementById("incident_summary") as HTMLInputElement;
    summaryElement.placeholder = "One-line summary of incident";
    if (incident!.summary) {
        summaryElement.value = incident!.summary;
        summaryElement.placeholder = "";
        return;
    }

    summaryElement.value = "";
    const summarized = ims.summarizeIncidentOrFR(incident!);
    // only replace the placeholder if it would be nonempty
    if (summarized) {
        summaryElement.placeholder = summarized;
    }
}


//
// Populate Rangers list
//

let _rangerItem: HTMLElement|null = null;

function drawRangers() {
    if (_rangerItem == null) {
        _rangerItem = document.getElementById("incident_rangers_list")!
            .getElementsByClassName("list-group-item")[0] as HTMLElement;
    }

    const handles: string[] = incident!.ranger_handles??[];
    handles.sort((a, b) => a.localeCompare(b));

    const rangersElement: HTMLElement = document.getElementById("incident_rangers_list")!;
    rangersElement.replaceChildren();
    for (const handle of handles) {

        const rangerContainer = _rangerItem!.cloneNode(true) as HTMLElement;
        rangerContainer.dataset["rangerHandle"] = handle;

        if (personnel?.[handle] == null) {
            const rangerNoLink = document.createElement("span");
            rangerNoLink.textContent = handle;
            rangerContainer.append(rangerNoLink!);
        } else {
            const person = personnel[handle];
            const rangerLink = document.createElement("a");
            rangerLink.textContent = rangerAsString(person);
            if (person.directory_id != null) {
                rangerLink.href = `${clubhousePersonURL}/${person.directory_id}`;
            }
            rangerContainer.append(rangerLink);
        }

        rangersElement.append(rangerContainer);
    }
}


function drawRangersToAdd(): void {
    const datalist = document.getElementById("ranger_handles") as HTMLDataListElement;

    const handles: string[] = [];
    for (const handle in personnel) {
        handles.push(handle);
    }
    handles.sort((a: string, b: string) => a.localeCompare(b));

    datalist.replaceChildren();
    datalist.append(document.createElement("option"));

    if (personnel != null) {
        for (const handle of handles) {
            const ranger = personnel[handle];
            if (ranger === undefined) {
                console.error(`no record for personnel with handle ${handle}`);
                continue;
            }

            const option: HTMLOptionElement = document.createElement("option");
            option.value = handle;
            option.text = rangerAsString(ranger);

            datalist.append(option);
        }
    }
}


function rangerAsString(ranger: Personnel): string {
    return ranger.handle;
}


//
// Populate incident types list
//

let _typesItem: HTMLElement|null = null;

function drawIncidentTypes() {
    if (_typesItem == null) {
        _typesItem = document.getElementById("incident_types_list")!
            .getElementsByClassName("list-group-item")[0] as HTMLElement;
    }

    const incidentTypes: string[] = incident!.incident_types??[];
    incidentTypes.sort();

    const typesElement: HTMLElement = document.getElementById("incident_types_list")!;
    typesElement.replaceChildren();

    for (const incidentType of incidentTypes) {
        const item = _typesItem!.cloneNode(true) as HTMLElement;
        const typeSpan = document.createElement("span");
        typeSpan.textContent = incidentType;
        item.append(typeSpan);
        item.dataset["incidentTypeName"] = incidentType;
        typesElement.append(item);
    }
}


function drawIncidentTypesToAdd() {
    const datalist = document.getElementById("incident_types") as HTMLDataListElement;
    datalist.replaceChildren();
    datalist.append(document.createElement("option"));
    for (const incidentType of incidentTypes) {
        const option: HTMLOptionElement = document.createElement("option");
        option.value = incidentType;
        datalist.append(option);
    }
}


//
// Populate location
//

function drawLocationName() {
    if (incident!.location?.name) {
        const locName = document.getElementById("incident_location_name") as HTMLInputElement;
        locName.value = incident!.location.name;
    }
}


function drawLocationAddressRadialHour() {
    let hour: string|null = null;
    if (incident!.location?.radial_hour != null) {
        hour = ims.padTwo(incident!.location.radial_hour);
    }
    ims.selectOptionWithValue(
        document.getElementById("incident_location_address_radial_hour") as HTMLSelectElement,
        hour,
    );
}


function drawLocationAddressRadialMinute() {
    let minute: string|null = null;
    if (incident!.location?.radial_minute != null) {
        minute = ims.normalizeMinute(incident!.location.radial_minute);
    }
    ims.selectOptionWithValue(
        document.getElementById("incident_location_address_radial_minute") as HTMLSelectElement,
        minute,
    );
}


function drawLocationAddressConcentric() {
    let concentric = null;
    if (incident!.location?.concentric) {
        concentric = incident!.location.concentric;
    }
    ims.selectOptionWithValue(
        document.getElementById("incident_location_address_concentric") as HTMLSelectElement,
        concentric,
    );
}


function drawLocationDescription() {
    if (incident!.location?.description) {
        const description = document.getElementById("incident_location_description") as HTMLInputElement;
        description.value = incident!.location.description;
    }
}


//
// Draw report entries
//

function drawMergedReportEntries(): void {
    const entries: ims.ReportEntry[] = (incident!.report_entries??[]).slice()

    if (attachedFieldReports) {
        const mergedCheckbox = document.getElementById("merge_reports_checkbox") as HTMLInputElement;
        if (mergedCheckbox.checked) {
            for (const report of attachedFieldReports) {
                for (const entry of report.report_entries??[]) {
                    entry.merged = report.number??null;
                    entries.push(entry);
                }
            }
        }
    }

    entries.sort(ims.compareReportEntries);

    ims.drawReportEntries(entries);
}


let _reportsItem: HTMLElement|null = null;

function drawAttachedFieldReports() {
    if (_reportsItem == null) {
         const elements = document.getElementById("attached_field_reports")!
            .getElementsByClassName("list-group-item");
        if (elements.length === 0) {
            console.error("found no reportsItem");
            return;
        }
        _reportsItem = elements[0] as HTMLElement;
    }

    const reports = attachedFieldReports??[];
    reports.sort();

    const container = document.getElementById("attached_field_reports")!;
    container.replaceChildren();

    for (const report of reports) {
        const link: HTMLAnchorElement = document.createElement("a");
        link.href = `${ims.urlReplace(url_viewFieldReports)}/${report.number}`;
        link.innerText = ims.fieldReportAsString(report);

        const item = _reportsItem.cloneNode(true) as HTMLElement;
        item.append(link);
        item.dataset["frNumber"] = report.number!.toString();

        container.append(item);
    }
}


function drawFieldReportsToAttach() {
    const container = document.getElementById("attached_field_report_add_container") as HTMLDivElement;
    const select = document.getElementById("attached_field_report_add") as HTMLSelectElement;

    select.replaceChildren();
    select.append(document.createElement("option"));

    if (!allFieldReports) {
        container.classList.add("hidden");
    } else {
        const unattachedGroup: HTMLOptGroupElement = document.createElement("optgroup");
        unattachedGroup.label = "Unattached to any incident";
        select.append(unattachedGroup);
        for (const report of allFieldReports) {
            // Skip field reports that *are* attached to an incident
            if (report.incident != null) {
                continue;
            }
            const option: HTMLOptionElement = document.createElement("option");
            option.value = report.number!.toString();
            option.text = ims.fieldReportAsString(report);
            select.append(option);
        }
        const attachedGroup: HTMLOptGroupElement = document.createElement("optgroup");
        attachedGroup.label = "Attached to another incident";
        select.append(attachedGroup);
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
            select.append(option);
        }
        select.append(document.createElement("optgroup"));

        container.classList.remove("hidden");
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

    const {resp, err} = await ims.fetchJsonNoThrow(url, {
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
        drawIncidentTitle();
        window.history.pushState(
            null, document.title, `${ims.urlReplace(url_viewIncidents)}/${newNumber}`
        );
    }

    await loadAndDisplayIncident();
    return {err: null};
}
ims.setSendEdits(sendEdits);

async function editState(): Promise<void> {
    const state = document.getElementById("incident_state") as HTMLSelectElement;

    if (state.value === "closed" && (incident!.incident_types??[]).length === 0) {
        window.alert(
            "Closing out this incident?\n"+
            "Please add an incident type!\n\n" +
            "Special cases:\n" +
            "    Junk: for erroneously-created Incidents\n" +
            "    Admin: for administrative information, i.e. not Incidents at all\n\n" +
            "See the Incident Types help link for more details.\n"
        );
    }

    await ims.editFromElement(state, "state");
}

async function overrideStartDate(): Promise<void> {

    const dateInput = document.getElementById("override_start_date") as HTMLInputElement;

    const prevDateISO = ims.localDateISO(new Date(incident?.started??0));
    if (dateInput.value === prevDateISO) {
        // nothing to do
        return;
    }

    const timeInput = document.getElementById("override_start_time") as HTMLInputElement;

    const tzValue = document.getElementById("override_start_tz")!.textContent??"";

    await ims.editFromElement(dateInput, "started", (_: string|null):string=> {
        return newDateTimeVal(dateInput.value, timeInput.value, tzValue);
    });
}

function newDateTimeVal(dateInput: string, timeInput: string, localTz: string): string {
    const val = `${dateInput.trim()} ${timeInput.trim()} ${localTz}`;
    const date = new Date(val);
    // Just do a check on the year to prevent obvious mistakes.
    // This will break in year 2099. Feel free to update maximum year.
    if (date.getFullYear() < 2000 || date.getFullYear() > 2099) {
        throw new Error(`year seems incorrect: ${date.getFullYear()}`);
    }
    return date.toISOString();
}

async function overrideStartTime(): Promise<void> {
    const dateInput = document.getElementById("override_start_date") as HTMLInputElement;
    const timeInput = document.getElementById("override_start_time") as HTMLInputElement;

    if (timeInput.value === localTimeHHMM(new Date(Date.parse(incident?.started??"")))) {
        // nothing to do
        return;
    }

    const tzValue = document.getElementById("override_start_tz")!.textContent??"";

    await ims.editFromElement(timeInput, "started", (_: string|null):string=> {
        return newDateTimeVal(dateInput.value, timeInput.value, tzValue);
    });
}


async function editIncidentSummary(): Promise<void> {
    const summaryInput = document.getElementById("incident_summary") as HTMLInputElement;
    await ims.editFromElement(summaryInput, "summary");
}


async function editLocationName(): Promise<void> {
    const locationInput = document.getElementById("incident_location_name") as HTMLInputElement;
    await ims.editFromElement(locationInput, "location.name");
}


function transformAddressInteger(value: string): string|null {
    return ims.parseInt10(value)?.toString()??null;
}


async function editLocationAddressRadialHour(): Promise<void> {
    const hourInput = document.getElementById("incident_location_address_radial_hour") as HTMLInputElement;
    await ims.editFromElement(hourInput, "location.radial_hour", transformAddressInteger);
}


async function editLocationAddressRadialMinute(): Promise<void> {
    const minuteInput = document.getElementById("incident_location_address_radial_minute") as HTMLInputElement;
    await ims.editFromElement(minuteInput, "location.radial_minute", transformAddressInteger);
}


async function editLocationAddressConcentric(): Promise<void> {
    const concentricInput = document.getElementById("incident_location_address_concentric") as HTMLSelectElement;
    await ims.editFromElement(concentricInput, "location.concentric");
}


async function editLocationDescription(): Promise<void> {
    const descriptionInput = document.getElementById("incident_location_description") as HTMLInputElement;
    await ims.editFromElement(descriptionInput, "location.description");
}


async function removeRanger(sender: HTMLElement): Promise<void> {
    const parent = sender.parentElement as HTMLElement;
    const rangerHandle = parent.dataset["rangerHandle"];

    await sendEdits(
        {
            "ranger_handles": (incident!.ranger_handles??[]).filter(
                function(h: string): boolean { return h !== rangerHandle; }
            ),
        },
    );
}


async function removeIncidentType(sender: HTMLElement): Promise<void> {
    const parent = sender.parentElement as HTMLElement;
    const incidentType = parent.dataset["incidentTypeName"];
    await sendEdits({
        "incident_types": (incident!.incident_types??[]).filter(
            function(t) { return t !== incidentType; }
        ),
    });
}

function normalize(str: string): string {
    return str.toLowerCase().trim();
}

async function addRanger(): Promise<void> {
    const addRanger = document.getElementById("ranger_add") as HTMLInputElement;
    let handle: string = addRanger.value;

    // make a copy of the handles
    const handles = (incident!.ranger_handles??[]).slice();

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
        addRanger.value = "";
        return;
    }

    if (handles.indexOf(handle) !== -1) {
        // Already in the list, so… move along.
        addRanger.value = "";
        return;
    }

    handles.push(handle);

    addRanger.disabled = true;
    const {err} = await sendEdits({"ranger_handles": handles});
    if (err !== null) {
        ims.controlHasError(addRanger);
        addRanger.value = "";
        addRanger.disabled = false;
        return;
    }
    addRanger.value = "";
    addRanger.disabled = false;
    ims.controlHasSuccess(addRanger, 1000);
    addRanger.focus();
}


async function addIncidentType(): Promise<void> {
    const addType = document.getElementById("incident_type_add") as HTMLInputElement;
    let incidentType = addType.value;

    // make a copy of the incident types
    const currentIncidentTypes = (incident!.incident_types??[]).slice();

    // fuzzy-match on incidentType, to allow case insensitivity and
    // leading/trailing whitespace.
    if (incidentTypes.indexOf(incidentType) === -1) {
        const normalized = normalize(incidentType);
        for (const validType of incidentTypes) {
            if (normalized === normalize(validType)) {
                incidentType = validType;
                break;
            }
        }
    }
    if (incidentTypes.indexOf(incidentType) === -1) {
        // Not a valid incident type
        addType.value = "";
        return;
    }

    if (currentIncidentTypes.indexOf(incidentType) !== -1) {
        // Already in the list, so… move along.
        addType.value = "";
        return;
    }

    currentIncidentTypes.push(incidentType);

    addType.disabled = true;
    const {err} = await sendEdits({"incident_types": currentIncidentTypes});
    if (err != null) {
        ims.controlHasError(addType);
        addType.value = "";
        addType.disabled = false;
        return;
    }
    addType.value = "";
    addType.disabled = false;
    ims.controlHasSuccess(addType, 1000);
    addType.focus();
}


async function detachFieldReport(sender: HTMLElement): Promise<void> {
    const parent: HTMLElement = sender.parentElement!;
    const frNumber = parent.dataset["frNumber"]!;

    const url = (
        `${ims.urlReplace(url_fieldReports)}/${frNumber}` +
        `?action=detach&incident=${ims.pathIds.incidentNumber}`
    );
    const {err} = await ims.fetchJsonNoThrow(url, {
        body: JSON.stringify({}),
    });
    if (err != null) {
        const message = `Failed to detach field report ${err}`;
        console.log(message);
        await loadAllFieldReports();
        renderFieldReportData();
        ims.setErrorMessage(message);
        return;
    }
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

    const select = document.getElementById("attached_field_report_add") as HTMLSelectElement;
    const fieldReportNumber = select.value;

    const url = (
        `${ims.urlReplace(url_fieldReports)}/${fieldReportNumber}` +
        `?action=attach&incident=${ims.pathIds.incidentNumber}`
    );
    const {err} = await ims.fetchJsonNoThrow(url, {
        body: JSON.stringify({}),
    });
    if (err != null) {
        const message = `Failed to attach field report: ${err}`;
        console.log(message);
        await loadAllFieldReports();
        renderFieldReportData();
        ims.setErrorMessage(message);
        ims.controlHasError(select);
        return;
    }
    await loadAllFieldReports();
    renderFieldReportData();
    ims.controlHasSuccess(select, 1000);
}


// The success callback for a report entry strike call.
async function onStrikeSuccess(): Promise<void> {
    await loadAndDisplayIncident();
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
    const attachFile = document.getElementById("attach_file_input") as HTMLInputElement;
    const formData = new FormData();

    for (const f of attachFile.files??[]) {
        // this must match the key sought by the server
        formData.append("imsAttachment", f);
    }

    const attachURL = ims.urlReplace(url_incidentAttachments)
        .replace("<incident_number>", (ims.pathIds.incidentNumber??"").toString());
    const {err} = await ims.fetchJsonNoThrow(attachURL, {
        body: formData
    });
    if (err != null) {
        const message = `Failed to attach file: ${err}`;
        ims.setErrorMessage(message);
        return;
    }
    ims.clearErrorMessage();
    attachFile.value = "";
    await loadAndDisplayIncident();
}
