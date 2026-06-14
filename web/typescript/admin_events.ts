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
        setLevel: (el: HTMLSelectElement)=>Promise<void>;
        addAccess: (el: HTMLInputElement)=>Promise<void>;
        fixAccess: (el: HTMLInputElement)=>Promise<void>;
        addEvent: (el: HTMLInputElement, type: "group"|"not-group")=>Promise<void>;
        removeAccess: (el: HTMLButtonElement)=>Promise<void>;
        setParentGroup: (el: HTMLInputElement) => Promise<void>;
        setMapURL: (el: HTMLInputElement) => Promise<void>;
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
    accessRuleTemplate: ims.typedElement("access_rule_template", HTMLTemplateElement),
    accessTargetList: ims.typedElement("access_target_list", HTMLDataListElement),
};

initAdminEventsPage();

async function initAdminEventsPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }

    window.setValidity = setValidity;
    window.setLevel = setLevel;
    window.addEvent = addEvent;
    window.addAccess = addAccess;
    window.fixAccess = fixAccess;
    window.removeAccess = removeAccess;
    window.setParentGroup = setParentGroup;
    window.setMapURL = setMapURL;

    el.browserTz.textContent = Intl.DateTimeFormat().resolvedOptions().timeZone;

    await Promise.all([loadAccessControlList(), loadAccessTargets()]);
    expandEventsWithRules();
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
    not_after?: string|null;
    expired?: boolean|null;
    not_before?: string|null;
    pending?: boolean|null;
    debug_info?: DebugInfo|null;
}

interface DebugInfo {
    matches_users?: string[]|null
    matches_all_users?: boolean|null
    matches_no_one?: boolean|null
    known_target?: boolean|null
}

interface AccessTargets {
    persons?: string[]|null;
    positions?: string[]|null;
    teams?: string[]|null;
}

const allAccessModes = ["readers", "writers", "reporters", "visit_writers"] as const;
type AccessMode = typeof allAccessModes[number];
type EventAccess = Partial<Record<AccessMode, Access[]>>;
// key is event name
type EventsAccess = Record<string, EventAccess|null>;

const indent = "    ";

let sortedEvents: ims.EventData[];
let accessControlList: EventsAccess|null = null;
// All valid rule expressions (e.g. "person:Tool"), or null if they couldn't be fetched.
let validExpressions: Set<string>|null = null;
let flatpickrIdCounter = 0;
// Names of events whose rule tables are currently expanded.
const expandedEvents = new Set<string>();
// Rules (keyed by event|mode|expression) whose date editors are currently shown.
const openDateEditors = new Set<string>();

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

async function loadAccessTargets(): Promise<void> {
    const {json, err} = await ims.fetchNoThrow<AccessTargets>(url_accessTargets, null);
    if (err != null || json == null) {
        // The typeahead and typo checking are conveniences; the page still works without them.
        console.error(`Failed to load access targets: ${err}`);
        return;
    }
    const expressions: string[] = ["*"];
    for (const person of json.persons??[]) {
        expressions.push(`person:${person}`);
    }
    for (const position of json.positions??[]) {
        expressions.push(`position:${position}`);
    }
    for (const position of json.positions??[]) {
        expressions.push(`onduty:${position}`);
    }
    for (const team of json.teams??[]) {
        expressions.push(`team:${team}`);
    }
    validExpressions = new Set(expressions);
    el.accessTargetList.replaceChildren(...expressions.map((expression: string): HTMLOptionElement => {
        const option = document.createElement("option");
        option.value = expression;
        return option;
    }));
}

function ruleHasIssue(accessEntry: Access): boolean {
    const unknownTarget = accessEntry.debug_info?.known_target !== true;
    const invalidInterval = !!(accessEntry.pending && accessEntry.expired);
    return unknownTarget || invalidInterval;
}

// Expand events that have at least one rule, so their rules (and any issues
// among them) are visible without any clicking around. Empty events stay
// collapsed to keep the page compact.
function expandEventsWithRules(): void {
    if (accessControlList == null) {
        return;
    }
    for (const [event, eventACL] of Object.entries(accessControlList)) {
        for (const mode of allAccessModes) {
            if ((eventACL?.[mode]??[]).length > 0) {
                expandedEvents.add(event);
            }
        }
    }
}

function drawAccess(): void {
    el.eventAccessContainer.replaceChildren();

    if (accessControlList == null) {
        return;
    }

    for (const event of sortedEvents) {
        el.eventAccessContainer.append(eventCard(event));
    }
}

function eventCard(event: ims.EventData): DocumentFragment {
    const cardFrag = el.eventAccessTemplate.content.cloneNode(true) as DocumentFragment;
    const card = cardFrag.querySelector(".event_access") as HTMLElement;
    card.dataset["eventName"] = event.name;

    let eventWithGroupName: string = event.name;
    if (event.is_group) {
        eventWithGroupName = `${eventWithGroupName} (Group)`;
    }
    if (event.parent_group) {
        const parentGroup = sortedEvents.find(value => {return value.id === event.parent_group});
        if (parentGroup) {
            eventWithGroupName += ` (inherits ${parentGroup.name})`;
        }
    }
    card.querySelector(".event_name")!.textContent = eventWithGroupName;

    // Wire up the collapsible rule table, restoring this event's expansion state.
    const collapse = card.querySelector(".access_rules_collapse") as HTMLElement;
    collapse.id = `access_rules_collapse_${event.id}`;
    const collapseToggle = card.querySelector(".access_collapse_toggle") as HTMLButtonElement;
    collapseToggle.setAttribute("data-bs-target", `#${collapse.id}`);
    collapseToggle.setAttribute("aria-controls", collapse.id);
    const chevron = card.querySelector(".access_collapse_chevron") as HTMLElement;
    const expanded = expandedEvents.has(event.name);
    collapseToggle.setAttribute("aria-expanded", expanded.toString());
    chevron.textContent = expanded ? "▾" : "▸";
    if (expanded) {
        collapse.classList.add("show");
    }
    // Track expansion on the "show"/"hide" events, which fire as soon as the
    // toggle is clicked. The after-animation "shown"/"hidden" events may never
    // fire if a rule edit redraws the cards mid-transition, and this set must
    // be current by then for the redrawn card to stay expanded.
    collapse.addEventListener("show.bs.collapse", (): void => {
        expandedEvents.add(event.name);
        collapseToggle.setAttribute("aria-expanded", "true");
        chevron.textContent = "▾";
    });
    collapse.addEventListener("hide.bs.collapse", (): void => {
        expandedEvents.delete(event.name);
        collapseToggle.setAttribute("aria-expanded", "false");
        chevron.textContent = "▸";
    });
    // Let a click anywhere else on the header toggle the collapse too, leaving
    // the header's buttons (event name toggle, Explain, Edit) to handle their own clicks.
    const cardHeader = card.querySelector(".card-header") as HTMLElement;
    cardHeader.addEventListener("click", (e: MouseEvent): void => {
        if ((e.target as HTMLElement).closest("button") == null) {
            collapseToggle.click();
        }
    });

    const editButton = card.querySelector(".show-edit-modal") as HTMLButtonElement;
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

        const mapURLInput = el.editEventModal.querySelector("#edit_map_url") as HTMLInputElement;
        // groups can't have map URLs
        mapURLInput.disabled = event.is_group??false;
        mapURLInput.value = event.map_url??"";

        editEventModal?.show();
    });

    const tbody = card.querySelector(".access_rules") as HTMLTableSectionElement;
    const eventACL: EventAccess|null|undefined = accessControlList?.[event.name];
    let ruleCount = 0;
    let issueCount = 0;
    for (const mode of allAccessModes) {
        const accessEntries = (eventACL?.[mode]??[]).toSorted((a, b) => a.expression.localeCompare(b.expression));
        for (const accessEntry of accessEntries) {
            ruleCount++;
            if (ruleHasIssue(accessEntry)) {
                issueCount++;
            }
            tbody.append(ruleRow(event.name, mode, accessEntry));
        }
    }

    card.querySelector(".rule_count")!.textContent =
        `${ruleCount} ${ruleCount === 1 ? "rule" : "rules"}`;
    if (issueCount > 0) {
        const issueBadge = card.querySelector(".issue_count") as HTMLElement;
        issueBadge.textContent = `${issueCount} ${issueCount === 1 ? "issue" : "issues"}`;
        issueBadge.classList.remove("d-none");
    }

    const explainButton = card.querySelector(".explain_button") as HTMLButtonElement;
    explainButton.addEventListener("click", (_e: MouseEvent): void => {
        el.explainModal.querySelector(".modal-title")!.textContent = `Current permissions for ${event.name}`;
        const explainMsgs = explainMsgsForEvent(event);
        const modalBody = el.explainModal.querySelector(".modal-body")!;
        modalBody.textContent = explainMsgs.length === 0 ? "No permissions" : explainMsgs.join("\n");
        if (event.is_group) {
            modalBody.textContent += "\n\nThis is an event group, so all permissions above also apply to its child events:\n";
            for (const child of sortedEvents) {
                if (child.parent_group === event.id) {
                    modalBody.textContent += `${indent}${child.name}\n`;
                }
            }
        }
        // Show permissions inherited from the parent group, if any.
        if (event.parent_group) {
            const parent = sortedEvents.find(value => value.id === event.parent_group);
            if (parent) {
                const parentMsgs = explainMsgsForEvent(parent);
                modalBody.textContent += `\n\nInherited from parent group "${parent.name}":\n`;
                modalBody.textContent += parentMsgs.length === 0 ? "No permissions" : parentMsgs.join("\n");
            }
        }
        explainModal?.show();
    });

    return cardFrag;
}

// Build the human-readable "Explain" lines for an event: one header per access
// mode, followed by each rule's expression and the users it currently matches.
function explainMsgsForEvent(event: ims.EventData): string[] {
    const eventACL: EventAccess|null|undefined = accessControlList?.[event.name];
    const explainMsgs: string[] = [];
    for (const mode of allAccessModes) {
        const accessEntries = (eventACL?.[mode]??[]).toSorted((a, b) => a.expression.localeCompare(b.expression));
        if (accessEntries.length > 0) {
            explainMsgs.push(`${displayMode(mode)}:`);
        }
        for (const accessEntry of accessEntries) {
            if (!accessEntry.debug_info) {
                continue;
            }
            let unknownSuffix: string = "";
            if (accessEntry.debug_info?.known_target !== true) {
                unknownSuffix = " (unknown target)";
            }

            let intervalSuffix = "";
            if (!accessEntry.pending && !accessEntry.expired) {
                // We're in the interval
                intervalSuffix = "";
            } else if (accessEntry.pending && !accessEntry.expired) {
                intervalSuffix = " (pending)";
            } else if (!accessEntry.pending && accessEntry.expired) {
                intervalSuffix = " (expired)";
            } else {
                intervalSuffix = " (invalid interval)";
            }

            let msg: string = `${indent}${accessEntry.expression} (${accessEntry.validity})${unknownSuffix}${intervalSuffix}\n`;
            if (accessEntry.debug_info.matches_no_one) {
                msg += `${indent}${indent}NO users`;
            } else if (accessEntry.debug_info.matches_all_users) {
                msg += `${indent}${indent}ALL authenticated users`;
            } else {
                msg += indent;
                msg += indent;
                msg += accessEntry.debug_info.matches_users?.join(`\n${indent}${indent}`);
            }
            explainMsgs.push(msg);
        }
    }
    return explainMsgs;
}

function ruleRow(event: string, mode: AccessMode, accessEntry: Access): HTMLTableRowElement {
    const rowFrag = el.accessRuleTemplate.content.cloneNode(true) as DocumentFragment;
    const row = rowFrag.querySelector("tr")!;

    row.dataset["expression"] = accessEntry.expression;
    row.dataset["mode"] = mode;
    row.dataset["validity"] = accessEntry.validity;
    row.dataset["not_after"] = accessEntry.not_after??"";
    row.dataset["not_before"] = accessEntry.not_before??"";

    row.querySelector(".access_expression")!.textContent = accessEntry.expression;

    const levelField = row.querySelector(".access_level") as HTMLSelectElement;
    levelField.value = mode;

    const validityField = row.querySelector(".access_validity") as HTMLSelectElement;
    validityField.value = accessEntry.validity;

    const notBeforeInput = row.querySelector(".access_not_before") as ims.FlatpickrHTMLInputElement;
    ims.newFlatpickr(notBeforeInput, `alt_not_before_${flatpickrIdCounter++}`, (selectedDates) => {
        saveNotBefore(row, selectedDates[0] ?? null);
    });
    if (accessEntry.not_before) {
        notBeforeInput._flatpickr.setDate(new Date(accessEntry.not_before), false, "Z");
    }

    const notAfterInput = row.querySelector(".access_not_after") as ims.FlatpickrHTMLInputElement;
    ims.newFlatpickr(notAfterInput, `alt_not_after_${flatpickrIdCounter++}`, (selectedDates) => {
        saveNotAfter(row, selectedDates[0] ?? null);
    });
    if (accessEntry.not_after) {
        notAfterInput._flatpickr.setDate(new Date(accessEntry.not_after), false, "Z");
    }

    // Most rules have no dates, so the date pickers stay hidden until the rule
    // has a date range (date badge) or the user asks for one ("Set dates" button).
    const datesKey = `${event}|${mode}|${accessEntry.expression}`;
    const datesSpan = row.querySelector(".access_dates") as HTMLElement;
    const datesToggle = row.querySelector(".access_dates_toggle") as HTMLButtonElement;
    const datesBadge = row.querySelector(".access_dates_badge") as HTMLButtonElement;
    const showDateEditors = (): void => {
        datesSpan.classList.remove("d-none");
        datesToggle.classList.add("d-none");
        datesBadge.classList.add("d-none");
        openDateEditors.add(datesKey);
    };
    datesToggle.addEventListener("click", showDateEditors);
    datesBadge.addEventListener("click", showDateEditors);
    if (accessEntry.not_before || accessEntry.not_after) {
        const notBefore = accessEntry.not_before ? notBeforeInput._flatpickr.altInput!.value : null;
        const notAfter = accessEntry.not_after ? notAfterInput._flatpickr.altInput!.value : null;
        if (notBefore && notAfter) {
            datesBadge.textContent = `${notBefore} → ${notAfter}`;
        } else if (notBefore) {
            datesBadge.textContent = `from ${notBefore}`;
        } else {
            datesBadge.textContent = `until ${notAfter}`;
        }
        datesBadge.classList.remove("d-none");
    } else {
        datesToggle.classList.remove("d-none");
    }
    if (openDateEditors.has(datesKey)) {
        showDateEditors();
    }

    const intervalText = row.querySelector(".access_interval_text") as HTMLSpanElement;
    let intervalStatus = "";
    if (accessEntry.pending && !accessEntry.expired) {
        intervalStatus = "Pending";
    } else if (!accessEntry.pending && accessEntry.expired) {
        intervalStatus = "Expired";
    } else if (accessEntry.pending && accessEntry.expired) {
        intervalStatus = "Invalid interval";
    }
    if (intervalStatus) {
        intervalText.textContent = intervalStatus;
        intervalText.classList.remove("d-none");
    }

    if (accessEntry.debug_info?.known_target !== true) {
        row.classList.add("table-danger");
        row.querySelector(".unknown_target_text")!.classList.remove("d-none");
        const fixButton = row.querySelector(".fix_button") as HTMLButtonElement;
        fixButton.classList.remove("d-none");
        const fixInput = row.querySelector(".access_fix") as HTMLInputElement;
        fixButton.addEventListener("click", (): void => {
            row.querySelector(".access_expression")!.classList.add("d-none");
            fixInput.value = accessEntry.expression;
            fixInput.classList.remove("d-none");
            fixInput.focus();
        });
    }

    return row;
}

function displayMode(m: AccessMode): string {
    switch (m) {
        case "readers":
            return "Read all";
        case "writers":
            return "Write all";
        case "reporters":
            return "Report own";
        case "visit_writers":
            return "Write visits";
        default:
            throw new Error(`unexpected access mode ${m satisfies never}`);
    }
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
    // Expand the new event, so the admin can start adding rules to it.
    expandedEvents.add(event);
    await loadAccessControlList();
    drawAccess();
    sender.value = "";  // Clear input field
}

// confirmExpression warns about suspicious-looking expressions, returning
// false if the user decided not to proceed with one.
function confirmExpression(expression: string): boolean {
    if (expression === "**") {
        return confirm(
            "Double-wildcard '**' ACLs are no longer supported, so this ACL will have " +
            "no effect.\n\n" +
            "Proceed with doing something pointless?"
        );
    }

    const validPrefix = expression === "*" ||
        expression.startsWith("person:") || expression.startsWith("position:") ||
        expression.startsWith("team:") || expression.startsWith("onduty:");
    if (!validPrefix) {
        return confirm(
            "WARNING: '" + expression + "' does not look like a valid ACL " +
            "expression. Example expressions include 'person:Hubcap' for an individual, " +
            "'position:007' for a role, 'onduty:007' for people currently on duty for a position, " +
            "and 'team:Council' for a team. Wildcards are " +
            "supported as well, e.g. '*'\n\n" +
            "Proceed with firing footgun?"
        );
    }

    if (validExpressions != null && !validExpressions.has(expression)) {
        return confirm(
            "'" + expression + "' does not match any known person, position, or team, " +
            "so this rule won't grant access to anyone. It will be flagged as an " +
            "issue on this page.\n\n" +
            "Add it anyway?"
        );
    }

    return true;
}

async function addAccess(sender: HTMLInputElement): Promise<void> {
    const card: HTMLElement = sender.closest(".event_access")!;
    const event = card.dataset["eventName"]!;
    const mode = (card.querySelector(".access_add_level") as HTMLSelectElement).value as AccessMode;
    const newExpression = sender.value.trim();

    if (newExpression === "") {
        return;
    }

    if (!confirmExpression(newExpression)) {
        sender.value = "";
        return;
    }

    let acl: Access[] = (accessControlList![event]![mode]??[]).slice();

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
    if (err != null) {
        ims.controlHasError(sender);
        return;
    }
    await loadAccessControlList();
    drawAccess();
    // Put focus back on this event's add field, to ease adding several rules in a row.
    const newCard = findEventCard(event);
    (newCard?.querySelector(".access_add") as HTMLInputElement|null)?.focus();
}

function findEventCard(event: string): HTMLElement|null {
    for (const card of el.eventAccessContainer.querySelectorAll<HTMLElement>(".event_access")) {
        if (card.dataset["eventName"] === event) {
            return card;
        }
    }
    return null;
}

// fixAccess replaces a rule's expression, keeping its other fields, e.g. to correct a typo.
async function fixAccess(sender: HTMLInputElement): Promise<void> {
    const card: HTMLElement = sender.closest(".event_access")!;
    const event = card.dataset["eventName"]!;
    const row = sender.closest("tr")!;
    const mode = row.dataset["mode"] as AccessMode;
    const oldExpression = row.dataset["expression"]!;
    const newExpression = sender.value.trim();

    if (newExpression === "" || newExpression === oldExpression) {
        drawAccess();
        return;
    }

    if (!confirmExpression(newExpression)) {
        drawAccess();
        return;
    }

    let acl: Access[] = (accessControlList![event]![mode]??[]).slice();
    acl = acl.filter((v: Access): boolean => {
        return v.expression !== oldExpression && v.expression !== newExpression;
    });

    const newVal: Access = {
        "expression": newExpression,
        "validity": row.dataset["validity"] === "onsite" ? Validity.onsite : Validity.always,
        "not_after": row.dataset["not_after"]||null,
        "not_before": row.dataset["not_before"]||null,
    };

    acl.push(newVal);

    const edits: EventsAccess = {};
    edits[event] = {};
    edits[event][mode] = acl;

    const {err} = await sendACL(edits);
    if (err != null) {
        ims.controlHasError(sender);
        return;
    }
    await loadAccessControlList();
    drawAccess();
}

async function removeAccess(sender: HTMLButtonElement): Promise<void> {
    const card: HTMLElement = sender.closest(".event_access")!;
    const event = card.dataset["eventName"]!;
    const row = sender.closest("tr")!;
    const mode = row.dataset["mode"] as AccessMode;
    const expression = row.dataset["expression"]!.trim();

    const acl: Access[] = (accessControlList![event]![mode]??[]).slice();

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
    drawAccess();
}

// setLevel moves a rule to a different access mode, keeping its other fields.
async function setLevel(sender: HTMLSelectElement): Promise<void> {
    const card: HTMLElement = sender.closest(".event_access")!;
    const event = card.dataset["eventName"]!;
    const row = sender.closest("tr")!;
    const oldMode = row.dataset["mode"] as AccessMode;
    const newMode = sender.value as AccessMode;
    if (newMode === oldMode) {
        return;
    }
    const expression = row.dataset["expression"]!.trim();

    const oldAcl: Access[] = (accessControlList![event]![oldMode]??[]).filter(
        (v: Access): boolean => {return v.expression !== expression});
    const newAcl: Access[] = (accessControlList![event]![newMode]??[]).filter(
        (v: Access): boolean => {return v.expression !== expression});

    newAcl.push({
        "expression": expression,
        "validity": row.dataset["validity"] === "onsite" ? Validity.onsite : Validity.always,
        "not_after": row.dataset["not_after"]||null,
        "not_before": row.dataset["not_before"]||null,
    });

    const edits: EventsAccess = {};
    edits[event] = {};
    edits[event][oldMode] = oldAcl;
    edits[event][newMode] = newAcl;

    // The two modes are updated in separate transactions server-side, so reload
    // and redraw even on error, in case only one of them was applied.
    await sendACL(edits);
    await loadAccessControlList();
    drawAccess();
}

async function setValidity(sender: HTMLSelectElement): Promise<void> {
    const card: HTMLElement = sender.closest(".event_access")!;
    const event = card.dataset["eventName"]!;
    const row = sender.closest("tr")!;
    const mode = row.dataset["mode"] as AccessMode;

    const expression = row.dataset["expression"]!.trim();
    const notAfter = row.dataset["not_after"]||null;
    const notBefore = row.dataset["not_before"]||null;

    let acl: Access[] = (accessControlList![event]![mode]??[]).slice();

    // remove other acls for this mode for the same expression
    acl = acl.filter((v: Access): boolean => {return v.expression !== expression});

    const newVal: Access = {
        "expression": expression,
        "validity": sender.value === "onsite" ? Validity.onsite : Validity.always,
        "not_after": notAfter,
        "not_before": notBefore,
    };

    acl.push(newVal);

    const edits: EventsAccess = {};
    edits[event] = {};
    edits[event][mode] = acl;

    const {err} = await sendACL(edits);
    if (err != null) {
        ims.controlHasError(sender);
        return;
    }
    await loadAccessControlList();
    drawAccess();
}

async function saveNotAfter(row: HTMLTableRowElement, date: Date|null): Promise<void> {
    const card: HTMLElement = row.closest(".event_access")!;
    const event = card.dataset["eventName"]!;
    const mode = row.dataset["mode"] as AccessMode;
    const expression = row.dataset["expression"]!.trim();
    const validity = row.dataset["validity"]!.trim();
    const notBefore = row.dataset["not_before"]||null;

    let acl: Access[] = (accessControlList![event]![mode]??[]).slice();
    acl = acl.filter((v: Access): boolean => {return v.expression !== expression});

    const notAfter: string|null = date?.toISOString() ?? null;
    if (notAfter) {
        console.log(`Setting not-after to ${notAfter}`);
    } else {
        console.log("Unsetting not-after");
    }

    const newVal: Access = {
        "expression": expression,
        "validity": validity === "onsite" ? Validity.onsite : Validity.always,
        "not_after": notAfter,
        "not_before": notBefore,
    };

    acl.push(newVal);

    const edits: EventsAccess = {};
    edits[event] = {};
    edits[event][mode] = acl;

    const {err} = await sendACL(edits);
    if (err != null) {
        ims.controlHasError(row.getElementsByClassName("access_not_after")[0] as HTMLElement);
        return;
    }
    await loadAccessControlList();
    drawAccess();
}

async function saveNotBefore(row: HTMLTableRowElement, date: Date|null): Promise<void> {
    const card: HTMLElement = row.closest(".event_access")!;
    const event = card.dataset["eventName"]!;
    const mode = row.dataset["mode"] as AccessMode;
    const expression = row.dataset["expression"]!.trim();
    const validity = row.dataset["validity"]!.trim();
    const notAfter = row.dataset["not_after"]||null;

    let acl: Access[] = (accessControlList![event]![mode]??[]).slice();
    acl = acl.filter((v: Access): boolean => {return v.expression !== expression});

    const notBefore: string|null = date?.toISOString() ?? null;
    if (notBefore) {
        console.log(`Setting not-before to ${notBefore}`);
    } else {
        console.log("Unsetting not-before");
    }

    const newVal: Access = {
        "expression": expression,
        "validity": validity === "onsite" ? Validity.onsite : Validity.always,
        "not_after": notAfter,
        "not_before": notBefore,
    };

    acl.push(newVal);

    const edits: EventsAccess = {};
    edits[event] = {};
    edits[event][mode] = acl;

    const {err} = await sendACL(edits);
    if (err != null) {
        ims.controlHasError(row.getElementsByClassName("access_not_before")[0] as HTMLElement);
        return;
    }
    await loadAccessControlList();
    drawAccess();
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

async function setMapURL(sender: HTMLInputElement): Promise<void> {
    const eventId = ims.parseInt10(el.editEventModal.dataset["eventId"])!;

    const requestBod: ims.EventData = {
        id: eventId,
        // @ts-expect-error the server is fine to receive null here. Really this field should allow null/undefined.
        name: null,
        map_url: sender.value,
    };
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
