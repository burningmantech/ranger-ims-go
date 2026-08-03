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
        updateTable: (el: HTMLElement) => Promise<void>;
    }
}

//
// Filters
//
let filterMinTime: Date|null = null;
let filterMaxTime: Date|null = null;
let filterUserName: string|null = null;
let filterPath: string|null = null;


//
// Initialize UI
//

const el = {
    filterMinTime: ims.typedElement("filter_min_time", HTMLInputElement),
    filterMaxTime: ims.typedElement("filter_max_time", HTMLInputElement),
    filterUserName: ims.typedElement("filter_user_name", HTMLInputElement),
    filterPath: ims.typedElement("filter_path", HTMLInputElement),
};

initAdminErrorLogsPage();

declare let DataTable: any;

let errorLogsTable: ims.DataTablesTable|null = null;

async function initAdminErrorLogsPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }

    window.updateTable = updateTable;

    const yesterday: Date = new Date();
    yesterday.setDate(new Date().getDate() - 1);
    el.filterMinTime.value = nerdDateTime.format(yesterday);
    updateFilters();

    errorLogsTable = new DataTable("#error_logs_table", {
        "deferRender": true,
        "paging": true,
        "lengthChange": false,
        "searching": true,
        "processing": true,
        "scrollX": false,
        "scrollY": false,
        "layout": {
            "topStart": null,
            "topEnd": null,
            "bottomStart": "info",
            "bottomEnd": "paging",
        },
        "pageLength": 100,
        "ajax": function (_data: unknown, callback: (resp: {data: ErrorLog[]})=>void, _settings: unknown): void {
            async function doAjax(): Promise<void> {

                const params = new URLSearchParams({});
                if (filterMinTime) {
                    params.set("minTimeUnixMs", filterMinTime.getTime().toString());
                }
                if (filterMaxTime) {
                    params.set("maxTimeUnixMs", filterMaxTime.getTime().toString());
                }
                if (filterUserName) {
                    params.set("userName", filterUserName);
                }
                if (filterPath) {
                    params.set("path", filterPath);
                }

                const {json, err} = await ims.fetchNoThrow<ErrorLog[]>(
                    `${url_errorlogs}?${params.toString()}`, null,
                );
                if (err != null || json == null) {
                    ims.setErrorMessage(`Failed to load table: ${err}`);
                    return;
                }
                callback({data: json});
            }

            doAjax();
        },
        "columns": [
            {   // 0
                "name": "log_control",
                "className": "dt-control",
                "data": null,
                "defaultContent": "",
                "orderable": false,
            },
            {   // 1
                "name": "log_id",
                "className": "text-right",
                "data": "id",
                "defaultContent": null,
                "render": DataTable.render.number(),
                "cellType": "th",
            },
            {   // 2
                "name": "log_time",
                "className": "text-center",
                "data": "created_at",
                "defaultContent": null,
                "render": renderDate,
            },
            {   // 3
                "name": "log_http_status",
                "className": "text-center",
                "data": "http_status",
                "defaultContent": null,
                "render": renderStatus,
            },
            {   // 4
                "name": "log_user_name",
                "className": "text-center",
                "data": "user_name",
                "defaultContent": null,
                "render": DataTable.render.text(),
            },
            {   // 5
                "name": "log_method",
                "className": "text-center",
                "data": "method",
                "defaultContent": null,
                "render": DataTable.render.text(),
            },
            {   // 6
                "name": "log_path",
                "className": "text-center",
                "data": "path",
                "defaultContent": null,
                "render": DataTable.render.text(),
            },
            {   // 7
                "name": "log_client_address",
                "className": "text-center",
                "data": "client_address",
                "defaultContent": null,
                "render": DataTable.render.text(),
            },
            {   // 8
                "name": "log_duration",
                "className": "text-center",
                "data": "duration",
                "defaultContent": null,
                "render": DataTable.render.text(),
            },
        ],
        "order": [
            // time descending
            [2, "dsc"],
        ],
    });

    errorLogsTable!.on("init", function (): void {
        ims.enableKeyboardSorting("error_logs_table");
    });

    // The interesting parts of an error (the message, the internal error chain,
    // the stack) are far too long for a table cell, so they live in a child row
    // that the leading control column toggles open.
    errorLogsTable!.on("click", "td.dt-control", function (this: HTMLElement): void {
        const row = errorLogsTable!.row(this.closest("tr"));
        if (row.child.isShown()) {
            row.child.hide();
        } else {
            row.child(renderDetail(row.data())).show();
        }
    });

    errorLogsTable!.draw();
}

// Build the expanded detail for one error row. This is assembled as DOM nodes
// with textContent, never as an HTML string, because the error text comes from
// whatever the server happened to fail on.
function renderDetail(errorLog: ErrorLog): HTMLElement {
    const container = document.createElement("div");
    container.className = "p-2";

    function section(label: string, value: string|null|undefined): void {
        if (!value) {
            return;
        }
        const heading = document.createElement("div");
        heading.className = "fw-bold";
        heading.textContent = label;
        container.append(heading);

        const body = document.createElement("pre");
        body.className = "text-wrap text-break small";
        body.textContent = value;
        container.append(body);
    }

    section("Response message", errorLog.response_message);
    section("Internal error", errorLog.internal_error);
    section("Stack trace", errorLog.stack_trace);
    section("Referrer", errorLog.referrer);

    if (container.childElementCount === 0) {
        container.textContent = "No further detail was recorded for this error.";
    }
    return container;
}

// Every row here is a failure, so colouring the status red would say nothing
// that the page doesn't already say (and it fails contrast on the striped rows).
function renderStatus(status: number|null, type: string, _data: any): string|number|undefined {
    switch (type) {
        case "display":
        case "filter":
            return status == null ? "" : status.toString();
        case "type":
        case "sort":
            return status??0;
    }
    return undefined;
}

async function updateTable(_el: HTMLElement): Promise<void> {
    updateFilters();
    errorLogsTable!.ajax.reload();
    errorLogsTable!.draw();
}

function updateFilters(): void {
    if (el.filterMinTime.value) {
        filterMinTime = new Date(el.filterMinTime.value);
    } else {
        filterMinTime = null;
    }
    if (el.filterMaxTime.value) {
        filterMaxTime = new Date(el.filterMaxTime.value);
    } else {
        filterMaxTime = null;
    }
    filterUserName = el.filterUserName.value ? el.filterUserName.value : null;
    filterPath = el.filterPath.value ? el.filterPath.value : null;
}

const nerdDateTime: Intl.DateTimeFormat = new Intl.DateTimeFormat("sv-SE", {
    year: "numeric",
    month: "numeric",
    day: "numeric",
    hour: "numeric",
    hour12: false,
    minute: "numeric",
    second: "numeric",
    // timeZone not specified; will use user's timezone
});

function renderDate(date: string, type: string, _incident: any): string|number|undefined {
    const d = Date.parse(date);
    const fullDate = ims.longFormatDate(d);
    switch (type) {
        case "display":
            const sp = document.createElement("span");
            sp.title = fullDate;
            sp.textContent = nerdDateTime.format(d);
            return sp.outerHTML;
        case "filter":
            return nerdDateTime.format(d);
        case "type":
        case "sort":
            return d;
    }
    return undefined;
}

export interface ErrorLog {
    id?: number|null;
    created_at?: string|null;
    http_status?: number|null;
    response_message?: string|null;
    internal_error?: string|null;
    stack_trace?: string|null;
    method?: string|null;
    path?: string|null;
    referrer?: string|null;
    user_id?: number|null;
    user_name?: string|null;
    position_id?: number|null;
    position_name?: string|null;
    client_address?: string|null;
    duration?: string|null;
}
