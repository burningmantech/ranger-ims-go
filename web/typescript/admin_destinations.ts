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

initAdminDestinationsPage();

async function initAdminDestinationsPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    window.loadDestinations = loadDestinations;
    const form = document.getElementById("destination-form") as HTMLFormElement;
    form.addEventListener("submit", async (e: SubmitEvent): Promise<void> => {
        e.preventDefault();
        await submit();
    })
}

function parseDestinations(): ims.Destinations {
    const destinations: ims.Destinations = {}
    {
        const artDataEl = document.getElementById("art-data") as HTMLTextAreaElement;
        const artExtDatas = JSON.parse(artDataEl.value) as ims.BMArt[];
        const arts: ims.Destination[] = [];
        for (const ed of artExtDatas) {
            arts.push({
                name: ed.name,
                type: "art",
                location_string: ed.location_string,
                external_data: ed,
            });
        }
        destinations.art = arts;
    }
    {
        const campDataEl = document.getElementById("camp-data") as HTMLTextAreaElement;
        const campExtDatas = JSON.parse(campDataEl.value) as ims.BMCamp[];
        const camps: ims.Destination[] = [];
        for (const ed of campExtDatas) {
            camps.push({
                name: ed.name,
                type: "camp",
                location_string: ed.location_string,
                external_data: ed,
            });
        }
        destinations.camp = camps;
    }
    {
        const otherDataEl = document.getElementById("other-data") as HTMLTextAreaElement;
        const otherExtDatas = JSON.parse(otherDataEl.value) as ims.Other[];
        const others: ims.Destination[] = [];
        for (const ed of otherExtDatas) {
            others.push({
                name: ed.name,
                type: "other",
                location_string: ed.location_string,
                external_data: ed,
            });
        }
        destinations.other = others;
    }
    return destinations
}

async function submit(): Promise<void> {
    ims.clearErrorMessage();
    let destinations: ims.Destinations|null = null;
    try {
        destinations = parseDestinations();
    } catch (e: any) {
        console.log(e);
        ims.setErrorMessage(e);
        return;
    }
    const eventName = (document.getElementById("event-name") as HTMLInputElement).value;

    const {err} = await ims.fetchNoThrow(
        url_destinations.replace("<event_id>", eventName), {
            body: JSON.stringify(destinations),
        });
    if (err != null) {
        const message = `Failed to create destination: ${err}`;
        console.log(message);
        ims.setErrorMessage(message);
    }
}

async function loadDestinations(): Promise<void> {
    ims.clearErrorMessage();
    const eventName = (document.getElementById("event-name") as HTMLInputElement).value;

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
        (document.getElementById("art-data") as HTMLTextAreaElement).value = JSON.stringify(arts, null, 2);
        (document.getElementById("art-data-label") as HTMLLabelElement).textContent = `JSON Data (${arts.length})`;
    }
    {
        const camps: ims.BMCamp[] = [];
        for (const ed of json.camp ?? []) {
            camps.push(ed.external_data! as ims.BMArt);
        }
        (document.getElementById("camp-data") as HTMLTextAreaElement).value = JSON.stringify(camps, null, 2);
        (document.getElementById("camp-data-label") as HTMLLabelElement).textContent = `JSON Data (${camps.length})`;
    }
    {
        const others: ims.Other[] = [];
        for (const ed of json.other ?? []) {
            others.push(ed.external_data! as ims.Other);
        }
        (document.getElementById("other-data") as HTMLTextAreaElement).value = JSON.stringify(others, null, 2);
        (document.getElementById("other-data-label") as HTMLLabelElement).textContent = `JSON Data (${others.length})`;
    }
}
