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
        setPreferredState: (el: HTMLSelectElement) => Promise<void>;
        setPreferredVisitsStatus: (el: HTMLSelectElement) => Promise<void>;
        setPreferredRowsPerPage: (el: HTMLSelectElement) => Promise<void>;
        setPreferredTheme: (el: HTMLSelectElement) => Promise<void>;
        setKeyboardShortcuts: (el: HTMLInputElement) => Promise<void>;
    }
}

//
// Initialize UI
//

const el = {
    preferredState: ims.typedElement("preferred_state", HTMLSelectElement),
    preferredVisitsStatus: ims.typedElement("preferred_visits_status", HTMLSelectElement),
    preferredRowsPerPage: ims.typedElement("preferred_rows_per_page", HTMLSelectElement),
    preferredTheme: ims.typedElement("preferred_theme", HTMLSelectElement),
    keyboardShortcuts: ims.typedElement("keyboard_shortcuts", HTMLInputElement),
};

initSettingsPage();

async function initSettingsPage(): Promise<void> {
    const initResult = await ims.commonPageInit();
    if (!initResult.authInfo.authenticated) {
        await ims.redirectToLogin();
        return;
    }
    const preferredState = ims.getIncidentsPreferredState();
    if (preferredState) {
        el.preferredState.value = preferredState;
    }
    const preferredVisitsStatus = ims.getVisitsPreferredStatus();
    if (preferredVisitsStatus) {
        el.preferredVisitsStatus.value = preferredVisitsStatus;
    }
    const preferredRowsPerPage = ims.getPreferredTableRowsPerPage();
    if (preferredRowsPerPage) {
        el.preferredRowsPerPage.value = preferredRowsPerPage;
    }
    el.preferredTheme.value = window.imsThemeSetting();
    el.keyboardShortcuts.checked = ims.keyboardShortcutsEnabled();

    // Keep the select in step with the navbar's theme dropdown.
    document.addEventListener("ims:themechange", (e: CustomEvent<ThemeSetting>): void => {
        el.preferredTheme.value = e.detail;
    });

    window.setPreferredState = setPreferredState;
    window.setPreferredVisitsStatus = setPreferredVisitsStatus;
    window.setPreferredRowsPerPage = setPreferredRowsPerPage;
    window.setPreferredTheme = setPreferredTheme;
    window.setKeyboardShortcuts = setKeyboardShortcuts;
}

async function setPreferredState(el: HTMLSelectElement): Promise<void> {
    if (ims.isValidIncidentsTableState(el.value)) {
        ims.setIncidentsPreferredState(el.value);
    } else {
        ims.setIncidentsPreferredState(null);
    }
    ims.controlHasSuccess(el);
}

async function setPreferredVisitsStatus(el: HTMLSelectElement): Promise<void> {
    if (ims.isValidVisitsTableStatus(el.value)) {
        ims.setVisitsPreferredStatus(el.value);
    } else {
        ims.setVisitsPreferredStatus(null);
    }
    ims.controlHasSuccess(el);
}

async function setPreferredRowsPerPage(el: HTMLSelectElement): Promise<void> {
    if (ims.isValidTableRowsPerPage(el.value)) {
        ims.setPreferredTableRowsPerPage(el.value);
    } else {
        ims.setPreferredTableRowsPerPage(null);
    }
    ims.controlHasSuccess(el);
}

async function setPreferredTheme(el: HTMLSelectElement): Promise<void> {
    const theme = el.value;
    if (theme === "auto" || theme === "light" || theme === "dark") {
        window.imsSetThemeSetting(theme);
        ims.controlHasSuccess(el);
    }
}

async function setKeyboardShortcuts(el: HTMLInputElement): Promise<void> {
    ims.setKeyboardShortcutsEnabled(el.checked);
    ims.announce(el.checked ? "Single-key shortcuts on" : "Single-key shortcuts off");
}
