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

// Tests for login.ts against the real templ-rendered login page (login.templ).

import { beforeEach, expect, test, vi } from "vitest";
import { type FetchHandler, jsonResponse, loadFixture, mockFetch, problemResponse } from "./helpers.ts";

beforeEach((): void => {
    // login.ts looks up its elements and runs its init at import time, so each
    // test must load the fixture first and then dynamically import a fresh
    // copy of the module.
    vi.resetModules();
    loadFixture("login.html");
});

// Import login.ts and wait for its init to attach the page's window functions.
async function initLoginPage(handler: FetchHandler) {
    const mock = mockFetch((url, init) => {
        // commonPageInit's auth check, a GET to url_auth.
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: false });
        }
        return handler(url, init);
    });
    await import("../typescript/login.ts");
    await vi.waitFor((): void => {
        expect(window.login).toBeTypeOf("function");
    });
    return mock;
}

test("a failed login reveals the authentication-failed banner", async (): Promise<void> => {
    await initLoginPage((url, init) => {
        if (url === url_auth && init?.body != null) {
            return problemResponse("The password was incorrect", 401);
        }
        return undefined;
    });

    const username = document.getElementById("username_input") as HTMLInputElement;
    const password = document.getElementById("password_input") as HTMLInputElement;
    username.value = "ranger@example.com";
    password.value = "wrong-password";

    const banner = document.querySelector(".if-authentication-failed")!;
    expect(banner.classList.contains("hidden")).toBe(true);

    // Submitting the form must trigger login(); login.ts preventDefaults the
    // native submission.
    document.getElementById("login_form")!.dispatchEvent(
        new Event("submit", { bubbles: true, cancelable: true }),
    );

    await vi.waitFor((): void => {
        expect(banner.classList.contains("hidden")).toBe(false);
    });
});

test("a successful login stores the tokens and redirects to the app", async (): Promise<void> => {
    const mock = await initLoginPage((url, init) => {
        if (url === url_auth && init?.body != null) {
            return jsonResponse({ token: "shinyNewToken", expires_unix_ms: 1700000000000 });
        }
        return undefined;
    });
    const replace = vi.spyOn(window.location, "replace").mockImplementation((): void => {});

    const username = document.getElementById("username_input") as HTMLInputElement;
    const password = document.getElementById("password_input") as HTMLInputElement;
    username.value = "ranger@example.com";
    password.value = "correct-password";

    window.login();

    await vi.waitFor((): void => {
        expect(replace).toHaveBeenCalledWith(url_app);
    });
    expect(localStorage.getItem("access_token")).toBe("shinyNewToken");
    expect(localStorage.getItem("access_token_refresh_after")).toBe("1700000000000");

    // The login POST carries the form's credentials.
    const loginCall = mock.mock.calls.find(([, init]) => init?.body != null)!;
    expect(JSON.parse(loginCall[1]!.body as string)).toEqual({
        identification: "ranger@example.com",
        password: "correct-password",
    });
});

test("the Show/Hide button toggles password visibility via its inline onclick handler", async (): Promise<void> => {
    await initLoginPage(() => undefined);

    // login.templ wires the button with onclick="toggleShowPassword()", so the
    // function must exist on window under exactly that name.
    const button = document.getElementById("password_show_hide") as HTMLButtonElement;
    expect(button.getAttribute("onclick")).toBe("toggleShowPassword()");

    const password = document.getElementById("password_input") as HTMLInputElement;
    expect(password.type).toBe("password");
    expect(button.textContent).toBe("Show");

    window.toggleShowPassword();
    expect(password.type).toBe("text");
    expect(button.textContent).toBe("Hide");

    window.toggleShowPassword();
    expect(password.type).toBe("password");
    expect(button.textContent).toBe("Show");
});
