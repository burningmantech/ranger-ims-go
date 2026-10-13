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

//
// Initialize UI
//

const el = {
    incidentTypes: ims.typedElement("incident_types", HTMLElement),
    typeLiTemplate: ims.typedElement("type_li_template", HTMLTemplateElement),
    editIncidentTypeModal: ims.typedElement("editIncidentTypeModal", HTMLDialogElement),
    editIncidentTypeName: ims.typedElement("edit_incident_type_name", HTMLInputElement),
    editIncidentTypeDescription: ims.typedElement("edit_incident_type_description", HTMLTextAreaElement),
    newIncidentType: ims.typedElement("new_incident_type", HTMLInputElement),
};

const editIncidentTypeModal = ims.dialogModal(el.editIncidentTypeModal);

initAdminTypesPage();

async function initAdminTypesPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }

    el.newIncidentType.addEventListener("change", (): Promise<void> => createIncidentType(el.newIncidentType));
    el.editIncidentTypeName.addEventListener("change", (): Promise<void> => setIncidentTypeName(el.editIncidentTypeName));
    el.editIncidentTypeDescription.addEventListener("change",
        (): Promise<void> => setIncidentTypeDescription(el.editIncidentTypeDescription));

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
        await ims.alertDialog(message);
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

        const hideButton: HTMLElement = entryItem.querySelector(".badge-visible")!;
        hideButton.addEventListener("click", (): Promise<void> => hideIncidentType(hideButton));
        const showButton: HTMLElement = entryItem.querySelector(".badge-hidden")!;
        showButton.addEventListener("click", (): Promise<void> => showIncidentType(showButton));
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


export async function createIncidentType(sender: HTMLInputElement): Promise<void> {
    // The server rejects a nameless type, and clearing this field and tabbing
    // out is a cancel, not a request to create one.
    if (!sender.value.trim()) {
        return;
    }
    const {err} = await sendIncidentTypes({"name": sender.value});
    if (err == null) {
        sender.value = "";
    }
    await loadAndDrawIncidentTypes();
}



export async function showIncidentType(sender: HTMLElement): Promise<void> {
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


export async function hideIncidentType(sender: HTMLElement): Promise<void> {
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

export async function setIncidentTypeName(sender: HTMLInputElement): Promise<void> {
    const id = ims.parseInt10(el.editIncidentTypeModal.dataset["incidentTypeId"]);
    if (id == null || !sender.value.trim()) {
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

export async function setIncidentTypeDescription(sender: HTMLTextAreaElement): Promise<void> {
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
    await ims.alertDialog(message);
    return {err: err};
}
