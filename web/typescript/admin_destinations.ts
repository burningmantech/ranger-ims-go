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
        loadDestinations: () => Promise<void>;
    }
}

//
// Initialize UI
//

const el = {
    destinationForm: ims.typedElement("destination-form", HTMLFormElement),
    eventName: ims.typedElement("event-name", HTMLInputElement),
    artData: ims.typedElement("art-data", HTMLTextAreaElement),
    campData: ims.typedElement("camp-data", HTMLTextAreaElement),
    otherData: ims.typedElement("other-data", HTMLTextAreaElement),
    artDataLabel: ims.typedElement("art-data-label", HTMLLabelElement),
    campDataLabel: ims.typedElement("camp-data-label", HTMLLabelElement),
    otherDataLabel: ims.typedElement("other-data-label", HTMLLabelElement),
};

initAdminDestinationsPage();

async function initAdminDestinationsPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    window.loadDestinations = loadDestinations;
    el.destinationForm.addEventListener("submit", async (e: SubmitEvent): Promise<void> => {
        e.preventDefault();
        await submit();
    })
}

function parseDestinations(artDataEl: HTMLTextAreaElement, campDataEl: HTMLTextAreaElement, otherDataEl: HTMLTextAreaElement): ims.Destinations {
    const destinations: ims.Destinations = {
        art: [],
        camp: [],
        other: [],
    }
    {
        const artExtDatas = JSON.parse(artDataEl.value) as ims.BMArt[];
        for (const ed of artExtDatas) {
            destinations.art!.push({
                name: ed.name,
                location_string: ed.location_string,
                external_data: ed,
            });
        }
    }
    {
        const campExtDatas = JSON.parse(campDataEl.value) as ims.BMCamp[];
        for (const ed of campExtDatas) {
            destinations.camp!.push({
                name: ed.name,
                location_string: ed.location_string,
                external_data: ed,
            });
        }
    }
    {
        const otherExtDatas = JSON.parse(otherDataEl.value) as ims.OtherDest[];
        for (const ed of otherExtDatas) {
            destinations.other!.push({
                name: ed.name,
                location_string: ed.location_string,
                external_data: ed,
            });
        }
    }
    return destinations
}

async function submit(): Promise<void> {
    ims.clearErrorMessage();
    let destinations: ims.Destinations|null = null;
    try {
        destinations = parseDestinations(el.artData, el.campData, el.otherData);
    } catch (e: any) {
        console.log(e);
        ims.setErrorMessage(e);
        return;
    }
    const eventName = el.eventName.value;

    const {err} = await ims.fetchNoThrow(
        url_destinations.replace("<event_id>", eventName), {
            body: JSON.stringify(destinations),
        });
    if (err != null) {
        const message = `Failed to create destination: ${err}`;
        console.log(message);
        ims.setErrorMessage(message);
        ims.controlHasError(el.artData);
        ims.controlHasError(el.campData);
        ims.controlHasError(el.otherData);
        return;
    }
    ims.controlHasSuccess(el.artData);
    ims.controlHasSuccess(el.campData);
    ims.controlHasSuccess(el.otherData);
}

async function loadDestinations(): Promise<void> {
    ims.clearErrorMessage();
    const eventName = el.eventName.value;

    const {json, err} = await ims.fetchNoThrow<ims.Destinations>(
        url_destinations.replace("<event_id>", eventName), {
            headers: {"Cache-Control": "no-cache"},
        },
    );
    if (err != null || json == null) {
        const message = `Failed to load destinations: ${err}`;
        console.error(message);
        ims.setErrorMessage(message);
        return;
    }

    {
        const arts: ims.BMArt[] = [];
        for (const ed of json.art ?? []) {
            arts.push(ed.external_data! as ims.BMArt);
        }
        el.artData.value = JSON.stringify(arts, null, 2);
        el.artDataLabel.textContent = `JSON Data (${arts.length})`;
    }
    {
        const camps: ims.BMCamp[] = [];
        for (const ed of json.camp ?? []) {
            camps.push(ed.external_data! as ims.BMCamp);
        }
        el.campData.value = JSON.stringify(camps, null, 2);
        el.campDataLabel.textContent = `JSON Data (${camps.length})`;
    }
    {
        const others: ims.OtherDest[] = [];
        for (const ed of json.other ?? []) {
            others.push(ed.external_data! as ims.OtherDest);
        }
        el.otherData.value = JSON.stringify(others, null, 2);
        el.otherDataLabel.textContent = `JSON Data (${others.length})`;
    }
}
