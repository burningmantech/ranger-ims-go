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

// Set the theme immediately, before the rest of the page loads, so that there's
// no flickering. We'll then come back later to applyTheme, which sets the navbar
// dropdown icons.
setTheme(getPreferredTheme());
document.addEventListener("DOMContentLoaded", applyTheme);

//
// Apply the HTML theme, light or dark or default.
//
// Adapted from https://getbootstrap.com/docs/5.3/customize/color-modes/#javascript
// Under Creative Commons Attribution 3.0 Unported License

function getStoredTheme(): string|null {
    return localStorage.getItem("theme");
}
function setStoredTheme(theme: string): void {
    localStorage.setItem("theme", theme);
}
function getPreferredTheme(): string {
    const stored = getStoredTheme();
    if (stored != null) {
        return stored;
    }
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}
function setTheme(theme: string): void {
    if (theme === "auto") {
        theme = window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    }
    document.documentElement.dataset["bsTheme"] = theme;
}
function applyTheme(): void {
    setTheme(getPreferredTheme());

    function showActiveTheme(theme: string, focus: boolean = false): void {
        const themeSwitcher: HTMLButtonElement|null = document.querySelector("#bd-theme");

        if (!themeSwitcher) {
            return;
        }

        const themeSwitcherText = document.querySelector("#bd-theme-text")!;
        const activeThemeIcon = document.querySelector(".theme-icon-active use") as SVGUseElement;
        const btnToActive: HTMLButtonElement = document.querySelector(`[data-bs-theme-value="${theme}"]`)!;
        const svgOfActiveBtn: string = (btnToActive.querySelector("svg use") as SVGUseElement).href.baseVal;

        for (const val of document.querySelectorAll("[data-bs-theme-value]")) {
            val.classList.remove("active");
            val.ariaPressed = "false";
        }

        btnToActive.classList.add("active");
        btnToActive.ariaPressed = "true";
        if (svgOfActiveBtn) {
            activeThemeIcon.href.baseVal = svgOfActiveBtn;
        }
        if (themeSwitcherText) {
            // Set theme switcher label
            themeSwitcher.ariaLabel = `${themeSwitcherText.textContent} (${btnToActive.dataset["bsThemeValue"]})`;
        }

        if (focus) {
            themeSwitcher.focus();
        }
    }

    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", (): void => {
        const storedTheme = getStoredTheme();
        if (storedTheme !== "light" && storedTheme !== "dark") {
            setTheme(getPreferredTheme());
        }
    });

    showActiveTheme(getPreferredTheme());

    for (const togEl of document.querySelectorAll("[data-bs-theme-value]")) {
        const toggle = togEl as HTMLElement;
        toggle.addEventListener("click", function(_e: MouseEvent): void {
            const theme = toggle.dataset["bsThemeValue"];
            if (theme) {
                setStoredTheme(theme);
                setTheme(theme);
                showActiveTheme(theme, true);
            }
        });
    }

    // Switch to light mode for printing, then switch back afterward.
    window.addEventListener("beforeprint", (_event: Event): void => {
        setTheme("light");
    });
    window.addEventListener("afterprint", (_event: Event): void => {
        setTheme(getPreferredTheme());
    });
}
