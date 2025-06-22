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
        createIncidentType: (el: HTMLInputElement)=>Promise<void>;
        deleteIncidentType: (el: HTMLElement)=>void;
        showIncidentType: (el: HTMLElement)=>Promise<void>;
        hideIncidentType: (el: HTMLElement)=>Promise<void>;
        setIncidentTypeName: (el: HTMLInputElement)=>Promise<void>;
        setIncidentTypeDescription: (el: HTMLTextAreaElement)=>Promise<void>;
    }
}

//
// Initialize UI
//

initAdminTypesPage();

async function initAdminTypesPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }

    window.createIncidentType = createIncidentType;
    window.deleteIncidentType = deleteIncidentType;
    window.showIncidentType = showIncidentType;
    window.hideIncidentType = hideIncidentType;
    window.setIncidentTypeName = setIncidentTypeName;
    window.setIncidentTypeDescription = setIncidentTypeDescription;

    await loadAndDrawIncidentTypes();
    ims.hideLoadingOverlay();
    ims.enableEditing();
}


async function loadAndDrawIncidentTypes(): Promise<void> {
    await loadAllIncidentTypes();
    drawAllIncidentTypes();
}


let adminIncidentTypes: ims.IncidentType[]|null = null;

async function loadAllIncidentTypes(): Promise<{err:string|null}> {
    const {json, err} = await ims.fetchJsonNoThrow<ims.IncidentType[]>(url_incidentTypes, {
        headers: {"Cache-Control": "no-cache"},
    });
    if (err != null || json == null) {
        const message = "Failed to load incident types:\n" + err;
        console.error(message);
        window.alert(message);
        return {err: message};
    }
    json.sort((a: ims.IncidentType, b: ims.IncidentType): number => (a.name??"").localeCompare(b.name??""));
    adminIncidentTypes = json;
    return {err: null};
}


let _entryTemplate: Element|null = null;

function drawAllIncidentTypes(): void {
    if (_entryTemplate == null) {
        _entryTemplate = document.querySelector("#incident_types li");
    }
    updateIncidentTypes();
}

const editModalElement: HTMLElement = document.getElementById("editIncidentTypeModal")!;

function updateIncidentTypes(): void {
    const incidentTypesElement: HTMLElement = document.getElementById("incident_types")!;

    const entryContainer = incidentTypesElement.getElementsByClassName("list-group")[0] as HTMLElement;
    entryContainer.replaceChildren();

    const editIncidentTypeModal = ims.bsModal(editModalElement);

    for (const incidentType of adminIncidentTypes??[]) {
        const entryItem = _entryTemplate!.cloneNode(true) as HTMLElement;

        if (incidentType.hidden) {
            entryItem.classList.add("item-hidden");
        } else {
            entryItem.classList.add("item-visible");
        }

        const typeSpan = document.createElement("div");
        typeSpan.textContent = incidentType.name??null;
        entryItem.append(typeSpan);

        if (incidentType.description) {
            const descriptionSpan = document.createElement("div");
            descriptionSpan.classList.add("text-body-secondary", "ms-3");
            descriptionSpan.textContent = `${incidentType.description}`;
            entryItem.append(descriptionSpan);
        }

        entryItem.dataset["incidentTypeId"] = incidentType.id?.toString();

        const showEditModal: HTMLElement = entryItem.querySelector(".show-edit-modal")!;
        showEditModal.addEventListener("click",
            function (_e: MouseEvent): void  {
                editModalElement.dataset["incidentTypeId"] = incidentType.id?.toString();
                (document.getElementById("edit_incident_type_name") as HTMLInputElement).value = incidentType.name??"";
                (document.getElementById("edit_incident_type_description") as HTMLTextAreaElement).value = incidentType.description??"";
                editIncidentTypeModal.show();
            },
        );

        entryContainer.append(entryItem);
    }
}


async function createIncidentType(sender: HTMLInputElement): Promise<void> {
    const {err} = await sendIncidentTypes({"name": sender.value});
    if (err == null) {
        sender.value = "";
    }
    await loadAndDrawIncidentTypes();
}


function deleteIncidentType(_sender: HTMLElement) {
    alert("Remove unimplemented");
}


async function showIncidentType(sender: HTMLElement): Promise<void> {
    const typeId = sender.closest("li")?.dataset["incidentTypeId"];
    if (!typeId) {
        return;
    }
    await sendIncidentTypes({
        "id": ims.parseInt10(typeId),
        "hidden": false,
    });
    await loadAndDrawIncidentTypes();
}


async function hideIncidentType(sender: HTMLElement): Promise<void> {
    const typeId = sender.closest("li")?.dataset["incidentTypeId"];
    if (!typeId) {
        return;
    }
    await sendIncidentTypes({
        "id": ims.parseInt10(typeId),
        "hidden": true,
    });
    await loadAndDrawIncidentTypes();
}

async function setIncidentTypeName(sender: HTMLInputElement): Promise<void> {
    const id = ims.parseInt10(editModalElement.dataset["incidentTypeId"]);
    if (id == null || !sender.value) {
        return;
    }
    const {err} = await sendIncidentTypes({
        "id": id,
        "name": sender.value,
    });
    if (err != null) {
        ims.controlHasError(sender);
        return;
    }
    ims.controlHasSuccess(sender, 1000);
    await loadAndDrawIncidentTypes();
}

async function setIncidentTypeDescription(sender: HTMLTextAreaElement): Promise<void> {
    const id = ims.parseInt10(editModalElement.dataset["incidentTypeId"]);
    if (id == null || !sender.value) {
        return;
    }
    const {err} = await sendIncidentTypes({
        "id": id,
        "description": sender.value,
    });
    if (err != null) {
        ims.controlHasError(sender);
        return;
    }
    ims.controlHasSuccess(sender, 1000);
    await loadAndDrawIncidentTypes();
}

async function sendIncidentTypes(edits: ims.IncidentType): Promise<{err:string|null}> {
    const {err} = await ims.fetchJsonNoThrow(url_incidentTypes, {
        body: JSON.stringify(edits),
    });
    if (err == null) {
        return {err: null};
    }
    const message = `Failed to edit incident types:\n${JSON.stringify(err)}`;
    console.log(message);
    window.alert(message);
    return {err: err};
}
