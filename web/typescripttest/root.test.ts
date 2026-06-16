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

// Tests for root.ts against the real templ-rendered landing page (root.templ).
// The page optionally handles a ?logout flow, then focuses the most useful
// control depending on whether the visitor is authenticated.

import { beforeEach, expect, test, vi } from "vitest";
import { type FetchHandler, jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

beforeEach((): void => {
    vi.resetModules();
    loadFixture("root.html");
    // Each test starts at the app's landing URL with no query string.
    window.history.replaceState(null, "", url_app);
});

// Import root.ts behind a fake server and wait for its init to settle (signaled
// by the auth check having been made).
async function initRootPage(authenticated: boolean, handler: FetchHandler = () => undefined) {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: authenticated, user: "Tester" });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse([]);
        }
        return handler(url, init);
    });
    await import("../typescript/root.ts");
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url]) => url === url_auth)).toBe(true);
    });
    return mock;
}

test("an authenticated visitor gets focus on the current-year link", async (): Promise<void> => {
    await initRootPage(true);
    await vi.waitFor((): void => {
        expect(document.activeElement?.id).toBe("current-year-link");
    });
});

test("an unauthenticated visitor gets focus on the login button", async (): Promise<void> => {
    await initRootPage(false);
    await vi.waitFor((): void => {
        expect(document.activeElement?.id).toBe("login-button");
    });
});

test("the ?logout flow clears stored tokens, hits the logout endpoint, and cleans the URL", async (): Promise<void> => {
    window.history.replaceState(null, "", `${url_app}?logout=true`);
    localStorage.setItem("access_token", "stale-token");
    sessionStorage.setItem("something", "cached");

    const mock = await initRootPage(true, (url) => {
        if (url === url_logout) {
            return new Response(null, { status: 200 });
        }
        return undefined;
    });

    expect(mock.mock.calls.some(([url]) => url === url_logout)).toBe(true);
    expect(localStorage.getItem("access_token")).toBeNull();
    expect(sessionStorage.getItem("something")).toBeNull();
    // The logout query string is stripped so a refresh won't log out again.
    expect(window.location.search).toBe("");
    expect(window.location.pathname).toBe(url_app);
});
