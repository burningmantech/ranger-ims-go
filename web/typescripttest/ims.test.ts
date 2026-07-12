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

import { expect, test, vi } from "vitest";
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
        <div id="error_info" class="hidden text-danger-emphasis" role="alert">
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

test("setErrorMessage reveals the alert region before writing into it", async (): Promise<void> => {
    // A live region that is display:none is not in the accessibility tree, so
    // text written into it while it is still hidden is never announced. The
    // banner must therefore be unhidden first.
    document.body.innerHTML = `
        <div id="error_info" class="hidden text-danger-emphasis" role="alert">
            <p id="error_text"></p>
        </div>
    `;
    const errInfo = document.getElementById("error_info")!;
    const errText = document.getElementById("error_text")!;

    let hiddenWhenTextWasWritten: boolean|null = null;
    new MutationObserver((): void => {
        hiddenWhenTextWasWritten ??= errInfo.classList.contains("hidden");
    }).observe(errText, { childList: true, characterData: true, subtree: true });

    ims.setErrorMessage("it broke");

    expect(errText.textContent).toBe("Error: it broke");
    // MutationObserver delivers its records on the microtask queue.
    await vi.waitFor((): void => {
        expect(hiddenWhenTextWasWritten).toBe(false);
    });
});

test("announce writes the message into the live region", async (): Promise<void> => {
    // This is the markup of the LiveRegion templ component (common.templ).
    document.body.innerHTML = `<div id="aria_live" role="status" aria-live="polite"></div>`;
    const region = document.getElementById("aria_live")!;

    ims.announce("Saved");
    await vi.waitFor((): void => {
        expect(region.textContent).toBe("Saved");
    });
});

test("announce re-announces a repeated message", async (): Promise<void> => {
    // Rewriting a live region with the text it already holds is not a mutation,
    // so the region has to be cleared in between or the second "Saved" would be
    // silent.
    document.body.innerHTML = `<div id="aria_live" role="status" aria-live="polite"></div>`;
    const region = document.getElementById("aria_live")!;

    const texts: string[] = [];
    new MutationObserver((): void => {
        texts.push(region.textContent ?? "");
    }).observe(region, { childList: true, characterData: true, subtree: true });

    ims.announce("Saved");
    await vi.waitFor((): void => {
        expect(region.textContent).toBe("Saved");
    });
    ims.announce("Saved");
    await vi.waitFor((): void => {
        expect(texts).toEqual(["Saved", "", "Saved"]);
    });
});

test("announce is a no-op on a page with no live region", (): void => {
    document.body.innerHTML = "";
    expect((): void => {
        ims.announce("Saved");
    }).not.toThrow();
});

test("newUpdateAnnouncer coalesces a burst of updates into one announcement", async (): Promise<void> => {
    vi.useFakeTimers();
    try {
        document.body.innerHTML = `<div id="aria_live" role="status" aria-live="polite"></div>`;
        const region = document.getElementById("aria_live")!;
        const announceUpdate = ims.newUpdateAnnouncer("Incident", 3000);

        announceUpdate();
        announceUpdate();
        announceUpdate();
        // Nothing is said while the updates are still arriving.
        await vi.advanceTimersByTimeAsync(2999);
        expect(region.textContent).toBe("");

        // Once they stop, the whole burst is summarized in one utterance.
        await vi.advanceTimersByTimeAsync(2);
        expect(region.textContent).toBe("3 Incidents updated");

        // A single later update is announced in the singular.
        announceUpdate();
        await vi.advanceTimersByTimeAsync(3001);
        expect(region.textContent).toBe("1 Incident updated");
    } finally {
        vi.useRealTimers();
    }
});

test("newRemoteUpdateAnnouncer stays quiet about the user's own edits", async (): Promise<void> => {
    vi.useFakeTimers();
    try {
        document.body.innerHTML = `<div id="aria_live" role="status" aria-live="polite"></div>`;
        const region = document.getElementById("aria_live")!;
        const remoteUpdates = ims.newRemoteUpdateAnnouncer("This Incident was updated", 3000);

        // The server echoes a local save straight back over SSE. That redraw
        // isn't news: the save already announced "Saved".
        remoteUpdates.noteLocalEdit();
        remoteUpdates.announceUpdate();
        await vi.advanceTimersByTimeAsync(3001);
        expect(region.textContent).toBe("");

        // An update that didn't follow a local edit came from someone else, and
        // is the one thing the user has no other way of noticing.
        await vi.advanceTimersByTimeAsync(10_000);
        remoteUpdates.announceUpdate();
        await vi.advanceTimersByTimeAsync(3001);
        expect(region.textContent).toBe("This Incident was updated");
    } finally {
        vi.useRealTimers();
    }
});

test("keyboard shortcuts are on by default and can be switched off", (): void => {
    localStorage.clear();
    expect(ims.keyboardShortcutsEnabled()).toBe(true);

    ims.setKeyboardShortcutsEnabled(false);
    expect(ims.keyboardShortcutsEnabled()).toBe(false);

    ims.setKeyboardShortcutsEnabled(true);
    expect(ims.keyboardShortcutsEnabled()).toBe(true);
});

test("blockKeyboardShortcutFieldActive blocks everything when shortcuts are off", (): void => {
    localStorage.clear();
    document.body.innerHTML = "";

    // With shortcuts on, a page with nothing focused runs them.
    expect(ims.blockKeyboardShortcutFieldActive()).toBe(false);

    // With them off, they're inert regardless of what's focused (WCAG 2.1.4).
    ims.setKeyboardShortcutsEnabled(false);
    expect(ims.blockKeyboardShortcutFieldActive()).toBe(true);

    localStorage.clear();
});

// This is the shape DataTables 2.x leaves behind: the table sits inside a
// .dt-container, and the header the user sees is a *clone* in a separate table
// (.dt-scroll-head), while the real table's own header is hidden. Sort controls
// are role="button" spans with an accessible name but no tabindex, so a
// keyboard user can neither reach them nor sort by them.
function dataTablesMarkup(): void {
    document.body.innerHTML = `
        <div id="queue_table_wrapper" class="dt-container">
          <div class="dt-scroll">
            <div class="dt-scroll-head">
              <table class="dataTable">
                <thead>
                  <tr>
                    <th scope="col">
                      <span class="dt-column-order" role="button" aria-label="#: Activate to sort"></span>
                    </th>
                  </tr>
                </thead>
              </table>
            </div>
            <div class="dt-scroll-body">
              <table id="queue_table" class="dataTable">
                <thead>
                  <tr>
                    <th scope="col">
                      <span class="dt-column-order" role="button" aria-label="#: Activate to sort"></span>
                    </th>
                  </tr>
                </thead>
                <tbody></tbody>
              </table>
            </div>
          </div>
        </div>
    `;
}

test("enableKeyboardSorting makes the visible sort controls reachable and operable", (): void => {
    dataTablesMarkup();
    // The one the user actually sees and clicks is in the cloned header.
    const visible = document.querySelector(".dt-scroll-head .dt-column-order") as HTMLElement;
    expect(visible.hasAttribute("tabindex")).toBe(false);

    let clicks = 0;
    visible.addEventListener("click", (): void => void clicks++);

    ims.enableKeyboardSorting("queue_table");
    expect(visible.tabIndex).toBe(0);

    visible.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    expect(clicks).toBe(1);

    visible.dispatchEvent(new KeyboardEvent("keydown", { key: " ", bubbles: true }));
    expect(clicks).toBe(2);

    // Other keys are left alone, so typing still works normally.
    visible.dispatchEvent(new KeyboardEvent("keydown", { key: "a", bubbles: true }));
    expect(clicks).toBe(2);
});

test("enableKeyboardSorting restores the tabindex when DataTables rebuilds the header", async (): Promise<void> => {
    dataTablesMarkup();
    ims.enableKeyboardSorting("queue_table");

    // DataTables replaces the header cells after init and on redraws, which
    // throws the tabindex away.
    const thead = document.querySelector(".dt-scroll-head thead")!;
    thead.innerHTML = `
        <tr>
          <th scope="col">
            <span class="dt-column-order" role="button" aria-label="#: Activate to sort"></span>
          </th>
        </tr>
    `;
    const rebuilt = document.querySelector(".dt-scroll-head .dt-column-order") as HTMLElement;
    expect(rebuilt.hasAttribute("tabindex")).toBe(false);

    // MutationObserver delivers its records on the microtask queue.
    await vi.waitFor((): void => {
        expect(rebuilt.tabIndex).toBe(0);
    });
});

test("enableKeyboardSorting is a no-op when the table isn't there", (): void => {
    document.body.innerHTML = "";
    expect((): void => {
        ims.enableKeyboardSorting("queue_table");
    }).not.toThrow();
});
