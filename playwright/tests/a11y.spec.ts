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

// Accessibility scans of every IMS page, using axe-core.
//
// Each page is scanned in both the light and dark themes, since the app's
// custom colors differ between them and color contrast is theme-dependent.
//
// A scan fails on any WCAG 2.1 A/AA violation that isn't in KNOWN_VIOLATIONS
// below. That allowlist exists so this suite can be a blocking gate from the
// start while the known backlog is worked down: adding a new violation to a
// page breaks the build immediately, and fixing an old one means deleting its
// entry here. The allowlist should only ever shrink.

import AxeBuilder from "@axe-core/playwright";
import {expect, Page, test} from "@playwright/test";

const baseURL = "http://localhost:8080";
const username = "Hardware";

// The event seeded into the dev stack by store/fakeimsdb/seed.sql, which has
// incidents, field reports and visits (and grants write access to everyone).
const seededEvent = "2026";
const seededIncident = 1;
const seededFieldReport = 1;
const seededVisit = 1;

// WCAG 2.1 Level A and AA, plus axe's "best-practice" rules. The latter
// aren't conformance failures, but they cover the structural things screen
// reader users actually rely on (landmarks, heading order, list semantics)
// and they're what Lighthouse's accessibility score reports.
const wcagTags = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "best-practice"];

// Known, not-yet-fixed violations, keyed by page name, as axe rule IDs. Every
// entry here is a bug we intend to fix; the goal is an empty object.
const KNOWN_VIOLATIONS: Record<string, string[]> = {};

async function login(page: Page): Promise<void> {
  await page.goto(`${baseURL}/ims/app/`);
  await expect(page.getByRole("button", {name: /^Log (In|Out)$/})).toBeVisible();
  if (await page.getByRole("button", {name: "Log In"}).isVisible()) {
    await page.getByRole("button", {name: "Log In"}).click();
    await page.getByPlaceholder("name@example.com").fill(username);
    await page.getByPlaceholder("Password").fill(username);
    await page.getByPlaceholder("Password").press("Enter");
  }
  await expect(page.getByRole("button", {name: "Log Out"})).toBeVisible();
}

// setTheme pins the theme, which is otherwise "auto" (i.e. dependent on the
// OS setting of whoever runs the tests).
async function setTheme(page: Page, theme: "light" | "dark"): Promise<void> {
  await page.addInitScript((t: string): void => {
    localStorage.setItem("theme", t);
  }, theme);
}

// scan runs axe over the current page and asserts that it finds no violation
// that isn't already known.
async function scan(page: Page, pageName: string): Promise<void> {
  const results = await new AxeBuilder({page}).withTags(wcagTags).analyze();

  const known = KNOWN_VIOLATIONS[pageName] ?? [];
  const unexpected = results.violations.filter((v): boolean => !known.includes(v.id));

  expect(describe(unexpected), `unexpected a11y violations on ${pageName}`).toEqual("");

  // Keep the allowlist honest: an entry that no longer fires is stale and
  // should be deleted, so that a regression can't hide behind it.
  const stillBroken = new Set(results.violations.map((v): string => v.id));
  const stale = known.filter((id): boolean => !stillBroken.has(id));
  expect(stale, `stale KNOWN_VIOLATIONS entries for ${pageName}; delete them`).toEqual([]);
}

// describe renders violations as a readable multi-line report, so a failure
// says what's wrong and where rather than just dumping a count.
function describe(violations: {id: string; help: string; helpUrl: string; nodes: {html: string}[]}[]): string {
  return violations.map((v): string => {
    const targets = v.nodes.slice(0, 5).map((n): string => `      ${n.html}`).join("\n");
    const more = v.nodes.length > 5 ? `\n      ...and ${v.nodes.length - 5} more` : "";
    return `  [${v.id}] ${v.help} (${v.helpUrl})\n${targets}${more}`;
  }).join("\n\n");
}

// The pages to scan, as a name and the work needed to get the browser onto
// that page in a state worth scanning (i.e. with its content loaded).
const pages: {name: string; goto: (page: Page) => Promise<void>}[] = [
  {
    name: "root",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/`);
      await expect(page.getByRole("button", {name: "Log Out"})).toBeVisible();
    },
  },
  {
    name: "incidents",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/events/${seededEvent}/incidents`);
      await expect(page.locator("#queue_table tbody tr").first()).toBeVisible();
    },
  },
  {
    name: "incident",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/events/${seededEvent}/incidents/${seededIncident}`);
      await expect(page.getByLabel("IMS #", {exact: true})).toHaveValue(String(seededIncident));
    },
  },
  {
    name: "field_reports",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/events/${seededEvent}/field_reports`);
      await expect(page.locator("#field_reports_table tbody tr").first()).toBeVisible();
    },
  },
  {
    name: "field_report",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/events/${seededEvent}/field_reports/${seededFieldReport}`);
      await expect(page.getByLabel("FR #")).toHaveValue(String(seededFieldReport));
    },
  },
  {
    name: "sanctuary_visits",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/events/${seededEvent}/visits`);
      await expect(page.locator("#visits_table tbody tr").first()).toBeVisible();
    },
  },
  {
    name: "sanctuary_visit",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/events/${seededEvent}/visits/${seededVisit}`);
      await expect(page.getByLabel("VS #")).toHaveValue(String(seededVisit));
    },
  },
  {
    name: "places",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/events/${seededEvent}/places`);
      await expect(page.locator("#doc-title")).toBeVisible();
    },
  },
  {
    name: "search",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/search`);
      await expect(page.locator("#doc-title")).toBeVisible();
    },
  },
  {
    name: "settings",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/settings`);
      await expect(page.locator("#doc-title")).toBeVisible();
    },
  },
  {
    name: "admin_root",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/admin`);
      await expect(page.getByRole("link", {name: "Events"})).toBeVisible();
    },
  },
  {
    name: "admin_events",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/admin/events`);
      await expect(page.locator(".event_access").first()).toBeVisible();
    },
  },
  {
    name: "admin_types",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/admin/types`);
      await expect(page.locator("#incident_types_container li").first()).toBeVisible();
    },
  },
  {
    name: "admin_places",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/admin/places`);
      await expect(page.locator("#doc-title")).toBeVisible();
    },
  },
  {
    name: "admin_directory",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/admin/directory`);
      await expect(page.locator("#doc-title")).toBeVisible();
    },
  },
  {
    name: "admin_action_logs",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/admin/actionlogs`);
      await expect(page.locator("#doc-title")).toBeVisible();
    },
  },
  {
    name: "admin_debug",
    goto: async (page): Promise<void> => {
      await page.goto(`${baseURL}/ims/app/admin/debug`);
      await expect(page.locator("#doc-title")).toBeVisible();
    },
  },
];

for (const theme of ["light", "dark"] as const) {
  test.describe(theme, (): void => {
    // The login page is the only one worth scanning logged out.
    test("login", async ({page}): Promise<void> => {
      await setTheme(page, theme);
      await page.goto(`${baseURL}/ims/auth/login`);
      await expect(page.getByPlaceholder("Password")).toBeVisible();
      await scan(page, "login");
    });

    for (const p of pages) {
      test(p.name, async ({page}): Promise<void> => {
        await setTheme(page, theme);
        await login(page);
        await p.goto(page);
        await scan(page, p.name);
      });
    }

    // Modals are rendered but hidden until opened, and axe only evaluates
    // what's visible, so open the big ones explicitly.
    test("incidents modals", async ({page}): Promise<void> => {
      await setTheme(page, theme);
      await login(page);
      await page.goto(`${baseURL}/ims/app/events/${seededEvent}/incidents`);
      await expect(page.locator("#queue_table tbody tr").first()).toBeVisible();

      await page.locator("body").press("?");
      await expect(page.locator("#helpModal")).toBeVisible();
      await scan(page, "incidents help modal");
      await page.locator("#helpModal").press("Escape");
      await expect(page.locator("#helpModal")).toBeHidden();

      await page.locator("body").press("m");
      await expect(page.locator("#multisearchModal")).toBeVisible();
      await scan(page, "multisearch modal");
    });
  });
}

// Axe checks the accessibility tree, but it can't tell you whether the page is
// actually operable without a mouse. These do.
test.describe("keyboard", (): void => {
  test("the skip link jumps past the navbar to the main content", async ({page}): Promise<void> => {
    await login(page);
    await page.goto(`${baseURL}/ims/app/events/${seededEvent}/incidents`);
    await expect(page.locator("#queue_table tbody tr").first()).toBeVisible();

    // The skip link is the very first thing a keyboard user reaches, and it's
    // only visible once it has focus.
    await page.keyboard.press("Tab");
    const skipLink = page.getByRole("link", {name: "Skip to main content"});
    await expect(skipLink).toBeFocused();
    await expect(skipLink).toBeVisible();

    await page.keyboard.press("Enter");
    await expect(page.locator("#main")).toBeFocused();
  });

  test("table columns can be sorted without a mouse", async ({page}): Promise<void> => {
    await login(page);
    await page.goto(`${baseURL}/ims/app/events/${seededEvent}/incidents`);
    await expect(page.locator("#queue_table tbody tr").first()).toBeVisible();

    // DataTables renders its sort controls as role="button" spans with no
    // tabindex, which are useless to a keyboard user. Note that the header the
    // user sees is a clone (.dt-scroll-head); #queue_table's own header is
    // hidden, so putting the tabindex only there would fix nothing.
    const header = page.locator(".dt-scroll-head thead th").first();
    const sortControl = header.locator(".dt-column-order");
    await sortControl.focus();
    await expect(sortControl).toBeFocused();

    await page.keyboard.press("Enter");
    await expect(header).toHaveAttribute("aria-sort", "ascending");
    await page.keyboard.press("Enter");
    await expect(header).toHaveAttribute("aria-sort", "descending");
  });

  test("single-key shortcuts can be switched off in settings", async ({page}): Promise<void> => {
    await login(page);
    await page.goto(`${baseURL}/ims/app/settings`);

    // WCAG 2.1.4: a single-character shortcut must be able to be turned off.
    const toggle = page.getByLabel("Single-key shortcuts");
    await expect(toggle).toBeChecked();
    await toggle.uncheck();

    await page.goto(`${baseURL}/ims/app/events/${seededEvent}/incidents`);
    await expect(page.locator("#queue_table tbody tr").first()).toBeVisible();
    await page.locator("body").press("?");
    await expect(page.locator("#helpModal")).toBeHidden();

    // ...and back on again.
    await page.goto(`${baseURL}/ims/app/settings`);
    await page.getByLabel("Single-key shortcuts").check();
    await page.goto(`${baseURL}/ims/app/events/${seededEvent}/incidents`);
    await expect(page.locator("#queue_table tbody tr").first()).toBeVisible();
    await page.locator("body").press("?");
    await expect(page.locator("#helpModal")).toBeVisible();
  });

  test("an edit is announced to assistive tech via the live region", async ({page}): Promise<void> => {
    await login(page);
    await page.goto(`${baseURL}/ims/app/events/${seededEvent}/incidents/${seededIncident}`);
    await expect(page.getByLabel("IMS #", {exact: true})).toHaveValue(String(seededIncident));

    // A save is otherwise signalled only by a green border, which a screen
    // reader user has no way of perceiving.
    const summary = page.getByLabel("Summary");
    await summary.fill(`Something bad ${Date.now()}`);
    await summary.press("Tab");

    await expect(page.locator("#aria_live")).toHaveText("Saved");
  });
});
