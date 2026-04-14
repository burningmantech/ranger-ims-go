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
        loadPlaces: () => Promise<void>;
    }
}

//
// Initialize UI
//

const el = {
    placeForm: ims.typedElement("place-form", HTMLFormElement),
    eventName: ims.typedElement("event-name", HTMLInputElement),
    artData: ims.typedElement("art-data", HTMLTextAreaElement),
    campData: ims.typedElement("camp-data", HTMLTextAreaElement),
    mvData: ims.typedElement("mv-data", HTMLTextAreaElement),
    otherData: ims.typedElement("other-data", HTMLTextAreaElement),
    artDataLabel: ims.typedElement("art-data-label", HTMLLabelElement),
    campDataLabel: ims.typedElement("camp-data-label", HTMLLabelElement),
    mvDataLabel: ims.typedElement("mv-data-label", HTMLLabelElement),
    otherDataLabel: ims.typedElement("other-data-label", HTMLLabelElement),
};

initAdminPlacesPage();

async function initAdminPlacesPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    window.loadPlaces = loadPlaces;
    el.placeForm.addEventListener("submit", async (e: SubmitEvent): Promise<void> => {
        e.preventDefault();
        await submit();
    })
}

function parsePlaces(artDataEl: HTMLTextAreaElement, campDataEl: HTMLTextAreaElement, mvDataEl: HTMLTextAreaElement, otherDataEl: HTMLTextAreaElement): ims.Places {
    const places: ims.Places = {
        art: [],
        camp: [],
        other: [],
        mv: [],
    }
    {
        const artExtDatas = JSON.parse(artDataEl.value) as ims.BMArt[];
        for (const ed of artExtDatas) {
            places.art!.push({
                name: ed.name,
                location_string: ed.location_string,
                external_data: ed,
            });
        }
    }
    {
        const campExtDatas = JSON.parse(campDataEl.value) as ims.BMCamp[];
        for (const ed of campExtDatas) {
            places.camp!.push({
                name: ed.name,
                location_string: ed.location_string,
                external_data: ed,
            });
        }
    }
    {
        const mvExtDatas = JSON.parse(mvDataEl.value) as ims.BMMV[];
        for (const ed of mvExtDatas) {
            places.mv!.push({
                name: ed.name,
                external_data: ed,
            });
        }
    }
    {
        const otherExtDatas = JSON.parse(otherDataEl.value) as ims.OtherDest[];
        for (const ed of otherExtDatas) {
            places.other!.push({
                name: ed.name,
                location_string: ed.location_string,
                external_data: ed,
            });
        }
    }
    return places
}

async function submit(): Promise<void> {
    ims.clearErrorMessage();
    let places: ims.Places|null = null;
    try {
        places = parsePlaces(el.artData, el.campData, el.mvData, el.otherData);
    } catch (e: any) {
        console.log(e);
        ims.setErrorMessage(e);
        return;
    }
    const eventName = el.eventName.value;

    const {err} = await ims.fetchNoThrow(
        url_places.replace("<event_id>", eventName), {
            body: JSON.stringify(places),
        });
    if (err != null) {
        const message = `Failed to create place: ${err}`;
        console.log(message);
        ims.setErrorMessage(message);
        ims.controlHasError(el.artData);
        ims.controlHasError(el.campData);
        ims.controlHasError(el.mvData);
        ims.controlHasError(el.otherData);
        return;
    }
    ims.controlHasSuccess(el.artData);
    ims.controlHasSuccess(el.campData);
    ims.controlHasSuccess(el.mvData);
    ims.controlHasSuccess(el.otherData);
}

async function loadPlaces(): Promise<void> {
    ims.clearErrorMessage();
    const eventName = el.eventName.value;

    const {json, err} = await ims.fetchNoThrow<ims.Places>(
        url_places.replace("<event_id>", eventName), {
            headers: {"Cache-Control": "no-cache"},
        },
    );
    if (err != null || json == null) {
        const message = `Failed to load places: ${err}`;
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
        const mvs: ims.BMMV[] = [];
        for (const ed of json.mv ?? []) {
            mvs.push(ed.external_data! as ims.BMMV);
        }
        el.mvData.value = JSON.stringify(mvs, null, 2);
        el.mvDataLabel.textContent = `JSON Data (${mvs.length})`;
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
