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
        addEvent: (el: HTMLInputElement, type: "group"|"not-group")=>Promise<void>;
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
    dateFormatExample: ims.typedElement("date_format_example", HTMLElement),
    explainModal: ims.typedElement("explainModal", HTMLElement),
    editEventModal: ims.typedElement("editEventModal", HTMLElement),
    eventAccessContainer: ims.typedElement("event_access_container", HTMLElement),
    eventAccessTemplate: ims.typedElement("event_access_template", HTMLTemplateElement),
    grantTemplate: ims.typedElement("grant_template", HTMLTemplateElement),
    whoChipTemplate: ims.typedElement("who_chip_template", HTMLTemplateElement),
    accessTargetList: ims.typedElement("access_target_list", HTMLDataListElement),
    eventDeleteWrapper: ims.typedElement("event_delete_wrapper", HTMLElement),
    eventDelete: ims.typedElement("event_delete", HTMLButtonElement),
};

initAdminEventsPage();

let eventDeletionAllowed = false;

async function initAdminEventsPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    eventDeletionAllowed = initResult.authInfo.event_deletion_allowed??false;
    el.eventDelete.disabled = !eventDeletionAllowed;
    el.eventDeleteWrapper.title = eventDeletionAllowed
        ? "Delete this event and all data associated with it"
        : "Event deletion is disabled on this server. It can be enabled by " +
          "setting IMS_EVENT_DELETION_ENABLED=true in the server configuration.";
    el.eventDelete.addEventListener("click", deleteEvent);

    window.addEvent = addEvent;
    window.setParentGroup = setParentGroup;
    window.setMapURL = setMapURL;

    el.browserTz.textContent = Intl.DateTimeFormat().resolvedOptions().timeZone;
    // Show the current time as the example, so it's useful to copy-paste from.
    el.dateFormatExample.textContent = formatDateForInput(new Date());

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
    description?: string|null;
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

// A Grant is a group of access rules that share an access level, validity, date
// window, and description. Its members ("whos") are the individual targets. The
// stored ACL has no notion of a grant; grants are derived from the rules on
// load, and a grant with N whos fans out into N per-target rules on save. The
// description is a shared, grant-level note, so every who in a grant carries the
// same value and editing it rewrites them all.
interface Grant {
    key: string;
    mode: AccessMode;
    validity: Validity;
    notBefore: string|null;
    notAfter: string|null;
    description: string;
    whos: Access[];
}

// A DraftGrant is a grant the admin is composing but hasn't populated yet. It
// has no stored rules until its first who is added, so it lives only in the UI.
interface DraftGrant {
    id: number;
    mode: AccessMode;
    validity: Validity;
    notBefore: string|null;
    notAfter: string|null;
    description: string;
}

const indent = "    ";

let sortedEvents: ims.EventData[];
let accessControlList: EventsAccess|null = null;
// All valid rule expressions (e.g. "person:Tool"), or null if they couldn't be fetched.
let validExpressions: Set<string>|null = null;
// Names of events whose grant lists are currently expanded.
const expandedEvents = new Set<string>();
// Draft grants being composed, keyed by event name.
const draftGrants = new Map<string, DraftGrant[]>();
let draftIdCounter = 0;

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

// Expand events that have at least one rule, so their grants (and any issues
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

//
// Grant grouping
//

function grantKey(mode: AccessMode, validity: Validity, notBefore: string|null, notAfter: string|null, description: string): string {
    // The description is free text and can contain any character, so JSON-encode
    // the whole tuple rather than joining on a separator. The result is still a
    // valid data-attribute value (and CSS.escape handles it in selectors).
    return JSON.stringify([mode, validity, notBefore??"", notAfter??"", description]);
}

// Group an event's stored rules into grants keyed by (mode, validity, dates,
// description).
function grantsForEvent(eventName: string): Grant[] {
    const eventACL = accessControlList?.[eventName];
    const byKey = new Map<string, Grant>();
    for (const mode of allAccessModes) {
        for (const a of eventACL?.[mode]??[]) {
            const validity = a.validity === Validity.onsite ? Validity.onsite : Validity.always;
            const notBefore = a.not_before??null;
            const notAfter = a.not_after??null;
            const description = a.description??"";
            const key = grantKey(mode, validity, notBefore, notAfter, description);
            let g = byKey.get(key);
            if (g == null) {
                g = {key, mode, validity, notBefore, notAfter, description, whos: []};
                byKey.set(key, g);
            }
            g.whos.push(a);
        }
    }
    const grants = [...byKey.values()];
    for (const g of grants) {
        g.whos.sort((a, b) => a.expression.localeCompare(b.expression));
    }
    grants.sort((a, b) => {
        const am = allAccessModes.indexOf(a.mode);
        const bm = allAccessModes.indexOf(b.mode);
        if (am !== bm) {
            return am - bm;
        }
        if (a.validity !== b.validity) {
            return a.validity === Validity.always ? -1 : 1;
        }
        const anb = a.notBefore??"";
        const bnb = b.notBefore??"";
        if (anb !== bnb) {
            return anb.localeCompare(bnb);
        }
        return (a.notAfter??"").localeCompare(b.notAfter??"");
    });
    return grants;
}

//
// Rendering
//

function showFlex(element: HTMLElement): void {
    element.classList.remove("d-none");
    element.classList.add("d-flex");
}
function hideFlex(element: HTMLElement): void {
    element.classList.add("d-none");
    element.classList.remove("d-flex");
}
function show(element: HTMLElement): void {
    element.classList.remove("d-none");
}
function hide(element: HTMLElement): void {
    element.classList.add("d-none");
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
            eventWithGroupName += ` (extends ${parentGroup.name})`;
        }
    }
    card.querySelector(".event_name")!.textContent = eventWithGroupName;

    // Wire up the collapsible grant list, restoring this event's expansion state.
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
        el.editEventModal.dataset["eventName"] = event.name;

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

    // Build the grant blocks, plus any drafts the admin is composing.
    const grantsContainer = card.querySelector(".grants_container") as HTMLElement;
    const grants = grantsForEvent(event.name);
    let peopleCount = 0;
    let issueCount = 0;
    for (const grant of grants) {
        for (const who of grant.whos) {
            peopleCount++;
            if (ruleHasIssue(who)) {
                issueCount++;
            }
        }
        grantsContainer.append(grantBlock(event.name, grant, null));
    }
    for (const draft of draftGrants.get(event.name)??[]) {
        grantsContainer.append(grantBlock(event.name, null, draft));
    }

    const grantWord = grants.length === 1 ? "grant" : "grants";
    const peopleWord = peopleCount === 1 ? "person" : "people";
    card.querySelector(".rule_count")!.textContent =
        `${grants.length} ${grantWord} · ${peopleCount} ${peopleWord}`;
    if (issueCount > 0) {
        const issueBadge = card.querySelector(".issue_count") as HTMLElement;
        issueBadge.textContent = `${issueCount} ${issueCount === 1 ? "issue" : "issues"}`;
        issueBadge.classList.remove("d-none");
    }

    const newGrantButton = card.querySelector(".new_grant_button") as HTMLButtonElement;
    newGrantButton.addEventListener("click", (): void => newGrant(event.name));

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

// grantBlock renders one grant. Exactly one of `grant` (an existing grant) or
// `draft` (a new one being composed) is non-null.
function grantBlock(eventName: string, grant: Grant|null, draft: DraftGrant|null): DocumentFragment {
    const frag = el.grantTemplate.content.cloneNode(true) as DocumentFragment;
    const grantEl = frag.querySelector(".grant") as HTMLElement;

    const mode = (grant ?? draft)!.mode;
    const validity = (grant ?? draft)!.validity;
    const notBefore = (grant ?? draft)!.notBefore;
    const notAfter = (grant ?? draft)!.notAfter;
    const description = (grant ?? draft)!.description;

    // The dataset carries the grant's terms. For a real grant these are the
    // current stored terms (addWho reads them; applyGrantTerms treats them as
    // the "old" terms to replace). For a draft they track the edit controls.
    grantEl.dataset["mode"] = mode;
    grantEl.dataset["validity"] = validity;
    grantEl.dataset["not_before"] = notBefore??"";
    grantEl.dataset["not_after"] = notAfter??"";
    grantEl.dataset["description"] = description;

    const display = grantEl.querySelector(".grant_terms_display") as HTMLElement;
    const editTerms = grantEl.querySelector(".grant_terms_edit") as HTMLElement;
    const levelSelect = grantEl.querySelector(".grant_level") as HTMLSelectElement;
    const validitySelect = grantEl.querySelector(".grant_validity") as HTMLSelectElement;
    const notBeforeInput = grantEl.querySelector(".grant_not_before") as HTMLInputElement;
    const notAfterInput = grantEl.querySelector(".grant_not_after") as HTMLInputElement;
    const descriptionInput = grantEl.querySelector(".grant_description") as HTMLInputElement;

    // Populate the edit controls (hidden for a real grant until "Edit terms").
    levelSelect.value = mode;
    validitySelect.value = validity;
    if (notBefore) {
        notBeforeInput.value = formatDateForInput(new Date(notBefore));
    }
    if (notAfter) {
        notAfterInput.value = formatDateForInput(new Date(notAfter));
    }
    descriptionInput.value = description;

    const editButton = grantEl.querySelector(".grant_edit_terms") as HTMLButtonElement;
    const applyButton = grantEl.querySelector(".grant_apply_terms") as HTMLButtonElement;
    const cancelButton = grantEl.querySelector(".grant_cancel_terms") as HTMLButtonElement;
    const removeDraftButton = grantEl.querySelector(".grant_remove_draft") as HTMLButtonElement;

    if (grant != null) {
        grantEl.dataset["grantKey"] = grant.key;
        renderGrantTermsDisplay(grantEl, grant);

        // "Edit terms" swaps the badges for the edit controls.
        editButton.addEventListener("click", (): void => {
            hideFlex(display);
            showFlex(editTerms);
            hide(editButton);
            show(applyButton);
            show(cancelButton);
        });
        cancelButton.addEventListener("click", (): void => drawAccess());
        applyButton.addEventListener("click", (): void => void applyGrantTerms(grantEl));

        // Render the grant's whos as chips before the "add who" input.
        const whoChips = grantEl.querySelector(".who_chips") as HTMLElement;
        const addInput = whoChips.querySelector(".who_add") as HTMLInputElement;
        for (const who of grant.whos) {
            whoChips.insertBefore(whoChip(grantEl, who), addInput);
        }
    } else if (draft != null) {
        grantEl.dataset["draftId"] = draft.id.toString();
        grantEl.classList.add("grant-draft");
        // A draft is composed in edit mode from the start, and its term controls
        // feed straight into the dataset (and draft object) as they change.
        hideFlex(display);
        showFlex(editTerms);
        hide(editButton);
        show(removeDraftButton);
        removeDraftButton.addEventListener("click", (): void => {
            removeDraft(eventName, draft.id);
            drawAccess();
        });
        const syncDraftTerms = (): void => {
            draft.mode = levelSelect.value as AccessMode;
            grantEl.dataset["mode"] = draft.mode;
            draft.validity = validitySelect.value === "onsite" ? Validity.onsite : Validity.always;
            grantEl.dataset["validity"] = draft.validity;
            const nb = parseDateInput(notBeforeInput.value);
            if (nb === undefined) {
                ims.controlHasError(notBeforeInput);
            } else {
                draft.notBefore = nb?.toISOString()??null;
                grantEl.dataset["not_before"] = draft.notBefore??"";
            }
            const na = parseDateInput(notAfterInput.value);
            if (na === undefined) {
                ims.controlHasError(notAfterInput);
            } else {
                draft.notAfter = na?.toISOString()??null;
                grantEl.dataset["not_after"] = draft.notAfter??"";
            }
            draft.description = descriptionInput.value;
            grantEl.dataset["description"] = draft.description;
        };
        levelSelect.addEventListener("change", syncDraftTerms);
        validitySelect.addEventListener("change", syncDraftTerms);
        notBeforeInput.addEventListener("change", syncDraftTerms);
        notAfterInput.addEventListener("change", syncDraftTerms);
        descriptionInput.addEventListener("change", syncDraftTerms);
    }

    const addInput = grantEl.querySelector(".who_add") as HTMLInputElement;
    addInput.addEventListener("change", (): void => void addWho(grantEl));

    return frag;
}

// renderGrantTermsDisplay fills in the read-only badges for a grant's terms.
function renderGrantTermsDisplay(grantEl: HTMLElement, grant: Grant): void {
    (grantEl.querySelector(".grant_level_badge") as HTMLElement).textContent = displayMode(grant.mode);

    const whenBadge = grantEl.querySelector(".grant_when_badge") as HTMLElement;
    if (grant.validity === Validity.onsite) {
        whenBadge.textContent = "On-Site";
        whenBadge.classList.add("text-bg-warning");
    } else {
        whenBadge.textContent = "Always";
        whenBadge.classList.add("text-bg-secondary");
    }

    if (grant.notBefore) {
        const badge = grantEl.querySelector(".grant_not_before_badge") as HTMLElement;
        badge.textContent = `not-before ${formatDateForInput(new Date(grant.notBefore))}`;
        badge.classList.remove("d-none");
    }
    if (grant.notAfter) {
        const badge = grantEl.querySelector(".grant_not_after_badge") as HTMLElement;
        badge.textContent = `not-after ${formatDateForInput(new Date(grant.notAfter))}`;
        badge.classList.remove("d-none");
    }

    // pending/expired are computed server-side from the grant's dates, so every
    // who in the grant shares the same interval status; take it from the first.
    const first = grant.whos[0];
    let intervalStatus = "";
    if (first?.pending && first.expired) {
        intervalStatus = "Invalid interval";
    } else if (first?.pending) {
        intervalStatus = "Pending";
    } else if (first?.expired) {
        intervalStatus = "Expired";
    }
    if (intervalStatus) {
        const badge = grantEl.querySelector(".grant_interval_badge") as HTMLElement;
        badge.textContent = intervalStatus;
        badge.classList.remove("d-none");
    }

    if (grant.description) {
        const descriptionEl = grantEl.querySelector(".grant_description_display") as HTMLElement;
        descriptionEl.textContent = `"${grant.description}"`;
        descriptionEl.classList.remove("d-none");
    }
}

// whoChip renders one target within a grant, with remove and (if the target is
// unknown) fix affordances.
function whoChip(grantEl: HTMLElement, who: Access): DocumentFragment {
    const frag = el.whoChipTemplate.content.cloneNode(true) as DocumentFragment;
    const chip = frag.querySelector(".who-chip") as HTMLElement;
    const expressionSpan = chip.querySelector(".who_expression") as HTMLElement;
    expressionSpan.textContent = who.expression;

    const removeButton = chip.querySelector(".who-remove") as HTMLButtonElement;
    removeButton.addEventListener("click", (): void => void removeWho(grantEl, who.expression));

    if (who.debug_info?.known_target !== true) {
        chip.classList.add("who-unknown");
        chip.title = "Unknown target: doesn't match any known person, position, or team";
        const fixButton = chip.querySelector(".who_fix") as HTMLButtonElement;
        const fixInput = chip.querySelector(".who_fix_input") as HTMLInputElement;
        fixButton.classList.remove("d-none");
        fixButton.addEventListener("click", (): void => {
            hide(expressionSpan);
            hide(fixButton);
            fixInput.value = who.expression;
            fixInput.classList.remove("d-none");
            fixInput.focus();
        });
        fixInput.addEventListener("change", (): void => void fixWho(grantEl, who.expression, fixInput));
    }

    return frag;
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

// Format a date for display in a grant's date input, in the browser's time zone,
// e.g. "Sun 2026-08-23 @ 12:00". This matches the format parseDateInput accepts.
function formatDateForInput(date: Date): string {
    const weekday = new Intl.DateTimeFormat("en-US", {weekday: "short"}).format(date);
    return `${weekday} ${ims.localDateISO(date)} @ ${ims.localTimeHHMM(date)}`;
}

// Parse a human-entered datetime into a Date in the browser's time zone. Returns
// null for empty input (the date is being cleared), or undefined if the input
// couldn't be parsed. It's lenient: an optional weekday prefix and "@" separator
// are ignored, so it round-trips formatDateForInput's output, and the time is
// optional (defaulting to midnight).
function parseDateInput(input: string): Date|null|undefined {
    const trimmed = input.trim();
    if (trimmed === "") {
        return null;
    }
    const match = trimmed.match(/(\d{4})-(\d{1,2})-(\d{1,2})(?:[\sT@]+(\d{1,2}):(\d{2}))?\s*$/);
    if (match == null) {
        return undefined;
    }
    const year = ims.parseInt10(match[1]!)!;
    const month = ims.parseInt10(match[2]!)!;
    const day = ims.parseInt10(match[3]!)!;
    const hour = match[4] != null ? ims.parseInt10(match[4])! : 0;
    const minute = match[5] != null ? ims.parseInt10(match[5])! : 0;
    if (month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59) {
        return undefined;
    }
    const date = new Date(year, month - 1, day, hour, minute);
    if (Number.isNaN(date.getTime())) {
        return undefined;
    }
    return date;
}

//
// Grant / who mutations
//

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
    // Expand the new event, so the admin can start adding grants to it.
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

// grantTerms reads a grant element's current terms from its dataset.
function grantTerms(grantEl: HTMLElement): {mode: AccessMode; validity: Validity; notBefore: string|null; notAfter: string|null; description: string} {
    return {
        mode: grantEl.dataset["mode"] as AccessMode,
        validity: grantEl.dataset["validity"] === "onsite" ? Validity.onsite : Validity.always,
        notBefore: grantEl.dataset["not_before"] || null,
        notAfter: grantEl.dataset["not_after"] || null,
        description: grantEl.dataset["description"] ?? "",
    };
}

function eventNameOf(grantEl: HTMLElement): string {
    return (grantEl.closest(".event_access") as HTMLElement).dataset["eventName"]!;
}

// addWho adds one target to the grant identified by grantEl, using that grant's
// current terms. For a draft grant, this is what turns it into a real one.
async function addWho(grantEl: HTMLElement): Promise<void> {
    const addInput = grantEl.querySelector(".who_add") as HTMLInputElement;
    const newExpression = addInput.value.trim();
    if (newExpression === "") {
        return;
    }
    if (!confirmExpression(newExpression)) {
        addInput.value = "";
        return;
    }

    const event = eventNameOf(grantEl);
    const {mode, validity, notBefore, notAfter, description} = grantTerms(grantEl);

    let acl: Access[] = (accessControlList?.[event]?.[mode]??[]).slice();
    // Remove any existing rule for this expression in this mode, then add ours.
    acl = acl.filter((v: Access): boolean => v.expression !== newExpression);
    acl.push({
        "expression": newExpression,
        "validity": validity,
        "not_before": notBefore,
        "not_after": notAfter,
        "description": description,
    });

    const edits: EventsAccess = {};
    edits[event] = {};
    edits[event]![mode] = acl;

    const {err} = await sendACL(edits);
    if (err != null) {
        ims.controlHasError(addInput);
        return;
    }
    // A matching real grant now exists, so drop the draft if this was one.
    const draftId = grantEl.dataset["draftId"];
    if (draftId != null) {
        removeDraft(event, ims.parseInt10(draftId)!);
    }
    const key = grantKey(mode, validity, notBefore, notAfter, description);
    await loadAccessControlList();
    drawAccess();
    // Put focus back on this grant's add field, to ease adding several in a row.
    focusGrantAdd(event, key);
}

async function removeWho(grantEl: HTMLElement, expression: string): Promise<void> {
    const event = eventNameOf(grantEl);
    const {mode} = grantTerms(grantEl);

    const acl: Access[] = (accessControlList?.[event]?.[mode]??[])
        .filter((v: Access): boolean => v.expression !== expression);

    const edits: EventsAccess = {};
    edits[event] = {};
    edits[event]![mode] = acl;

    await sendACL(edits);
    await loadAccessControlList();
    drawAccess();
}

// fixWho replaces a target's expression, keeping the grant's terms, e.g. to
// correct a typo on an unknown target.
async function fixWho(grantEl: HTMLElement, oldExpression: string, sender: HTMLInputElement): Promise<void> {
    const newExpression = sender.value.trim();
    if (newExpression === "" || newExpression === oldExpression) {
        drawAccess();
        return;
    }
    if (!confirmExpression(newExpression)) {
        drawAccess();
        return;
    }

    const event = eventNameOf(grantEl);
    const {mode, validity, notBefore, notAfter, description} = grantTerms(grantEl);

    let acl: Access[] = (accessControlList?.[event]?.[mode]??[]).slice();
    acl = acl.filter((v: Access): boolean => v.expression !== oldExpression && v.expression !== newExpression);
    acl.push({
        "expression": newExpression,
        "validity": validity,
        "not_before": notBefore,
        "not_after": notAfter,
        "description": description,
    });

    const edits: EventsAccess = {};
    edits[event] = {};
    edits[event]![mode] = acl;

    const {err} = await sendACL(edits);
    if (err != null) {
        ims.controlHasError(sender);
        return;
    }
    await loadAccessControlList();
    drawAccess();
}

// applyGrantTerms rewrites every who in a grant to a new set of terms, read from
// the grant's edit controls. When the level changes, this moves the whos from
// one access mode to another.
async function applyGrantTerms(grantEl: HTMLElement): Promise<void> {
    const event = eventNameOf(grantEl);
    const {mode: oldMode, validity: oldValidity, notBefore: oldNotBefore, notAfter: oldNotAfter, description: oldDescription} = grantTerms(grantEl);

    const levelSelect = grantEl.querySelector(".grant_level") as HTMLSelectElement;
    const validitySelect = grantEl.querySelector(".grant_validity") as HTMLSelectElement;
    const notBeforeInput = grantEl.querySelector(".grant_not_before") as HTMLInputElement;
    const notAfterInput = grantEl.querySelector(".grant_not_after") as HTMLInputElement;
    const descriptionInput = grantEl.querySelector(".grant_description") as HTMLInputElement;

    const newMode = levelSelect.value as AccessMode;
    const newValidity = validitySelect.value === "onsite" ? Validity.onsite : Validity.always;
    const newDescription = descriptionInput.value;

    const nb = parseDateInput(notBeforeInput.value);
    if (nb === undefined) {
        ims.controlHasError(notBeforeInput);
        return;
    }
    const na = parseDateInput(notAfterInput.value);
    if (na === undefined) {
        ims.controlHasError(notAfterInput);
        return;
    }
    const newNotBefore = nb?.toISOString()??null;
    const newNotAfter = na?.toISOString()??null;

    // Nothing changed: just leave edit mode.
    if (newMode === oldMode && newValidity === oldValidity
        && newNotBefore === oldNotBefore && newNotAfter === oldNotAfter
        && newDescription === oldDescription) {
        drawAccess();
        return;
    }

    const oldModeAll = accessControlList?.[event]?.[oldMode]??[];
    const inGrant = (a: Access): boolean => sameTerms(a, oldValidity, oldNotBefore, oldNotAfter, oldDescription);
    const expressions = oldModeAll.filter(inGrant).map(a => a.expression);
    const newEntries: Access[] = expressions.map((expression): Access => ({
        "expression": expression,
        "validity": newValidity,
        "not_before": newNotBefore,
        "not_after": newNotAfter,
        "description": newDescription,
    }));

    const edits: EventsAccess = {};
    edits[event] = {};
    if (newMode === oldMode) {
        // Keep the mode's other grants; replace this grant's entries in place.
        let list = oldModeAll.filter(a => !inGrant(a));
        list = list.filter(a => !expressions.includes(a.expression));
        list.push(...newEntries);
        edits[event]![oldMode] = list;
    } else {
        edits[event]![oldMode] = oldModeAll.filter(a => !inGrant(a));
        let newList = (accessControlList?.[event]?.[newMode]??[]).slice();
        newList = newList.filter(a => !expressions.includes(a.expression));
        newList.push(...newEntries);
        edits[event]![newMode] = newList;
    }

    // The two modes are updated in separate transactions server-side, so reload
    // and redraw even on error, in case only one of them was applied.
    await sendACL(edits);
    await loadAccessControlList();
    drawAccess();
}

function sameTerms(a: Access, validity: Validity, notBefore: string|null, notAfter: string|null, description: string): boolean {
    return a.validity === validity
        && (a.not_before??null) === (notBefore||null)
        && (a.not_after??null) === (notAfter||null)
        && (a.description??"") === description;
}

// newGrant starts composing a new, empty grant on the given event.
function newGrant(eventName: string): void {
    const draft: DraftGrant = {
        id: ++draftIdCounter,
        mode: "readers",
        validity: Validity.always,
        notBefore: null,
        notAfter: null,
        description: "",
    };
    const list = draftGrants.get(eventName)??[];
    list.push(draft);
    draftGrants.set(eventName, list);
    expandedEvents.add(eventName);
    drawAccess();
    focusDraftAdd(eventName, draft.id);
}

function removeDraft(eventName: string, id: number): void {
    const list = (draftGrants.get(eventName)??[]).filter(d => d.id !== id);
    if (list.length === 0) {
        draftGrants.delete(eventName);
    } else {
        draftGrants.set(eventName, list);
    }
}

function findEventCard(event: string): HTMLElement|null {
    for (const card of el.eventAccessContainer.querySelectorAll<HTMLElement>(".event_access")) {
        if (card.dataset["eventName"] === event) {
            return card;
        }
    }
    return null;
}

function focusGrantAdd(event: string, key: string): void {
    const card = findEventCard(event);
    if (card == null) {
        return;
    }
    // The grant key is JSON (it embeds the free-text description), so match on
    // the dataset value rather than building a CSS attribute selector from it.
    for (const grantEl of card.querySelectorAll<HTMLElement>(".grant")) {
        if (grantEl.dataset["grantKey"] === key) {
            (grantEl.querySelector(".who_add") as HTMLInputElement|null)?.focus();
            return;
        }
    }
}

function focusDraftAdd(event: string, draftId: number): void {
    const card = findEventCard(event);
    const addInput = card?.querySelector(`.grant[data-draft-id="${draftId}"] .who_add`) as HTMLInputElement|null;
    addInput?.focus();
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

// deleteEvent deletes the event shown in the edit modal, along with all the
// data associated with it. The server only permits this when it's configured
// with event deletion enabled, which is meant for dev and staging use.
async function deleteEvent(): Promise<void> {
    const eventName = el.editEventModal.dataset["eventName"];
    if (!eventName || !eventDeletionAllowed) {
        return;
    }
    if (!confirm(
        `Delete the event "${eventName}"?\n\n` +
        "This will permanently delete all of its data, including incidents, " +
        "field reports, and visits. This cannot be undone.")) {
        return;
    }
    const {err} = await ims.fetchNoThrow(url_event.replace("<event_id>", eventName), {
        method: "DELETE",
    });
    if (err != null) {
        const message = `Failed to delete event: ${err}`;
        console.log(message);
        window.alert(message);
        return;
    }
    editEventModal?.hide();
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
