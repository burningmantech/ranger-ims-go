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

import { beforeEach, expect, onTestFinished, test, vi } from "vitest";
import type * as ims from "../typescript/ims.ts";
import { captureLinkClicks, jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

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
    if (/\/attachments\/\d+$/.test(url) && !hasBody) {
        return new Response("file contents", { status: 200 });
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

test("an entry's attachment is fetched from the field report's own endpoint", async (): Promise<void> => {
    serverFieldReport.report_entries![0]!.attachment = { name: "found.jpg", previewable: true };
    const mock = await initFieldReportPage();
    const links = captureLinkClicks();

    const entry = document.querySelector<HTMLDivElement>("#report_entries .report_entry")!;
    const download = [...entry.querySelectorAll("button")]
        .find((b: HTMLButtonElement): boolean => (b.textContent ?? "").includes("Download"))!;
    mock.mockClear();
    download.click();

    await vi.waitFor((): void => {
        expect(mock.mock.calls.map(([url]): string => url)).toContain(`${frUrl}/7/attachments/1`);
        expect(links.length).toBe(1);
    });
    expect(links[0]!.download).toBe("found.jpg");
    expect(document.getElementById("error_text")!.textContent).toBe("");
});

test("downloading an attachment shows progress on the button, then restores it", async (): Promise<void> => {
    serverFieldReport.report_entries![0]!.attachment = { name: "found.jpg", previewable: true };

    // A body the test feeds by hand, so the button can be inspected mid-download.
    let push: (chunk: Uint8Array) => void = (): void => {};
    let finish: () => void = (): void => {};
    const body = new ReadableStream<Uint8Array>({
        start(controller): void {
            push = (chunk: Uint8Array): void => controller.enqueue(chunk);
            finish = (): void => controller.close();
        },
    });
    await initFieldReportPage((url, init) => {
        if (/\/attachments\/\d+$/.test(url) && init?.body == null) {
            return new Response(body, { status: 200, headers: { "Content-Length": "10" } });
        }
        return frRoutes(url, init);
    });
    const links = captureLinkClicks();
    const blobs: Blob[] = [];
    vi.spyOn(window.URL, "createObjectURL").mockImplementation((blob: Blob|MediaSource): string => {
        blobs.push(blob as Blob);
        return "blob:fake";
    });

    const entry = document.querySelector<HTMLDivElement>("#report_entries .report_entry")!;
    const download = [...entry.querySelectorAll("button")]
        .find((b: HTMLButtonElement): boolean => (b.textContent ?? "").includes("Download"))!;
    download.click();

    // Halfway through, the button is unusable and reports how far along it is.
    push(new Uint8Array([1, 2, 3, 4, 5]));
    await vi.waitFor((): void => {
        expect(download.textContent).toContain("50%");
    });
    expect(download.disabled).toBe(true);

    push(new Uint8Array([6, 7, 8, 9, 10]));
    finish();

    await vi.waitFor((): void => {
        expect(links.length).toBe(1);
    });
    // The whole file made it through the streamed read.
    expect(await blobs[0]!.arrayBuffer()).toEqual(new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8, 9, 10]).buffer);
    // The button goes back to being a Download button.
    expect(download.disabled).toBe(false);
    expect(download.textContent).toContain("Download");
    expect(download.textContent).not.toContain("%");
});

// A slow preview download outlives the click that started it, and the browser
// would block the new tab as a popup, so the file waits for a second click.
test("a preview whose download outlives its click waits to be opened", async (): Promise<void> => {
    serverFieldReport.report_entries![0]!.attachment = { name: "found.jpg", previewable: true };
    // The click no longer counts as user activation, as after a long download.
    Object.defineProperty(navigator, "userActivation", {
        configurable: true,
        value: { isActive: false, hasBeenActive: true },
    });
    onTestFinished((): void => {
        Reflect.deleteProperty(navigator, "userActivation");
    });

    await initFieldReportPage();
    const links = captureLinkClicks();
    vi.spyOn(window.URL, "createObjectURL").mockReturnValue("blob:fake");

    const entry = document.querySelector<HTMLDivElement>("#report_entries .report_entry")!;
    const preview = [...entry.querySelectorAll("button")]
        .find((b: HTMLButtonElement): boolean => (b.textContent ?? "").includes("Preview"))!;
    preview.click();

    // The file arrived, but opening it now would be blocked, so the button says
    // it's holding one rather than opening a tab.
    await vi.waitFor((): void => {
        expect(preview.textContent).toContain("Preview Ready");
    });
    expect(links.length).toBe(0);
    expect(preview.disabled).toBe(false);

    // The second click opens the file it already has, without re-fetching.
    preview.click();
    expect(links.length).toBe(1);
    expect(links[0]!.target).toBe("_blank");
    expect(links[0]!.href).toContain("blob:fake");
    expect(preview.textContent).toContain("Preview");
    expect(preview.textContent).not.toContain("Ready");
});

// The report entries are rebuilt from scratch on every update, including ones
// that arrive from other users, which replaces the button a transfer is running
// on. The replacement has to pick up where its predecessor left off.
test("a redraw mid-download hands the progress to the newly drawn button", async (): Promise<void> => {
    serverFieldReport.report_entries![0]!.attachment = { name: "found.jpg", previewable: true };

    let push: (chunk: Uint8Array) => void = (): void => {};
    let finish: () => void = (): void => {};
    const body = new ReadableStream<Uint8Array>({
        start(controller): void {
            push = (chunk: Uint8Array): void => controller.enqueue(chunk);
            finish = (): void => controller.close();
        },
    });
    await initFieldReportPage((url, init) => {
        if (/\/attachments\/\d+$/.test(url) && init?.body == null) {
            return new Response(body, { status: 200, headers: { "Content-Length": "10" } });
        }
        return frRoutes(url, init);
    });
    const links = captureLinkClicks();
    vi.spyOn(window.URL, "createObjectURL").mockReturnValue("blob:fake");
    const ims = await import("../typescript/ims.ts");

    const downloadButton = (): HTMLButtonElement =>
        [...document.querySelectorAll<HTMLDivElement>("#report_entries .report_entry")[0]!.querySelectorAll("button")]
            .find((b: HTMLButtonElement): boolean => (b.textContent ?? "").includes("Download"))!;

    const firstButt = downloadButton();
    firstButt.click();
    push(new Uint8Array([1, 2, 3, 4, 5]));
    await vi.waitFor((): void => {
        expect(firstButt.textContent).toContain("50%");
    });

    // Someone else's update redraws every entry, discarding the button that was
    // showing the progress.
    ims.drawReportEntries(serverFieldReport.report_entries!);
    const redrawnButt = downloadButton();
    expect(redrawnButt).not.toBe(firstButt);
    expect(redrawnButt.textContent).toContain("50%");
    expect(redrawnButt.disabled).toBe(true);

    push(new Uint8Array([6, 7, 8, 9, 10]));
    finish();

    // The download still completes, and it's the new button that gets cleaned up.
    await vi.waitFor((): void => {
        expect(links.length).toBe(1);
    });
    expect(redrawnButt.disabled).toBe(false);
    expect(redrawnButt.textContent).toContain("Download");
    expect(redrawnButt.textContent).not.toContain("%");
});

// A file parked for a second click has to survive a redraw too, or the only
// reference to it is lost and the user has to download it all over again.
test("a redraw keeps a preview that's waiting to be opened", async (): Promise<void> => {
    serverFieldReport.report_entries![0]!.attachment = { name: "found.jpg", previewable: true };
    Object.defineProperty(navigator, "userActivation", {
        configurable: true,
        value: { isActive: false, hasBeenActive: true },
    });
    onTestFinished((): void => {
        Reflect.deleteProperty(navigator, "userActivation");
    });

    const mock = await initFieldReportPage();
    const links = captureLinkClicks();
    vi.spyOn(window.URL, "createObjectURL").mockReturnValue("blob:fake");
    const ims = await import("../typescript/ims.ts");

    const previewButton = (): HTMLButtonElement =>
        [...document.querySelectorAll<HTMLDivElement>("#report_entries .report_entry")[0]!.querySelectorAll("button")]
            .find((b: HTMLButtonElement): boolean => (b.textContent ?? "").includes("Preview"))!;

    previewButton().click();
    await vi.waitFor((): void => {
        expect(previewButton().textContent).toContain("Preview Ready");
    });

    ims.drawReportEntries(serverFieldReport.report_entries!);
    const redrawnButt = previewButton();
    expect(redrawnButt.textContent).toContain("Preview Ready");

    // The redrawn button opens the file its predecessor fetched, without going
    // back to the server for it.
    mock.mockClear();
    redrawnButt.click();
    expect(links.length).toBe(1);
    expect(links[0]!.href).toContain("blob:fake");
    expect(mock.mock.calls.some(([url]): boolean => url.includes("/attachments/"))).toBe(false);
    expect(redrawnButt.textContent).toContain("Preview");
    expect(redrawnButt.textContent).not.toContain("Ready");
});

test("attachFile shows an uploading state, posts the file, then confirms and reverts", async (): Promise<void> => {
    const mock = await initFieldReportPage();
    const button = document.getElementById("attach_file") as HTMLInputElement;
    expect(button.value).toBe("Attach file");

    vi.useFakeTimers();
    try {
        // The synchronous prefix of attachFile disables the button and relabels
        // it before the upload fetch is awaited.
        const pending = window.attachFile();
        expect(button.disabled).toBe(true);
        expect(button.value).toBe("Uploading...");

        await pending;

        // The file form data went to the attachments endpoint.
        expect(mock.mock.calls.some(([url, init]) =>
            url === `${frUrl}/7/attachments` && init?.body instanceof FormData)).toBe(true);

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
    await initFieldReportPage((url, init) => {
        if (url === `${frUrl}/7/attachments` && init?.body != null) {
            return undefined;
        }
        return frRoutes(url, init);
    });
    const button = document.getElementById("attach_file") as HTMLInputElement;

    await window.attachFile();

    // The button is left usable, keeps its default label (no success), and the
    // failure is shown to the user.
    expect(button.disabled).toBe(false);
    expect(button.value).toBe("Attach file");
    expect(document.getElementById("error_text")!.textContent).toContain("Failed to attach file");
});
