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

// Tests for admin_root.ts against the real templ-rendered admin landing page
// (adminroot.templ). The module's only job is to gate the page behind
// authentication, redirecting anonymous visitors to the login page.

import { beforeEach, expect, test, vi } from "vitest";
import { jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

beforeEach((): void => {
    vi.resetModules();
    loadFixture("admin_root.html");
});

async function initAdminRootPage(authenticated: boolean) {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: authenticated, user: "Tester", admin: true });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse([]);
        }
        if (url === url_logout) {
            return new Response(null, { status: 200 });
        }
        return undefined;
    });
    await import("../typescript/admin_root.ts");
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url]) => url === url_auth)).toBe(true);
    });
    return mock;
}

test("an authenticated admin is not redirected away from the page", async (): Promise<void> => {
    const replace = vi.spyOn(window.location, "replace").mockImplementation((): void => {});
    await initAdminRootPage(true);
    // Give any stray redirect a chance to fire before asserting it didn't.
    await Promise.resolve();
    expect(replace).not.toHaveBeenCalled();
});

test("an unauthenticated visitor is redirected to the login page", async (): Promise<void> => {
    const replace = vi.spyOn(window.location, "replace").mockImplementation((): void => {});
    await initAdminRootPage(false);
    await vi.waitFor((): void => {
        expect(replace).toHaveBeenCalled();
    });
    expect((replace.mock.calls[0]![0] as string).startsWith(url_login)).toBe(true);
});
