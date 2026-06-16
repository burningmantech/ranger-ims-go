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

// Tests for admin_debug.ts against the real templ-rendered debug page
// (admindebug.templ). The page lazily fetches build info, runtime metrics,
// and triggers GC, revealing each result panel on demand.

import { beforeEach, expect, test, vi } from "vitest";
import { jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

beforeEach((): void => {
    vi.resetModules();
    loadFixture("admin_debug.html");
});

// Build info comes back as plain text from the server's debug endpoint.
function textResponse(body: string): Response {
    return new Response(body, { status: 200, headers: { "content-type": "text/plain" } });
}

async function initAdminDebugPage() {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: true, user: "Tester", admin: true });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse([]);
        }
        if (url === url_debugBuildInfo) {
            return textResponse(
                "go\tgo1.99\nbuild\tvcs.revision=abcdef0123456789\nbuild\tvcs.modified=true\n",
            );
        }
        if (url === url_debugRuntimeMetrics) {
            return textResponse("/sched/goroutines:goroutines 7");
        }
        if (url === url_debugGC) {
            return textResponse("GC complete");
        }
        return undefined;
    });
    await import("../typescript/admin_debug.ts");
    await vi.waitFor((): void => {
        expect(window.fetchBuildInfo).toBeTypeOf("function");
    });
    return mock;
}

test("fetching build info shows the panel and a revision link to GitHub", async (): Promise<void> => {
    await initAdminDebugPage();

    const div = document.getElementById("build-info-div") as HTMLDivElement;
    // The panel is hidden until the data is fetched.
    expect(div.style.display).toBe("none");

    await window.fetchBuildInfo(document.body);

    expect(div.style.display).toBe("");
    expect(document.getElementById("build-info")!.textContent).toContain("vcs.revision=abcdef0123456789");

    // The summary line links to the built revision and flags a dirty tree.
    const link = document.querySelector("#build-info-p a") as HTMLAnchorElement;
    expect(link.href).toBe("https://github.com/burningmantech/ranger-ims-go/tree/abcdef0123456789");
    expect(link.textContent).toContain("abcdef012345");
    expect(link.textContent).toContain("(dirty)");
});

test("fetching runtime metrics reveals the metrics panel", async (): Promise<void> => {
    await initAdminDebugPage();

    const div = document.getElementById("runtime-metrics-div") as HTMLDivElement;
    expect(div.style.display).toBe("none");

    await window.fetchRuntimeMetrics(document.body);

    expect(div.style.display).toBe("");
    expect(document.getElementById("runtime-metrics")!.textContent).toContain("goroutines");
});

test("performing GC posts to the GC endpoint and reveals its panel", async (): Promise<void> => {
    const mock = await initAdminDebugPage();

    const div = document.getElementById("gc-div") as HTMLDivElement;
    expect(div.style.display).toBe("none");

    await window.performGC(document.body);

    expect(div.style.display).toBe("");
    expect(document.getElementById("gc")!.textContent).toBe("GC complete");
    // GC is a mutation, so it's sent with a request body.
    const gcCall = mock.mock.calls.find(([url]) => url === url_debugGC)!;
    expect(gcCall[1]!.body).toBeDefined();
});
