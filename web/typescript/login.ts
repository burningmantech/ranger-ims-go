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
        login: ()=>void;
    }
}

//
// Initialize UI
//

initLoginPage();

async function initLoginPage(): Promise<void> {
    await ims.commonPageInit();
    document.getElementById("login_form")!.addEventListener("submit", (e: SubmitEvent): void => {
        e.preventDefault();
        login();
    });
    document.getElementById("username_input")?.focus();
}

async function login(): Promise<void> {
    const username = (document.getElementById("username_input") as HTMLInputElement).value;
    const password = (document.getElementById("password_input") as HTMLInputElement).value;
    const {json, err} = await ims.fetchJsonNoThrow<AuthResponse>(url_auth, {
        body: JSON.stringify({
            "identification": username,
            "password": password,
        }),
    });
    if (err != null || json == null) {
        ims.unhide(".if-authentication-failed");
        return;
    }
    ims.setAccessToken(json.token);
    ims.setRefreshTokenBy(json.expires_unix_ms);
    const redirect = new URLSearchParams(window.location.search).get("o");

    // There are dangers with using redirects to destinations from unsafe strings.
    // We can limit this by requiring the destination be within IMS and not contain
    // exotic characters.
    //
    // https://github.com/burningmantech/ranger-ims-go/security/code-scanning/4
    // https://github.com/burningmantech/ranger-ims-go/security/code-scanning/6
    const internalDest = (str: string): boolean => str.startsWith("/ims/");
    const looksSafe = (str: string): boolean => /^[\w\-/?=]+$/.test(str);
    if (redirect != null && internalDest(redirect) && looksSafe(redirect)) {
        window.location.replace(redirect);
    } else {
        window.location.replace(url_app);
    }
}

type AuthResponse = {
    token: string;
    expires_unix_ms: number;
}
