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

// These types mirror the imsjson.Directory types on the server.
interface DirectoryPerson {
    id?: number;
    handle?: string|null;
    email?: string|null;
    active?: boolean|null;
    onsite?: boolean|null;
    team_ids?: number[]|null;
    position_ids?: number[]|null;
}
interface DirectoryGroup {
    id?: number;
    title?: string|null;
    active?: boolean|null;
}
interface Directory {
    persons: DirectoryPerson[];
    teams: DirectoryGroup[];
    positions: DirectoryGroup[];
}

declare global {
    interface Window {
        createPerson: (el: HTMLInputElement)=>Promise<void>;
        createTeam: (el: HTMLInputElement)=>Promise<void>;
        createPosition: (el: HTMLInputElement)=>Promise<void>;
        setPersonHandle: (el: HTMLInputElement)=>Promise<void>;
        setPersonEmail: (el: HTMLInputElement)=>Promise<void>;
        setPersonActive: (el: HTMLInputElement)=>Promise<void>;
        setPersonOnsite: (el: HTMLInputElement)=>Promise<void>;
        setPersonPassword: (el: HTMLElement)=>Promise<void>;
        deletePerson: (el: HTMLElement)=>Promise<void>;
    }
}

const el = {
    personsTbody: ims.typedElement("persons_tbody", HTMLElement),
    personRowTemplate: ims.typedElement("person_row_template", HTMLTemplateElement),
    groupLiTemplate: ims.typedElement("group_li_template", HTMLTemplateElement),
    teamsList: ims.typedElement("teams_list", HTMLElement),
    positionsList: ims.typedElement("positions_list", HTMLElement),
    editPersonModal: ims.typedElement("editPersonModal", HTMLElement),
    editPersonHandle: ims.typedElement("edit_person_handle", HTMLInputElement),
    editPersonEmail: ims.typedElement("edit_person_email", HTMLInputElement),
    editPersonActive: ims.typedElement("edit_person_active", HTMLInputElement),
    editPersonOnsite: ims.typedElement("edit_person_onsite", HTMLInputElement),
    editPersonTeams: ims.typedElement("edit_person_teams", HTMLElement),
    editPersonPositions: ims.typedElement("edit_person_positions", HTMLElement),
    editPersonPassword: ims.typedElement("edit_person_password", HTMLInputElement),
};

initAdminDirectoryPage();

async function initAdminDirectoryPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }

    window.createPerson = createPerson;
    window.createTeam = createTeam;
    window.createPosition = createPosition;
    window.setPersonHandle = setPersonHandle;
    window.setPersonEmail = setPersonEmail;
    window.setPersonActive = setPersonActive;
    window.setPersonOnsite = setPersonOnsite;
    window.setPersonPassword = setPersonPassword;
    window.deletePerson = deletePerson;

    await loadAndDrawDirectory();
    ims.hideLoadingOverlay();
    ims.enableEditing();
}

let directory: Directory|null = null;

async function loadAndDrawDirectory(): Promise<void> {
    const {err} = await loadDirectory();
    if (err == null) {
        drawPersons();
        drawGroups();
    }
}

async function loadDirectory(): Promise<{err: string|null}> {
    const {json, err} = await ims.fetchNoThrow<Directory>(url_directory, {
        headers: {"Cache-Control": "no-cache"},
    });
    if (err != null || json == null) {
        const message = "Failed to load the directory:\n" + err;
        console.error(message);
        ims.setErrorMessage(message);
        return {err: message};
    }
    json.persons.sort((a, b) => (a.handle??"").localeCompare(b.handle??""));
    json.teams.sort((a, b) => (a.title??"").localeCompare(b.title??""));
    json.positions.sort((a, b) => (a.title??"").localeCompare(b.title??""));
    directory = json;
    return {err: null};
}

function groupTitlesByID(groups: DirectoryGroup[]): Map<number, string> {
    const byID = new Map<number, string>();
    for (const group of groups) {
        if (group.id != null) {
            byID.set(group.id, group.title??"");
        }
    }
    return byID;
}

function drawPersons(): void {
    el.personsTbody.querySelectorAll("tr").forEach(row => {row.remove();});

    const teamTitles = groupTitlesByID(directory?.teams??[]);
    const positionTitles = groupTitlesByID(directory?.positions??[]);

    for (const person of directory?.persons??[]) {
        const rowFrag = el.personRowTemplate.content.cloneNode(true) as DocumentFragment;
        const row = rowFrag.querySelector("tr")!;
        row.dataset["personId"] = person.id?.toString();

        row.getElementsByClassName("person-handle")[0]!.textContent = person.handle??"";
        row.getElementsByClassName("person-email")[0]!.textContent = person.email??"";
        row.getElementsByClassName("person-active")[0]!.textContent =
            person.active ? "Active" : "Deactivated";
        row.getElementsByClassName("person-onsite")[0]!.textContent =
            person.onsite ? "Onsite" : "";
        row.getElementsByClassName("person-teams")[0]!.textContent =
            (person.team_ids??[]).map(id => teamTitles.get(id)??"").join(", ");
        row.getElementsByClassName("person-positions")[0]!.textContent =
            (person.position_ids??[]).map(id => positionTitles.get(id)??"").join(", ");

        const showEditModal: HTMLElement = row.querySelector(".show-edit-modal")!;
        showEditModal.addEventListener("click", (_e: MouseEvent): void => {
            showPersonModal(person);
        });

        el.personsTbody.append(rowFrag);
    }
}

function showPersonModal(person: DirectoryPerson): void {
    el.editPersonModal.dataset["personId"] = person.id?.toString();
    el.editPersonHandle.value = person.handle??"";
    el.editPersonEmail.value = person.email??"";
    el.editPersonActive.checked = person.active??false;
    el.editPersonOnsite.checked = person.onsite??false;
    el.editPersonPassword.value = "";
    drawMembershipCheckboxes(el.editPersonTeams, directory?.teams??[], person.team_ids??[], "team");
    drawMembershipCheckboxes(el.editPersonPositions, directory?.positions??[], person.position_ids??[], "position");
    ims.bsModal(el.editPersonModal).show();
}

function drawMembershipCheckboxes(
    container: HTMLElement,
    groups: DirectoryGroup[],
    memberOf: number[],
    kind: "team"|"position",
): void {
    container.replaceChildren();
    for (const group of groups) {
        if (group.id == null) {
            continue;
        }
        const groupID = group.id;
        const div = document.createElement("div");
        div.classList.add("form-check", "form-check-inline");
        const input = document.createElement("input");
        input.classList.add("form-check-input", `person-${kind}-checkbox`);
        input.type = "checkbox";
        input.id = `person_${kind}_${groupID}`;
        input.dataset["groupId"] = groupID.toString();
        input.checked = memberOf.includes(groupID);
        input.addEventListener("change", async (_e: Event): Promise<void> => {
            await setPersonMemberships(kind);
        });
        const label = document.createElement("label");
        label.classList.add("form-check-label");
        label.htmlFor = input.id;
        label.textContent = group.title??"";
        div.append(input, label);
        container.append(div);
    }
}

function drawGroups(): void {
    drawGroupList(el.teamsList, directory?.teams??[], "team");
    drawGroupList(el.positionsList, directory?.positions??[], "position");
}

function drawGroupList(list: HTMLElement, groups: DirectoryGroup[], kind: "team"|"position"): void {
    list.querySelectorAll("li").forEach(entry => {entry.remove();});

    for (const group of groups) {
        if (group.id == null) {
            continue;
        }
        const groupID = group.id;
        const liFrag = el.groupLiTemplate.content.cloneNode(true) as DocumentFragment;
        const li = liFrag.querySelector("li")!;
        li.dataset["groupId"] = groupID.toString();
        if (group.active) {
            li.classList.add("item-visible");
        } else {
            li.classList.add("item-hidden");
        }
        li.getElementsByClassName("group-title")[0]!.textContent = group.title??"";

        const activeButton: HTMLElement = li.querySelector(".group-active")!;
        activeButton.addEventListener("click", async (_e: MouseEvent): Promise<void> => {
            await sendGroup(kind, {id: groupID, active: false});
            await loadAndDrawDirectory();
        });
        const inactiveButton: HTMLElement = li.querySelector(".group-inactive")!;
        inactiveButton.addEventListener("click", async (_e: MouseEvent): Promise<void> => {
            await sendGroup(kind, {id: groupID, active: true});
            await loadAndDrawDirectory();
        });
        const renameButton: HTMLElement = li.querySelector(".group-rename")!;
        renameButton.addEventListener("click", async (_e: MouseEvent): Promise<void> => {
            const newTitle = prompt(
                `Rename ${kind} "${group.title??""}"?\n\n` +
                "Note that event access rules refer to teams and positions by name, " +
                "so renaming will change which rules apply to this group's members.",
                group.title??"",
            );
            if (!newTitle || newTitle === group.title) {
                return;
            }
            await sendGroup(kind, {id: groupID, title: newTitle});
            await loadAndDrawDirectory();
        });
        const deleteButton: HTMLElement = li.querySelector(".group-delete")!;
        deleteButton.addEventListener("click", async (_e: MouseEvent): Promise<void> => {
            if (!confirm(`Delete ${kind} "${group.title??""}"? This removes all its memberships.`)) {
                return;
            }
            const url = kind === "team"
                ? url_directoryTeam.replace("<team_id>", groupID.toString())
                : url_directoryPosition.replace("<position_id>", groupID.toString());
            const {err} = await ims.fetchNoThrow(url, {method: "DELETE"});
            if (err != null) {
                alertFailure(`Failed to delete ${kind}`, err);
            }
            await loadAndDrawDirectory();
        });

        list.append(liFrag);
    }
}

//
// Persons
//

async function createPerson(sender: HTMLInputElement): Promise<void> {
    const {err} = await sendPerson({handle: sender.value});
    if (err == null) {
        sender.value = "";
    }
    await loadAndDrawDirectory();
}

function modalPersonID(): number|null {
    return ims.parseInt10(el.editPersonModal.dataset["personId"]);
}

async function setPersonHandle(sender: HTMLInputElement): Promise<void> {
    const id = modalPersonID();
    if (id == null || !sender.value) {
        return;
    }
    await sendPersonFromControl(sender, {id: id, handle: sender.value});
}

async function setPersonEmail(sender: HTMLInputElement): Promise<void> {
    const id = modalPersonID();
    if (id == null) {
        return;
    }
    await sendPersonFromControl(sender, {id: id, email: sender.value});
}

async function setPersonActive(sender: HTMLInputElement): Promise<void> {
    const id = modalPersonID();
    if (id == null) {
        return;
    }
    await sendPersonFromControl(sender, {id: id, active: sender.checked});
}

async function setPersonOnsite(sender: HTMLInputElement): Promise<void> {
    const id = modalPersonID();
    if (id == null) {
        return;
    }
    await sendPersonFromControl(sender, {id: id, onsite: sender.checked});
}

async function setPersonMemberships(kind: "team"|"position"): Promise<void> {
    const id = modalPersonID();
    if (id == null) {
        return;
    }
    const container = kind === "team" ? el.editPersonTeams : el.editPersonPositions;
    const ids: number[] = [];
    for (const box of container.querySelectorAll<HTMLInputElement>("input[type=checkbox]")) {
        if (box.checked) {
            const groupID = ims.parseInt10(box.dataset["groupId"]);
            if (groupID != null) {
                ids.push(groupID);
            }
        }
    }
    const edits: DirectoryPerson = {id: id};
    if (kind === "team") {
        edits.team_ids = ids;
    } else {
        edits.position_ids = ids;
    }
    await sendPerson(edits);
    await loadAndDrawDirectory();
}

async function setPersonPassword(sender: HTMLElement): Promise<void> {
    const id = modalPersonID();
    if (id == null || !el.editPersonPassword.value) {
        return;
    }
    const url = url_directoryPersonPassword.replace("<person_id>", id.toString());
    const {err} = await ims.fetchNoThrow(url, {
        body: JSON.stringify({password: el.editPersonPassword.value}),
    });
    if (err != null) {
        alertFailure("Failed to set password", err);
        ims.controlHasError(sender);
        return;
    }
    el.editPersonPassword.value = "";
    ims.controlHasSuccess(sender);
}

async function deletePerson(_sender: HTMLElement): Promise<void> {
    const id = modalPersonID();
    if (id == null) {
        return;
    }
    if (!confirm(
        "Delete this person outright? Consider deactivating them instead, " +
        "so that their handle stays valid on old incidents.",
    )) {
        return;
    }
    const url = url_directoryPerson.replace("<person_id>", id.toString());
    const {err} = await ims.fetchNoThrow(url, {method: "DELETE"});
    if (err != null) {
        alertFailure("Failed to delete person", err);
        return;
    }
    ims.bsModal(el.editPersonModal).hide();
    await loadAndDrawDirectory();
}

async function sendPersonFromControl(sender: HTMLElement, edits: DirectoryPerson): Promise<void> {
    const {err} = await sendPerson(edits);
    if (err != null) {
        ims.controlHasError(sender);
        return;
    }
    ims.controlHasSuccess(sender);
    await loadAndDrawDirectory();
}

async function sendPerson(edits: DirectoryPerson): Promise<{err: string|null}> {
    const {err} = await ims.fetchNoThrow(url_directoryPersons, {
        body: JSON.stringify(edits),
    });
    if (err != null) {
        alertFailure("Failed to edit person", err);
    }
    return {err: err};
}

//
// Teams and positions
//

async function createTeam(sender: HTMLInputElement): Promise<void> {
    const {err} = await sendGroup("team", {title: sender.value});
    if (err == null) {
        sender.value = "";
    }
    await loadAndDrawDirectory();
}

async function createPosition(sender: HTMLInputElement): Promise<void> {
    const {err} = await sendGroup("position", {title: sender.value});
    if (err == null) {
        sender.value = "";
    }
    await loadAndDrawDirectory();
}

async function sendGroup(kind: "team"|"position", edits: DirectoryGroup): Promise<{err: string|null}> {
    const url = kind === "team" ? url_directoryTeams : url_directoryPositions;
    const {err} = await ims.fetchNoThrow(url, {
        body: JSON.stringify(edits),
    });
    if (err != null) {
        alertFailure(`Failed to edit ${kind}`, err);
    }
    return {err: err};
}

function alertFailure(message: string, err: string): void {
    const full = `${message}:\n${err}`;
    console.error(full);
    window.alert(full);
}
