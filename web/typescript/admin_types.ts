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

const el = {
    incidentTypes: ims.typedElement("incident_types", HTMLElement),
    typeLiTemplate: ims.typedElement("type_li_template", HTMLTemplateElement),
    editIncidentTypeModal: ims.typedElement("editIncidentTypeModal", HTMLElement),
    editIncidentTypeName: ims.typedElement("edit_incident_type_name", HTMLInputElement),
    editIncidentTypeDescription: ims.typedElement("edit_incident_type_description", HTMLTextAreaElement),
};

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
    const {json, err} = await ims.fetchNoThrow<ims.IncidentType[]>(url_incidentTypes, {
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


function drawAllIncidentTypes(): void {
    updateIncidentTypes();
}

function updateIncidentTypes(): void {
    const entryContainer = el.incidentTypes.querySelector("ul")!;
    entryContainer.querySelectorAll("li")!.forEach(entry => {entry.remove()});

    const editIncidentTypeModal = ims.bsModal(el.editIncidentTypeModal);

    for (const incidentType of adminIncidentTypes??[]) {
        const entryItemFrag = el.typeLiTemplate.content.cloneNode(true) as DocumentFragment;
        const entryItem = entryItemFrag.querySelector("li")!;

        if (incidentType.hidden) {
            entryItem.classList.add("item-hidden");
        } else {
            entryItem.classList.add("item-visible");
        }

        const typeSpan = entryItem.getElementsByClassName("type-name")[0]!;
        typeSpan.textContent = incidentType.name??null;

        const descriptionSpan = entryItem.getElementsByClassName("type-description")[0]!;
        descriptionSpan.textContent = `${incidentType.description??""}`;

        entryItem.dataset["incidentTypeId"] = incidentType.id?.toString();

        const showEditModal: HTMLElement = entryItem.querySelector(".show-edit-modal")!;
        showEditModal.addEventListener("click",
            function (_e: MouseEvent): void  {
                el.editIncidentTypeModal.dataset["incidentTypeId"] = incidentType.id?.toString();
                el.editIncidentTypeName.value = incidentType.name??"";
                el.editIncidentTypeDescription.value = incidentType.description??"";
                editIncidentTypeModal.show();
            },
        );

        entryContainer.append(entryItemFrag);
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
    const id = ims.parseInt10(el.editIncidentTypeModal.dataset["incidentTypeId"]);
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
    ims.controlHasSuccess(sender);
    await loadAndDrawIncidentTypes();
}

async function setIncidentTypeDescription(sender: HTMLTextAreaElement): Promise<void> {
    const id = ims.parseInt10(el.editIncidentTypeModal.dataset["incidentTypeId"]);
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
    ims.controlHasSuccess(sender);
    await loadAndDrawIncidentTypes();
}

async function sendIncidentTypes(edits: ims.IncidentType): Promise<{err:string|null}> {
    const {err} = await ims.fetchNoThrow(url_incidentTypes, {
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
