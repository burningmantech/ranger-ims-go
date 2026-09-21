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

// Tests for field_report.ts against the real templ-rendered field report page
// (field_report.templ).

import { beforeEach, expect, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { jsonResponse, loadFixture, mockFetch, mockXHR, problemResponse } from "./helpers.ts";

const eventName = "2025";
const eventId = 1;
const frUrl = `/ims/api/events/${eventName}/field_reports`;

let serverEventAccess: ims.AuthInfoEventAccess;
let serverFieldReport: ims.FieldReport;
let serverEvents: ims.EventData[];

beforeEach((): void => {
    vi.resetModules();
    loadFixture("field_report.html");
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/field_reports/7`);

    // The event source lock needs a secure context and the Web Locks API, which
    // happy-dom doesn't provide; park the request so init doesn't loop. (Same
    // approach as incident.test.ts.)
    vi.stubGlobal("isSecureContext", true);
    Object.defineProperty(navigator, "locks", {
        configurable: true,
        value: { request: (): Promise<undefined> => new Promise<undefined>((): void => {}) },
    });

    serverEventAccess = {
        event_id: eventId,
        readIncidents: true,
        writeIncidents: true,
        writeFieldReports: true,
        readVisits: true,
        writeVisits: true,
        attachFiles: true,
    };
    serverFieldReport = {
        number: 7,
        summary: "Lost child near center camp",
        incident: null,
        report_entries: [
            { id: 1, created: "2025-08-25T10:00:00Z", author: "Tool", text: "Found them", system_entry: false },
        ],
    };
    serverEvents = [{ id: eventId, name: eventName }];
});

function frRoutes(url: string, init?: RequestInit): Response | undefined {
    const hasBody = init?.body != null;
    if (url === `/ims/api/auth?event_id=${eventName}`) {
        return jsonResponse({
            authenticated: true,
            user: "Tester",
            admin: false,
            event_access: { [eventName]: serverEventAccess },
        });
    }
    if (url === "/ims/api/events") {
        return jsonResponse(serverEvents);
    }
    if (url === `${frUrl}/7` && !hasBody) {
        return jsonResponse(serverFieldReport, 200);
    }
    if (url.startsWith(`${frUrl}/7`) && hasBody) {
        // Edits and attach/detach.
        return new Response(null, { status: 204 });
    }
    if (url === `/ims/api/events/${eventName}/incidents` && hasBody) {
        return new Response(null, { status: 201, headers: { "IMS-Incident-Number": "42" } });
    }
    return undefined;
}

async function initFieldReportPage(handler: (url: string, init?: RequestInit) => Response | undefined = frRoutes) {
    const mock = mockFetch(handler);
    await import("../typescript/field_report.ts");
    await vi.waitFor((): void => {
        expect(document.getElementById("loading-overlay")!.style.display).toBe("none");
    });
    return mock;
}

function inputValue(id: string): string {
    return (document.getElementById(id) as HTMLInputElement).value;
}

// The JSON bodies POSTed to the given URL, oldest first.
function postedBodies(mock: ReturnType<typeof mockFetch>, url: string): unknown[] {
    return mock.mock.calls
        .filter(([u, init]): boolean => u === url && init?.body != null)
        .map(([, init]): unknown => JSON.parse(init!.body as string));
}

function errorText(): string {
    return document.getElementById("error_text")!.textContent ?? "";
}

test("page init draws the field report number and summary from the API", async (): Promise<void> => {
    await initFieldReportPage();

    expect(inputValue("field_report_number")).toBe("7");
    expect(inputValue("field_report_summary")).toBe("Lost child near center camp");
    expect(document.title).toContain("Lost child near center camp");
    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(true);
});

test("an unattached field report offers the create-incident button to incident writers", async (): Promise<void> => {
    await initFieldReportPage();

    expect(document.getElementById("create_incident")!.classList.contains("hidden")).toBe(false);
    // No incident is linked yet.
    expect(inputValue("incident_number")).toBe("");
});

test("the IMS # placeholder tells a non-incident-writer to put the number in the summary", async (): Promise<void> => {
    serverEventAccess.writeIncidents = false;

    await initFieldReportPage();

    const incidentInput = document.getElementById("incident_number") as HTMLInputElement;
    expect(incidentInput.placeholder).toBe("(include in summary)");
    expect(incidentInput.readOnly).toBe(true);
    expect(document.getElementById("create_incident")!.classList.contains("hidden")).toBe(true);
});

test("the IMS # placeholder is just \"(none)\" for an incident writer", async (): Promise<void> => {
    await initFieldReportPage();

    expect((document.getElementById("incident_number") as HTMLInputElement).placeholder).toBe("(none)");
});

test("makeIncident creates an incident, attaches the report, and links it", async (): Promise<void> => {
    const mock = await initFieldReportPage();

    // After creation the reload should report the FR attached to incident 42.
    serverFieldReport.incident = 42;
    await window.makeIncident();

    const incidentCreate = mock.mock.calls.find(([url, init]) =>
        url === `/ims/api/events/${eventName}/incidents` && init?.body != null)!;
    expect(JSON.parse(incidentCreate[1]!.body as string)).toEqual({
        summary: "Lost child near center camp",
        ranger_handles: ["Tool"],
    });
    // The FR is then attached to the freshly-created incident 42.
    expect(mock.mock.calls.some(([url, init]) =>
        url === `${frUrl}/7?action=attach&incident=42` && init?.body != null)).toBe(true);
});

test("updateIncident attaches the field report to a typed-in incident number", async (): Promise<void> => {
    const mock = await initFieldReportPage();

    const incidentInput = document.getElementById("incident_number") as HTMLInputElement;
    incidentInput.value = "13";
    await window.updateIncident(incidentInput);

    expect(mock.mock.calls.some(([url, init]) =>
        url === `${frUrl}/7?action=attach&incident=13` && init?.body != null)).toBe(true);
});

test("a viewer without field-report read access sees an authorization error", async (): Promise<void> => {
    serverEventAccess.readIncidents = false;
    serverEventAccess.writeFieldReports = false;

    await initFieldReportPage();

    expect(document.getElementById("error_info")!.classList.contains("hidden")).toBe(false);
    expect(document.getElementById("error_text")!.textContent).toContain("not currently authorized");
});

// Find the Preview or Download link on the first report entry.
function attachmentLink(label: string): HTMLAnchorElement | undefined {
    const entry = document.querySelector<HTMLDivElement>("#report_entries .report_entry")!;
    return [...entry.querySelectorAll("a")]
        .find((a: HTMLAnchorElement): boolean => (a.textContent ?? "").includes(label));
}

test("an entry's attachment links to the field report's own endpoint", async (): Promise<void> => {
    serverFieldReport.report_entries![0]!.attachment = { name: "found.jpg", previewable: true };
    await initFieldReportPage();

    const preview = attachmentLink("Preview")!;
    expect(preview.getAttribute("href")).toBe(`${frUrl}/7/attachments/1`);
    expect(preview.target).toBe("_blank");

    const download = attachmentLink("Download")!;
    expect(download.getAttribute("href")).toBe(`${frUrl}/7/attachments/1?download=true`);
    expect(download.target).toBe("");
});

test("attachment links stay usable for a reader who can't write the field report", async (): Promise<void> => {
    serverEventAccess.writeFieldReports = false;
    serverFieldReport.report_entries![0]!.attachment = { name: "found.jpg", previewable: true };
    await initFieldReportPage();

    // Editing is off for this user.
    expect((document.getElementById("field_report_summary") as HTMLInputElement).disabled).toBe(true);

    // disableEditing() works through the form-control-lite class, which these
    // links must not carry.
    const preview = attachmentLink("Preview")!;
    const download = attachmentLink("Download")!;
    expect(preview.classList.contains("form-control-lite")).toBe(false);
    expect(download.classList.contains("form-control-lite")).toBe(false);
    expect(preview.getAttribute("href")).toBe(`${frUrl}/7/attachments/1`);
    expect(download.getAttribute("href")).toBe(`${frUrl}/7/attachments/1?download=true`);
});

test("attachFile shows an uploading state, posts the file, then confirms and reverts", async (): Promise<void> => {
    await initFieldReportPage();
    const button = document.getElementById("attach_file") as HTMLInputElement;
    expect(button.value).toBe("Attach file");

    const labels: string[] = [];
    const uploads = mockXHR(
        (url) => url === `${frUrl}/7/attachments` ? new Response(null, { status: 204 }) : undefined,
        { progress: [0.25, 1], onProgress: (): void => { labels.push(button.value); } },
    );

    vi.useFakeTimers();
    try {
        // The synchronous prefix of attachFile disables the button and relabels
        // it before the upload is awaited.
        const pending = window.attachFile();
        expect(button.disabled).toBe(true);
        expect(button.value).toBe("Uploading …");

        await pending;

        // The file form data went to the attachments endpoint.
        expect(uploads.length).toBe(1);
        expect(uploads[0]!.url).toBe(`${frUrl}/7/attachments`);
        expect(uploads[0]!.body).toBeInstanceOf(FormData);

        // The button tracked the upload, then waited on the server to store it.
        expect(labels).toEqual(["Uploading 25%", "Uploading 100%", "Uploading …"]);

        // On success the button re-enables and briefly confirms.
        expect(button.disabled).toBe(false);
        expect(button.value).toBe("Uploaded ✓");

        // The confirmation reverts to the default label after a moment.
        vi.advanceTimersByTime(2000);
        expect(button.value).toBe("Attach file");
    } finally {
        vi.useRealTimers();
    }
});

test("a failed attachment re-enables the button and surfaces the error", async (): Promise<void> => {
    await initFieldReportPage();
    mockXHR(() => undefined);
    const button = document.getElementById("attach_file") as HTMLInputElement;

    // A failure has to clear the file input too, or picking the same file again
    // fires no change event and the retry does nothing. Neither happy-dom nor a
    // browser lets a test assign a file input's value, so watch the assignment.
    const cleared = vi.fn();
    Object.defineProperty(document.getElementById("attach_file_input")!, "value", {
        configurable: true,
        get: (): string => "",
        set: cleared,
    });

    await window.attachFile();

    // The button is left usable, keeps its default label (no success), and the
    // failure is shown to the user.
    expect(button.disabled).toBe(false);
    expect(button.value).toBe("Attach file");
    expect(document.getElementById("error_text")!.textContent).toContain("Failed to attach file");
    expect(cleared).toHaveBeenCalledWith("");
});

test("editing the summary sends the edit and reloads the field report", async (): Promise<void> => {
    const mock = await initFieldReportPage();

    const summary = document.getElementById("field_report_summary") as HTMLInputElement;
    summary.value = "Child found";
    serverFieldReport.summary = "Child found (as saved)";
    await window.editSummary();

    expect(postedBodies(mock, `${frUrl}/7`)).toEqual([{ summary: "Child found", number: 7 }]);
    expect(inputValue("field_report_summary")).toBe("Child found (as saved)");
    expect(summary.classList.contains("is-valid")).toBe(true);
});

test("a failed summary edit reloads the field report and shows the error", async (): Promise<void> => {
    await initFieldReportPage((url: string, init?: RequestInit): Response | undefined => {
        if (url === `${frUrl}/7` && init?.body != null) {
            return problemResponse("database on fire", 500);
        }
        return frRoutes(url, init);
    });

    const summary = document.getElementById("field_report_summary") as HTMLInputElement;
    summary.value = "Child found";
    await window.editSummary();

    expect(errorText()).toContain("Failed to apply edit");
    expect(errorText()).toContain("database on fire");
    // The reload put the server's summary back.
    expect(summary.value).toBe("Lost child near center camp");
    expect(summary.classList.contains("is-invalid")).toBe(true);
});

test("a new field report is created on the first edit and adopts the server's number", async (): Promise<void> => {
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/field_reports/new`);
    const mock = await initFieldReportPage((url: string, init?: RequestInit): Response | undefined => {
        if (url === frUrl && init?.body != null) {
            return new Response(null, { status: 201, headers: { "IMS-Field-Report-Number": "7" } });
        }
        return frRoutes(url, init);
    });

    expect(inputValue("field_report_number")).toBe("(new)");
    expect(document.activeElement!.id).toBe("field_report_summary");

    const summary = document.getElementById("field_report_summary") as HTMLInputElement;
    summary.value = "Lost child near center camp";
    await window.editSummary();

    // A new field report has no number to send yet.
    expect(postedBodies(mock, frUrl)).toEqual([{ summary: "Lost child near center camp" }]);
    expect(inputValue("field_report_number")).toBe("7");
    expect(window.location.pathname).toBe(`/ims/app/events/${eventName}/field_reports/7`);
    // Later edits go to the new field report rather than creating another.
    summary.value = "Child found";
    await window.editSummary();
    expect(postedBodies(mock, `${frUrl}/7`)).toEqual([{ summary: "Child found", number: 7 }]);
    expect(postedBodies(mock, frUrl)).toHaveLength(1);
});

test("a new field report opens the instructions for a Ranger without incident access", async (): Promise<void> => {
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/field_reports/new`);
    serverEventAccess.readIncidents = false;
    serverEventAccess.writeIncidents = false;
    const instructions = document.getElementById("fr-instructions")!;
    const clicked = vi.fn();
    instructions.addEventListener("click", clicked);

    await initFieldReportPage();

    expect(clicked).toHaveBeenCalledOnce();
});

test("creating a field report without a number in the response flags the field", async (): Promise<void> => {
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/field_reports/new`);
    await initFieldReportPage((url: string, init?: RequestInit): Response | undefined => {
        if (url === frUrl && init?.body != null) {
            return new Response(null, { status: 201 });
        }
        return frRoutes(url, init);
    });

    const summary = document.getElementById("field_report_summary") as HTMLInputElement;
    summary.value = "Lost child";
    await window.editSummary();

    expect(summary.classList.contains("is-invalid")).toBe(true);
    expect(inputValue("field_report_number")).toBe("(new)");
    expect(window.location.pathname).toBe(`/ims/app/events/${eventName}/field_reports/new`);
});

test("creating a field report with a non-integer number in the response flags the field", async (): Promise<void> => {
    window.history.replaceState(null, "", `/ims/app/events/${eventName}/field_reports/new`);
    await initFieldReportPage((url: string, init?: RequestInit): Response | undefined => {
        if (url === frUrl && init?.body != null) {
            return new Response(null, { status: 201, headers: { "IMS-Field-Report-Number": "seven" } });
        }
        return frRoutes(url, init);
    });

    const summary = document.getElementById("field_report_summary") as HTMLInputElement;
    summary.value = "Lost child";
    await window.editSummary();

    expect(summary.classList.contains("is-invalid")).toBe(true);
    expect(inputValue("field_report_number")).toBe("(new)");
});

test("a field report that fails to load shows an error", async (): Promise<void> => {
    await initFieldReportPage((url: string, init?: RequestInit): Response | undefined => {
        if (url === `${frUrl}/7`) {
            return problemResponse("No such field report", 404);
        }
        return frRoutes(url, init);
    });

    expect(errorText()).toContain("Failed to load field report");
    expect(errorText()).toContain("No such field report");
    expect((document.getElementById("field_report_summary") as HTMLInputElement).disabled).toBe(true);
});

test("makeIncident reports a failure to create the incident", async (): Promise<void> => {
    const mock = await initFieldReportPage((url: string, init?: RequestInit): Response | undefined => {
        if (url === `/ims/api/events/${eventName}/incidents`) {
            return problemResponse("no incidents today", 500);
        }
        return frRoutes(url, init);
    });

    await window.makeIncident();

    expect(errorText()).toContain("Failed to create incident");
    expect(errorText()).toContain("no incidents today");
    // Nothing got attached.
    expect(mock.mock.calls.some(([url]): boolean => url.includes("action=attach"))).toBe(false);
});

test("makeIncident reports a created incident with no number", async (): Promise<void> => {
    const mock = await initFieldReportPage((url: string, init?: RequestInit): Response | undefined => {
        if (url === `/ims/api/events/${eventName}/incidents`) {
            return new Response(null, { status: 201 });
        }
        return frRoutes(url, init);
    });

    await window.makeIncident();

    expect(errorText()).toContain("no IMS Incident Number provided");
    expect(mock.mock.calls.some(([url]): boolean => url.includes("action=attach"))).toBe(false);
});

test("makeIncident reports a failure to attach the field report", async (): Promise<void> => {
    await initFieldReportPage((url: string, init?: RequestInit): Response | undefined => {
        if (url.includes("action=attach")) {
            return problemResponse("attach refused", 500);
        }
        return frRoutes(url, init);
    });

    await window.makeIncident();

    expect(errorText()).toContain("Failed to attach field report");
    expect(errorText()).toContain("attach refused");
});

test("updateIncident detaches the field report when the number is cleared", async (): Promise<void> => {
    serverFieldReport.incident = 42;
    const mock = await initFieldReportPage();
    expect(inputValue("incident_number")).toBe("42");

    serverFieldReport.incident = null;
    const incidentInput = document.getElementById("incident_number") as HTMLInputElement;
    incidentInput.value = "";
    await window.updateIncident(incidentInput);

    expect(mock.mock.calls.some(([url, init]) =>
        url === `${frUrl}/7?action=detach&incident=42` && init?.body != null)).toBe(true);
    expect(incidentInput.classList.contains("is-valid")).toBe(true);
});

test("updateIncident rejects a non-numeric incident number", async (): Promise<void> => {
    const mock = await initFieldReportPage();
    mock.mockClear();

    const incidentInput = document.getElementById("incident_number") as HTMLInputElement;
    incidentInput.value = "twelve";
    await window.updateIncident(incidentInput);

    expect(incidentInput.classList.contains("is-invalid")).toBe(true);
    expect(mock).not.toHaveBeenCalled();
});

test("updateIncident refuses a user who can't write incidents", async (): Promise<void> => {
    serverEventAccess.writeIncidents = false;
    const mock = await initFieldReportPage();

    const incidentInput = document.getElementById("incident_number") as HTMLInputElement;
    incidentInput.value = "13";
    await window.updateIncident(incidentInput);

    expect(incidentInput.classList.contains("is-invalid")).toBe(true);
    expect(mock.mock.calls.some(([url]): boolean => url.includes("action=attach"))).toBe(false);
});

test("a field report broadcast for this field report reloads it", async (): Promise<void> => {
    await initFieldReportPage();

    serverFieldReport.summary = "Updated by someone else";
    const channel = new BroadcastChannel("field_report_update");
    channel.postMessage({ event_id: eventId, field_report_number: 7 });

    await vi.waitFor((): void => {
        expect(inputValue("field_report_summary")).toBe("Updated by someone else");
    });
    channel.close();
});

test("a field report broadcast for some other field report is ignored", async (): Promise<void> => {
    await initFieldReportPage();
    const log = vi.spyOn(console, "log");

    const channel = new BroadcastChannel("field_report_update");
    channel.postMessage({ event_id: eventId, field_report_number: 8 });
    channel.postMessage({ event_id: eventId + 1, field_report_number: 7 });
    // An update_all queued behind them marks when they've all been handled.
    serverFieldReport.summary = "Updated by someone else";
    channel.postMessage({ update_all: true });

    await vi.waitFor((): void => {
        expect(inputValue("field_report_summary")).toBe("Updated by someone else");
    });
    const reloads = log.mock.calls.filter(([msg]): boolean => String(msg).startsWith("Got field report update"));
    expect(reloads.every(([msg]): boolean => String(msg).includes("update_all = true"))).toBe(true);
    channel.close();
});

test("keyboard shortcuts toggle history and jump to the entry box", async (): Promise<void> => {
    await initFieldReportPage();

    const checkbox = document.getElementById("history_checkbox") as HTMLInputElement;
    expect(checkbox.checked).toBe(false);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "h", bubbles: true }));
    expect(checkbox.checked).toBe(true);

    document.dispatchEvent(new KeyboardEvent("keydown", { key: "a", bubbles: true }));
    expect(document.activeElement!.id).toBe("report_entry_add");

    const open = vi.fn((): Window => window);
    vi.stubGlobal("open", open);
    (document.activeElement as HTMLElement).blur();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "n", bubbles: true }));
    expect(open).toHaveBeenCalledWith("./new", "_blank");
});

test("the strike button strikes a field report entry and reloads", async (): Promise<void> => {
    const mock = await initFieldReportPage();

    const entry = document.querySelector<HTMLDivElement>("#report_entries .report_entry_user")!;
    const strike = entry.querySelector("button")!;
    expect(strike.textContent).toBe("Strike");
    serverFieldReport.report_entries![0]!.stricken = true;
    strike.click();

    await vi.waitFor((): void => {
        expect(document.querySelector("#report_entries .report_entry_stricken")).not.toBeNull();
    });
    expect(postedBodies(mock, `${frUrl}/7/report_entries/1`)).toEqual([{ stricken: true }]);
});

test("a failed strike shows an error", async (): Promise<void> => {
    await initFieldReportPage((url: string, init?: RequestInit): Response | undefined => {
        if (url === `${frUrl}/7/report_entries/1`) {
            return problemResponse("strike refused", 500);
        }
        return frRoutes(url, init);
    });

    document.querySelector<HTMLDivElement>("#report_entries .report_entry_user")!.querySelector("button")!.click();

    await vi.waitFor((): void => {
        expect(errorText()).toContain("Failed to set report entry strike status");
    });
    expect(errorText()).toContain("strike refused");
});
