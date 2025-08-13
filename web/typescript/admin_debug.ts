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
        fetchBuildInfo: (el: HTMLElement) => Promise<void>;
        fetchRuntimeMetrics: (el: HTMLElement) => Promise<void>;
        performGC: (el: HTMLElement) => Promise<void>;
    }
}
//
// Initialize UI
//

initAdminDebugPage();

async function initAdminDebugPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }

    window.fetchBuildInfo = fetchBuildInfo;
    window.fetchRuntimeMetrics = fetchRuntimeMetrics;
    window.performGC = performGC;
}

async function fetchBuildInfo(): Promise<void> {
    const {text, err} = await ims.fetchNoThrow(url_debugBuildInfo, {});
    if (err != null || text == null) {
        throw err;
    }
    const buildInfoText = text;
    const targetPre = document.getElementById("build-info") as HTMLPreElement
    targetPre.textContent = buildInfoText;

    const ref = substringBetween(buildInfoText, "build\tvcs.revision=", "\n")
    const dirty = buildInfoText.indexOf("vcs.modified=true") >= 0;
    const targetP = document.getElementById("build-info-p") as HTMLParagraphElement;
    const link = document.createElement("a");
    link.text = `The server was built at revision ${ref.substring(0,12)} ${dirty ? " (dirty)" : ""}`;
    link.href = `https://github.com/burningmantech/ranger-ims-go/tree/${ref}`;
    targetP.replaceChildren(link);
    const targetDiv = document.getElementById("build-info-div") as HTMLParagraphElement;
    targetDiv.style.display = "";
}

function substringBetween(s: string, start: string, end: string): string {
    const startInd = s.indexOf(start);
    if (startInd < 0) {
        return "";
    }
    const substrBeginInd = startInd + start.length;
    let endInd = s.indexOf(end, substrBeginInd);
    if (endInd < 0) {
        endInd = s.length;
    }
    return s.substring(substrBeginInd, endInd);
}

async function fetchRuntimeMetrics(): Promise<void> {
    const {text, err} = await ims.fetchNoThrow(url_debugRuntimeMetrics, {});
    if (err != null || text == null) {
        throw err;
    }
    const targetPre = document.getElementById("runtime-metrics") as HTMLPreElement
    targetPre.textContent = text;
    const targetDiv = document.getElementById("runtime-metrics-div") as HTMLParagraphElement;
    targetDiv.style.display = "";
}

async function performGC(): Promise<void> {
    const {text, err} = await ims.fetchNoThrow(url_debugGC, {body: JSON.stringify({})});
    if (err != null || text == null) {
        throw err;
    }
    const targetPre = document.getElementById("gc") as HTMLPreElement
    targetPre.textContent = text;
    const targetDiv = document.getElementById("gc-div") as HTMLParagraphElement;
    targetDiv.style.display = "";
}
