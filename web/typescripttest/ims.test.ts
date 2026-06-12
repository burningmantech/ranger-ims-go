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

import { expect, test } from "vitest";
import * as ims from "../typescript/ims.ts";

test("parseInt10 parses base-10 integers and rejects garbage", (): void => {
    expect(ims.parseInt10("0")).toBe(0);
    expect(ims.parseInt10("007")).toBe(7);
    expect(ims.parseInt10("-12")).toBe(-12);
    expect(ims.parseInt10(null)).toBeNull();
    expect(ims.parseInt10(undefined)).toBeNull();
    expect(ims.parseInt10("")).toBeNull();
    expect(ims.parseInt10("bananas")).toBeNull();
});

test("padTwo pads to two digits", (): void => {
    expect(ims.padTwo(5)).toBe("05");
    expect(ims.padTwo("5")).toBe("05");
    expect(ims.padTwo(12)).toBe("12");
    expect(ims.padTwo(0)).toBe("00");
    expect(ims.padTwo(null)).toBe("?");
    expect(ims.padTwo(undefined)).toBe("?");
});

test("normalizeMinute rounds to the nearest five minutes", (): void => {
    expect(ims.normalizeMinute(0)).toBe("00");
    expect(ims.normalizeMinute(7)).toBe("05");
    expect(ims.normalizeMinute(8)).toBe("10");
    expect(ims.normalizeMinute(32)).toBe("30");
});

test("compareReportEntries orders by creation time, then system entries first, then text", (): void => {
    const earlier: ims.ReportEntry = { created: "2025-08-25T10:00:00Z", system_entry: false, text: "b" };
    const later: ims.ReportEntry = { created: "2025-08-25T11:00:00Z", system_entry: false, text: "a" };
    expect(ims.compareReportEntries(earlier, later)).toBe(-1);
    expect(ims.compareReportEntries(later, earlier)).toBe(1);

    const system: ims.ReportEntry = { created: "2025-08-25T10:00:00Z", system_entry: true, text: "b" };
    expect(ims.compareReportEntries(system, earlier)).toBe(-1);
    expect(ims.compareReportEntries(earlier, system)).toBe(1);

    const earlierTextA: ims.ReportEntry = { created: "2025-08-25T10:00:00Z", system_entry: false, text: "a" };
    expect(ims.compareReportEntries(earlierTextA, earlier)).toBe(-1);
    expect(ims.compareReportEntries(earlier, earlier)).toBe(0);
});

test("summarizeIncidentOrFR prefers the explicit summary", (): void => {
    const incident: ims.Incident = {
        summary: "Dust storm",
        report_entries: [{ text: "first line", system_entry: false }],
    };
    expect(ims.summarizeIncidentOrFR(incident)).toBe("Dust storm");
});

test("summarizeIncidentOrFR falls back to the first line of the first non-system entry", (): void => {
    const incident: ims.Incident = {
        report_entries: [
            { text: "Changed state: closed", system_entry: true },
            { text: "\nperson lost a hat\nmore detail", system_entry: false },
        ],
    };
    expect(ims.summarizeIncidentOrFR(incident)).toBe("person lost a hat");
});

test("summarizeIncidentOrFR returns empty string when there is nothing to summarize", (): void => {
    expect(ims.summarizeIncidentOrFR({})).toBe("");
    expect(ims.summarizeIncidentOrFR({ report_entries: [{ text: "sys", system_entry: true }] })).toBe("");
});

test("incidentAsString renders new and existing incidents", (): void => {
    expect(ims.incidentAsString({ number: null })).toBe("New Incident");
    expect(ims.incidentAsString({ number: 42, summary: "Dust storm" })).toBe("#42 Dust storm");
});

test("fieldReportAsString includes number, author, and summary", (): void => {
    expect(ims.fieldReportAsString({ number: null })).toBe("New Field Report");
    const fr: ims.FieldReport = {
        number: 7,
        summary: "Lost child",
        report_entries: [{ author: "Hot Slots", text: "Lost child", system_entry: false }],
    };
    expect(ims.fieldReportAsString(fr)).toBe("FR #7 (Hot Slots): Lost child");
});

test("visitAsString uses the preferred name, then the legal name", (): void => {
    expect(ims.visitAsString({ number: null })).toBe("New Visit");
    expect(ims.visitAsString({ number: 3, guest_preferred_name: "Sparkle" })).toBe("VS #3: Sparkle");
    expect(ims.visitAsString({ number: 4, guest_legal_name: "Jane Doe" })).toBe("VS #4: Jane Doe");
    expect(ims.visitAsString({ number: 5 })).toBe("VS #5: ");
});

test("reportTextFromIncident merges incident text with linked field reports", (): void => {
    const incident: ims.Incident = {
        summary: "Art car crash",
        report_entries: [
            { text: "entered state: on_scene", system_entry: true },
            { text: "ranger dispatched", system_entry: false },
        ],
        field_reports: [9],
    };
    const fieldReports: ims.FieldReportsByNumber = {
        9: {
            number: 9,
            summary: "FR summary",
            report_entries: [{ text: "fr detail", system_entry: false }],
        },
    };
    const text = ims.reportTextFromIncident(incident, fieldReports, {});
    expect(text).toContain("Art car crash");
    expect(text).toContain("ranger dispatched");
    expect(text).toContain("FR summary");
    expect(text).toContain("fr detail");
    expect(text).not.toContain("entered state");
});

test("localDateISO and localTimeHHMM format in local time", (): void => {
    const d = new Date(2026, 7, 30, 9, 5);
    expect(ims.localDateISO(d)).toBe("2026-08-30");
    expect(ims.localTimeHHMM(d)).toBe("09:05");
});

test("isValidIncidentsTableState accepts only known states", (): void => {
    expect(ims.isValidIncidentsTableState("all")).toBe(true);
    expect(ims.isValidIncidentsTableState("open")).toBe(true);
    expect(ims.isValidIncidentsTableState("active")).toBe(true);
    expect(ims.isValidIncidentsTableState("closed")).toBe(false);
    expect(ims.isValidIncidentsTableState(null)).toBe(false);
});

test("coalesceRowsPerPage picks the first valid value and throws when there is none", (): void => {
    expect(ims.coalesceRowsPerPage(null, "banana", "25", "50")).toBe("25");
    expect(ims.coalesceRowsPerPage("all")).toBe("all");
    expect((): void => {
        ims.coalesceRowsPerPage(null, "banana");
    }).toThrowError("No valid TableRowsPerPage value found");
});

test("hide and unhide toggle the hidden class on matching elements", (): void => {
    document.body.innerHTML = `
        <div class="if-admin"></div>
        <div class="if-admin hidden"></div>
        <div class="other"></div>
    `;
    ims.hide(".if-admin");
    for (const el of document.querySelectorAll(".if-admin")) {
        expect(el.classList.contains("hidden")).toBe(true);
    }
    expect(document.querySelector(".other")!.classList.contains("hidden")).toBe(false);

    ims.unhide(".if-admin");
    for (const el of document.querySelectorAll(".if-admin")) {
        expect(el.classList.contains("hidden")).toBe(false);
    }
});

test("setErrorMessage and clearErrorMessage drive the ErrorInfo markup", (): void => {
    // This is the markup of the ErrorInfo templ component (common.templ).
    document.body.innerHTML = `
        <div id="error_info" class="hidden text-danger">
            <p id="error_text"></p>
        </div>
    `;
    ims.setErrorMessage("it broke");
    expect(document.getElementById("error_text")!.textContent).toBe("Error: it broke");
    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);

    ims.clearErrorMessage();
    expect(document.getElementById("error_text")!.textContent).toBe("");
    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(true);
});

test("typedElement returns the element when the type matches", (): void => {
    document.body.innerHTML = `<input id="some_input" type="text"/>`;
    const input = ims.typedElement("some_input", HTMLInputElement);
    expect(input.id).toBe("some_input");
});

test("typedElement throws a descriptive error on a type mismatch", (): void => {
    document.body.innerHTML = `<div id="some_div"></div>`;
    expect((): void => {
        ims.typedElement("some_div", HTMLInputElement);
    }).toThrowError(/some_div.*HTMLInputElement/);
});

test("typedElement throws when the element does not exist", (): void => {
    document.body.innerHTML = "";
    expect((): void => {
        ims.typedElement("no_such_element", HTMLInputElement);
    }).toThrowError();
});
