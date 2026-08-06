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
    // Present for the place types the Burning Man API can provide, i.e. all but
    // "other".
    apiImport?: PlaceImport;
    // Converts the pasted external data (e.g. a Burning Man API response) into
    // the Places the IMS API takes.
    parse: (value: string) => ims.Place[];
}

// The controls for having the server fetch a place type straight from the
// Burning Man API, rather than an admin pasting the response in by hand.
interface PlaceImport {
    // Plural, lowercase, for use in messages, e.g. "camps".
    noun: string;
    wrapperEl: HTMLElement;
    yearEl: HTMLInputElement;
    buttonEl: HTMLButtonElement;
    statusEl: HTMLElement;
}

function placeImport(placeType: ims.PlaceType, noun: string): PlaceImport {
    return {
        noun: noun,
        wrapperEl: ims.typedElement(`${placeType}-api-wrapper`, HTMLElement),
        yearEl: ims.typedElement(`${placeType}-api-year`, HTMLInputElement),
        buttonEl: ims.typedElement(`${placeType}-api-import`, HTMLButtonElement),
        statusEl: ims.typedElement(`${placeType}-api-status`, HTMLElement),
    };
}

// Whether this server has a Burning Man API key. Without one, the "Set from
// API" buttons stay disabled.
let placesImportAllowed: boolean = false;

// How many places of each type the server last told us it had, used to say what
// an import is about to destroy.
const loadedCounts = new Map<ims.PlaceType, number>();

const fields: PlaceField[] = [
    {
        placeType: "art",
        labelText: "Art JSON Data",
        dataEl: ims.typedElement("art-data", HTMLTextAreaElement),
        labelEl: ims.typedElement("art-data-label", HTMLLabelElement),
        saveEl: ims.typedElement("art-save", HTMLButtonElement),
        apiImport: placeImport("art", "art"),
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
        apiImport: placeImport("camp", "camps"),
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
        apiImport: placeImport("mv", "mutant vehicles"),
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
    placesImportAllowed = initResult.authInfo.places_import_allowed??false;
    for (const field of fields) {
        field.saveEl.addEventListener("click", async (): Promise<void> => {
            await save(field);
        });
        const apiImport: PlaceImport|undefined = field.apiImport;
        if (!apiImport) {
            continue;
        }
        apiImport.buttonEl.disabled = !placesImportAllowed;
        apiImport.wrapperEl.title = placesImportAllowed
            ? `Replace this event's ${apiImport.noun} with the Burning Man API's data for this year`
            : "This IMS server has no Burning Man API key configured";
        apiImport.buttonEl.addEventListener("click", async (): Promise<void> => {
            await importFromAPI(field);
        });
    }
    setYearInputs(el.eventName.value);
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
    setYearInputs(eventName);
    for (const field of fields) {
        if (field.apiImport) {
            field.apiImport.statusEl.textContent = "";
        }
    }
    if (!eventName) {
        return;
    }
    await fetchPlaces(fields);
}

// The Burning Man API year isn't necessarily the IMS event name, but it usually
// is, so start from any four-digit year in the event name and fall back to the
// current year. The admin can always type something else.
function setYearInputs(eventName: string): void {
    const fromName: RegExpMatchArray|null = eventName.match(/(?:^|\D)(\d{4})(?:\D|$)/);
    const year: string = fromName?.[1] ?? new Date().getFullYear().toString();
    for (const field of fields) {
        if (field.apiImport) {
            field.apiImport.yearEl.value = year;
        }
    }
}

// Has the server fetch this place type from the Burning Man API and store the
// result, replacing whatever the event had. The server does the fetching, since
// that's where the API key lives.
async function importFromAPI(field: PlaceField): Promise<void> {
    const apiImport: PlaceImport|undefined = field.apiImport;
    if (!apiImport || !placesImportAllowed) {
        return;
    }
    ims.clearErrorMessage();
    apiImport.statusEl.textContent = "";
    const eventName = el.eventName.value;
    if (!eventName) {
        ims.setErrorMessage("Select an event before setting places from the API.");
        return;
    }
    const year: number|null = ims.parseInt10(apiImport.yearEl.value);
    if (year == null) {
        ims.setErrorMessage(`Enter the year to fetch ${apiImport.noun} from the API.`);
        ims.controlHasError(apiImport.yearEl);
        return;
    }
    const existing: number|undefined = loadedCounts.get(field.placeType);
    const doomed: string = existing == null
        ? `all ${apiImport.noun} currently stored`
        : `the ${existing} ${apiImport.noun} currently stored`;
    if (!confirm(
        `Set ${apiImport.noun} for event "${eventName}" from the Burning Man API's ${year} data?\n\n` +
        `This will delete ${doomed} for this event. This cannot be undone.`)) {
        return;
    }

    const params: URLSearchParams = new URLSearchParams({
        place_type: field.placeType,
        year: year.toString(),
    });
    apiImport.buttonEl.disabled = true;
    apiImport.statusEl.textContent = `Fetching ${apiImport.noun} for ${year}…`;
    const {json, err} = await ims.fetchNoThrow<ims.ImportPlacesResponse>(
        `${url_placesImport.replace("<event_id>", eventName)}?${params.toString()}`, {
            method: "POST",
        });
    apiImport.buttonEl.disabled = false;
    if (err != null || json == null) {
        apiImport.statusEl.textContent = "";
        const message = `Failed to set ${apiImport.noun} from the API: ${err}`;
        console.error(message);
        ims.setErrorMessage(message);
        ims.controlHasError(field.dataEl);
        return;
    }
    apiImport.statusEl.textContent =
        `Saved ${json.count} ${apiImport.noun} from the ${year} Burning Man API data.`;
    ims.controlHasSuccess(field.dataEl);
    ims.announce(apiImport.statusEl.textContent);
    await fetchPlaces([field]);
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
        loadedCounts.set(field.placeType, externalDatas.length);
    }
}
