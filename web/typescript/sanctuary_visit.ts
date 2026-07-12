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
        editParentIncident: () => void;

        editGuestPreferredName: () => void;
        editGuestLegalName: () => void;
        editGuestDescription: () => void;
        editGuestActionPlan: () => void;
        editGuestCampName: () => void;
        editGuestCampAddress: () => void;
        editGuestCampDescription: () => void;
        editGuestCampContacts: () => void;

        editArrivalMethod: () => void;
        editArrivalState: () => void;
        editArrivalReason: () => void;
        editArrivalBelongings: () => void;

        editDepartureMethod: () => void;
        editDepartureState: () => void;

        editResourceSitter: () => void;
        editResourceBedID: () => void;
        editResourceRest: () => void;
        editResourceClothes: () => void;
        editResourcePogs: () => void;
        editResourceFoodBev: () => void;
        editResourceOther: () => void;

        addRanger: () => void;
        removeRanger: (el: HTMLElement)=>void;
        setRangerRole: (el: HTMLInputElement)=>void;

        toggleShowHistory: () => void;
        reportEntryEdited: ()=>void;
        submitReportEntry: ()=>void;
        attachFile: () => void;
    }
}

let visit: ims.Visit|null = null;

// Announces (to assistive tech) that someone else has changed this Visit while
// it's open, which is otherwise a silent redraw of the page.
const remoteUpdates = ims.newRemoteUpdateAnnouncer("This Visit was updated");

// The ETag from the last read of this visit, sent back as If-Match on edits
// so that the server can reject an edit based on stale data (HTTP 412).
let visitETag: string|null = null;

//
// Initialize UI
//

const el = {
    visitNumber: ims.typedElement("visit_number", HTMLInputElement),
    parentIncident: ims.typedElement("parent_incident", HTMLInputElement),
    parentIncidentLink: ims.typedElement("parent_incident_link", HTMLAnchorElement),

    guestPreferredName: ims.typedElement("guest_preferred_name", HTMLInputElement),
    guestLegalName: ims.typedElement("guest_legal_name", HTMLInputElement),
    guestDescription: ims.typedElement("guest_description", HTMLInputElement),
    guestActionPlan: ims.typedElement("guest_action_plan", HTMLInputElement),
    guestCampName: ims.typedElement("guest_camp_name", HTMLInputElement),
    guestCampAddress: ims.typedElement("guest_camp_address", HTMLInputElement),
    guestCampDescription: ims.typedElement("guest_camp_description", HTMLInputElement),
    guestCampContacts: ims.typedElement("guest_camp_contacts", HTMLInputElement),

    arrivalTime: ims.typedElement("arrival_time", HTMLInputElement) as ims.FlatpickrHTMLInputElement,
    arrivalMethod: ims.typedElement("arrival_method", HTMLInputElement),
    arrivalState: ims.typedElement("arrival_state", HTMLInputElement),
    arrivalReason: ims.typedElement("arrival_reason", HTMLTextAreaElement),
    arrivalBelongings: ims.typedElement("arrival_belongings", HTMLTextAreaElement),

    departureTime: ims.typedElement("departure_time", HTMLInputElement) as ims.FlatpickrHTMLInputElement,
    departureMethod: ims.typedElement("departure_method", HTMLInputElement),
    departureState: ims.typedElement("departure_state", HTMLInputElement),

    resourceSitter: ims.typedElement("resource_sitter", HTMLInputElement),
    resourceBedID: ims.typedElement("resource_bed_id", HTMLInputElement),
    resourceRest: ims.typedElement("resource_rest", HTMLInputElement),
    resourceClothes: ims.typedElement("resource_clothes", HTMLInputElement),
    resourcePogs: ims.typedElement("resource_pogs", HTMLInputElement),
    resourceFoodBev: ims.typedElement("resource_food_bev", HTMLInputElement),
    resourceOther: ims.typedElement("resource_other", HTMLInputElement),

    rangerHandles: ims.typedElement("ranger_handles", HTMLDataListElement),
    addRanger: ims.typedElement("ranger_add", HTMLInputElement),

    historyCheckbox: ims.typedElement("history_checkbox", HTMLInputElement),
    reportEntryAdd: ims.typedElement("report_entry_add", HTMLTextAreaElement),
    attachFile: ims.typedElement("attach_file", HTMLInputElement),
    attachFileInput: ims.typedElement("attach_file_input", HTMLInputElement),
};

initSanctuaryVisitPage();

async function initSanctuaryVisitPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    if (!ims.eventAccess!.readVisits) {
        ims.setErrorMessage(
            `You're not currently authorized to read Visits in Event "${ims.pathIds.eventName}".`
        );
        ims.hideLoadingOverlay();
        return;
    }

    // TODO: window assignments go here
    window.editParentIncident = editParentIncident;

    window.editGuestPreferredName = editGuestPreferredName;
    window.editGuestLegalName = editGuestLegalName;
    window.editGuestDescription = editGuestDescription;
    window.editGuestActionPlan = editGuestActionPlan;
    window.editGuestCampName = editGuestCampName;
    window.editGuestCampAddress = editGuestCampAddress;
    window.editGuestCampDescription = editGuestCampDescription;
    window.editGuestCampContacts = editGuestCampContacts;

    window.editArrivalMethod = editArrivalMethod;
    window.editArrivalState = editArrivalState;
    window.editArrivalReason = editArrivalReason;
    window.editArrivalBelongings = editArrivalBelongings;

    window.editDepartureMethod = editDepartureMethod;
    window.editDepartureState = editDepartureState;

    window.editResourceSitter = editResourceSitter;
    window.editResourceBedID = editResourceBedID;
    window.editResourceRest = editResourceRest;
    window.editResourceClothes = editResourceClothes;
    window.editResourcePogs = editResourcePogs;
    window.editResourceFoodBev = editResourceFoodBev;
    window.editResourceOther = editResourceOther;

    window.addRanger = addRanger;
    window.removeRanger = removeRanger;
    window.setRangerRole = setRangerRole;

    window.toggleShowHistory = ims.toggleShowHistory;
    window.reportEntryEdited = ims.reportEntryEdited;
    window.submitReportEntry = ims.submitReportEntry;
    window.attachFile = attachFile;

    // load everything from the APIs concurrently
    await Promise.all([
        await loadVisit(),
        await loadPersonnel(),
    ])

    // const onChange = function(selectedDates: Date[], _dateStr: string, instance: ims.Flatpickr): void {
    //     instance.input!.title = ims.longFormatDate(selectedDates[0]!);
    //     instance.altInput!.title = ims.longFormatDate(selectedDates[0]!);
    // };

    ims.newFlatpickr(el.arrivalTime, "alt_arrival_time", editArrivalTime);
    ims.newFlatpickr(el.departureTime, "alt_departure_time", editDepartureTime);

    ims.disableEditing();
    displayVisit();
    if (visit == null) {
        return;
    }

    drawRangers();
    drawRangersToAdd();

    // TODO: draw other fields

    ims.hideLoadingOverlay();

    // For a new visit, jump to the name field
    if (visit!.number == null) {
        el.guestPreferredName.focus();
    }

    // Warn the user if they're about to navigate away with unsaved text.
    window.addEventListener("beforeunload", function (e: BeforeUnloadEvent): void {
        if (el.reportEntryAdd.value !== "") {
            e.preventDefault();
        }
    });

    ims.requestEventSourceLock();

    ims.newVisitChannel().onmessage = async function (e: MessageEvent<ims.VisitBroadcast>): Promise<void> {
        const number = e.data.visit_number;
        const eventId = e.data.event_id;
        const updateAll = e.data.update_all??false;

        if (updateAll || (eventId === ims.pathIds.eventId && number === ims.pathIds.visitNumber)) {
            console.log("Got visit update: " + number);
            await loadAndDisplayVisit();
            remoteUpdates.announceUpdate();
        }
    }

    const helpModal = ims.bsModal(document.getElementById("helpModal")!);

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
        // n --> new visit
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
    el.reportEntryAdd.addEventListener("keydown", function (e: KeyboardEvent): void {
        const submitEnabled = !document.getElementById("report_entry_submit")!.classList.contains("disabled");
        if (submitEnabled && (e.ctrlKey || e.altKey) && e.key === "Enter") {
            ims.submitReportEntry();
        }
    });

    window.addEventListener("beforeprint", (_event: Event): void => {
        drawVisitTitle("for_print_to_pdf");
    });
    window.addEventListener("afterprint", (_event: Event): void => {
        drawVisitTitle("for_display");
    });
}

async function loadAndDisplayVisit(): Promise<void> {
    await loadVisit();
    displayVisit();
}

async function loadVisit(): Promise<{err: string|null}> {
    let number: number|null;
    if (visit == null) {
        // First time here. Use page initial value.
        number = ims.pathIds.visitNumber??null;
    } else {
        // We have a visit already. Use that number.
        number = visit.number!;
    }

    if (number == null) {
        visit = {
            "number": null,
        };
        visitETag = null;
    } else {
        const {resp, json, err} = await ims.fetchNoThrow<ims.Visit>(
            `${ims.urlReplace(url_visits)}/${number}`, null);
        if (err != null) {
            ims.disableEditing();
            const message = `Failed to load Visit ${number}: ${err}`;
            console.error(message);
            ims.setErrorMessage(message);
            return {err: message};
        }
        visit = json;
        visitETag = ims.etagOf(resp);
    }
    return {err: null};
}

function displayVisit(): void {
    if (visit == null) {
        const message = "Visit failed to load";
        console.log(message);
        ims.setErrorMessage(message);
        return;
    }

    drawVisitFields();
    ims.toggleShowHistory();
    ims.drawReportEntries(visit.report_entries??[]);
    ims.clearErrorMessage();

    el.reportEntryAdd.addEventListener("input", ims.reportEntryEdited);

    if (ims.eventAccess?.writeVisits) {
        ims.enableEditing();
    } else {
        ims.disableEditing();
    }

    if (ims.eventAccess?.attachFiles) {
        el.attachFile.classList.remove("hidden");
    }
}

function drawVisitFields(): void {
    drawVisitTitle("for_display");
    el.visitNumber.value = (visit?.number??"(new)").toString();

    let docTitle = "Sanctuary Visit";
    if (visit?.number == null) {
        docTitle = `New ${docTitle}`;
    } else if (visit?.departure_time) {
        docTitle = `Past ${docTitle}`;
    } else {
        docTitle = `Current ${docTitle}`;
    }
    if (visit?.guest_preferred_name) {
        docTitle = `${docTitle} (${visit?.guest_preferred_name})`;
    } else if (visit?.guest_legal_name) {
        docTitle = `${docTitle} (${visit?.guest_legal_name})`;
    } else if (visit?.number) {
        docTitle = `${docTitle} (no name)`;
    }
    document.getElementById("doc-title")!.textContent = docTitle;
    if (visit?.incident) {
        ims.setInputValue(el.parentIncident, (visit.incident?.toString())??"");
        el.parentIncidentLink.href = ims.urlReplace(`${url_viewIncidents}/${visit.incident}`);
    } else {
        ims.setInputValue(el.parentIncident, "");
    }
    el.parentIncident.placeholder = "(none)";

    ims.setInputValue(el.guestPreferredName, (visit?.guest_preferred_name?.toString())??"");
    ims.setInputValue(el.guestLegalName, (visit?.guest_legal_name?.toString())??"");
    ims.setInputValue(el.guestDescription, (visit?.guest_description?.toString())??"");
    ims.setInputValue(el.guestActionPlan, (visit?.guest_action_plan?.toString())??"");
    ims.setInputValue(el.guestCampName, (visit?.guest_camp_name?.toString())??"");
    ims.setInputValue(el.guestCampAddress, (visit?.guest_camp_address?.toString())??"");
    ims.setInputValue(el.guestCampDescription, (visit?.guest_camp_description?.toString())??"");
    ims.setInputValue(el.guestCampContacts, (visit?.guest_camp_contacts?.toString())??"");

    if (visit?.arrival_time && ims.setFlatpickrDate(el.arrivalTime, visit.arrival_time)) {
        const fullDate = ims.longFormatDate(new Date(visit.arrival_time));
        el.arrivalTime._flatpickr.input!.title = fullDate;
        el.arrivalTime._flatpickr.altInput!.title = fullDate;
    }
    ims.setInputValue(el.arrivalMethod, (visit?.arrival_method?.toString())??"");
    ims.setInputValue(el.arrivalState, (visit?.arrival_state?.toString())??"");
    ims.setInputValue(el.arrivalReason, (visit?.arrival_reason?.toString())??"");
    ims.setInputValue(el.arrivalBelongings, (visit?.arrival_belongings?.toString())??"");

    if (visit?.departure_time && ims.setFlatpickrDate(el.departureTime, visit.departure_time)) {
        const fullDate = ims.longFormatDate(new Date(visit.departure_time));
        el.departureTime._flatpickr.input!.title = fullDate;
        el.departureTime._flatpickr.altInput!.title = fullDate;
    }
    ims.setInputValue(el.departureMethod, (visit?.departure_method?.toString())??"");
    ims.setInputValue(el.departureState, (visit?.departure_state?.toString())??"");

    ims.setInputValue(el.resourceSitter, (visit?.resource_sitter?.toString())??"");
    ims.setInputValue(el.resourceBedID, (visit?.resource_bed_id?.toString())??"");
    ims.setInputValue(el.resourceRest, (visit?.resource_rest?.toString())??"");
    ims.setInputValue(el.resourceClothes, (visit?.resource_clothes?.toString())??"");
    ims.setInputValue(el.resourcePogs, (visit?.resource_pogs?.toString())??"");
    ims.setInputValue(el.resourceFoodBev, (visit?.resource_food_bev?.toString())??"");
    ims.setInputValue(el.resourceOther, (visit?.resource_other?.toString())??"");

    drawRangers();
}

function drawVisitTitle(mode: "for_display"|"for_print_to_pdf"): void {
    let newTitle: string = "";
    if (mode === "for_print_to_pdf" && visit?.number) {
        newTitle = `Visit-${ims.pathIds.eventName}-${visit.number}_${visit.guest_preferred_name??""}`;
    } else {
        const eventSuffix: string = ims.pathIds.eventName != null ? ` | ${ims.pathIds.eventName}` : "";
        newTitle = `${ims.visitAsString(visit!)}${eventSuffix}`;
    }
    document.title = newTitle;
}


// Sequences sendEdits calls, so that rapid autosaves don't race one another:
// each edit must carry the ETag produced by the previous one.
let sendEditsChain: Promise<{err:string|null}> = Promise.resolve({err: null});

function sendEdits(edits: ims.Visit): Promise<{err:string|null}> {
    remoteUpdates.noteLocalEdit();
    sendEditsChain = sendEditsChain.then(
        () => sendEditsNow(edits),
        () => sendEditsNow(edits),
    );
    return sendEditsChain;
}

async function sendEditsNow(edits: ims.Visit): Promise<{err:string|null}> {
    const number = visit!.number;
    let url = ims.urlReplace(url_visits);

    if (number == null) {
        // We're creating a new visit. Assume the guest checked in now.
        edits.arrival_time = new Date().toISOString();
    } else {
        // We're editing an existing visit.
        edits.number = number;
        url += `/${number}`;
    }

    // Report-entry appends can't lose data, so they're sent unconditionally
    // rather than failing on a stale ETag when someone else edits a field.
    const noteOnly = Object.keys(edits).every(
        (key) => key === "report_entries" || key === "number");
    const headers: HeadersInit = {};
    if (number != null && visitETag != null && !noteOnly) {
        headers["If-Match"] = visitETag;
    }
    const {resp, err} = await ims.fetchNoThrow(url, {
        headers: headers,
        body: JSON.stringify(edits),
    });

    if (err != null) {
        let message = `Failed to apply edit: ${err}`;
        if (resp?.status === 412) {
            message = "Someone else has edited this visit. " +
                "The page has been refreshed with their changes; please retry your edit.";
        }
        await loadAndDisplayVisit();
        ims.setErrorMessage(message);
        return {err: message};
    }

    visitETag = ims.etagOf(resp) ?? visitETag;

    if (number == null && resp != null) {
        // We created a new visit.
        // We need to find out the created visit number so that future
        // edits don't keep creating new resources.

        let newNumber: string|number|null = resp.headers.get("IMS-Visit-Number");
        // Check that we got a value back
        if (newNumber == null) {
            const msg = "No IMS-Visit-Number header provided.";
            ims.setErrorMessage(msg);
            return {err: msg};
        }

        newNumber = ims.parseInt10(newNumber);
        // Check that the value we got back is valid
        if (newNumber == null) {
            const msg = "Non-integer IMS-Visit-Number header provided:" + newNumber;
            ims.setErrorMessage(msg);
            return {err: msg};
        }

        // Store the new number in our visit object
        ims.pathIds.visitNumber = visit!.number = newNumber;

        // Update browser history to update URL
        drawVisitTitle("for_display");
        window.history.pushState(
            null, document.title, `${ims.urlReplace(url_viewVisits)}/${newNumber}`
        );

        // Fetch auth info again with the newly updated URL, just to update
        // the action log.
        await ims.getAuthInfo();
    }

    await loadAndDisplayVisit();
    return {err: null};
}
ims.setSendEdits(sendEdits);

async function editParentIncident(): Promise<void> {
    const transform = (value: string): number|null => {
        if (value === "") {
            return 0;
        }
        return ims.parseInt10(value);
    }
    await ims.editFromElement(el.parentIncident, "incident", transform);
}

async function editGuestPreferredName(): Promise<void> {
    await ims.editFromElement(el.guestPreferredName, "guest_preferred_name");
}

async function editGuestLegalName(): Promise<void> {
    await ims.editFromElement(el.guestLegalName, "guest_legal_name");
}

async function editGuestDescription(): Promise<void> {
    await ims.editFromElement(el.guestDescription, "guest_description");
}

async function editGuestActionPlan(): Promise<void> {
    await ims.editFromElement(el.guestActionPlan, "guest_action_plan");
}

async function editGuestCampName(): Promise<void> {
    await ims.editFromElement(el.guestCampName, "guest_camp_name");
}

async function editGuestCampAddress(): Promise<void> {
    await ims.editFromElement(el.guestCampAddress, "guest_camp_address");
}

async function editGuestCampDescription(): Promise<void> {
    await ims.editFromElement(el.guestCampDescription, "guest_camp_description");
}

async function editGuestCampContacts(): Promise<void> {
    await ims.editFromElement(el.guestCampContacts, "guest_camp_contacts");
}

const zeroTimeValue = "0001-01-01T00:00:00Z";

async function editArrivalTime(selectedDates: Date[], _dateStr: string, sender: ims.Flatpickr): Promise<void> {
    const prevDate: Date|undefined = visit?.arrival_time ? new Date(visit.arrival_time) : undefined;
    const newDate: Date|undefined = selectedDates[0];
    if (newDate?.getTime() === prevDate?.getTime()) {
        // Either they're the same valid time, or neither is set, so there's nothing to do.
        return;
    }
    const newDateStr = ()=> (newDate?.toISOString()) || zeroTimeValue;
    await ims.editFromElement(sender.altInput!, "arrival_time", newDateStr);
}
async function editArrivalMethod(): Promise<void> {
    await ims.editFromElement(el.arrivalMethod, "arrival_method");
}
async function editArrivalState(): Promise<void> {
    await ims.editFromElement(el.arrivalState, "arrival_state");
}
async function editArrivalReason(): Promise<void> {
    await ims.editFromElement(el.arrivalReason, "arrival_reason");
}
async function editArrivalBelongings(): Promise<void> {
    await ims.editFromElement(el.arrivalBelongings, "arrival_belongings");
}

async function editDepartureTime(selectedDates: Date[], _dateStr: string, sender: ims.Flatpickr): Promise<void> {
    const prevDate: Date|undefined = visit?.departure_time ? new Date(visit.departure_time) : undefined;
    const newDate: Date|undefined = selectedDates[0];
    if (newDate?.getTime() === prevDate?.getTime()) {
        // Either they're the same valid time, or neither is set, so there's nothing to do.
        return;
    }
    const newDateStr = ()=> (newDate?.toISOString()) || zeroTimeValue;
    await ims.editFromElement(sender.altInput!, "departure_time", newDateStr);
}
async function editDepartureMethod(): Promise<void> {
    await ims.editFromElement(el.departureMethod, "departure_method");
}
async function editDepartureState(): Promise<void> {
    await ims.editFromElement(el.departureState, "departure_state");
}

async function editResourceSitter(): Promise<void> {
    await ims.editFromElement(el.resourceSitter, "resource_sitter");
}
async function editResourceBedID(): Promise<void> {
    await ims.editFromElement(el.resourceBedID, "resource_bed_id");
}
async function editResourceRest(): Promise<void> {
    await ims.editFromElement(el.resourceRest, "resource_rest");
}
async function editResourceClothes(): Promise<void> {
    await ims.editFromElement(el.resourceClothes, "resource_clothes");
}
async function editResourcePogs(): Promise<void> {
    await ims.editFromElement(el.resourcePogs, "resource_pogs");
}
async function editResourceFoodBev(): Promise<void> {
    await ims.editFromElement(el.resourceFoodBev, "resource_food_bev");
}
async function editResourceOther(): Promise<void> {
    await ims.editFromElement(el.resourceOther, "resource_other");
}

// The success callback for a report entry strike call.
async function onStrikeSuccess(): Promise<void> {
    await loadAndDisplayVisit();
    ims.clearErrorMessage();
}
ims.setOnStrikeSuccess(onStrikeSuccess);

// Handle for the pending "Uploaded ✓" revert, so a fresh upload can cancel a
// stale revert from a previous one.
let attachFileRevertTimeout: number|null = null;

async function attachFile(): Promise<void> {
    if (attachFileRevertTimeout != null) {
        window.clearTimeout(attachFileRevertTimeout);
        attachFileRevertTimeout = null;
    }
    if (ims.pathIds.visitNumber == null) {
        // Visit doesn't exist yet. Create it first.
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

    const attachURL = ims.urlReplace(url_visitAttachments)
        .replace("<visit_number>", (ims.pathIds.visitNumber??"").toString());

    el.attachFile.disabled = true;
    el.attachFile.value = "Uploading...";
    try {
        const {err} = await ims.fetchNoThrow(attachURL, {
            body: formData,
        });
        if (err != null) {
            const message = `Failed to attach file: ${err}`;
            ims.setErrorMessage(message);
            el.attachFile.value = "Attach file";
            return;
        }
        ims.clearErrorMessage();
        el.attachFileInput.value = "";
        await loadAndDisplayVisit();

        // Brief confirmation, then revert.
        el.attachFile.value = "Uploaded ✓";
        attachFileRevertTimeout = window.setTimeout((): void => {
            el.attachFile.value = "Attach file";
            attachFileRevertTimeout = null;
        }, 2000);
    } finally {
        el.attachFile.disabled = false;
    }
}

let personnel: ims.PersonnelMap|null = null;

async function loadPersonnel(): Promise<void> {
    const res = await ims.fetchPersonnel();
    if (res.err != null || res.personnel == null) {
        ims.setErrorMessage(res.err??"");
    }
    personnel = res.personnel;
}

function normalize(str: string): string {
    return str.toLowerCase().trim();
}

async function addRanger(): Promise<void> {
    let handle: string = el.addRanger.value;

    // make a copy of the rangers
    const rangers = (visit?.rangers??[]).slice();
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
        el.addRanger.value = "";
        return;
    }

    if (handles.indexOf(handle) !== -1) {
        // Already in the list, so… move along.
        el.addRanger.value = "";
        return;
    }

    rangers.push({handle: handle});

    el.addRanger.disabled = true;

    if (ims.pathIds.visitNumber == null) {
        // Visit doesn't exist yet. Create it first.
        const {err} = await sendEdits({});
        if (err != null) {
            return;
        }
    }

    const url = (
        ims.urlReplace(url_visitRanger)
            .replace("<visit_number>", ims.pathIds.visitNumber!.toString())
            .replace("<ranger_name>", encodeURIComponent(handle))
    );
    const {resp, err} = await ims.fetchNoThrow(url, {
        body: JSON.stringify({
            handle: handle,
        }),
    });
    if (err !== null) {
        ims.controlHasError(el.addRanger);
        el.addRanger.value = "";
        el.addRanger.disabled = false;
        return;
    }
    // The roster change moved the visit's version on the server.
    visitETag = ims.etagOf(resp) ?? visitETag;
    el.addRanger.value = "";
    el.addRanger.disabled = false;
    ims.controlHasSuccess(el.addRanger);
    el.addRanger.focus();
}

async function removeRanger(sender: HTMLElement): Promise<void> {
    const parent = sender.parentElement as HTMLElement;
    const rangerHandle = parent.dataset["rangerHandle"];
    if (!rangerHandle) {
        return;
    }

    const url = (
        ims.urlReplace(url_visitRanger)
            .replace("<visit_number>", ims.pathIds.visitNumber!.toString())
            .replace("<ranger_name>", encodeURIComponent(rangerHandle))
    );
    const {resp} = await ims.fetchNoThrow(url, {
        method: "DELETE",
    });
    // The roster change moved the visit's version on the server.
    visitETag = ims.etagOf(resp) ?? visitETag;
}


async function setRangerRole(sender: HTMLInputElement): Promise<void> {
    const handle = sender.closest("li")?.dataset["rangerHandle"];
    if (!handle) {
        console.log("no Ranger handle for element");
        return;
    }

    const url = (
        ims.urlReplace(url_visitRanger)
            .replace("<visit_number>", ims.pathIds.visitNumber!.toString())
            .replace("<ranger_name>", encodeURIComponent(handle))
    );
    const {resp, err} = await ims.fetchNoThrow(url, {
        body: JSON.stringify({
            handle: handle,
            role: sender.value,
        }),
    });
    if (err !== null) {
        ims.controlHasError(sender);
        return;
    }
    visitETag = ims.etagOf(resp) ?? visitETag;
    ims.controlHasSuccess(sender);

    return;
}

function drawRangers() {
    const rangers: ims.VisitRanger[] = visit?.rangers??[];
    rangers.sort((a: ims.VisitRanger, b: ims.VisitRanger) => (a.handle??"").localeCompare(b.handle??""));

    const rangerItemTemplate = document.getElementById("visit_rangers_li_template") as HTMLTemplateElement;

    const rangersElement: HTMLElement = document.getElementById("visit_rangers_list")!;
    rangersElement.querySelectorAll("li").forEach((el: HTMLElement) => {el.remove()});

    for (const ranger of rangers) {
        if (!ranger.handle) {
            continue;
        }
        const handle = ranger.handle;

        const rangerFragment = rangerItemTemplate.content.cloneNode(true) as DocumentFragment;
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
        rangerLi.querySelector("button")!.ariaLabel = `Remove Ranger ${handle}`;

        rangersElement.append(rangerFragment);
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
