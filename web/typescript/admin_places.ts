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
    eventName: ims.typedElement("event-name", HTMLSelectElement),
};

// Each kind of place gets its own textarea, its own Save button, and its own
// trip to the API, so that saving one kind can't clobber the other three.
interface PlaceField {
    placeType: ims.PlaceType;
    labelText: string;
    dataEl: HTMLTextAreaElement;
    labelEl: HTMLLabelElement;
    saveEl: HTMLButtonElement;
    // Converts the pasted external data (e.g. a Burning Man API response) into
    // the Places the IMS API takes.
    parse: (value: string) => ims.Place[];
}

const fields: PlaceField[] = [
    {
        placeType: "art",
        labelText: "Art JSON Data",
        dataEl: ims.typedElement("art-data", HTMLTextAreaElement),
        labelEl: ims.typedElement("art-data-label", HTMLLabelElement),
        saveEl: ims.typedElement("art-save", HTMLButtonElement),
        parse: (value: string): ims.Place[] =>
            (JSON.parse(value) as ims.BMArt[]).map((ed: ims.BMArt): ims.Place => ({
                name: ed.name,
                location_string: ed.location_string,
                external_data: ed,
            })),
    },
    {
        placeType: "camp",
        labelText: "Camp JSON Data",
        dataEl: ims.typedElement("camp-data", HTMLTextAreaElement),
        labelEl: ims.typedElement("camp-data-label", HTMLLabelElement),
        saveEl: ims.typedElement("camp-save", HTMLButtonElement),
        parse: (value: string): ims.Place[] =>
            (JSON.parse(value) as ims.BMCamp[]).map((ed: ims.BMCamp): ims.Place => ({
                name: ed.name,
                location_string: ed.location_string,
                external_data: ed,
            })),
    },
    {
        placeType: "mv",
        labelText: "Mutant vehicle JSON Data",
        dataEl: ims.typedElement("mv-data", HTMLTextAreaElement),
        labelEl: ims.typedElement("mv-data-label", HTMLLabelElement),
        saveEl: ims.typedElement("mv-save", HTMLButtonElement),
        parse: (value: string): ims.Place[] =>
            (JSON.parse(value) as ims.BMMV[]).map((ed: ims.BMMV): ims.Place => ({
                name: ed.name,
                external_data: ed,
            })),
    },
    {
        placeType: "other",
        labelText: "Other JSON Data",
        dataEl: ims.typedElement("other-data", HTMLTextAreaElement),
        labelEl: ims.typedElement("other-data-label", HTMLLabelElement),
        saveEl: ims.typedElement("other-save", HTMLButtonElement),
        parse: (value: string): ims.Place[] =>
            (JSON.parse(value) as ims.OtherDest[]).map((ed: ims.OtherDest): ims.Place => ({
                name: ed.name,
                location_string: ed.location_string,
                external_data: ed,
            })),
    },
];

initAdminPlacesPage();

async function initAdminPlacesPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    window.loadPlaces = loadPlaces;
    for (const field of fields) {
        field.saveEl.addEventListener("click", async (): Promise<void> => {
            await save(field);
        });
    }
    drawEventNames(await initResult.eventDatas);

    // An "event_id" query param (which holds an event name, as elsewhere in IMS)
    // preselects that event and loads its places.
    const requestedEvent = new URLSearchParams(window.location.search).get("event_id");
    if (requestedEvent) {
        el.eventName.value = requestedEvent;
        if (el.eventName.value === requestedEvent) {
            await loadPlaces();
        } else {
            // Setting an unmatched value leaves the select with nothing selected,
            // so fall back to the placeholder option.
            el.eventName.value = "";
            ims.setErrorMessage(`No such event: ${requestedEvent}`);
        }
    }
}

// Keep the "event_id" query param in sync with the selected event.
function setEventURLParam(eventName: string): void {
    const params: URLSearchParams = new URLSearchParams(window.location.search);
    if (eventName) {
        params.set("event_id", eventName);
    } else {
        params.delete("event_id");
    }
    const query: string = params.toString();
    window.history.replaceState(null, "", query ? `?${query}` : window.location.pathname);
}

function drawEventNames(events: ims.EventData[]|null): void {
    const sortedEvents = events?.toSorted((a, b) => {
        // reverse alphabetical order
        return b.name.localeCompare(a.name);
    });
    for (const event of sortedEvents??[]) {
        // groups are containers for events; they have no places of their own
        if (event.is_group) {
            continue;
        }
        const option: HTMLOptionElement = document.createElement("option");
        option.value = event.name;
        option.textContent = event.name;
        el.eventName.append(option);
    }
}

async function save(field: PlaceField): Promise<void> {
    ims.clearErrorMessage();
    const eventName = el.eventName.value;
    if (!eventName) {
        ims.setErrorMessage("Select an event before saving.");
        return;
    }
    let places: ims.Places;
    try {
        places = {[field.placeType]: field.parse(field.dataEl.value)};
    } catch (e: any) {
        console.log(e);
        ims.setErrorMessage(`Invalid ${field.labelText}: ${e}`);
        ims.controlHasError(field.dataEl);
        return;
    }

    // The API only replaces the place types present in the body, so this leaves
    // the event's other three kinds of place untouched.
    const {err} = await ims.fetchNoThrow(
        url_places.replace("<event_id>", eventName), {
            body: JSON.stringify(places),
        });
    if (err != null) {
        const message = `Failed to save ${field.placeType} places: ${err}`;
        console.log(message);
        ims.setErrorMessage(message);
        ims.controlHasError(field.dataEl);
        return;
    }
    ims.controlHasSuccess(field.dataEl);
    await fetchPlaces([field]);
}

async function loadPlaces(): Promise<void> {
    ims.clearErrorMessage();
    const eventName = el.eventName.value;
    setEventURLParam(eventName);
    if (!eventName) {
        return;
    }
    await fetchPlaces(fields);
}

// Fetches the selected event's places and redraws the given fields from the
// response, so that a just-saved field shows what the server actually stored.
async function fetchPlaces(toDraw: PlaceField[]): Promise<void> {
    const {json, err} = await ims.fetchNoThrow<ims.Places>(
        url_places.replace("<event_id>", el.eventName.value), {
            headers: {"Cache-Control": "no-cache"},
        },
    );
    if (err != null || json == null) {
        const message = `Failed to load places: ${err}`;
        console.error(message);
        ims.setErrorMessage(message);
        return;
    }

    for (const field of toDraw) {
        const externalDatas = (json[field.placeType] ?? []).map(
            (place: ims.Place) => place.external_data,
        );
        field.dataEl.value = JSON.stringify(externalDatas, null, 2);
        field.labelEl.textContent = `${field.labelText} (${externalDatas.length})`;
    }
}
