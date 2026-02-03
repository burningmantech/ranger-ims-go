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
        fetchActionLogs: (el: HTMLElement) => Promise<void>;
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

initAdminActionLogsPage();

declare let DataTable: any;

let actionLogsTable: ims.DataTablesTable|null = null;

async function initAdminActionLogsPage(): Promise<void> {
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

    // DataTable.ext.errMode = "none";
    actionLogsTable = new DataTable("#action_logs_table", {
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
        "ajax": function (_data: unknown, callback: (resp: {data: ActionLog[]})=>void, _settings: unknown): void {
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

                const {json, err} = await ims.fetchNoThrow<ActionLog[]>(
                    `${url_actionlogs}?${params.toString()}`, null,
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
                "name": "log_id",
                "className": "text-right",
                "data": "id",
                "defaultContent": null,
                "render": DataTable.render.number(),
                "cellType": "th",
            },
            {   // 1
                "name": "log_time",
                "className": "text-center",
                "data": "created_at",
                "defaultContent": null,
                "render": renderDate,
            },
            {   // 2
                "name": "log_user_name",
                "className": "text-center",
                "data": "user_name",
                "defaultContent": null,
                "render": DataTable.render.text(),
            },
            {   // 3
                "name": "log_page",
                "className": "text-center",
                "data": "referrer",
                "defaultContent": null,
                "render": renderPage,
            },
            {   // 4
                "name": "log_method",
                "className": "text-center",
                "data": "method",
                "defaultContent": null,
                "render": DataTable.render.text(),
            },
            {   // 5
                "name": "log_path",
                "className": "text-center",
                "data": "path",
                "defaultContent": null,
                "render": DataTable.render.text(),
            },
            {   // 6
                "name": "log_position_name",
                "className": "text-center",
                "data": "position_name",
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
            [1, "dsc"],
        ],
    });

    actionLogsTable!.draw();
}

function renderPage(pagePath: string|null, type: string, _data: any): string|undefined {
    pagePath = pagePath??"";
    switch (type) {
        case "display":
            if (pagePath == "") {
                return "";
            }
            const link = document.createElement("a");
            link.href = pagePath;
            link.target = "_blank";
            link.text = pagePath;
            return link.outerHTML;
        case "filter":
        case "type":
        case "sort":
            return pagePath;
    }
    return undefined;
}

async function updateTable(_el: HTMLElement): Promise<void> {
    updateFilters();
    actionLogsTable!.ajax.reload();
    actionLogsTable!.draw();
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
    // weekday: "short",
    year: "numeric",
    month: "numeric",
    day: "numeric",
    hour: "numeric",
    hour12: false,
    minute: "numeric",
    second: "numeric",
    // timeZoneName: "short",
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

export interface ActionLog {
    id?: number|null;
    created?: string|null;
    action_type?: string|null;
    method?: string|null;
    path?: string|null;
    referrer?: string|null;
    user_id?: number|null;
    user_name?: string|null;
    position_id?: number|null,
    position_name?: string|null,
    client_address?: string|null,
    http_status?: number|null,
    duration?: string|null;
}
