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
        addEvent: (el: HTMLInputElement)=>Promise<void>;
        removeAccess: (el: HTMLButtonElement)=>Promise<void>;
    }
}

let explainModal: ims.bootstrap.Modal|null = null;

//
// Initialize UI
//

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

    document.getElementById("browser_tz")!.textContent = ims.localTzLongName(new Date());

    await loadAccessControlList();
    drawAccess();

    explainModal = ims.bsModal(document.getElementById("explainModal")!);

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

const allAccessModes = ["readers", "writers", "reporters"] as const;
type AccessMode = typeof allAccessModes[number];
type EventAccess = Partial<Record<AccessMode, Access[]>>;
// key is event name
type EventsAccess = Record<string, EventAccess|null>;

let accessControlList: EventsAccess|null = null;

async function loadAccessControlList() : Promise<{err: string|null}> {
    // we don't actually need the response from this API, but we want to
    // invalidate the local HTTP cache in the admin's browser
    ims.fetchNoThrow<ims.EventData[]>(url_events, {
        headers: {"Cache-Control": "no-cache"},
    });
    const {json, err} = await ims.fetchNoThrow<EventsAccess>(url_acl, null);
    if (err != null) {
        const message = `Failed to load access control list: ${err}`;
        console.error(message);
        window.alert(message);
        return {err: message};
    }
    accessControlList = json;
    return {err: null};
}


let _accessTemplate : Element|null = null;
let _eventsEntryTemplate : Element|null = null;

function drawAccess(): void {
    const container: HTMLElement = document.getElementById("event_access_container")!;
    if (_accessTemplate == null) {
        _accessTemplate = container.getElementsByClassName("event_access")[0]!;
        _eventsEntryTemplate = _accessTemplate
            .getElementsByClassName("list-group")[0]!
            .getElementsByClassName("list-group-item")[0]!
        ;
    }

    container.replaceChildren();

    if (accessControlList == null) {
        return;
    }
    const events: string[] = Object.keys(accessControlList);
    for (const event of events) {
        for (const mode of allAccessModes) {
            const eventAccess = _accessTemplate.cloneNode(true) as HTMLElement;
            // Add an id to the element for future reference
            eventAccess.id = eventAccessContainerId(event, mode);

            // Add to container
            container.append(eventAccess);

            updateEventAccess(event, mode);
        }
    }
}

function eventAccessContainerId(event: string, mode: string): string {
    return "event_access_" + event + "_" + mode;
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
    eventAccess.getElementsByClassName("access_mode")[0]!.textContent = mode;

    const entryContainer = eventAccess.getElementsByClassName("list-group")[0]!;

    entryContainer.replaceChildren();

    let explainMsgs: string[] = [];
    const indent = "    ";
    const accessEntries = (eventACL[mode]??[]).toSorted((a, b) => a.expression.localeCompare(b.expression));
    for (const accessEntry of accessEntries) {
        const entryItem = _eventsEntryTemplate!.cloneNode(true) as HTMLElement;

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
            expiredText.classList.remove("hidden");
        } else {
            expiredText.classList.add("hidden");
        }
        const unknownTargetText = entryItem.getElementsByClassName("unknown_target_text")[0] as HTMLSpanElement;
        if (accessEntry.debug_info?.known_target !== true) {
            unknownTargetText.classList.remove("hidden");
        } else {
            unknownTargetText.classList.add("hidden");
        }

        entryContainer.append(entryItem);
    }

    const explainButton = eventAccess.getElementsByClassName("explain_button")[0] as HTMLButtonElement;
    explainButton.addEventListener("click", (_e: MouseEvent): void => {
        const modal = document.getElementById("explainModal")!;
        modal.querySelector(".modal-title")!.textContent = `Current ${event} ${mode}`;
        if (explainMsgs.length === 0) {
            explainMsgs.push("No permissions");
        }
        modal.querySelector(".modal-body")!.textContent = explainMsgs.join("\n");
        explainModal?.show();
    })
}


async function addEvent(sender: HTMLInputElement): Promise<void> {
    const event = sender.value.trim();
    const {err} = await ims.fetchNoThrow(url_events, {
        body: JSON.stringify({
            "add": [event],
        }),
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
    const event = container.getElementsByClassName("event_name")[0]!.textContent!;
    const mode = container.getElementsByClassName("access_mode")[0]!.textContent as AccessMode;
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
    const event = container.getElementsByClassName("event_name")[0]!.textContent!;
    const mode = container.getElementsByClassName("access_mode")[0]!.textContent! as AccessMode;
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
    const event: string = container.getElementsByClassName("event_name")[0]!.textContent!;
    const mode = container.getElementsByClassName("access_mode")[0]!.textContent! as AccessMode;

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
    const event: string = container.getElementsByClassName("event_name")[0]!.textContent!;
    const mode = container.getElementsByClassName("access_mode")[0]!.textContent! as AccessMode;

    const accessRow = sender.closest("li") as HTMLLIElement;
    const expression = accessRow.dataset["expression"]!.trim();
    const validity = accessRow.dataset["validity"]!.trim();

    let acl: Access[] = accessControlList![event]![mode]!.slice();

    // remove other acls for this mode for the same expression
    acl = acl.filter((v: Access): boolean => {return v.expression !== expression});

    let expires: string|null = null;
    if (sender.value) {
        const theDate = new Date(`${sender.value}${ims.localTzOffset(new Date())}`);
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
    // sender.value = "";  // Clear input field
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
