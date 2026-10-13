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

// Tests for admin_error_logs.ts against the real templ-rendered error logs page
// (adminerrorlogs.templ). Like the action logs page, it drives a DataTables grid
// whose ajax source builds query params from the filter inputs; on top of that
// it renders an expandable child row with the recorded error text. DataTables
// itself is a classic-script dependency, so a small stand-in captures the
// options the page passes and runs the ajax source on draw/reload.

import { beforeEach, expect, test, vi } from "vitest";
import { change, jsonResponse, loadFixture, mockFetch, problemResponse } from "./helpers.ts";

interface AjaxCallback { (resp: { data: unknown[] }): void; }
interface DataTableOptions {
    ajax: (data: unknown, callback: AjaxCallback, settings: unknown) => void;
    columns: { name: string; render?: (value: any, type: string, row: any) => unknown }[];
}

// A child row, as DataTables models it: callable to set its content, and
// hidden until the control cell is clicked.
interface MockChild {
    (content: unknown): MockChild;
    content: unknown;
    show(): void;
    hide(): void;
    isShown(): boolean;
}

function newMockChild(): MockChild {
    let shown = false;
    const child = ((content: unknown): MockChild => {
        child.content = content;
        return child;
    }) as MockChild;
    child.content = null;
    child.show = (): void => void (shown = true);
    child.hide = (): void => void (shown = false);
    child.isShown = (): boolean => shown;
    return child;
}

// A minimal DataTables stand-in: it records the options the page constructs,
// runs the ajax source whenever the table is drawn or reloaded, and hands out
// one row object with a child, which is enough for the expansion handler.
class MockDataTable {
    static lastInstance: MockDataTable | null = null;
    options: DataTableOptions;
    lastData: unknown[] = [];
    // The (event, selector, handler) listeners the page delegated to the table.
    delegated: Record<string, (this: HTMLElement) => void> = {};
    // The row that row() hands back, standing in for whichever one was clicked.
    clickedRow: { data: () => unknown; child: MockChild };

    static render = {
        number: () => ({ display: (s: unknown): unknown => s }),
        text: () => ({ display: (s: unknown): unknown => s }),
    };

    ajax = { reload: (): void => this.runAjax() };
    private initHandlers: (() => void)[] = [];
    private rowData: unknown = {};

    constructor(_selector: string, options: DataTableOptions) {
        this.options = options;
        MockDataTable.lastInstance = this;
        this.clickedRow = {
            data: (): unknown => this.rowData,
            child: newMockChild(),
        };
    }

    setRowData(data: unknown): void {
        this.rowData = data;
    }

    row(_tr: unknown) {
        return this.clickedRow;
    }

    on(event: string, selectorOrCb: any, cb?: any): MockDataTable {
        if (event === "init") {
            this.initHandlers.push(selectorOrCb);
        } else if (cb != null) {
            this.delegated[`${event} ${selectorOrCb}`] = cb;
        }
        return this;
    }

    draw(): void {
        for (const cb of this.initHandlers.splice(0)) {
            cb();
        }
        this.runAjax();
    }

    private runAjax(): void {
        this.options.ajax(null, (resp): void => { this.lastData = resp.data; }, null);
    }

    column(name: string) {
        return this.options.columns.find(c => c.name === name);
    }
}

beforeEach((): void => {
    vi.resetModules();
    loadFixture("admin_error_logs.html");
    MockDataTable.lastInstance = null;
    vi.stubGlobal("DataTable", MockDataTable);
    authenticated = true;
    logsFail = false;
});

let authenticated: boolean;
// Whether the fake server fails the logs fetch.
let logsFail: boolean;

async function initErrorLogsPage(rows: unknown[] = []) {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: authenticated, user: "Tester", admin: true });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse([]);
        }
        if (url === url_logout) {
            return new Response(null, { status: 204 });
        }
        if (url.startsWith(url_errorlogs)) {
            return logsFail ? problemResponse("logs unavailable", 500) : jsonResponse(rows);
        }
        return undefined;
    });
    await import("../typescript/admin_error_logs.ts");
    await vi.waitFor((): void => {
        expect(MockDataTable.lastInstance).not.toBeNull();
    });
    return mock;
}

test("the table is fetched from the error logs endpoint with the default min-time filter", async (): Promise<void> => {
    const mock = await initErrorLogsPage([{ id: 1, user_name: "Tester", http_status: 500 }]);

    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url]) => (url as string).startsWith(url_errorlogs))).toBe(true);
    });
    const logCall = mock.mock.calls.find(([url]) => (url as string).startsWith(url_errorlogs))!;
    const params = new URL(logCall[0] as string, "https://localhost").searchParams;
    // init defaults the min-time input to yesterday, so the fetch is bounded.
    expect(params.get("minTimeUnixMs")).not.toBeNull();
});

test("the path filter starts empty, unlike the action logs page", async (): Promise<void> => {
    const mock = await initErrorLogsPage();

    const logCall = mock.mock.calls.find(([url]) => (url as string).startsWith(url_errorlogs))!;
    const params = new URL(logCall[0] as string, "https://localhost").searchParams;
    expect(params.get("path")).toBeNull();
});

test("changing a filter folds the filter inputs into the query params and reloads", async (): Promise<void> => {
    const mock = await initErrorLogsPage();

    (document.getElementById("filter_user_name") as HTMLInputElement).value = "Hubcap";
    (document.getElementById("filter_path") as HTMLInputElement).value = "/ims/api/events";
    (document.getElementById("filter_min_time") as HTMLInputElement).value = "";

    change(document.getElementById("filter_user_name")!);

    await vi.waitFor((): void => {
        const last = mock.mock.calls.at(-1)!;
        expect((last[0] as string).startsWith(url_errorlogs)).toBe(true);
    });
    const last = mock.mock.calls.at(-1)!;
    const params = new URL(last[0] as string, "https://localhost").searchParams;
    expect(params.get("userName")).toBe("Hubcap");
    expect(params.get("path")).toBe("/ims/api/events");
    // Clearing the min-time input drops that bound.
    expect(params.get("minTimeUnixMs")).toBeNull();
});

test("clicking the control cell expands a child row with the error detail as text", async (): Promise<void> => {
    await initErrorLogsPage();

    const table = MockDataTable.lastInstance!;
    table.setRowData({
        id: 1,
        http_status: 500,
        response_message: "The server malfunctioned",
        internal_error: "[getIncident]: <script>alert(1)</script>",
        stack_trace: "goroutine 1 [running]:\nmain.boom()",
    });

    const handler = table.delegated["click td.dt-control"]!;
    const cell = document.createElement("td");
    document.body.append(cell);
    handler.call(cell);

    const detail = table.clickedRow.child.content as HTMLElement;
    expect(detail.textContent).toContain("The server malfunctioned");
    expect(detail.textContent).toContain("goroutine 1 [running]:");
    // The error text is set as text, so markup in it never becomes markup.
    expect(detail.textContent).toContain("<script>alert(1)</script>");
    expect(detail.querySelector("script")).toBeNull();
    expect(detail.innerHTML).toContain("&lt;script&gt;");
    expect(table.clickedRow.child.isShown()).toBe(true);

    // A second click collapses it again.
    handler.call(cell);
    expect(table.clickedRow.child.isShown()).toBe(false);
});

test("a row with no recorded detail still expands to something readable", async (): Promise<void> => {
    await initErrorLogsPage();

    const table = MockDataTable.lastInstance!;
    table.setRowData({ id: 2, http_status: 500 });

    const handler = table.delegated["click td.dt-control"]!;
    handler.call(document.createElement("td"));

    const detail = table.clickedRow.child.content as HTMLElement;
    expect(detail.textContent).toContain("No further detail");
});

test("the status column renders the HTTP status", async (): Promise<void> => {
    await initErrorLogsPage();

    const statusColumn = MockDataTable.lastInstance!.column("log_http_status")!;
    expect(statusColumn.render!(500, "display", {}) as string).toContain("500");
    expect(statusColumn.render!(503, "sort", {})).toBe(503);
});

test("an unauthenticated visitor is sent to the login page, and no table is built", async (): Promise<void> => {
    authenticated = false;
    const replace = vi.spyOn(window.location, "replace").mockImplementation((): void => {});
    mockFetch((url) => {
        if (url === url_auth) {
            return jsonResponse({ authenticated: false });
        }
        if (url === url_events || url === url_logout) {
            return jsonResponse([]);
        }
        return undefined;
    });
    await import("../typescript/admin_error_logs.ts");

    await vi.waitFor((): void => {
        expect(replace).toHaveBeenCalledWith(expect.stringContaining(url_login));
    });
    expect(MockDataTable.lastInstance).toBeNull();
});

test("a failure to load the logs is shown, and the table gets no rows", async (): Promise<void> => {
    logsFail = true;
    await initErrorLogsPage([{ id: 1 }]);

    await vi.waitFor((): void => {
        expect(document.getElementById("error_text")!.textContent).toContain("Failed to load table: logs unavailable (HTTP 500)");
    });
    expect(MockDataTable.lastInstance!.lastData).toEqual([]);
});

test("a max-time filter bounds the fetch at that time", async (): Promise<void> => {
    const mock = await initErrorLogsPage();

    const maxTime = document.getElementById("filter_max_time") as HTMLInputElement;
    maxTime.value = "2025-08-30 12:00:00";
    change(maxTime);

    await vi.waitFor((): void => {
        const params = new URL(mock.mock.calls.at(-1)![0] as string, "https://localhost").searchParams;
        expect(params.get("maxTimeUnixMs")).toBe(new Date("2025-08-30 12:00:00").getTime().toString());
    });
});

test("the time column shows a local timestamp with the full date on hover, and sorts by instant", async (): Promise<void> => {
    await initErrorLogsPage();

    const timeColumn = MockDataTable.lastInstance!.column("log_time")!;
    const instant = "2025-08-30T19:04:05Z";
    const local = new Intl.DateTimeFormat("sv-SE", {
        year: "numeric", month: "numeric", day: "numeric",
        hour: "numeric", hour12: false, minute: "numeric", second: "numeric",
    }).format(Date.parse(instant));

    const cell = document.createElement("div");
    cell.innerHTML = timeColumn.render!(instant, "display", {}) as string;
    expect(cell.textContent).toBe(local);
    expect(cell.querySelector("span")!.title).toMatch(/2025-08-3\d/);
    expect(timeColumn.render!(instant, "filter", {})).toBe(local);
    expect(timeColumn.render!(instant, "sort", {})).toBe(Date.parse(instant));
    expect(timeColumn.render!(instant, "type", {})).toBe(Date.parse(instant));
    expect(timeColumn.render!(instant, "export", {})).toBeUndefined();
});

test("a missing status renders blank and sorts as zero", async (): Promise<void> => {
    await initErrorLogsPage();

    const statusColumn = MockDataTable.lastInstance!.column("log_http_status")!;
    expect(statusColumn.render!(null, "display", {})).toBe("");
    expect(statusColumn.render!(null, "filter", {})).toBe("");
    expect(statusColumn.render!(null, "type", {})).toBe(0);
    expect(statusColumn.render!(500, "export", {})).toBeUndefined();
});
