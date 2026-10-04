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

// End-to-end tests of the Search Everywhere page. The page's client logic is
// covered by web/typescripttest/search.test.ts and the matching itself by
// api/integration/search_test.go, so these only check that the pieces are
// wired together in a real browser.
//
// These tests create nothing. They search for records seeded into the dev
// stack by store/fakeimsdb/seed.sql, and they log in through the API once per
// worker rather than through the login page in every test.

import {test as base, expect, Locator, Page} from "@playwright/test";

const baseURL = "http://localhost:8080";
const username = "Hardware";
const seededEvent = "2026";

const test = base.extend<{}, {workerStorageState: string}>({
  storageState: ({workerStorageState}, use) => use(workerStorageState),
  workerStorageState: [async ({playwright}, use, workerInfo): Promise<void> => {
    const request = await playwright.request.newContext({baseURL});
    const resp = await request.post("/ims/api/auth", {
      data: {identification: username, password: username},
    });
    expect(resp.ok()).toBeTruthy();
    const file = workerInfo.project.outputDir + `/.auth/search-${workerInfo.parallelIndex}.json`;
    await request.storageState({path: file});
    await request.dispose();
    await use(file);
  }, {scope: "worker"}],
});

function resultRow(page: Page, kind: string, num: number): Locator {
  return page.locator("#search_results_table tbody tr")
    .filter({has: page.locator(".result-event", {hasText: new RegExp(`^${seededEvent}$`)})})
    .filter({has: page.locator(".result-kind", {hasText: new RegExp(`^${kind}$`)})})
    .filter({has: page.locator(".result-number", {hasText: new RegExp(`^${num}$`)})});
}

async function search(page: Page, query: string): Promise<void> {
  await page.goto(`${baseURL}/ims/app/search`);
  // Page init ends by focusing the search box, so that's the signal that
  // pressing Enter will run a search.
  await expect(page.locator("#search_input")).toBeFocused();
  await page.locator("#search_input").fill(query);
  await page.locator("#search_input").press("Enter");
}

test("finds an incident and links to it", async ({page}) => {
  await search(page, "Lost participant near sound camps");

  const row = resultRow(page, "Incident", 3);
  await expect(row).toBeVisible();
  await expect(row.locator(".result-summary")).toHaveText("Lost participant near sound camps");

  await row.getByRole("link", {name: "3", exact: true}).click();
  await expect(page).toHaveURL(`${baseURL}/ims/app/events/${seededEvent}/incidents/3`);
  await expect(page.getByLabel("IMS #", {exact: true})).toHaveValue("3");
});

test("finds a field report by its report entry text", async ({page}) => {
  await search(page, "out in the dust");

  const row = resultRow(page, "Field Report", 1);
  await expect(row).toBeVisible();
  await expect(row.locator(".result-snippet")).toContainText("Something happened out in the dust");
  await expect(row.getByRole("link", {name: "1", exact: true}))
    .toHaveAttribute("href", `/ims/app/events/${seededEvent}/field_reports/1`);
});

test("finds a visit by guest name", async ({page}) => {
  await search(page, "Wayne Wilson");

  const row = resultRow(page, "Visit", 1);
  await expect(row).toBeVisible();
  await expect(row.getByRole("link", {name: "1", exact: true}))
    .toHaveAttribute("href", `/ims/app/events/${seededEvent}/visits/1`);
});

test("runs the search from the URL fragment, with its type filter", async ({page}) => {
  // "Something happened" is in report entries on both Incident 1 and Field
  // Report 1, but the fragment limits the search to Field Reports.
  await page.goto(`${baseURL}/ims/app/search#q=Something+happened&kinds=field_report`);

  await expect(page.locator("#search_input")).toHaveValue("Something happened");
  await expect(page.locator("#show_kind")).toHaveText("Field Reports");
  await expect(resultRow(page, "Field Report", 1)).toBeVisible();
  await expect(resultRow(page, "Incident", 1)).toHaveCount(0);
});
