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

//
// Initialize UI
//

initRootPage();

async function initRootPage(): Promise<void> {
    const params = new URLSearchParams(window.location.search);
    if (params.get("logout") != null) {
        // this clears the access token cookie
        await fetch(url_logout);
        ims.clearLocalStorage();
        ims.clearSessionStorage();
        window.history.replaceState(null, "", url_app);
    }
    const result = await ims.commonPageInit();

    if (!result.authInfo.authenticated) {
        document.getElementById("login-button")?.focus();
        return;
    }

    const activeEvent = (await result.eventDatas)?.find(e => e.is_active);
    if (activeEvent != null) {
        const link = document.getElementById("active-event-link") as HTMLAnchorElement;
        link.href = url_viewIncidents.replace("<event_id>", activeEvent.name);
        document.getElementById("active-event-name")!.textContent = activeEvent.name;
        document.getElementById("active-event-jump")!.classList.remove("hidden");
        document.getElementById("no-active-event")!.classList.add("hidden");
        link.focus();
    }
}
