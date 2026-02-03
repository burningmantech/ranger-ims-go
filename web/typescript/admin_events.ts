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
        setValidity: (el: HTMLSelectElement)=>Promise<void>;
        setExpires: (el: HTMLInputElement) => Promise<void>;
        showExpiresInput: (el: HTMLButtonElement) => Promise<void>;
        addAccess: (el: HTMLInputElement)=>Promise<void>;
        addEvent: (el: HTMLInputElement, type: "group"|"not-group")=>Promise<void>;
        removeAccess: (el: HTMLButtonElement)=>Promise<void>;
        setParentGroup: (el: HTMLInputElement) => Promise<void>;
    }
}

let explainModal: ims.bootstrap.Modal|null = null;
let editEventModal: ims.bootstrap.Modal|null = null;

//
// Initialize UI
//

const el = {
    browserTz: ims.typedElement("browser_tz", HTMLElement),
    explainModal: ims.typedElement("explainModal", HTMLElement),
    editEventModal: ims.typedElement("editEventModal", HTMLElement),
    eventAccessContainer: ims.typedElement("event_access_container", HTMLElement),
    eventAccessTemplate: ims.typedElement("event_access_template", HTMLTemplateElement),
    eventAccessModeTemplate: ims.typedElement("event_access_mode_template", HTMLTemplateElement),
    permissionTemplate: ims.typedElement("permission_template", HTMLTemplateElement),
};

initAdminEventsPage();

async function initAdminEventsPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }

    window.setValidity = setValidity;
    window.setExpires = setExpires;
    window.showExpiresInput = showExpiresInput;
    window.addEvent = addEvent;
    window.addAccess = addAccess;
    window.removeAccess = removeAccess;
    window.setParentGroup = setParentGroup;

    el.browserTz.textContent = Intl.DateTimeFormat().resolvedOptions().timeZone;

    await loadAccessControlList();
    drawAccess();

    explainModal = ims.bsModal(el.explainModal);
    editEventModal = ims.bsModal(el.editEventModal);

    ims.hideLoadingOverlay();
    ims.enableEditing();
}

enum Validity {
    always = "always",
    onsite = "onsite",
}

interface Access {
    expression: string;
    validity: Validity;
    expires?: string|null;
    expired?: boolean|null;
    debug_info?: DebugInfo|null;
}

interface DebugInfo {
    matches_users?: string[]|null
    matches_all_users?: boolean|null
    matches_no_one?: boolean|null
    known_target?: boolean|null
}

const allAccessModes = ["readers", "writers", "reporters", "stay_writers"] as const;
type AccessMode = typeof allAccessModes[number];
type EventAccess = Partial<Record<AccessMode, Access[]>>;
// key is event name
type EventsAccess = Record<string, EventAccess|null>;

let sortedEvents: ims.EventData[];
let accessControlList: EventsAccess|null = null;

async function loadAccessControlList() : Promise<{err: string|null}> {
    const {json: eventsJson, err: eventsErr} = await ims.fetchNoThrow<ims.EventData[]>(url_events + "?include_groups=true", {
        headers: {"Cache-Control": "no-cache"},
    });
    if (eventsErr != null) {
        const message = `Failed to load events: ${eventsErr}`;
        console.error(message);
        window.alert(message);
        return {err: message};
    }
    const events = eventsJson??[];
    const {json, err} = await ims.fetchNoThrow<EventsAccess>(url_acl, null);
    if (err != null) {
        const message = `Failed to load access control list: ${err}`;
        console.error(message);
        window.alert(message);
        return {err: message};
    }
    accessControlList = json;
    // Sort by group status first, then by name
    sortedEvents = events.toSorted((a: ims.EventData, b: ims.EventData): number => {
        const aGroup = a.is_group ? a.id : (a.parent_group??-1);
        const bGroup = b.is_group ? b.id : (b.parent_group??-1);
        if (aGroup !== bGroup) {
            return bGroup-aGroup;
        }
        if (a.is_group !== b.is_group) {
            return a.is_group ? -1 : 1;
        }
        return b.name.localeCompare(a.name);
    });
    return {err: null};
}

function drawAccess(): void {
    el.eventAccessContainer.replaceChildren();

    if (accessControlList == null) {
        return;
    }

    for (const event of sortedEvents) {
        const eventAccessFrag = el.eventAccessTemplate.content.cloneNode(true) as DocumentFragment;

        let eventWithGroupName: string = event.name;
        if (event.is_group) {
            eventWithGroupName = `Group: ${eventWithGroupName}`;
        }
        if (event.parent_group) {
            const parentGroup = sortedEvents.find(value => {return value.id === event.parent_group});
            if (parentGroup) {
                eventWithGroupName += ` (${parentGroup.name})`;
            }
        }

        eventAccessFrag.querySelector(".event_name")!.textContent = eventWithGroupName;

        const editButton = eventAccessFrag.querySelector(".show-edit-modal") as HTMLButtonElement;
        editButton.addEventListener("click", (_e: MouseEvent): void => {
            el.editEventModal.querySelector(".modal-title")!.textContent = event.name;
            el.editEventModal.dataset["eventId"] = event.id.toString();

            const isGroupInput = el.editEventModal.querySelector("#is_group") as HTMLInputElement;
            isGroupInput.disabled = true;
            isGroupInput.value = (event.is_group??false).toString();

            const parentGroupInput = el.editEventModal.querySelector("#edit_parent_group") as HTMLInputElement;

            // groups can't have parent groups
            parentGroupInput.disabled = event.is_group??false;
            const currentParent = sortedEvents.find(value => {return value.id === event.parent_group});
            parentGroupInput.value = currentParent?.name??"";

            editEventModal?.show();
        })

        for (const mode of allAccessModes) {
            const eventModeAccessFrag = el.eventAccessModeTemplate.content.cloneNode(true) as DocumentFragment;
            const eventAccess = eventModeAccessFrag.querySelector("div")!;
            // Add an id to the element for future reference
            eventAccess.id = eventAccessContainerId(event.name, mode);
            eventAccess.dataset["accessMode"] = mode;
            eventAccess.dataset["eventName"] = event.name;
            eventAccessFrag.append(eventModeAccessFrag);
        }

        el.eventAccessContainer.append(eventAccessFrag);

        for (const mode of allAccessModes) {
            updateEventAccess(event.name, mode);
        }
    }
}

function eventAccessContainerId(event: string, mode: string): string {
    return "event_access_" + event + "_" + mode;
}

function displayMode(m: AccessMode): string {
    switch (m) {
        case "readers":
            return "Full readers";
        case "writers":
            return "Full writers";
        case "reporters":
            return "Reporters";
        case "stay_writers":
            return "Stay writers";
        default:
            throw new Error(`unexpected access mode ${m satisfies never}`);
    }
}

function updateEventAccess(event: string, mode: AccessMode): void {
    if (accessControlList == null) {
        return;
    }
    const eventACL: EventAccess|null|undefined = accessControlList[event];
    if (eventACL == null) {
        return;
    }

    const eventAccess: HTMLElement = document.getElementById(eventAccessContainerId(event, mode))!;

    // Set displayed event name and mode
    eventAccess.getElementsByClassName("event_name")[0]!.textContent = event;
    eventAccess.getElementsByClassName("access_mode")[0]!.textContent = displayMode(mode);

    const entryContainer = eventAccess.getElementsByClassName("list-group")[0]!;

    entryContainer.replaceChildren();

    let explainMsgs: string[] = [];
    const indent = "    ";
    const accessEntries = (eventACL[mode]??[]).toSorted((a, b) => a.expression.localeCompare(b.expression));
    for (const accessEntry of accessEntries) {
        const entryItemFrag = el.permissionTemplate.content.cloneNode(true) as DocumentFragment;
        const entryItem = entryItemFrag.querySelector("li")!;

        entryItem.append(accessEntry.expression);
        entryItem.dataset["expression"] = accessEntry.expression;
        entryItem.dataset["validity"] = accessEntry.validity;

        entryItem.dataset["expires"] = accessEntry.expires??"";
        entryItem.dataset["expired"] = accessEntry.expired ? "true" : "false";

        if (accessEntry.debug_info) {
            let unknownSuffix: string = "";
            if (accessEntry.debug_info?.known_target !== true) {
                unknownSuffix = " (Unknown)";
            }
            let expiredSuffix: string = "";
            if (accessEntry.expired) {
                expiredSuffix = " (Expired)"
            }
            let msg: string = `${accessEntry.expression} (${accessEntry.validity})${unknownSuffix}${expiredSuffix}\n`;
            if (accessEntry.debug_info.matches_no_one) {
                msg += `${indent}NO users`;
            } else if (accessEntry.debug_info.matches_all_users) {
                msg += `${indent}ALL authenticated users`;
            } else {
                msg += indent;
                msg += accessEntry.debug_info.matches_users?.join(`\n${indent}`);
            }
            explainMsgs.push(msg);
        }

        const validityField = entryItem.getElementsByClassName("access_validity")[0] as HTMLSelectElement;
        validityField.value = accessEntry.validity;

        const expiresField = entryItem.getElementsByClassName("access_expires")[0] as HTMLInputElement;
        const expiresButton = entryItem.getElementsByClassName("access_expires_button")[0] as HTMLButtonElement;
        if (accessEntry.expires) {
            const d = new Date(accessEntry.expires);
            expiresField.value = `${ims.localDateISO(d)}T${ims.localTimeHHMM(d)}`;
            expiresField.classList.remove("hidden");
            expiresButton.classList.add("hidden");
        } else {
            expiresField.value = "";
            expiresField.classList.add("hidden");
            expiresButton.classList.remove("hidden");
        }
        const expiredText = entryItem.getElementsByClassName("access_expired_text")[0] as HTMLSpanElement;
        if (accessEntry.expired) {
            expiredText.textContent = "Expired";
        } else {
            expiredText.textContent = "";
        }
        const unknownTargetText = entryItem.getElementsByClassName("unknown_target_text")[0] as HTMLSpanElement;
        if (accessEntry.debug_info?.known_target !== true) {
            unknownTargetText.textContent = "Unknown";
        } else {
            unknownTargetText.textContent = "";
        }

        entryContainer.append(entryItemFrag);
    }

    const explainButton = eventAccess.getElementsByClassName("explain_button")[0] as HTMLButtonElement;
    explainButton.addEventListener("click", (_e: MouseEvent): void => {
        el.explainModal.querySelector(".modal-title")!.textContent = `Current ${event} ${mode}`;
        if (explainMsgs.length === 0) {
            explainMsgs.push("No permissions");
        }
        const modalBody = el.explainModal.querySelector(".modal-body")!;
        modalBody.textContent = explainMsgs.join("\n");
        const eventData = sortedEvents.find(value => {return value.name === event});
        if (eventData && eventData.is_group) {
            modalBody.textContent += "\n\nThis is an event group, so all permissions above also apply to its child events:\n";
            for (const event of sortedEvents) {
                if (event.parent_group === eventData.id) {
                    modalBody.textContent += `${indent}${event.name}\n`;
                }
            }
        }
        explainModal?.show();
    })
}

async function addEvent(sender: HTMLInputElement, type: "group"|"not-group"): Promise<void> {
    const event = sender.value.trim();
    const requestBod: ims.EventData = {
        id: 0,
        name: event,
    };
    requestBod.is_group = type === "group";
    const {err} = await ims.fetchNoThrow(url_events, {
        body: JSON.stringify(requestBod),
    });
    if (err != null) {
        const message = `Failed to add event: ${err}`;
        console.log(message);
        window.alert(message);
        await loadAccessControlList();
        drawAccess();
        ims.controlHasError(sender);
        return;
    }
    await loadAccessControlList();
    drawAccess();
    sender.value = "";  // Clear input field
}


async function addAccess(sender: HTMLInputElement): Promise<void> {
    const container: HTMLElement = sender.closest(".event_access")!;
    const event = container.dataset["eventName"]!;
    const mode = container.dataset["accessMode"] as AccessMode;
    const newExpression = sender.value.trim();

    if (newExpression === "") {
        return;
    }

    if (newExpression === "**") {
        const confirmed = confirm(
            "Double-wildcard '**' ACLs are no longer supported, so this ACL will have " +
            "no effect.\n\n" +
            "Proceed with doing something pointless?"
        );
        if (!confirmed) {
            sender.value = "";
            return;
        }
    }

    const validExpression = newExpression === "**" || newExpression === "*" ||
        newExpression.startsWith("person:") || newExpression.startsWith("position:") ||
        newExpression.startsWith("team:") || newExpression.startsWith("onduty:");
    if (!validExpression) {
        const confirmed = confirm(
            "WARNING: '" + newExpression + "' does not look like a valid ACL " +
            "expression. Example expressions include 'person:Hubcap' for an individual, " +
            "'position:007' for a role, 'onduty:007' for people currently on duty for a position, " +
            "and 'team:Council' for a team. Wildcards are " +
            "supported as well, e.g. '*'\n\n" +
            "Proceed with firing footgun?"
        );
        if (!confirmed) {
            sender.value = "";
            return;
        }
    }

    let acl: Access[] = accessControlList![event]![mode]!.slice();

    // remove other acls for this mode for the same expression
    acl = acl.filter((v: Access): boolean => {return v.expression !== newExpression});

    const newVal: Access = {
        "expression": newExpression,
        "validity": Validity.always,
    };

    acl.push(newVal);

    const edits: EventsAccess = {};
    edits[event] = {};
    edits[event][mode] = acl;

    const {err} = await sendACL(edits);
    await loadAccessControlList();
    for (const mode of allAccessModes) {
        updateEventAccess(event, mode);
    }
    if (err != null) {
        ims.controlHasError(sender);
        return;
    }
    sender.value = "";  // Clear input field
}


async function removeAccess(sender: HTMLButtonElement): Promise<void> {
    const container: HTMLElement = sender.closest(".event_access")!;
    const event = container.dataset["eventName"]!;
    const mode = container.dataset["accessMode"] as AccessMode;
    const expression = sender.closest("li")!.dataset["expression"]!.trim();

    const acl: Access[] = accessControlList![event]![mode]!.slice();

    let foundIndex: number = -1;
    for (const [i, access] of acl.entries()) {
        if (access.expression === expression) {
            foundIndex = i;
            break;
        }
    }
    if (foundIndex < 0) {
        console.error("no such ACL: " + expression);
        return;
    }

    acl.splice(foundIndex, 1);

    const edits: EventsAccess = {};
    edits[event] = {};
    edits[event][mode] = acl;

    await sendACL(edits);
    await loadAccessControlList();
    for (const mode of allAccessModes) {
        updateEventAccess(event, mode);
    }
}

async function setValidity(sender: HTMLSelectElement): Promise<void> {
    const container: HTMLElement = sender.closest(".event_access")!;
    const event = container.dataset["eventName"]!;
    const mode = container.dataset["accessMode"] as AccessMode;

    const accessRow = sender.closest("li") as HTMLLIElement;
    const expression = accessRow.dataset["expression"]!.trim();
    const expires = accessRow.dataset["expires"]||null;

    let acl: Access[] = accessControlList![event]![mode]!.slice();

    // remove other acls for this mode for the same expression
    acl = acl.filter((v: Access): boolean => {return v.expression !== expression});

    const newVal: Access = {
        "expression": expression,
        "validity": sender.value === "onsite" ? Validity.onsite : Validity.always,
        "expires": expires,
    };

    acl.push(newVal);

    const edits: EventsAccess = {};
    edits[event] = {};
    edits[event][mode] = acl;

    const {err} = await sendACL(edits);
    await loadAccessControlList();
    for (const mode of allAccessModes) {
        updateEventAccess(event, mode);
    }
    if (err != null) {
        ims.controlHasError(sender);
        return;
    }
    sender.value = "";  // Clear input field
}

async function setExpires(sender: HTMLInputElement): Promise<void> {
    const container: HTMLElement = sender.closest(".event_access")!;
    const event = container.dataset["eventName"]!;
    const mode = container.dataset["accessMode"] as AccessMode;

    const accessRow = sender.closest("li") as HTMLLIElement;
    const expression = accessRow.dataset["expression"]!.trim();
    const validity = accessRow.dataset["validity"]!.trim();

    let acl: Access[] = accessControlList![event]![mode]!.slice();

    // remove other acls for this mode for the same expression
    acl = acl.filter((v: Access): boolean => {return v.expression !== expression});

    let expires: string|null = null;
    if (sender.value) {
        const theDate = new Date(`${sender.value}${ims.localTzOffset(new Date(sender.value))}`);
        expires = theDate.toISOString();
        console.log(`Setting expiration to ${expires}`);
    } else {
        console.log("Unsetting expiration");
    }

    const newVal: Access = {
        "expression": expression,
        "validity": validity === "onsite" ? Validity.onsite : Validity.always,
        "expires": expires,
    };

    acl.push(newVal);

    const edits: EventsAccess = {};
    edits[event] = {};
    edits[event][mode] = acl;

    const {err} = await sendACL(edits);
    await loadAccessControlList();
    for (const mode of allAccessModes) {
        updateEventAccess(event, mode);
    }
    if (err != null) {
        ims.controlHasError(sender);
        return;
    }
}

async function showExpiresInput(sender: HTMLButtonElement): Promise<void> {
    const accessRow = sender.closest("li") as HTMLLIElement;
    sender.classList.add("hidden");
    const expiryField = accessRow.getElementsByClassName("access_expires")[0] as HTMLInputElement;
    expiryField.classList.remove("hidden");
}

async function sendACL(edits: EventsAccess): Promise<{err:string|null}> {
    const {err} = await ims.fetchNoThrow(url_acl, {
        body: JSON.stringify(edits),
    });
    if (err == null) {
        return {err: null};
    }
    const message = `Failed to edit ACL:\n${JSON.stringify(err)}`;
    console.log(message);
    window.alert(message);
    return {err: err};
}

async function setParentGroup(sender: HTMLInputElement): Promise<void> {
    const eventId = ims.parseInt10(el.editEventModal.dataset["eventId"])!;

    const requestBod: ims.EventData = {
        id: eventId,
        // @ts-expect-error the server is fine to receive null here. Really this field should allow null/undefined.
        name: null,
    };

    const parentGroupName = sender.value;
    if (parentGroupName === "") {
        // unset parent_group
        requestBod.parent_group = 0;
    } else {
        const newParent = sortedEvents.find(value => {return value.name === parentGroupName});
        if (!newParent) {
            const message = `No group by that name`;
            console.log(message);
            window.alert(message);
            await loadAccessControlList();
            drawAccess();
            ims.controlHasError(sender);
            return;
        }
        requestBod.parent_group = newParent.id;
    }
    const {err} = await ims.fetchNoThrow(url_events, {
        body: JSON.stringify(requestBod),
    });
    if (err != null) {
        const message = `Failed to edit event: ${err}`;
        console.log(message);
        window.alert(message);
        await loadAccessControlList();
        drawAccess();
        ims.controlHasError(sender);
        return;
    }
    ims.controlHasSuccess(sender);
    await loadAccessControlList();
    drawAccess();
}
