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
        editParentIncident: () => void;

        editGuestPreferredName: () => void;
        editGuestLegalName: () => void;
        editGuestDescription: () => void;
        editGuestCampName: () => void;
        editGuestCampAddress: () => void;
        editGuestCampDescription: () => void;

        editArrivalMethod: () => void;
        editArrivalState: () => void;
        editArrivalReason: () => void;
        editArrivalBelongings: () => void;

        editDepartureMethod: () => void;
        editDepartureState: () => void;

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

let stay: ims.Stay|null = null;

//
// Initialize UI
//

const el = {
    stayNumber: ims.typedElement("stay_number", HTMLInputElement),
    parentIncident: ims.typedElement("parent_incident", HTMLInputElement),
    parentIncidentLink: ims.typedElement("parent_incident_link", HTMLAnchorElement),

    guestPreferredName: ims.typedElement("guest_preferred_name", HTMLInputElement),
    guestLegalName: ims.typedElement("guest_legal_name", HTMLInputElement),
    guestDescription: ims.typedElement("guest_description", HTMLInputElement),
    guestCampName: ims.typedElement("guest_camp_name", HTMLInputElement),
    guestCampAddress: ims.typedElement("guest_camp_address", HTMLInputElement),
    guestCampDescription: ims.typedElement("guest_camp_description", HTMLInputElement),

    arrivalTime: ims.typedElement("arrival_time", HTMLInputElement) as ims.FlatpickrHTMLInputElement,
    arrivalMethod: ims.typedElement("arrival_method", HTMLInputElement),
    arrivalState: ims.typedElement("arrival_state", HTMLInputElement),
    arrivalReason: ims.typedElement("arrival_reason", HTMLTextAreaElement),
    arrivalBelongings: ims.typedElement("arrival_belongings", HTMLTextAreaElement),

    departureTime: ims.typedElement("departure_time", HTMLInputElement) as ims.FlatpickrHTMLInputElement,
    departureMethod: ims.typedElement("departure_method", HTMLInputElement),
    departureState: ims.typedElement("departure_state", HTMLInputElement),

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

initSanctuaryStayPage();

async function initSanctuaryStayPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    if (!ims.eventAccess!.readStays) {
        ims.setErrorMessage(
            `You're not currently authorized to read Stays in Event "${ims.pathIds.eventName}".`
        );
        ims.hideLoadingOverlay();
        return;
    }

    // TODO: window assignments go here
    window.editParentIncident = editParentIncident;

    window.editGuestPreferredName = editGuestPreferredName;
    window.editGuestLegalName = editGuestLegalName;
    window.editGuestDescription = editGuestDescription;
    window.editGuestCampName = editGuestCampName;
    window.editGuestCampAddress = editGuestCampAddress;
    window.editGuestCampDescription = editGuestCampDescription;

    window.editArrivalMethod = editArrivalMethod;
    window.editArrivalState = editArrivalState;
    window.editArrivalReason = editArrivalReason;
    window.editArrivalBelongings = editArrivalBelongings;

    window.editDepartureMethod = editDepartureMethod;
    window.editDepartureState = editDepartureState;

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
        await loadStay(),
        await loadPersonnel(),
    ])

    // const onChange = function(selectedDates: Date[], _dateStr: string, instance: ims.Flatpickr): void {
    //     instance.input!.title = ims.longFormatDate(selectedDates[0]!);
    //     instance.altInput!.title = ims.longFormatDate(selectedDates[0]!);
    // };

    ims.newFlatpickr(el.arrivalTime, "alt_arrival_time", editArrivalTime);
    ims.newFlatpickr(el.departureTime, "alt_departure_time", editDepartureTime);

    ims.disableEditing();
    displayStay();
    if (stay == null) {
        return;
    }

    drawRangers();
    drawRangersToAdd();

    // TODO: draw other fields

    ims.hideLoadingOverlay();

    // For a new stay, jump to the name field
    if (stay!.number == null) {
        el.guestPreferredName.focus();
    }

    // Warn the user if they're about to navigate away with unsaved text.
    window.addEventListener("beforeunload", function (e: BeforeUnloadEvent): void {
        if (el.reportEntryAdd.value !== "") {
            e.preventDefault();
        }
    });

    ims.requestEventSourceLock();

    ims.newStayChannel().onmessage = async function (e: MessageEvent<ims.StayBroadcast>): Promise<void> {
        const number = e.data.stay_number;
        const eventId = e.data.event_id;
        const updateAll = e.data.update_all??false;

        if (updateAll || (eventId === ims.pathIds.eventId && number === ims.pathIds.stayNumber)) {
            console.log("Got stay update: " + number);
            await loadAndDisplayStay();
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
        // n --> new stay
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
        drawStayTitle("for_print_to_pdf");
    });
    window.addEventListener("afterprint", (_event: Event): void => {
        drawStayTitle("for_display");
    });
}

async function loadAndDisplayStay(): Promise<void> {
    await loadStay();
    displayStay();
}

async function loadStay(): Promise<{err: string|null}> {
    let number: number|null;
    if (stay == null) {
        // First time here. Use page initial value.
        number = ims.pathIds.stayNumber??null;
    } else {
        // We have a stay already. Use that number.
        number = stay.number!;
    }

    if (number == null) {
        stay = {
            "number": null,
        };
    } else {
        const {json, err} = await ims.fetchNoThrow<ims.Stay>(
            `${ims.urlReplace(url_stays)}/${number}`, null);
        if (err != null) {
            ims.disableEditing();
            const message = `Failed to load Stay ${number}: ${err}`;
            console.error(message);
            ims.setErrorMessage(message);
            return {err: message};
        }
        stay = json;
    }
    return {err: null};
}

function displayStay(): void {
    if (stay == null) {
        const message = "Stay failed to load";
        console.log(message);
        ims.setErrorMessage(message);
        return;
    }

    drawStayFields();
    ims.toggleShowHistory();
    ims.drawReportEntries(stay.report_entries??[]);
    ims.clearErrorMessage();

    el.reportEntryAdd.addEventListener("input", ims.reportEntryEdited);

    if (ims.eventAccess?.writeStays) {
        ims.enableEditing();
    } else {
        ims.disableEditing();
    }

    if (ims.eventAccess?.attachFiles) {
        el.attachFile.classList.remove("hidden");
    }
}

function drawStayFields(): void {
    drawStayTitle("for_display");
    el.stayNumber.value = (stay?.number??"(new)").toString();
    if (stay?.incident) {
        el.parentIncident.value = (stay.incident?.toString())??"";
        el.parentIncidentLink.href = ims.urlReplace(`${url_viewIncidents}/${stay.incident}`);
    } else {
        el.parentIncident.value = "";
    }
    el.parentIncident.placeholder = "(none)";

    el.guestPreferredName.value = (stay?.guest_preferred_name?.toString())??"";
    el.guestLegalName.value = (stay?.guest_legal_name?.toString())??"";
    el.guestDescription.value = (stay?.guest_description?.toString())??"";
    el.guestCampName.value = (stay?.guest_camp_name?.toString())??"";
    el.guestCampAddress.value = (stay?.guest_camp_address?.toString())??"";
    el.guestCampDescription.value = (stay?.guest_camp_description?.toString())??"";

    if (stay?.arrival_time) {
        el.arrivalTime._flatpickr.setDate(stay.arrival_time, false, "Z");
    }
    el.arrivalMethod.value = (stay?.arrival_method?.toString())??"";
    el.arrivalState.value = (stay?.arrival_state?.toString())??"";
    el.arrivalReason.value = (stay?.arrival_reason?.toString())??"";
    el.arrivalBelongings.value = (stay?.arrival_belongings?.toString())??"";

    if (stay?.departure_time) {
        el.departureTime._flatpickr.setDate(stay.departure_time, false, "Z");
    }
    el.departureMethod.value = (stay?.departure_method?.toString())??"";
    el.departureState.value = (stay?.departure_state?.toString())??"";

    el.resourceRest.value = (stay?.resource_rest?.toString())??"";
    el.resourceClothes.value = (stay?.resource_clothes?.toString())??"";
    el.resourcePogs.value = (stay?.resource_pogs?.toString())??"";
    el.resourceFoodBev.value = (stay?.resource_food_bev?.toString())??"";
    el.resourceOther.value = (stay?.resource_other?.toString())??"";

    drawRangers();
}

function drawStayTitle(mode: "for_display"|"for_print_to_pdf"): void {
    let newTitle: string = "";
    if (mode === "for_print_to_pdf" && stay?.number) {
        newTitle = `Stay-${ims.pathIds.eventName}-${stay.number}_${stay.guest_preferred_name}`;
    } else {
        const eventSuffix: string = ims.pathIds.eventName != null ? ` | ${ims.pathIds.eventName}` : "";
        newTitle = `${ims.stayAsString(stay!)}${eventSuffix}`;
    }
    document.title = newTitle;
}


async function sendEdits(edits: ims.Stay): Promise<{err:string|null}> {
    const number = stay!.number;
    let url = ims.urlReplace(url_stays);

    if (number == null) {
        // We're creating a new stay. Assume the guest checked in now.
        edits.arrival_time = new Date().toISOString();
    } else {
        // We're editing an existing stay.
        edits.number = number;
        url += `/${number}`;
    }

    const {resp, err} = await ims.fetchNoThrow(url, {
        body: JSON.stringify(edits),
    });

    if (err != null) {
        const message = `Failed to apply edit: ${err}`;
        await loadAndDisplayStay();
        ims.setErrorMessage(message);
        return {err: message};
    }

    if (number == null && resp != null) {
        // We created a new stay.
        // We need to find out the created stay number so that future
        // edits don't keep creating new resources.

        let newNumber: string|number|null = resp.headers.get("IMS-Stay-Number");
        // Check that we got a value back
        if (newNumber == null) {
            const msg = "No IMS-Stay-Number header provided.";
            ims.setErrorMessage(msg);
            return {err: msg};
        }

        newNumber = ims.parseInt10(newNumber);
        // Check that the value we got back is valid
        if (newNumber == null) {
            const msg = "Non-integer IMS-Stay-Number header provided:" + newNumber;
            ims.setErrorMessage(msg);
            return {err: msg};
        }

        // Store the new number in our stay object
        ims.pathIds.stayNumber = stay!.number = newNumber;

        // Update browser history to update URL
        drawStayTitle("for_display");
        window.history.pushState(
            null, document.title, `${ims.urlReplace(url_viewStays)}/${newNumber}`
        );

        // Fetch auth info again with the newly updated URL, just to update
        // the action log.
        await ims.getAuthInfo();
    }

    await loadAndDisplayStay();
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

async function editGuestCampName(): Promise<void> {
    await ims.editFromElement(el.guestCampName, "guest_camp_name");
}

async function editGuestCampAddress(): Promise<void> {
    await ims.editFromElement(el.guestCampAddress, "guest_camp_address");
}

async function editGuestCampDescription(): Promise<void> {
    await ims.editFromElement(el.guestCampDescription, "guest_camp_description");
}

const zeroTimeValue = "0001-01-01T00:00:00Z";

async function editArrivalTime(selectedDates: Date[], _dateStr: string, sender: ims.Flatpickr): Promise<void> {
    const prevDate: Date|undefined = stay?.arrival_time ? new Date(stay.arrival_time) : undefined;
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
    const prevDate: Date|undefined = stay?.departure_time ? new Date(stay.departure_time) : undefined;
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
    await loadAndDisplayStay();
    ims.clearErrorMessage();
}
ims.setOnStrikeSuccess(onStrikeSuccess);

async function attachFile(): Promise<void> {
    if (ims.pathIds.stayNumber == null) {
        // Stay doesn't exist yet. Create it first.
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

    const attachURL = ims.urlReplace(url_stayAttachments)
        .replace("<stay_number>", (ims.pathIds.stayNumber??"").toString());
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
    await loadAndDisplayStay();
}

let personnel: ims.PersonnelMap|null = null;

async function loadPersonnel(): Promise<void> {
    const res = await fetchPersonnel();
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
    const rangers = (stay?.rangers??[]).slice();
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

    if (ims.pathIds.stayNumber == null) {
        // Stay doesn't exist yet. Create it first.
        const {err} = await sendEdits({});
        if (err != null) {
            return;
        }
    }

    const url = (
        ims.urlReplace(url_stayRanger)
            .replace("<stay_number>", ims.pathIds.stayNumber!.toString())
            .replace("<ranger_name>", encodeURIComponent(handle))
    );
    const {err} = await ims.fetchNoThrow(url, {
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
        ims.urlReplace(url_stayRanger)
            .replace("<stay_number>", ims.pathIds.stayNumber!.toString())
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
        ims.urlReplace(url_stayRanger)
            .replace("<stay_number>", ims.pathIds.stayNumber!.toString())
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

function drawRangers() {
    const rangers: ims.StayRanger[] = stay?.rangers??[];
    rangers.sort((a: ims.StayRanger, b: ims.StayRanger) => (a.handle??"").localeCompare(b.handle??""));

    const rangerItemTemplate = document.getElementById("stay_rangers_li_template") as HTMLTemplateElement;

    const rangersElement: HTMLElement = document.getElementById("stay_rangers_list")!;
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
