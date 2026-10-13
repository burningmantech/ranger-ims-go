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

// Tests for help.ts against the real templ-rendered help page (help.templ).
// The module's only job is to gate the page behind authentication,
// redirecting anonymous visitors to the login page.

import { beforeEach, expect, test, vi } from "vitest";
import { jsonResponse, loadFixture, mockFetch, waitForPageInit } from "./helpers.ts";

beforeEach((): void => {
    vi.resetModules();
    loadFixture("help.html");
});

async function initHelpPage(authenticated: boolean) {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: authenticated, user: "Tester" });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse([]);
        }
        if (url === url_logout) {
            return new Response(null, { status: 200 });
        }
        return undefined;
    });
    await import("../typescript/help.ts");
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url]) => url === url_auth)).toBe(true);
    });
    return mock;
}

test("an authenticated user stays on the help page, named in the navbar", async (): Promise<void> => {
    const replace = vi.spyOn(window.location, "replace").mockImplementation((): void => {});
    await initHelpPage(true);
    await waitForPageInit();
    expect(document.querySelector(".logged-in-user")!.textContent).toBe("Tester");
    expect(replace).not.toHaveBeenCalled();
});

test("an unauthenticated visitor is redirected to the login page, and back here afterward", async (): Promise<void> => {
    const replace = vi.spyOn(window.location, "replace").mockImplementation((): void => {});
    const mock = await initHelpPage(false);
    await vi.waitFor((): void => {
        expect(replace).toHaveBeenCalled();
    });
    expect(replace.mock.calls[0]![0]).toBe(`${url_login}?o=${encodeURIComponent(window.location.pathname)}`);
    expect(mock.mock.calls.some(([url]) => url === url_logout)).toBe(true);
});
