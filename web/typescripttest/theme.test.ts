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

// Tests for theme.ts against the theme toggle markup that Nav (nav.templ)
// renders into every page. The login fixture is used as a representative page.

import { beforeEach, expect, test, vi } from "vitest";
import { loadFixture } from "./helpers.ts";

beforeEach((): void => {
    vi.resetModules();
    loadFixture("login.html");
});

// theme.ts is loaded as a classic script in production (head.templ), so it has
// no exports; importing it just runs it, like the browser does.
async function runThemeScript(): Promise<void> {
    // @ts-expect-error TS2306: theme.ts is intentionally not a module (see above).
    await import("../typescript/theme.ts");
    // theme.js runs before DOMContentLoaded in a real page load; here the
    // document is already complete, so deliver the event by hand to run
    // applyTheme.
    document.dispatchEvent(new Event("DOMContentLoaded", { bubbles: true }));
}

test("the theme is applied at script load from the matchMedia preference", async (): Promise<void> => {
    await runThemeScript();
    // happy-dom's matchMedia does not match (prefers-color-scheme: dark).
    expect(document.documentElement.dataset["bsTheme"]).toBe("light");
});

test("a stored theme preference wins over the media query", async (): Promise<void> => {
    localStorage.setItem("theme", "dark");
    await runThemeScript();
    expect(document.documentElement.dataset["bsTheme"]).toBe("dark");
});

test("with no stored theme the toggle shows the resolved system theme", async (): Promise<void> => {
    await runThemeScript();

    // happy-dom's matchMedia does not match (prefers-color-scheme: dark), so
    // the unchosen default resolves to light.
    const themeToggle = document.querySelector<HTMLButtonElement>("#bd-theme")!;
    const activeIcon = themeToggle.querySelector("use") as SVGUseElement;
    expect(activeIcon.href.baseVal).toBe("#sun-fill");
    expect(themeToggle.ariaLabel).toBe("Toggle theme (light)");
});

test("a stored auto preference (from older versions) still follows the system theme", async (): Promise<void> => {
    localStorage.setItem("theme", "auto");
    await runThemeScript();
    expect(document.documentElement.dataset["bsTheme"]).toBe("light");
});

test("clicking the theme toggle switches to dark and persists the choice", async (): Promise<void> => {
    await runThemeScript();

    const themeToggle = document.querySelector<HTMLButtonElement>("#bd-theme")!;

    // Starting from the unchosen default (resolved to light), clicking
    // switches to dark.
    themeToggle.click();

    expect(document.documentElement.dataset["bsTheme"]).toBe("dark");
    expect(localStorage.getItem("theme")).toBe("dark");

    // The toggle icon and label switch to the dark theme.
    const activeIcon = themeToggle.querySelector("use") as SVGUseElement;
    expect(activeIcon.href.baseVal).toBe("#moon-stars-fill");
    expect(themeToggle.ariaLabel).toBe("Toggle theme (dark)");

    // Clicking again switches back to light.
    themeToggle.click();
    expect(document.documentElement.dataset["bsTheme"]).toBe("light");
    expect(localStorage.getItem("theme")).toBe("light");
    expect(activeIcon.href.baseVal).toBe("#sun-fill");
});

test("clicking the theme toggle from a stored dark theme switches to light", async (): Promise<void> => {
    localStorage.setItem("theme", "dark");
    await runThemeScript();

    const themeToggle = document.querySelector<HTMLButtonElement>("#bd-theme")!;
    themeToggle.click();

    expect(document.documentElement.dataset["bsTheme"]).toBe("light");
    expect(localStorage.getItem("theme")).toBe("light");

    const activeIcon = themeToggle.querySelector("use") as SVGUseElement;
    expect(activeIcon.href.baseVal).toBe("#sun-fill");
});

// Stub matchMedia with a system preference that a test can flip, returning a
// function that flips it and notifies the listeners, as the browser would.
function stubSystemTheme(dark: boolean): (dark: boolean) => void {
    const listeners: (() => void)[] = [];
    vi.stubGlobal("matchMedia", (): object => ({
        get matches(): boolean {
            return dark;
        },
        addEventListener: (_type: string, listener: () => void): void => void listeners.push(listener),
    }));
    return (nowDark: boolean): void => {
        dark = nowDark;
        for (const listener of listeners) {
            listener();
        }
    };
}

test("with no stored theme, the page follows a change in the system theme", async (): Promise<void> => {
    const setSystemDark = stubSystemTheme(false);
    await runThemeScript();
    const themeToggle = document.querySelector<HTMLButtonElement>("#bd-theme")!;
    expect(document.documentElement.dataset["bsTheme"]).toBe("light");

    setSystemDark(true);

    expect(document.documentElement.dataset["bsTheme"]).toBe("dark");
    expect(themeToggle.ariaLabel).toBe("Toggle theme (dark)");
});

test("a chosen theme ignores a change in the system theme", async (): Promise<void> => {
    localStorage.setItem("theme", "light");
    const setSystemDark = stubSystemTheme(false);
    await runThemeScript();

    setSystemDark(true);

    expect(document.documentElement.dataset["bsTheme"]).toBe("light");
});

test("printing switches to the light theme, then back", async (): Promise<void> => {
    localStorage.setItem("theme", "dark");
    await runThemeScript();

    window.dispatchEvent(new Event("beforeprint"));
    expect(document.documentElement.dataset["bsTheme"]).toBe("light");
    window.dispatchEvent(new Event("afterprint"));
    expect(document.documentElement.dataset["bsTheme"]).toBe("dark");
});

test("a page without the navbar toggle still gets its theme", async (): Promise<void> => {
    document.querySelector("#bd-theme")!.remove();
    await runThemeScript();

    (window as Window & { imsSetThemeSetting: (theme: string) => void }).imsSetThemeSetting("dark");
    expect(document.documentElement.dataset["bsTheme"]).toBe("dark");
});
