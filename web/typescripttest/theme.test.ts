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

// Tests for theme.ts against the theme dropdown markup that Nav (nav.templ)
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

test("clicking the Dark theme button switches and persists the theme", async (): Promise<void> => {
    await runThemeScript();

    const darkButton = document.querySelector<HTMLButtonElement>('[data-bs-theme-value="dark"]')!;
    darkButton.click();

    expect(document.documentElement.dataset["bsTheme"]).toBe("dark");
    expect(localStorage.getItem("theme")).toBe("dark");

    // The dropdown reflects the active choice (markup from nav.templ).
    expect(darkButton.classList.contains("active")).toBe(true);
    expect(darkButton.ariaPressed).toBe("true");
    const autoButton = document.querySelector<HTMLButtonElement>('[data-bs-theme-value="auto"]')!;
    expect(autoButton.classList.contains("active")).toBe(false);
    expect(autoButton.ariaPressed).toBe("false");

    // The navbar icon switches to the dark-theme icon.
    const activeIcon = document.querySelector(".theme-icon-active use") as SVGUseElement;
    expect(activeIcon.href.baseVal).toBe("#moon-stars-fill");
});
