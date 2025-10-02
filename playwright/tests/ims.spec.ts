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

import {test, expect, Page} from "@playwright/test";

const username = "Hardware";

function randomName(prefix: string): string {
  return `${prefix}-${crypto.randomUUID()}`;
}

async function login(page: Page): Promise<void> {
  await page.goto("http://localhost:8080/ims/app/");
  // wait for one of the buttons to be shown
  await expect(page.getByRole("button", { name: /^Log (In|Out)$/ })).toBeVisible();
  if (await page.getByRole("button", { name: "Log In" }).isVisible()) {
    await page.getByRole("button", { name: "Log In" }).click();
    await page.getByPlaceholder("name@example.com").click();
    await page.getByPlaceholder("name@example.com").fill(username);
    await page.getByPlaceholder("Password").fill(username);
    await page.getByPlaceholder("Password").press("Enter");
  }
  await expect(page.getByRole("button", { name: "Log Out" })).toBeVisible();
}

async function adminPage(page: Page): Promise<void> {
  await maybeOpenNav(page);
  await page.getByRole("button", { name: username }).click();
  await page.getByRole("link", { name: "Admin" }).click();
}

async function incidentTypePage(page: Page): Promise<void> {
  await adminPage(page);
  await page.getByRole("link", { name: "Incident Types" }).click();
}

async function eventsPage(page: Page): Promise<void> {
  await adminPage(page);
  await page.getByRole("link", { name: "Events" }).click();
}

async function addIncidentType(page: Page, incidentType: string): Promise<void> {
  await incidentTypePage(page);
  await page.getByPlaceholder("Chooch").fill(incidentType);
  await page.getByPlaceholder("Chooch").press("Enter");
}

async function addEvent(page: Page, eventName: string): Promise<void> {
  await eventsPage(page);
  await page.getByPlaceholder("Burn-A-Matic 3000").fill(eventName);
  await page.getByPlaceholder("Burn-A-Matic 3000").press("Enter");

  await expect(page.getByText(`Access for ${eventName} (readers)`)).toBeVisible();
  await expect(page.getByText(`Access for ${eventName} (writers)`)).toBeVisible();
  await expect(page.getByText(`Access for ${eventName} (reporters)`)).toBeVisible();
}

async function addWriter(page: Page, eventName: string, writer: string): Promise<void> {
  await eventsPage(page);

  const writers = page.locator("div.card").filter({has: page.getByText(`Access for ${eventName} (writers)`)});

  await writers.getByRole("textbox").fill(writer);
  await writers.getByRole("textbox").press("Enter");
  await expect(writers.getByText(writer)).toBeVisible({timeout: 5000});
}

async function addReporter(page: Page, eventName: string, writer: string): Promise<void> {
  await eventsPage(page);

  const reporters = page.locator("div.card").filter({has: page.getByText(`Access for ${eventName} (reporters)`)});

  await reporters.getByRole("textbox").fill(writer);
  await reporters.getByRole("textbox").press("Enter");
  await expect(reporters.getByText(writer)).toBeVisible({timeout: 5000});
}

async function maybeOpenNav(page: Page): Promise<void> {
  const toggler = page.getByLabel("Toggle navigation");
  await expect(async (): Promise<void> => {
    if (await toggler.isVisible() && (await toggler.getAttribute("aria-expanded")) === "false") {
      await page.locator(".navbar-toggler").click();
      expect(toggler.getAttribute("aria-expanded")).toEqual("true");
    }
  }).toPass();
}

test("themes", async ({ page }) => {
  await page.goto("http://localhost:8080/ims/app/");

  await maybeOpenNav(page);
  await page.getByTitle("Color scheme").getByRole("button").click();
  await page.getByRole("button", { name: "Dark" }).click();
  expect(await page.locator("html").getAttribute("data-bs-theme")).toEqual("dark");

  await page.reload();
  expect(await page.locator("html").getAttribute("data-bs-theme")).toEqual("dark");
  await maybeOpenNav(page);
  await page.getByTitle("Color scheme").getByRole("button").click();
  await page.getByRole("button", { name: "Light" }).click();
  expect(await page.locator("html").getAttribute("data-bs-theme")).toEqual("light");

  await page.reload();
  expect(await page.locator("html").getAttribute("data-bs-theme")).toEqual("light");
})

test("admin_incident_types", async ({ page }) => {
  await login(page);

  const incidentType: string = randomName("type");
  await addIncidentType(page, incidentType);

  await incidentTypePage(page);

  const newLi = page.locator("li", {hasText: incidentType});
  await expect(newLi).toBeVisible();
  await expect(newLi.getByRole("button", {name: "Active"})).toBeVisible();
  await expect(newLi.getByRole("button", {name: "Hidden"})).toBeHidden();

  await newLi.getByRole("button", {name: "Active"}).click();
  await expect(newLi.getByRole("button", {name: "Active"})).toBeHidden();
  await expect(newLi.getByRole("button", {name: "Hidden"})).toBeVisible();
});

test("admin_events", async ({ browser }) => {
  const ctx = await browser.newContext();
  const page = await ctx.newPage()
  await login(page);

  const eventName: string = randomName("event");
  await addEvent(page, eventName);
  await addWriter(page, eventName, "person:SomeGuy");

  const writers = page.locator("div.card").filter({has: page.getByText(`Access for ${eventName} (writers)`)});
  // it's hard to tell on the client side when this has completed, hence the toPass block below
  await writers.locator("select").selectOption("On-Site");

  const page2 = await ctx.newPage();
  await login(page2);
  await eventsPage(page2);
  await expect(async (): Promise<void> => {
    const writers = page2.locator("div.card").filter({has: page2.getByText(`Access for ${eventName} (writers)`)});
    await expect(writers).toBeVisible();
    await expect(writers.getByText("person:SomeGuy")).toBeVisible();
    await expect(writers.locator("select")).toHaveValue("onsite");
    await expect(writers).not.toContainText("Expired");
    await expect(writers).toContainText("Unknown");

    await writers.getByRole("button", { name: "Set expiration" }).click();
    const expirationTime = writers.getByRole("textbox", {name: "Expiration time"});
    await expect(expirationTime).toBeVisible();
    await expirationTime.fill("2025-05-05T05:55");
    // focus anywhere else, so that the expirationTime oninput fires
    await writers.getByRole("textbox", { name: "person:Tool" }).focus();
    await expect(writers).toContainText("Expired");
    await expect(writers).toContainText("Unknown");

  }).toPass();

  await page2.close();
  await page.close();
  await ctx.close();
})

test("incidents", async ({ page, browser }) => {
  test.slow();

  // make a new event with a writer
  await login(page);
  const eventName: string = randomName("event");
  await addEvent(page, eventName);
  await addWriter(page, eventName, "person:" + username);

  // check that we can navigate to the incidents page for that event
  await page.goto("http://localhost:8080/ims/app/");
  await maybeOpenNav(page);
  await page.getByRole("button", {name: "Event"}).click();
  await page.getByRole("link", {name: eventName}).click();
  expect(page.url()).toBe(`http://localhost:8080/ims/app/events/${eventName}/incidents`);

  await page.close();

  for (let i = 0; i < 3; i++) {
    const ctx = await browser.newContext();
    const page = await ctx.newPage()
    await login(page);

    await page.goto(`http://localhost:8080/ims/app/events/${eventName}/incidents`);
    const incidentsPage = page;

    const incidentPage = await ctx.newPage();
    await incidentPage.goto(`http://localhost:8080/ims/app/events/${eventName}/incidents`);
    await incidentPage.getByRole("button", {name: "New"}).click();

    await expect(incidentPage.getByLabel("IMS #", {exact: true})).toHaveValue("(new)");
    const incidentSummary = randomName("summary");
    await incidentPage.getByLabel("Summary").fill(incidentSummary);
    await incidentPage.getByLabel("Summary").press("Tab");
    // wait for the new incident to be persisted
    await expect(incidentPage.getByLabel("IMS #", {exact: true})).toHaveValue(/^\d+$/);

    // check that the BroadcastChannel update to the first page worked
    await expect(incidentsPage.getByText(incidentSummary)).toBeVisible();

    // change the summary
    const newIncidentSummary = incidentSummary + " with suffix";
    await incidentPage.getByLabel("Summary").fill(newIncidentSummary);
    await incidentPage.getByLabel("Summary").press("Tab");
    // check that the BroadcastChannel update to the first page worked
    await expect(incidentsPage.getByText(newIncidentSummary)).toBeVisible();

    await incidentPage.getByLabel("State").selectOption("on_hold");
    await incidentPage.getByLabel("State").press("Tab");

    // add several incident types to the incident
    {
      async function addType(page: Page, type: string): Promise<void> {
        await page.getByLabel("Add Incident Type").fill(type);
        await page.getByLabel("Add Incident Type").press("Tab");

        await expect(
            page.locator("div.card").filter(
                {has: page.getByText("Incident Types")}
            ).locator("li", {hasText: type})).toBeVisible({timeout: 5000});
        await expect(page.getByLabel("Add Incident Type")).toHaveValue("");
      }

      await addType(incidentPage, "Admin");
      await addType(incidentPage, "Junk");
    }

    // add several Rangers to the incident
    {
      async function addRanger(page: Page, rangerName: string): Promise<void> {
        await page.getByLabel("Add Ranger Handle").fill("");
        await page.getByLabel("Add Ranger Handle").press("Tab");
        await page.getByLabel("Add Ranger Handle").fill(rangerName);
        await page.getByLabel("Add Ranger Handle").press("Tab");
        await expect(page.locator("li", {hasText: rangerName})).toBeVisible({timeout: 5000});
        await expect(page.getByLabel("Add Ranger Handle")).toHaveValue("");
      }

      await addRanger(incidentPage, "Doggy");
      await addRanger(incidentPage, "Runner");
      await addRanger(incidentPage, "Loosy");
      await addRanger(incidentPage, "TheMan");
    }

    // override start time
    {
      await incidentPage.getByTitle("Override Start Time").click();
      await incidentPage.getByRole("textbox", { name: "Start Date" }).fill("2025-01-27");
      await incidentPage.getByRole("textbox", { name: "Start Date" }).blur();
      await incidentPage.getByRole("textbox", { name: "Start Time" }).focus();
      // We want to look for a substring in the value, and that requires a regex.
      await expect(incidentPage.locator("#started_datetime")).toHaveValue(/Mon, Jan 27, 2025/);
      await incidentPage.getByRole("textbox", { name: "Start Time" }).clear();
      await incidentPage.getByRole("textbox", { name: "Start Time" }).fill("21:34");
      await incidentPage.getByRole("textbox", { name: "Start Date" }).focus();
      // We want to look for a substring in the value, and that requires a regex.
      await expect(incidentPage.locator("#started_datetime")).toHaveValue(/Mon, Jan 27, 2025, 21:34/);
      // Close the modal
      await incidentPage.getByRole("textbox", { name: "Start Time" }).press("Escape");
    }

    // add location details
    {
      await incidentPage.getByLabel("Location name").click();
      await incidentPage.getByLabel("Location name").fill("Somewhere");
      await incidentPage.getByLabel("Location name").press("Tab");
      await incidentPage.getByLabel("Incident location address radial hour").selectOption("3");
      await incidentPage.getByLabel("Incident location address radial minute").selectOption("15");
      await incidentPage.getByLabel("Additional location description").click();
      await incidentPage.getByLabel("Additional location description").fill("other there");
      await incidentPage.getByLabel("Additional location description").press("Tab");
    }
    // add a report entry
    const reportEntry = `This is some text - ${randomName("text")}`;
    {
      await incidentPage.getByLabel("New report entry text").fill(reportEntry);
      await incidentPage.getByLabel("Submit report entry").click();
      await expect(incidentPage.getByText(reportEntry)).toBeVisible();
    }
    // strike the entry, verified it's stricken
    {
      await incidentPage.getByText(reportEntry).hover();
      await incidentPage.getByRole("button", {name: "Strike"}).click();
      await expect(incidentPage.getByText(reportEntry)).toBeHidden();
    }
    // but the entry is shown when the right checkbox is ticked
    {
      await incidentPage.getByLabel("Show history and stricken").check();
      await expect(incidentPage.getByText(reportEntry)).toBeVisible();
    }
    // unstrike the entry and see it return to the default view
    {
      await incidentPage.getByText(reportEntry).hover();
      await incidentPage.getByRole("button", {name: "Unstrike"}).click();
      await incidentPage.getByLabel("Show history and stricken").uncheck();
      await expect(incidentPage.getByText(reportEntry)).toBeVisible();
    }

    // link the incident to another incident
    {
      if (i > 0) {
        await incidentPage.getByLabel("Link IMS #").fill("1");
        await incidentPage.getByLabel("Link IMS #").press("Enter");
        const linkedIncident = incidentPage.getByText(`IMS ${eventName} #1: `);
        await expect(linkedIncident).toBeVisible();
      }
    }

    // try searching for the incident by its report text
    {
      await incidentsPage.getByRole("searchbox").fill(reportEntry);
      await incidentsPage.getByRole("searchbox").press("Enter");
      await expect(incidentsPage.getByText(newIncidentSummary)).toBeVisible();
      await incidentsPage.getByRole("searchbox").fill("The wrong text!");
      await incidentsPage.getByRole("searchbox").press("Enter");
      await expect(incidentsPage.getByText(newIncidentSummary)).toBeHidden();
      await incidentsPage.getByRole("searchbox").clear();
      await incidentsPage.getByRole("searchbox").press("Enter");
      await expect(incidentsPage.getByText(newIncidentSummary)).toBeVisible();
    }

    // close the incident and see it disappear from the default Incidents page view
    {
      await incidentPage.getByLabel("State").selectOption("closed");
      await incidentPage.getByLabel("State").press("Tab");
      await expect(incidentsPage.getByText(newIncidentSummary)).toBeHidden();
    }

    await incidentPage.close();
    await incidentsPage.close();
    await ctx.close();
  }
})


test("field_reports", async ({ page, browser }) => {
  test.slow();

  // make a new event with a writer
  await login(page);
  const eventName: string = randomName("event");
  await addEvent(page, eventName);
  await addReporter(page, eventName, "person:" + username);

  // check that we can navigate to the incidents page for that event
  await page.goto("http://localhost:8080/ims/app/");
  await maybeOpenNav(page);
  await page.getByRole("button", {name: "Event"}).click();
  await page.getByRole("link", {name: eventName}).click();
  // we'll first hit the Incidents page, but because we're a reporter, we'll
  // get auto-redirected to Field Reports.
  await page.waitForURL(`http://localhost:8080/ims/app/events/${eventName}/field_reports`)

  await page.close();

  for (let i = 0; i < 3; i++) {
    const ctx = await browser.newContext();
    const page = await ctx.newPage()
    await login(page);

    await page.goto(`http://localhost:8080/ims/app/events/${eventName}/field_reports`);
    const tablePage = page;

    const frPage = await ctx.newPage();
    await frPage.goto(`http://localhost:8080/ims/app/events/${eventName}/field_reports`);
    await frPage.getByRole("button", {name: "New"}).click();

    await expect(frPage.getByLabel("FR #")).toHaveValue("(new)");
    const frSummary = randomName("summary");
    await frPage.getByLabel("Summary").fill(frSummary);
    await frPage.getByLabel("Summary").press("Tab");
    // wait for the new incident to be persisted
    await expect(frPage.getByLabel("FR #")).toHaveValue(/^\d+$/);

    // check that the BroadcastChannel update to the first page worked
    await expect(tablePage.getByText(frSummary)).toBeVisible();

      // change the summary
      const newSummary = frSummary + " with suffix";
      await frPage.getByLabel("Summary").fill(newSummary);
      await frPage.getByLabel("Summary").press("Tab");
      // check that the BroadcastChannel update to the first page worked
      await expect(tablePage.getByText(newSummary)).toBeVisible();

      // add a report entry
      const reportEntry = `This is some text - ${randomName("text")}`;
      {
        await frPage.getByLabel("New report entry text").fill(reportEntry);
        await frPage.getByLabel("Submit report entry").click();
        await expect(frPage.getByLabel("New report entry text")).toBeEmpty();
        await expect(frPage.getByText(reportEntry)).toBeVisible();
      }
      // strike the entry, verified it's stricken
      {
        await frPage.getByText(reportEntry).hover();
        await frPage.getByRole("button", {name: "Strike"}).click({force: true});
        await expect(frPage.getByText(reportEntry)).toBeHidden();
      }
      // but the entry is shown when the right checkbox is ticked
      {
        await frPage.getByLabel("Show history and stricken").check();
        await expect(frPage.getByText(reportEntry)).toBeVisible();
      }
      // unstrike the entry and see it return to the default view
      {
        await frPage.getByText(reportEntry).hover();
        await frPage.getByRole("button", {name: "Unstrike"}).click({force: true});
        await frPage.getByLabel("Show history and stricken").uncheck();
        await expect(frPage.getByText(reportEntry)).toBeVisible();
      }

      // try searching for the incident by its report text
      {
        await tablePage.getByRole("searchbox").fill(reportEntry);
        await tablePage.getByRole("searchbox").press("Enter");
        await expect(tablePage.getByText(newSummary)).toBeVisible();
        await tablePage.getByRole("searchbox").fill("The wrong text!");
        await tablePage.getByRole("searchbox").press("Enter");
        await expect(tablePage.getByText(newSummary)).toBeHidden();
        await tablePage.getByRole("searchbox").clear();
        await tablePage.getByRole("searchbox").press("Enter");
        await expect(tablePage.getByText(newSummary)).toBeVisible();
      }

      await frPage.close();
      await tablePage.close();
      await ctx.close();
  }
})
