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

const el = {
    loginButton: ims.typedElement("login-button", HTMLAnchorElement),
    activeEventJump: ims.typedElement("active-event-jump", HTMLSpanElement),
    activeEventLink: ims.typedElement("active-event-link", HTMLAnchorElement),
    activeEventName: ims.typedElement("active-event-name", HTMLSpanElement),
    noActiveEvent: ims.typedElement("no-active-event", HTMLSpanElement),
};

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
        el.loginButton.focus();
        return;
    }

    const activeEvent = (await result.eventDatas)?.find(e => e.is_active);
    if (activeEvent != null) {
        el.activeEventLink.href = url_viewIncidents.replace("<event_id>", activeEvent.name);
        el.activeEventName.textContent = activeEvent.name;
        el.activeEventJump.classList.remove("hidden");
        el.noActiveEvent.classList.add("hidden");
        el.activeEventLink.focus();
    }
}
