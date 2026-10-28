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
// (admindebug.templ). The page lazily fetches build info, the redacted server
// config, and runtime metrics, and triggers GC, revealing each result panel on
// demand.

import { beforeEach, expect, test, vi } from "vitest";
import { jsonResponse, loadFixture, mockFetch, waitForPageInit } from "./helpers.ts";

beforeEach((): void => {
    vi.resetModules();
    loadFixture("admin_debug.html");
    buildInfo = "Database schema version: 42\n\ngo\tgo1.99\nbuild\tvcs.revision=abcdef0123456789\nbuild\tvcs.modified=true\n";
});

// Build info comes back as plain text from the server's debug endpoint.
function textResponse(body: string): Response {
    return new Response(body, { status: 200, headers: { "content-type": "text/plain" } });
}

let buildInfo: string;

async function initAdminDebugPage() {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: true, user: "Tester", admin: true });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse([]);
        }
        if (url === url_debugBuildInfo) {
            return textResponse(buildInfo);
        }
        if (url === url_debugConfig) {
            return textResponse("Core\n    JWTSecret = [\u{1F910}\u{1F910}]\n");
        }
        if (url === url_debugRuntimeMetrics) {
            return textResponse("/sched/goroutines:goroutines 7");
        }
        if (url === url_debugDBInfo) {
            return textResponse("IMS database\n    VERSION():    123.456.789-MariaDB\n");
        }
        if (url === url_debugDBVariables) {
            return textResponse("SHOW SESSION VARIABLES\n    max_connections = 151\n");
        }
        if (url === url_debugGC) {
            return textResponse("GC complete");
        }
        return undefined;
    });
    await import("../typescript/admin_debug.ts");
    await waitForPageInit();
    return mock;
}

test("fetching build info shows the panel and a revision link to GitHub", async (): Promise<void> => {
    await initAdminDebugPage();

    const div = document.getElementById("build-info-div") as HTMLDivElement;
    // The panel is hidden until the data is fetched.
    expect(div.style.display).toBe("none");

    document.getElementById("show-build-info")!.click();

    await vi.waitFor((): void => {
        expect(div.style.display).toBe("");
    });
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

    document.getElementById("show-metrics")!.click();

    await vi.waitFor((): void => {
        expect(div.style.display).toBe("");
    });
    expect(document.getElementById("runtime-metrics")!.textContent).toContain("goroutines");
});

test("fetching the server config reveals the config panel", async (): Promise<void> => {
    await initAdminDebugPage();

    const div = document.getElementById("config-div") as HTMLDivElement;
    expect(div.style.display).toBe("none");

    document.getElementById("show-config")!.click();

    await vi.waitFor((): void => {
        expect(div.style.display).toBe("");
    });
    expect(document.getElementById("config")!.textContent).toContain("JWTSecret");
});

test("fetching database info reveals the database info panel", async (): Promise<void> => {
    await initAdminDebugPage();

    const div = document.getElementById("db-info-div") as HTMLDivElement;
    expect(div.style.display).toBe("none");

    document.getElementById("show-db-info")!.click();

    await vi.waitFor((): void => {
        expect(div.style.display).toBe("");
    });
    expect(document.getElementById("db-info")!.textContent).toContain("123.456.789-MariaDB");
    // The full variables dump is a separate panel, and stays hidden.
    expect((document.getElementById("db-variables-div") as HTMLDivElement).style.display).toBe("none");
});

test("fetching all database variables reveals the variables panel", async (): Promise<void> => {
    await initAdminDebugPage();

    const div = document.getElementById("db-variables-div") as HTMLDivElement;
    expect(div.style.display).toBe("none");

    document.getElementById("show-db-variables")!.click();

    await vi.waitFor((): void => {
        expect(div.style.display).toBe("");
    });
    expect(document.getElementById("db-variables")!.textContent).toContain("max_connections = 151");
});

test("performing GC posts to the GC endpoint and reveals its panel", async (): Promise<void> => {
    const mock = await initAdminDebugPage();

    const div = document.getElementById("gc-div") as HTMLDivElement;
    expect(div.style.display).toBe("none");

    document.getElementById("perform-gc")!.click();

    await vi.waitFor((): void => {
        expect(div.style.display).toBe("");
    });
    expect(document.getElementById("gc")!.textContent).toBe("GC complete");
    // GC is a mutation, so it's sent with a request body.
    const gcCall = mock.mock.calls.find(([url]) => url === url_debugGC)!;
    expect(gcCall[1]!.body).toBeDefined();
});

test("build info ending on the revision still links to it, without a dirty flag", async (): Promise<void> => {
    buildInfo = "go\tgo1.99\nbuild\tvcs.modified=false\nbuild\tvcs.revision=0123456789abcdef";
    await initAdminDebugPage();

    document.getElementById("show-build-info")!.click();

    const link = await vi.waitFor((): HTMLAnchorElement => {
        const link = document.querySelector<HTMLAnchorElement>("#build-info-p a");
        expect(link).not.toBeNull();
        return link!;
    });
    expect(link.href).toBe("https://github.com/burningmantech/ranger-ims-go/tree/0123456789abcdef");
    expect(link.textContent).not.toContain("(dirty)");
});

test("build info without a revision links to the repository's tree root", async (): Promise<void> => {
    buildInfo = "go\tgo1.99\n";
    await initAdminDebugPage();

    document.getElementById("show-build-info")!.click();

    const link = await vi.waitFor((): HTMLAnchorElement => {
        const link = document.querySelector<HTMLAnchorElement>("#build-info-p a");
        expect(link).not.toBeNull();
        return link!;
    });
    expect(link.href).toBe("https://github.com/burningmantech/ranger-ims-go/tree/");
});

test("an unauthenticated visitor is sent to the login page", async (): Promise<void> => {
    const replace = vi.spyOn(window.location, "replace").mockImplementation((): void => {});
    mockFetch((url) => {
        if (url === url_auth) {
            return jsonResponse({ authenticated: false });
        }
        if (url === url_events || url === url_logout) {
            return jsonResponse([]);
        }
        return undefined;
    });
    await import("../typescript/admin_debug.ts");

    await vi.waitFor((): void => {
        expect(replace).toHaveBeenCalledWith(expect.stringContaining(url_login));
    });
});
