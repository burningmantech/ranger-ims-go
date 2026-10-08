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
import type { EventData } from "../typescript/ims.ts";
import { type FetchHandler, jsonResponse, loadFixture, mockFetch } from "./helpers.ts";

beforeEach((): void => {
    vi.resetModules();
    loadFixture("root.html");
    // Each test starts at the app's landing URL with no query string.
    window.history.replaceState(null, "", url_app);
});

// Import root.ts behind a fake server and wait for its init to settle (signaled
// by the auth check having been made).
async function initRootPage(
    authenticated: boolean,
    handler: FetchHandler = () => undefined,
    events: EventData[] = [],
) {
    const mock = mockFetch((url, init) => {
        if (url === url_auth && init?.body == null) {
            return jsonResponse({ authenticated: authenticated, user: "Tester" });
        }
        if (url === url_events && init?.body == null) {
            return jsonResponse(events);
        }
        return handler(url, init);
    });
    await import("../typescript/root.ts");
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url]) => url === url_auth)).toBe(true);
    });
    return mock;
}

test("an authenticated visitor gets a focused link to the active event", async (): Promise<void> => {
    await initRootPage(true, undefined, [
        { id: 1, name: "2025", is_active: false },
        { id: 2, name: "2026", is_active: true },
    ]);
    await vi.waitFor((): void => {
        expect(document.activeElement?.id).toBe("active-event-link");
    });
    const link = document.getElementById("active-event-link") as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe(url_viewIncidents.replace("<event_id>", "2026"));
    expect(link.textContent).toBe("Jump to the 2026 event");
    expect(document.getElementById("active-event-jump")!.classList.contains("hidden")).toBe(false);
    expect(document.getElementById("no-active-event")!.classList.contains("hidden")).toBe(true);
});

test("with no active event, an authenticated visitor is told to pick one from the dropdown", async (): Promise<void> => {
    const mock = await initRootPage(true, undefined, [
        { id: 1, name: "2025", is_active: false },
    ]);
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url]) => url === url_events)).toBe(true);
    });
    await vi.waitFor((): void => {
        expect(document.querySelectorAll("#nav-events a").length).toBe(1);
    });
    expect(document.getElementById("active-event-jump")!.classList.contains("hidden")).toBe(true);
    expect(document.getElementById("no-active-event")!.classList.contains("hidden")).toBe(false);
});

test("an unauthenticated visitor gets focus on the login button", async (): Promise<void> => {
    await initRootPage(false);
    await vi.waitFor((): void => {
        expect(document.activeElement?.id).toBe("login-button");
    });
});

test("the ?logout flow clears browser storage, hits the logout endpoint, and cleans the URL", async (): Promise<void> => {
    window.history.replaceState(null, "", `${url_app}?logout=true`);
    localStorage.setItem("preferred_incidents_state", "open");
    sessionStorage.setItem("something", "cached");

    const mock = await initRootPage(true, (url) => {
        if (url === url_logout) {
            return new Response(null, { status: 200 });
        }
        return undefined;
    });

    expect(mock.mock.calls.some(([url]) => url === url_logout)).toBe(true);
    expect(localStorage.getItem("preferred_incidents_state")).toBeNull();
    expect(sessionStorage.getItem("something")).toBeNull();
    // The logout query string is stripped so a refresh won't log out again.
    expect(window.location.search).toBe("");
    expect(window.location.pathname).toBe(url_app);
});

function pressCtrlK(): void {
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "k", ctrlKey: true, bubbles: true, cancelable: true }));
}

function goToEventOptions(): string[] {
    return [...(document.getElementById("goto-event") as HTMLSelectElement).options]
        .map((opt: HTMLOptionElement): string => opt.value);
}

test("Ctrl+K on the home page opens Go to… on the active event", async (): Promise<void> => {
    await initRootPage(true, undefined, [
        { id: 1, name: "2025", is_active: false },
        { id: 2, name: "2024", is_active: true },
    ]);
    await vi.waitFor((): void => {
        expect(goToEventOptions()).toEqual(["2025", "2024"]);
    });

    pressCtrlK();
    expect((document.getElementById("goToModal") as HTMLDialogElement).open).toBe(true);
    expect((document.getElementById("goto-event") as HTMLSelectElement).value).toBe("2024");
    expect((document.getElementById("goto-kind-incident") as HTMLInputElement).checked).toBe(true);
});

test("with no active event, Go to… on the home page starts on the most recently created event", async (): Promise<void> => {
    await initRootPage(true, undefined, [
        { id: 3, name: "2024", is_active: false },
        { id: 1, name: "Test", is_active: false },
        { id: 2, name: "2025", is_active: false },
    ]);
    await vi.waitFor((): void => {
        expect(goToEventOptions()).toEqual(["Test", "2025", "2024"]);
    });

    // Not the first one listed, which only sorts first by name.
    pressCtrlK();
    expect((document.getElementById("goto-event") as HTMLSelectElement).value).toBe("2024");
});

test("Go to… on the home page looks up and opens a record in the chosen event", async (): Promise<void> => {
    await initRootPage(true, (url) => {
        if (url === "/ims/api/events/2024/visits/3") {
            return jsonResponse({ number: 3, event: "2024", guest_preferred_name: "Stardust" });
        }
        return undefined;
    }, [
        { id: 1, name: "2025", is_active: true },
        { id: 2, name: "2024", is_active: false },
    ]);
    const assign = vi.spyOn(window.location, "assign").mockImplementation((): void => {});
    await vi.waitFor((): void => {
        expect(goToEventOptions()).toEqual(["2025", "2024"]);
    });
    pressCtrlK();

    const select = document.getElementById("goto-event") as HTMLSelectElement;
    select.value = "2024";
    select.dispatchEvent(new Event("change", { bubbles: true }));
    const visitRadio = document.getElementById("goto-kind-visit") as HTMLInputElement;
    visitRadio.checked = true;
    visitRadio.dispatchEvent(new Event("change", { bubbles: true }));
    const input = document.getElementById("goto-number") as HTMLInputElement;
    input.value = "3";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    await vi.waitFor((): void => {
        expect(document.getElementById("goto-preview")!.textContent).toBe("VS #3: Stardust");
    });

    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true }));
    await vi.waitFor((): void => {
        expect(assign).toHaveBeenCalledWith("/ims/app/events/2024/visits/3");
    });
});

test("Go to… doesn't open when there are no events to go to", async (): Promise<void> => {
    const mock = await initRootPage(true);
    await vi.waitFor((): void => {
        expect(mock.mock.calls.some(([url]) => url === url_events)).toBe(true);
    });

    pressCtrlK();
    expect((document.getElementById("goToModal") as HTMLDialogElement).open).toBe(false);
});

test("Go to… is off for a visitor who isn't logged in", async (): Promise<void> => {
    await initRootPage(false, undefined, [{ id: 1, name: "2025", is_active: true }]);

    pressCtrlK();
    expect((document.getElementById("goToModal") as HTMLDialogElement).open).toBe(false);
});
