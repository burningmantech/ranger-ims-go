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
    }
}
//
// Initialize UI
//

initAdminActionLogsPage();

declare let DataTable: any;

let actionLogsTable: ims.DataTablesTable|null = null;

async function initAdminActionLogsPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }

    window.fetchActionLogs = fetchActionLogs;

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
                const {json, err} = await ims.fetchJsonNoThrow<ActionLog[]>(
                    url_actionlogs, null,
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
                "render": DataTable.render.text(),
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
        "initComplete": function (): void {
            this.api()
                .columns("log_user_name:name")
                .every(function (_colInd: number) {
                    // This is a DataTables "X" type
                    // @ts-ignore
                    let column: any = this;
                    console.log("column is " + column.constructor.name);

                    // Create select element
                    let select: HTMLSelectElement = document.createElement("select");
                    const showAll: HTMLOptionElement = document.createElement("option")
                    showAll.text = "User";
                    showAll.selected = true;
                    showAll.dataset["showAll"] = "true";
                    select.add(showAll);
                    column.footer().replaceChildren(select);

                    // Apply listener for user change in value
                    select.addEventListener("change", function(): void {
                        column
                            .search(
                                select.selectedOptions[0]!.dataset["showAll"]
                                    ? ""
                                    : select.selectedOptions[0]!.value,
                                {exact: true})
                            .draw();
                    });

                    // Add list of options
                    column
                        .data()
                        .unique()
                        .sort()
                        .each(function (username: string, _ind: number): void {
                            select.add(new Option(username));
                        });
                });
        }
    });

    actionLogsTable!.draw();
}

async function fetchActionLogs(): Promise<void> {
    const {json, err} = await ims.fetchJsonNoThrow<ActionLog>(url_actionlogs, {});
    if (err != null) {
        throw err;
    }
    const actionLogsText = JSON.stringify(json, null, 2);
    const targetPre = document.getElementById("action-logs") as HTMLPreElement
    targetPre.textContent = actionLogsText;

    const targetDiv = document.getElementById("show-action-logs-div") as HTMLParagraphElement;
    targetDiv.style.display = "";
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
    const fullDate = ims.fullDateTime.format(d);
    switch (type) {
        case "display":
            return `<span title="${fullDate}">${nerdDateTime.format(d)}</span>`;
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
