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

// Tests for admin_action_logs.ts against the real templ-rendered action logs
// page (adminactionlogs.templ). The page drives a DataTables grid whose ajax
// source builds query params from the filter inputs. DataTables itself is a
// classic-script dependency, so a small stand-in captures the options the page
// passes and runs the ajax source on draw/reload.

import { beforeEach, expect, test, vi } from "vitest";
import { jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

interface AjaxCallback { (resp: { data: unknown[] }): void; }
interface DataTableOptions {
    ajax: (data: unknown, callback: AjaxCallback, settings: unknown) => void;
    columns: { name: string; render?: (value: any, type: string, row: any) => unknown }[];
}

// A minimal DataTables stand-in: it records the options the page constructs and
// runs the ajax source whenever the table is drawn or reloaded.
class MockDataTable {
    static lastInstance: MockDataTable | null = null;
    options: DataTableOptions;
    lastData: unknown[] = [];

    static render = {
        number: () => ({ display: (s: unknown): unknown => s }),
        text: () => ({ display: (s: unknown): unknown => s }),
    };

    ajax = { reload: (): void => this.runAjax() };

    constructor(_selector: string, options: DataTableOptions) {
        this.options = options;
        MockDataTable.lastInstance = this;
    }

    draw(): void {
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
    loadFixture("admin_action_logs.html");
    MockDataTable.lastInstance = null;
    vi.stubGlobal("DataTable", MockDataTable);
});

async function initActionLogsPage(rows: unknown[] = []) {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: true, user: "Tester", admin: true });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse([]);
        }
        if (url.startsWith(url_actionlogs)) {
            return jsonResponse(rows);
        }
        return undefined;
    });
    await import("../typescript/admin_action_logs.ts");
    await vi.waitFor((): void => {
        expect(window.updateTable).toBeTypeOf("function");
        expect(MockDataTable.lastInstance).not.toBeNull();
    });
    return mock;
}

test("the table is fetched from the action logs endpoint with the default min-time filter", async (): Promise<void> => {
    const mock = await initActionLogsPage([{ id: 1, user_name: "Tester" }]);

    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url]) => (url as string).startsWith(url_actionlogs))).toBe(true);
    });
    const logCall = mock.mock.calls.find(([url]) => (url as string).startsWith(url_actionlogs))!;
    const params = new URL(logCall[0] as string, "https://localhost").searchParams;
    // init defaults the min-time input to yesterday, so the fetch is bounded.
    expect(params.get("minTimeUnixMs")).not.toBeNull();
});

test("updateTable folds the filter inputs into the query params and reloads", async (): Promise<void> => {
    const mock = await initActionLogsPage();

    (document.getElementById("filter_user_name") as HTMLInputElement).value = "Hubcap";
    (document.getElementById("filter_path") as HTMLInputElement).value = "/ims/api/events";
    (document.getElementById("filter_min_time") as HTMLInputElement).value = "";

    await window.updateTable(document.body);

    await vi.waitFor((): void => {
        const last = mock.mock.calls.at(-1)!;
        expect((last[0] as string).startsWith(url_actionlogs)).toBe(true);
    });
    const last = mock.mock.calls.at(-1)!;
    const params = new URL(last[0] as string, "https://localhost").searchParams;
    expect(params.get("userName")).toBe("Hubcap");
    expect(params.get("path")).toBe("/ims/api/events");
    // Clearing the min-time input drops that bound.
    expect(params.get("minTimeUnixMs")).toBeNull();
});

test("the page column renders a referrer path as a new-tab link", async (): Promise<void> => {
    await initActionLogsPage();

    const pageColumn = MockDataTable.lastInstance!.column("log_page")!;
    const html = pageColumn.render!("/ims/app/admin", "display", {}) as string;
    expect(html).toContain('href="/ims/app/admin"');
    expect(html).toContain('target="_blank"');

    // An empty path renders nothing rather than an empty link.
    expect(pageColumn.render!("", "display", {})).toBe("");
});
