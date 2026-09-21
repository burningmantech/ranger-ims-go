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
import { MockEventSource } from "./helpers.ts";

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
    expect(ims.isValidIncidentsTableState("on_hold")).toBe(true);
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

// This is the shape DataTables leaves behind: the table sits inside a
// .dt-container, and the header the user sees is a *clone* in a separate table
// (.dt-scroll-head), while the real table's own header is hidden. Sort controls
// carry role="button" and an accessible name. This markup leaves off the
// tabindex that newer DataTables adds itself, so the test covers both.
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

test("clearLocalStorage drops the browser-local settings, including the theme", (): void => {
    localStorage.setItem("preferred_incidents_state", "open");
    localStorage.setItem("preferred_visits_status", "current");
    localStorage.setItem("preferred_table_rows_per_page", "50");
    localStorage.setItem("keyboard_shortcuts_enabled", "false");
    localStorage.setItem("theme", "dark");

    ims.clearLocalStorage();

    expect(localStorage.getItem("preferred_incidents_state")).toBeNull();
    expect(localStorage.getItem("preferred_visits_status")).toBeNull();
    expect(localStorage.getItem("preferred_table_rows_per_page")).toBeNull();
    expect(localStorage.getItem("keyboard_shortcuts_enabled")).toBeNull();
    expect(localStorage.getItem("theme")).toBeNull();
});

function linkifiedHTML(text: string): string {
    const p = document.createElement("p");
    p.append(...ims.linkify(text));
    return p.innerHTML;
}

test("linkify leaves text without URLs alone", (): void => {
    expect(ims.linkify("no links here")).toEqual(["no links here"]);
    expect(ims.linkify("")).toEqual([]);
});

test("linkify turns a URL mid-text into a new-tab link without a referrer", (): void => {
    const nodes = ims.linkify("see https://example.com/a?b=c#d for more");
    expect(nodes).toHaveLength(3);
    expect(nodes[0]).toBe("see ");
    expect(nodes[2]).toBe(" for more");
    const link = nodes[1] as HTMLAnchorElement;
    expect(link.href).toBe("https://example.com/a?b=c#d");
    expect(link.textContent).toBe("https://example.com/a?b=c#d");
    expect(link.target).toBe("_blank");
    expect(link.rel).toBe("noopener noreferrer");
});

test("linkify handles several URLs, including plain http", (): void => {
    const nodes = ims.linkify("http://a.example and https://b.example");
    expect(nodes).toHaveLength(3);
    expect((nodes[0] as HTMLAnchorElement).href).toBe("http://a.example/");
    expect(nodes[1]).toBe(" and ");
    expect((nodes[2] as HTMLAnchorElement).href).toBe("https://b.example/");
});

test("linkify trims trailing punctuation", (): void => {
    const nodes = ims.linkify("Go to https://example.com/x.");
    expect((nodes[1] as HTMLAnchorElement).textContent).toBe("https://example.com/x");
    expect(nodes[2]).toBe(".");

    const quoted = ims.linkify(`"https://example.com/y", she said`);
    expect((quoted[1] as HTMLAnchorElement).textContent).toBe("https://example.com/y");
    expect(quoted[2]).toBe(`", she said`);
});

test("linkify drops an unmatched closing paren but keeps a matched one", (): void => {
    const nodes = ims.linkify("(see https://example.com/a).");
    expect((nodes[1] as HTMLAnchorElement).textContent).toBe("https://example.com/a");
    expect(nodes[2]).toBe(").");

    const wiki = ims.linkify("https://en.wikipedia.org/wiki/Burning_Man_(festival)");
    expect(wiki).toHaveLength(1);
    expect((wiki[0] as HTMLAnchorElement).textContent).toBe("https://en.wikipedia.org/wiki/Burning_Man_(festival)");
});

test("linkify ignores other schemes and scheme-like text inside words", (): void => {
    expect(ims.linkify("javascript:alert(1)")).toEqual(["javascript:alert(1)"]);
    expect(ims.linkify("data:text/html,<b>hi</b>")).toEqual(["data:text/html,<b>hi</b>"]);
    expect(ims.linkify("xhttps://example.com")).toEqual(["xhttps://example.com"]);
    expect(ims.linkify("www.example.com")).toEqual(["www.example.com"]);
    expect(ims.linkify("https://")).toEqual(["https://"]);
});

test("linkify never interprets markup", (): void => {
    expect(linkifiedHTML(`<img src=x onerror=alert(1)> https://example.com/"><script>`)).toBe(
        `&lt;img src=x onerror=alert(1)&gt; <a href="https://example.com/" target="_blank" rel="noopener noreferrer">https://example.com/</a>"&gt;&lt;script&gt;`,
    );
});

// The number renderers link using the page's event, which comes from the URL.
function withEventName(eventName: string, fn: () => void): void {
    const previous = ims.pathIds.eventName;
    ims.pathIds.eventName = eventName;
    try {
        fn();
    } finally {
        ims.pathIds.eventName = previous;
    }
}

test("renderIncidentNumber links the number for display and returns it raw otherwise", (): void => {
    withEventName("2025", (): void => {
        const link = ims.renderIncidentNumber(5, "display", {}) as HTMLAnchorElement;
        expect(link.getAttribute("href")).toBe("/ims/app/events/2025/incidents/5");
        expect(link.textContent).toBe("5");
    });
    expect(ims.renderIncidentNumber(null, "display", {})).toBeNull();
    expect(ims.renderIncidentNumber(5, "sort", {})).toBe(5);
    expect(ims.renderIncidentNumber(5, "filter", {})).toBe(5);
    expect(ims.renderIncidentNumber(5, "bogus", {})).toBeUndefined();
});

test("renderFieldReportNumber links the number for display and returns it raw otherwise", (): void => {
    withEventName("2025", (): void => {
        const link = ims.renderFieldReportNumber(7, "display", {}) as HTMLAnchorElement;
        expect(link.getAttribute("href")).toBe("/ims/app/events/2025/field_reports/7");
        expect(link.textContent).toBe("7");
    });
    expect(ims.renderFieldReportNumber(null, "display", {})).toBeNull();
    expect(ims.renderFieldReportNumber(7, "sort", {})).toBe(7);
    expect(ims.renderFieldReportNumber(7, "type", {})).toBe(7);
    expect(ims.renderFieldReportNumber(7, "bogus", {})).toBeUndefined();
});

test("renderVisitNumber links the number for display and returns it raw otherwise", (): void => {
    withEventName("2025", (): void => {
        const link = ims.renderVisitNumber(3, "display", {}) as HTMLAnchorElement;
        expect(link.getAttribute("href")).toBe("/ims/app/events/2025/visits/3");
        expect(link.textContent).toBe("3");
    });
    expect(ims.renderVisitNumber(null, "display", {})).toBeNull();
    expect(ims.renderVisitNumber(3, "sort", {})).toBe(3);
    expect(ims.renderVisitNumber(3, undefined as unknown as string, {})).toBe(3);
    expect(ims.renderVisitNumber(3, "bogus", {})).toBeUndefined();
});

test("renderDate shows a short date and time, with the full date on hover", (): void => {
    const iso = "2025-08-25T10:05:00Z";
    const millis = Date.parse(iso);

    const span = ims.renderDate(iso, "display", {}) as HTMLSpanElement;
    expect(span.title).toBe(ims.longFormatDate(millis));
    expect(span.textContent).toBe(ims.shortDate.format(millis) + ims.shortTime.format(millis));
    expect(span.querySelector("br")).not.toBeNull();

    expect(ims.renderDate(iso, "filter", {})).toBe(`${ims.shortDate.format(millis)} ${ims.shortTime.format(millis)}`);
    expect(ims.renderDate(iso, "sort", {})).toBe(millis);
    expect(ims.renderDate(iso, "bogus", {})).toBeUndefined();
    expect(ims.renderDate(undefined, "display", {})).toBeUndefined();
});

test("renderState shows the state's name and sorts in workflow order", (): void => {
    expect(ims.renderState("new", "display", {})).toBe("New");
    expect(ims.renderState("on_hold", "filter", {})).toBe("On Hold");
    expect(ims.renderState("dispatched", "type", {})).toBe("Dispatched");
    expect(ims.renderState("on_scene", "display", {})).toBe("On Scene");
    expect(ims.renderState("closed", "display", {})).toBe("Closed");

    expect(ims.renderState("new", "sort", {})).toBe(1);
    expect(ims.renderState("on_hold", "sort", {})).toBe(2);
    expect(ims.renderState("dispatched", "sort", {})).toBe(3);
    expect(ims.renderState("on_scene", "sort", {})).toBe(4);
    expect(ims.renderState("closed", "sort", {})).toBe(5);

    expect(ims.renderState("closed", "bogus", {})).toBeUndefined();
});

test("renderState falls back to the incident's state, and to Unknown", (): void => {
    const warn = vi.spyOn(console, "warn").mockImplementation((): void => {});

    expect(ims.renderState(null as unknown as ims.IncidentState, "display", { state: "on_scene" })).toBe("On Scene");

    expect(ims.renderState("null", "display", {})).toBe("Unknown");
    expect(ims.renderState("null", "sort", {})).toBeUndefined();
    expect(ims.renderState(null as unknown as ims.IncidentState, "display", {})).toBe("Unknown");
    expect(warn).toHaveBeenCalled();
});

test("renderLocation shows the name and address", (): void => {
    const location: NonNullable<ims.Incident["location"]> = { name: "The Man", address: "12:00 & A" };

    const span = ims.renderLocation(location, "display", {}) as HTMLSpanElement;
    expect(span.textContent).toBe("The Man (12:00 & A)");
    // The address may wrap onto its own line.
    expect(span.querySelector("wbr")).not.toBeNull();

    expect(ims.renderLocation(location, "filter", {})).toBe("The Man (12:00 & A)");
    expect(ims.renderLocation(location, "sort", {})).toBe("The Man (12:00 & A)");
    expect(ims.renderLocation(location, "bogus" as ims.RenderType, {})).toBeUndefined();
    expect(ims.renderLocation(null, "display", {})).toBeUndefined();
});

test("renderLocation leaves out a missing address", (): void => {
    const location: NonNullable<ims.Incident["location"]> = { name: "The Man" };

    const span = ims.renderLocation(location, "display", {}) as HTMLSpanElement;
    expect(span.textContent).toBe("The Man");
    expect(span.querySelector("wbr")).toBeNull();
});

test("renderRangerHandles lists the handles sorted, skipping missing ones", (): void => {
    const rangers: ims.IncidentRanger[] = [{ handle: "Tool" }, { handle: null }, { handle: "Hot Slots", role: "lead" }];

    const span = ims.renderRangerHandles(rangers, "display", {}) as HTMLSpanElement;
    expect(span.textContent).toBe("Hot Slots, Tool");
    expect(span.querySelectorAll("wbr")).toHaveLength(1);

    expect(ims.renderRangerHandles(rangers, "filter", {})).toBe("Hot Slots, Tool");
    expect(ims.renderRangerHandles(rangers, "sort", {})).toBe("Hot Slots, Tool");
    expect(ims.renderRangerHandles(rangers, "bogus" as ims.RenderType, {})).toBeUndefined();
    expect(ims.renderRangerHandles(null, "display", {})).toBeUndefined();
});

test("localTzOffset gives the browser's UTC offset", (): void => {
    // Empty in UTC itself, which formats as a bare "GMT".
    expect(ims.localTzOffset(new Date())).toMatch(/^([+-]\d{2}:\d{2})?$/);
});

test("requestEventSourceLock refuses to run in an insecure context", (): void => {
    document.body.innerHTML = `
        <div id="error_info" class="hidden text-danger-emphasis" role="alert">
            <p id="error_text"></p>
        </div>
    `;
    vi.stubGlobal("isSecureContext", false);
    const request = vi.fn();
    Object.defineProperty(navigator, "locks", { configurable: true, value: { request: request } });

    ims.requestEventSourceLock();

    expect(document.getElementById("error_text")!.textContent).toContain("insecure browsing context");
    expect(request).not.toHaveBeenCalled();
    expect(MockEventSource.instances).toHaveLength(0);
});

// Take the EventSource lock the way a page does, with a Web Locks stand-in
// that grants the lock once and then parks any later request. Resolves with
// the EventSource the leader opened, and a promise that settles when the
// leader gives the lock back up.
async function takeEventSourceLock(): Promise<{ source: MockEventSource, released: Promise<void> }> {
    vi.stubGlobal("isSecureContext", true);
    const released = Promise.withResolvers<void>();
    let granted = false;
    Object.defineProperty(navigator, "locks", {
        configurable: true,
        value: {
            request: (_name: string, callback: () => Promise<void>): Promise<void> => {
                if (granted) {
                    return new Promise<void>((): void => {});
                }
                granted = true;
                return callback().then(released.resolve);
            },
        },
    });

    ims.requestEventSourceLock();

    await vi.waitFor((): void => {
        expect(MockEventSource.instances).toHaveLength(1);
    });
    return { source: MockEventSource.instances[0]!, released: released.promise };
}

// Collect what arrives on a BroadcastChannel.
function listen(channelName: string): { messages: unknown[], channel: BroadcastChannel } {
    const messages: unknown[] = [];
    const channel = new BroadcastChannel(channelName);
    channel.onmessage = (e: MessageEvent): void => void messages.push(e.data);
    return { messages: messages, channel: channel };
}

test("the EventSource leader relays each update to its BroadcastChannel", async (): Promise<void> => {
    const { source } = await takeEventSourceLock();
    expect(source.url).toBe("/ims/api/eventsource");
    expect(source.withCredentials).toBe(true);

    const incidents = listen("incident_update");
    const fieldReports = listen("field_report_update");
    const visits = listen("visit_update");

    source.dispatchEvent(new MessageEvent("Incident", {
        data: JSON.stringify({ event_id: 1, incident_number: 4 }), lastEventId: "10",
    }));
    source.dispatchEvent(new MessageEvent("FieldReport", {
        data: JSON.stringify({ event_id: 1, field_report_number: 7 }), lastEventId: "11",
    }));
    source.dispatchEvent(new MessageEvent("Visit", {
        data: JSON.stringify({ event_id: 1, visit_number: 2 }), lastEventId: "12",
    }));

    await vi.waitFor((): void => {
        expect(incidents.messages).toEqual([{ event_id: 1, incident_number: 4 }]);
        expect(fieldReports.messages).toEqual([{ event_id: 1, field_report_number: 7 }]);
        expect(visits.messages).toEqual([{ event_id: 1, visit_number: 2 }]);
    });
    // The last event seen is remembered, so a reconnect can tell whether it missed any.
    expect(localStorage.getItem("last_sse_id")).toBe("12");

    incidents.channel.close();
    fieldReports.channel.close();
    visits.channel.close();
});

test("an InitialEvent asks every page to reload, unless nothing was missed", async (): Promise<void> => {
    const { source } = await takeEventSourceLock();
    const incidents = listen("incident_update");
    const fieldReports = listen("field_report_update");

    // A fresh connection, with no record of what came before.
    source.dispatchEvent(new MessageEvent("InitialEvent", { lastEventId: "20" }));
    await vi.waitFor((): void => {
        expect(incidents.messages).toEqual([{ update_all: true }]);
        expect(fieldReports.messages).toEqual([{ update_all: true }]);
    });
    expect(localStorage.getItem("last_sse_id")).toBe("20");

    // A reconnect that picks up where the last connection left off needs no reload.
    // A later Incident event marks when the InitialEvent has been handled.
    source.dispatchEvent(new MessageEvent("InitialEvent", { lastEventId: "20" }));
    source.dispatchEvent(new MessageEvent("Incident", {
        data: JSON.stringify({ event_id: 1, incident_number: 4 }), lastEventId: "21",
    }));
    await vi.waitFor((): void => {
        expect(incidents.messages).toEqual([{ update_all: true }, { event_id: 1, incident_number: 4 }]);
    });
    expect(fieldReports.messages).toEqual([{ update_all: true }]);

    incidents.channel.close();
    fieldReports.channel.close();
});

test("the EventSource leader gives up the lock only once the connection is closed", async (): Promise<void> => {
    const { source, released } = await takeEventSourceLock();
    let isReleased = false;
    void released.then((): void => { isReleased = true; });

    // A transient error, which EventSource reconnects from by itself.
    source.readyState = MockEventSource.OPEN;
    source.dispatchEvent(new Event("error"));
    await Promise.resolve();
    expect(isReleased).toBe(false);

    source.readyState = MockEventSource.CLOSED;
    source.dispatchEvent(new Event("error"));
    await released;
    expect(isReleased).toBe(true);
});
