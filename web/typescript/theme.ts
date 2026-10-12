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

// This file is loaded as a classic script rather than a module, so it can't be
// imported from. Pages that offer their own theme control (the settings page)
// use these window functions, and listen for the "ims:themechange" event to
// follow along when the theme is changed from the navbar toggle.
interface Window {
    imsThemeSetting: () => ThemeSetting;
    imsEffectiveTheme: () => ResolvedTheme;
    imsSetThemeSetting: (theme: ThemeSetting) => void;
}
interface DocumentEventMap {
    "ims:themechange": CustomEvent<ThemeSetting>;
}
window.imsThemeSetting = themeSetting;
window.imsEffectiveTheme = (): ResolvedTheme => effectiveTheme(themeSetting());
window.imsSetThemeSetting = chooseTheme;

// Set the theme immediately, before the rest of the page loads, so that there's
// no flickering. We'll then come back later to applyTheme, which sets the
// navbar toggle's icon.
setTheme(themeSetting());
document.addEventListener("DOMContentLoaded", applyTheme);

//
// Apply the HTML theme, light or dark.
//
// People who haven't made an explicit choice get "auto", which follows the
// system preference. Auto isn't offered as a choice in the theme controls;
// it's just the state before someone picks Light or Dark.
//
// Adapted from https://getbootstrap.com/docs/5.3/customize/color-modes/#javascript
// Under Creative Commons Attribution 3.0 Unported License

type ThemeSetting = "auto"|"light"|"dark";
type ResolvedTheme = Exclude<ThemeSetting, "auto">;

function asThemeSetting(value: string|null|undefined): ThemeSetting|null {
    if (value === "auto" || value === "light" || value === "dark") {
        return value;
    }
    return null;
}
function themeSetting(): ThemeSetting {
    return asThemeSetting(localStorage.getItem("theme")) ?? "auto";
}
function effectiveTheme(setting: ThemeSetting): ResolvedTheme {
    if (setting !== "auto") {
        return setting;
    }
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}
function setTheme(theme: ThemeSetting): void {
    document.documentElement.dataset["bsTheme"] = effectiveTheme(theme);
}
function chooseTheme(theme: ThemeSetting): void {
    localStorage.setItem("theme", theme);
    setTheme(theme);
    updateThemeToggle(theme);
    document.dispatchEvent(new CustomEvent("ims:themechange", {detail: theme}));
}
function updateThemeToggle(theme: ThemeSetting): void {
    const themeToggle: HTMLButtonElement|null = document.querySelector("#bd-theme");

    if (!themeToggle) {
        return;
    }

    // The toggle shows the theme that's active, which for the unchosen default
    // is the one the system preference resolves to.
    const shown: ResolvedTheme = effectiveTheme(theme);
    const activeThemeIcon = themeToggle.querySelector("use") as SVGUseElement;
    activeThemeIcon.href.baseVal = shown === "dark" ? "#moon-stars-fill" : "#sun-fill";
    themeToggle.ariaLabel = `Toggle theme (${shown})`;
}
function applyTheme(): void {
    setTheme(themeSetting());

    window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", (): void => {
        if (themeSetting() === "auto") {
            setTheme("auto");
            // Keep the toggle's icon and label on the resolved theme.
            updateThemeToggle("auto");
        }
    });

    updateThemeToggle(themeSetting());

    // The toggle switches between the two themes. When there's no explicit
    // choice stored yet, clicking makes one.
    const themeToggle: HTMLButtonElement|null = document.querySelector("#bd-theme");
    themeToggle?.addEventListener("click", (): void => {
        chooseTheme(effectiveTheme(themeSetting()) === "dark" ? "light" : "dark");
    });

    // Switch to light mode for printing, then switch back afterward.
    window.addEventListener("beforeprint", (_event: Event): void => {
        setTheme("light");
    });
    window.addEventListener("afterprint", (_event: Event): void => {
        setTheme(themeSetting());
    });
}
