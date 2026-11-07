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
    eventName: ims.typedElement("event-name", HTMLSelectElement),
    dayNames: ims.typedElement("day-names", HTMLTextAreaElement),
    save: ims.typedElement("day-names-save", HTMLButtonElement),
};

initAdminDayNamesPage();

async function initAdminDayNamesPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    el.eventName.addEventListener("change", loadDayNames);
    el.save.addEventListener("click", save);
    drawEventNames(await initResult.eventDatas);

    // As on the Places admin page, an "event_id" query param preselects an event.
    const requestedEvent = new URLSearchParams(window.location.search).get("event_id");
    if (requestedEvent) {
        el.eventName.value = requestedEvent;
        if (el.eventName.value === requestedEvent) {
            await loadDayNames();
        } else {
            el.eventName.value = "";
            ims.setErrorMessage(`No such event: ${requestedEvent}`);
        }
    }
}

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
    const sortedEvents = events?.toSorted((a, b) => b.name.localeCompare(a.name));
    for (const event of sortedEvents??[]) {
        // groups are containers for events; they have no days of their own
        if (event.is_group) {
            continue;
        }
        const option: HTMLOptionElement = document.createElement("option");
        option.value = event.name;
        option.textContent = event.name;
        el.eventName.append(option);
    }
}

export async function loadDayNames(): Promise<void> {
    ims.clearErrorMessage();
    const eventName = el.eventName.value;
    setEventURLParam(eventName);
    el.dayNames.value = "";
    el.dayNames.disabled = true;
    el.save.disabled = true;
    if (!eventName) {
        return;
    }
    const {json, err} = await ims.fetchNoThrow<ims.EventDayName[]>(
        url_dayNames.replace("<event_id>", eventName), {
            headers: {"Cache-Control": "no-cache"},
        },
    );
    // A slow response for a previously selected event mustn't fill in the
    // textarea, or saving would write that event's names to this one.
    if (el.eventName.value !== eventName) {
        return;
    }
    if (err != null || json == null) {
        const message = `Failed to load day names: ${err}`;
        console.error(message);
        ims.setErrorMessage(message);
        return;
    }
    el.dayNames.value = formatCSV(json);
    el.dayNames.disabled = false;
    el.save.disabled = false;
}

function formatCSV(dayNames: ims.EventDayName[]): string {
    return dayNames.map((dn: ims.EventDayName): string => {
        // Only a name that itself starts with a quote needs quoting, since
        // everything after the first comma is the name.
        const name = dn.name.startsWith('"') ? `"${dn.name.replaceAll('"', '""')}"` : dn.name;
        return `${dn.date},${name}\n`;
    }).join("");
}

const isoDate = /^\d{4}-\d{2}-\d{2}$/;

// Parses "date,name" lines, returning an error message naming the first bad
// line. A name wrapped in double quotes, as a spreadsheet's CSV export would
// write one containing a comma, is unquoted.
function parseCSV(text: string): {dayNames: ims.EventDayName[], error: string|null} {
    const dayNames: ims.EventDayName[] = [];
    const seen = new Set<string>();
    const lines = text.split(/\r?\n/);
    for (const [i, line] of lines.entries()) {
        const lineNum = i + 1;
        if (line.trim() === "") {
            continue;
        }
        const comma = line.indexOf(",");
        if (comma < 0) {
            return {dayNames: [], error: `Line ${lineNum}: expected "date,name", got "${line}".`};
        }
        const date = line.slice(0, comma).trim();
        let name = line.slice(comma + 1).trim();
        if (name.length >= 2 && name.startsWith('"') && name.endsWith('"')) {
            name = name.slice(1, -1).replaceAll('""', '"').trim();
        }
        if (!isoDate.test(date) || !isRealDate(date)) {
            return {dayNames: [], error: `Line ${lineNum}: "${date}" isn't a date in YYYY-MM-DD form.`};
        }
        if (seen.has(date)) {
            return {dayNames: [], error: `Line ${lineNum}: ${date} is listed more than once.`};
        }
        if (!name) {
            return {dayNames: [], error: `Line ${lineNum}: the name for ${date} is empty.`};
        }
        seen.add(date);
        dayNames.push({date: date, name: name});
    }
    return {dayNames: dayNames, error: null};
}

// Catches e.g. 2026-02-30, which the pattern alone allows.
function isRealDate(date: string): boolean {
    const d = new Date(`${date}T00:00:00Z`);
    return !isNaN(d.getTime()) && d.toISOString().startsWith(date);
}

async function save(): Promise<void> {
    ims.clearErrorMessage();
    const eventName = el.eventName.value;
    if (!eventName) {
        ims.setErrorMessage("Select an event before saving.");
        return;
    }
    const {dayNames, error} = parseCSV(el.dayNames.value);
    if (error != null) {
        ims.setErrorMessage(error);
        ims.controlHasError(el.dayNames);
        return;
    }
    const {err} = await ims.fetchNoThrow(
        url_dayNames.replace("<event_id>", eventName), {
            body: JSON.stringify(dayNames),
        });
    if (err != null) {
        const message = `Failed to save day names: ${err}`;
        console.error(message);
        ims.setErrorMessage(message);
        ims.controlHasError(el.save);
        return;
    }
    ims.controlHasSuccess(el.save);
    ims.announce("Saved day names");
    await loadDayNames();
}
