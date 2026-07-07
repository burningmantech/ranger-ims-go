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

import {test, expect, Locator, Page} from "@playwright/test";

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

// Events created by the current test, to be deleted again by the afterEach
// hook below. A worker runs one test at a time, so it's fine for a test and
// its cleanup hook to share this module-level list.
const createdEvents: string[] = [];

async function addEvent(page: Page, eventName: string): Promise<void> {
  await eventsPage(page);
  await page.getByPlaceholder("Burn-A-Matic-3000").fill(eventName);
  await page.getByPlaceholder("Burn-A-Matic-3000").press("Enter");

  await expect(eventCard(page, eventName)).toBeVisible();
  createdEvents.push(eventName);
}

// deleteEvent deletes an event and all its data through the admin events
// page's Edit modal. The Delete Event button in that modal is only enabled
// because the dev server runs with IMS_EVENT_DELETION_ENABLED=true.
async function deleteEvent(page: Page, eventName: string): Promise<void> {
  await eventsPage(page);
  // accept the deletion confirm dialog
  autoAcceptDialogs(page);

  const card = eventCard(page, eventName);
  await card.getByRole("button", {name: "Edit"}).click();
  await page.getByRole("button", {name: "Delete Event"}).click();
  await expect(card).toBeHidden();
}

// Delete the events made by each test, so that test runs don't pile up
// events on the server. This runs on failed tests too, and the pages the
// test itself used may already be closed, so it uses a fresh context.
test.afterEach(async ({ browser }): Promise<void> => {
  if (createdEvents.length === 0) {
    return;
  }
  const events = createdEvents.splice(0);
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  await login(page);
  for (const eventName of events) {
    await deleteEvent(page, eventName);
  }
  await ctx.close();
});

function eventCard(page: Page, eventName: string): Locator {
  return page.locator(".event_access").filter({hasText: eventName});
}

// expandEventCard expands the event's rule table, if it's collapsed.
async function expandEventCard(page: Page, eventName: string): Promise<Locator> {
  const card = eventCard(page, eventName);
  const toggle = card.getByRole("button", {name: eventName});
  await expect(toggle).toBeVisible();
  if ((await toggle.getAttribute("aria-expanded")) !== "true") {
    await toggle.click();
  }
  return card;
}

// Adding a rule for an unknown target pops up a confirm dialog; accept any
// such dialogs on this page. Playwright dismisses dialogs by default.
const dialogsAutoAccepted = new WeakSet<Page>();
function autoAcceptDialogs(page: Page): void {
  if (dialogsAutoAccepted.has(page)) {
    return;
  }
  dialogsAutoAccepted.add(page);
  page.on("dialog", (dialog) => void dialog.accept().catch((): void => {}));
}

async function addRule(page: Page, eventName: string, target: string, level: string): Promise<void> {
  await eventsPage(page);
  autoAcceptDialogs(page);

  const card = await expandEventCard(page, eventName);
  await card.getByLabel("Access level for new rule").selectOption(level);
  await card.getByLabel("New rule target").fill(target);
  // Commit with Tab (blur): WebKit doesn't fire "change" on Enter for inputs
  // backed by a datalist.
  await card.getByLabel("New rule target").press("Tab");
  await expect(card.getByText(target)).toBeVisible({timeout: 5000});
}

async function addWriter(page: Page, eventName: string, writer: string): Promise<void> {
  await addRule(page, eventName, writer, "writers");
}

async function addReporter(page: Page, eventName: string, reporter: string): Promise<void> {
  await addRule(page, eventName, reporter, "reporters");
}

async function addVisitWriter(page: Page, eventName: string, writer: string): Promise<void> {
  await addRule(page, eventName, writer, "visit_writers");
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
  test.slow();

  const ctx = await browser.newContext();
  const page = await ctx.newPage()
  await login(page);

  const eventName: string = randomName("event");
  await addEvent(page, eventName);
  await addWriter(page, eventName, "person:SomeGuy");

  const row = eventCard(page, eventName).locator("tr.access_rule").filter({hasText: "person:SomeGuy"});
  await expect(row.getByLabel("Access level")).toHaveValue("writers");
  // it's hard to tell on the client side when this has completed, hence the toPass block below
  await row.getByLabel("Validity").selectOption("On-Site");

  const page2 = await ctx.newPage();
  await login(page2);
  await eventsPage(page2);
  const card2 = eventCard(page2, eventName);
  const row2 = card2.locator("tr.access_rule").filter({hasText: "person:SomeGuy"});
  await expect(async (): Promise<void> => {
    // The unknown person:SomeGuy is an issue, so the card auto-expands and
    // shows an issue count.
    await expect(card2).toBeVisible();
    await expect(card2.locator(".rule_count")).toHaveText("1 rule");
    await expect(card2.locator(".issue_count")).toHaveText("1 issue");
    await expect(row2).toBeVisible();
    await expect(row2.getByLabel("Validity")).toHaveValue("onsite");
    await expect(row2).not.toContainText("Expired");
    await expect(row2).toContainText("Unknown target");
  }).toPass();

  // The date editors are hidden until the rule has dates; disclose them.
  const expirationTime = row2.getByRole("textbox", {name: "Not after"});
  if (!(await expirationTime.isVisible())) {
    await row2.getByRole("button", {name: "Set dates"}).click();
  }
  // On mobile, flatpickr swaps in a native date picker that's harder to
  // drive, so skip date editing there (as in the incidents test).
  const onMobile = await row2.locator(".flatpickr-mobile").first().isVisible();
  if (!onMobile) {
    // Filling the date can race with a redraw of the rule row, which loses
    // the typed value before it's committed; retry until the save shows up
    // as the row's "Expired" chip.
    await expect(async (): Promise<void> => {
      if (await row2.getByRole("button", {name: "Set dates"}).isVisible()) {
        await row2.getByRole("button", {name: "Set dates"}).click();
      }
      await expect(expirationTime).toBeVisible({timeout: 2000});
      await expirationTime.fill("Mon 2025-01-27 @ 11:11");
      // focus anywhere else, so that the expirationTime oninput fires
      await card2.getByLabel("New rule target").focus();
      await expect(row2).toContainText("Expired", {timeout: 3000});
    }).toPass();
    await expect(row2).toContainText("Unknown target");
  }

  // move the rule to a different access level via its dropdown
  await row2.getByLabel("Access level").selectOption("reporters");
  await expect(row2.getByLabel("Access level")).toHaveValue("reporters");
  // the rule keeps its other fields
  await expect(row2.getByLabel("Validity")).toHaveValue("onsite");
  if (!onMobile) {
    await expect(row2).toContainText("Expired");
  }

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
    await incidentPage.getByLabel("State").press("Enter");

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
        const roleField = page.locator("li", {hasText: rangerName}).getByRole("textbox");
        await roleField.fill(`${rangerName} Role`);
        await roleField.press("Tab");
        // The value of the roleField is checked later on in this test
      }

      await addRanger(incidentPage, "Doggy");
      await addRanger(incidentPage, "Runner");
      await addRanger(incidentPage, "Loosy");
      await addRanger(incidentPage, "TheMan");
    }

    // override start time
    let altStartedDatetime = incidentPage.locator("#alt_started_datetime");
    let altStartedDateTimeStr = "Mon 2025-01-27 @ 22:11";
    let ignoreDatetimeCheck = false;

    if (!await altStartedDatetime.isVisible()) {
      // The mobile datetime picker is harder to work with, and we can't just
      // fill the text field. We'll leave this problem for another day for mobile
      // (Mobile Chrome and Mobile Safari).
      if (await incidentPage.locator(".flatpickr-mobile").isVisible()) {
        ignoreDatetimeCheck = true;
      }
    }

    if (!ignoreDatetimeCheck) {
      await expect(altStartedDatetime).toBeVisible();
      // The earlier edits (rangers, location) each trigger a refresh that
      // redraws the started-time field, and one of those can clobber the value
      // typed here before it's committed, making the page save the clobbered
      // value (or nothing). Retry until a save with the intended date (2025,
      // in whatever UTC day the local-time string lands on) is observed.
      await expect(async (): Promise<void> => {
        await altStartedDatetime.clear();
        await altStartedDatetime.fill(altStartedDateTimeStr);
        const responsePromise = incidentPage.waitForResponse(response =>
            response.url().includes(`/ims/api/events/${eventName}/incidents/`)
            && response.request().method() === "POST"
            && (response.request().postData() ?? "").includes('"started":"2025-01-2'),
            {timeout: 3000},
        );
        await altStartedDatetime.press("Tab");
        await responsePromise;
      }).toPass();
    }

    // add location details
    {
      await incidentPage.getByLabel("Location name").click();
      await incidentPage.getByLabel("Location name").fill("Somewhere");
      await incidentPage.getByLabel("Location name").press("Tab");
      await incidentPage.getByLabel("Location address").fill("4:20 & F");
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

    // reload the page, make sure some data loads again
    {
      await incidentPage.reload();
      const runnerRanger = incidentPage.getByLabel("Runner");
      await expect(runnerRanger).toBeVisible();
      const runnerRow = incidentPage.getByRole("listitem").filter({has: runnerRanger}).getByRole("textbox");
      await expect(runnerRow).toHaveValue("Runner Role");
      if (!ignoreDatetimeCheck) {
        await expect(altStartedDatetime).toBeVisible();
        await expect(altStartedDatetime).toHaveValue(altStartedDateTimeStr);
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
        // The save can transiently fail when the dev server is busy, leaving
        // the entry text in place; retry the submit until it's accepted.
        await expect(async (): Promise<void> => {
          await frPage.getByLabel("Submit report entry").click();
          await expect(frPage.getByLabel("New report entry text")).toBeEmpty({timeout: 3000});
        }).toPass();
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

// expandAccordion expands the named accordion section on a Sanctuary Visit
// page, if it's collapsed.
async function expandAccordion(page: Page, name: string): Promise<void> {
  const toggle = page.getByRole("button", {name: name});
  await expect(toggle).toBeVisible();
  if ((await toggle.getAttribute("aria-expanded")) !== "true") {
    await toggle.click();
  }
}

// commitVisitField fills a Sanctuary Visit field and commits it with Tab.
// Each save triggers a redraw of the whole page, and a redraw from a previous
// field's save can clobber the typed value before it's committed; retry until
// a save containing this field's value is observed.
async function commitVisitField(page: Page, field: Locator, jsonField: string, value: string): Promise<void> {
  await expect(async (): Promise<void> => {
    await field.fill(value);
    const responsePromise = page.waitForResponse(response =>
        response.url().includes("/ims/api/events/")
        && response.url().includes("/visits")
        && response.request().method() === "POST"
        && response.ok()
        && (response.request().postData() ?? "").includes(`"${jsonField}":${JSON.stringify(value)}`),
        {timeout: 3000},
    );
    await field.press("Tab");
    await responsePromise;
  }).toPass();
}

test("sanctuary_visits", async ({ page, browser }) => {
  test.slow();

  // make a new event in which our user may (only) write Sanctuary Visits
  await login(page);
  const eventName: string = randomName("event");
  await addEvent(page, eventName);
  await addVisitWriter(page, eventName, "person:" + username);
  await page.close();

  for (let i = 0; i < 2; i++) {
    const ctx = await browser.newContext();
    const tablePage = await ctx.newPage();
    await login(tablePage);
    await tablePage.goto(`http://localhost:8080/ims/app/events/${eventName}/visits`);

    const visitPage = await ctx.newPage();
    await visitPage.goto(`http://localhost:8080/ims/app/events/${eventName}/visits`);
    await visitPage.getByRole("button", {name: "New"}).click();
    await visitPage.waitForURL(`http://localhost:8080/ims/app/events/${eventName}/visits/new`);
    await expect(visitPage.getByLabel("VS #")).toHaveValue("(new)");

    // the first edit persists the visit and gives it a number
    const guestName = randomName("guest");
    await commitVisitField(visitPage, visitPage.getByLabel("Preferred name"), "guest_preferred_name", guestName);
    await expect(visitPage.getByLabel("VS #")).toHaveValue(/^\d+$/);
    const visitNumber = await visitPage.getByLabel("VS #").inputValue();
    // creating the visit rewrites the URL from ".../new" to the visit number
    expect(visitPage.url()).toBe(`http://localhost:8080/ims/app/events/${eventName}/visits/${visitNumber}`);
    await expect(visitPage.locator("#doc-title")).toHaveText(`Current Sanctuary Visit (${guestName})`);

    // check that the BroadcastChannel update to the table page worked
    await expect(tablePage.getByText(guestName)).toBeVisible();

    // fill in the rest of the Basics
    await commitVisitField(visitPage, visitPage.getByLabel("Legal name"), "guest_legal_name", "Legal " + guestName);
    await commitVisitField(visitPage, visitPage.getByLabel("Physical description"), "guest_description", "tall, dusty, faux fur coat");
    await commitVisitField(visitPage, visitPage.getByLabel("Action plan"), "guest_action_plan", "water, rest, then walk home");

    // arrival/departure details live in a collapsed accordion
    await expandAccordion(visitPage, "Arrival/Departure");
    await commitVisitField(visitPage, visitPage.getByLabel("Arrival method"), "arrival_method", "walked in");
    await commitVisitField(visitPage, visitPage.getByLabel("Arrival state"), "arrival_state", "disoriented");
    await commitVisitField(visitPage, visitPage.getByLabel("Reason"), "arrival_reason", "needed a quiet space");
    await commitVisitField(visitPage, visitPage.getByLabel("Items brought"), "arrival_belongings", "backpack, goggles");

    // camp and contact details
    await expandAccordion(visitPage, "Camp and Contacts");
    await commitVisitField(visitPage, visitPage.getByLabel("Camp name"), "guest_camp_name", "Camp Somewhere");
    await commitVisitField(visitPage, visitPage.getByLabel("Camp address"), "guest_camp_address", "4:20 & F");
    await commitVisitField(visitPage, visitPage.getByLabel("Guest camp contact information"), "guest_camp_contacts", "campmate: Dusty");

    // resources; the sitter and bed ID show up in the visits table
    await expandAccordion(visitPage, "Resources");
    await commitVisitField(visitPage, visitPage.getByLabel("Sanctuary sitter"), "resource_sitter", "Sitter McSit");
    await commitVisitField(visitPage, visitPage.getByLabel("Bed ID"), "resource_bed_id", "bed-42");

    // add Rangers to the visit
    {
      async function addVisitRanger(page: Page, rangerName: string): Promise<void> {
        await page.getByLabel("Add Ranger Handle").fill("");
        await page.getByLabel("Add Ranger Handle").press("Tab");
        await page.getByLabel("Add Ranger Handle").fill(rangerName);
        await page.getByLabel("Add Ranger Handle").press("Tab");
        await expect(page.locator("li", {hasText: rangerName})).toBeVisible({timeout: 5000});
        await expect(page.getByLabel("Add Ranger Handle")).toHaveValue("");
      }

      await expandAccordion(visitPage, "Rangers Involved");
      await addVisitRanger(visitPage, "Doggy");
      await addVisitRanger(visitPage, "Runner");

      await commitVisitField(visitPage, visitPage.getByLabel("Ranger role for Doggy"), "role", "Doggy Role");
      // The value of the role field is checked after the reload below

      // Remove a Ranger from the visit. The X button only shows while the
      // row is hovered, and a redraw can replace the row under the cursor
      // (losing the hover state), so retry the hover and click together.
      const runnerLi = visitPage.locator("li", {hasText: "Runner"});
      await expect(async (): Promise<void> => {
        await runnerLi.hover();
        await runnerLi.getByRole("button", {name: "X"}).click({timeout: 2000});
      }).toPass();
      await expect(runnerLi).toBeHidden();
    }

    // add a report entry
    const reportEntry = `This is some text - ${randomName("text")}`;
    {
      await visitPage.getByLabel("New report entry text").fill(reportEntry);
      // The save can transiently fail when the dev server is busy, leaving
      // the entry text in place; retry the submit until it's accepted.
      await expect(async (): Promise<void> => {
        await visitPage.getByLabel("Submit report entry").click();
        await expect(visitPage.getByLabel("New report entry text")).toBeEmpty({timeout: 3000});
      }).toPass();
      await expect(visitPage.getByText(reportEntry)).toBeVisible();
    }
    // strike the entry, verify it's stricken
    {
      // The Strike button only shows while the entry is hovered, and a
      // redraw can replace the entry under the cursor (losing the hover
      // state), so retry the hover and click together.
      await expect(async (): Promise<void> => {
        await visitPage.getByText(reportEntry).hover();
        await visitPage.getByRole("button", {name: "Strike"}).click({timeout: 2000});
      }).toPass();
      await expect(visitPage.getByText(reportEntry)).toBeHidden();
    }
    // but the entry is shown when the right checkbox is ticked
    {
      await visitPage.getByLabel("Show history and stricken").check();
      await expect(visitPage.getByText(reportEntry)).toBeVisible();
    }
    // unstrike the entry and see it return to the default view
    {
      await expect(async (): Promise<void> => {
        await visitPage.getByText(reportEntry).hover();
        await visitPage.getByRole("button", {name: "Unstrike"}).click({timeout: 2000});
      }).toPass();
      await visitPage.getByLabel("Show history and stricken").uncheck();
      await expect(visitPage.getByText(reportEntry)).toBeVisible();
    }

    // reload the page, make sure the data loads again
    {
      await visitPage.reload();
      await expect(visitPage.getByLabel("VS #")).toHaveValue(visitNumber);
      await expect(visitPage.getByLabel("Preferred name")).toHaveValue(guestName);
      await expect(visitPage.getByLabel("Arrival method")).toHaveValue("walked in");
      await expect(visitPage.getByLabel("Camp name")).toHaveValue("Camp Somewhere");
      await expect(visitPage.getByLabel("Sanctuary sitter")).toHaveValue("Sitter McSit");
      await expect(visitPage.getByLabel("Ranger role for Doggy")).toHaveValue("Doggy Role");
    }

    // the sitter and bed ID are shown in the visit's row in the table
    {
      const row = tablePage.locator("#visits_table tbody tr").filter({hasText: guestName});
      await expect(row).toContainText("Sitter McSit");
      await expect(row).toContainText("bed-42");
    }

    // try searching for the visit by guest name
    {
      await tablePage.getByRole("searchbox").fill(guestName);
      await tablePage.getByRole("searchbox").press("Enter");
      await expect(tablePage.getByText(guestName)).toBeVisible();
      await tablePage.getByRole("searchbox").fill("The wrong text!");
      await tablePage.getByRole("searchbox").press("Enter");
      await expect(tablePage.getByText(guestName)).toBeHidden();
      await tablePage.getByRole("searchbox").clear();
      await tablePage.getByRole("searchbox").press("Enter");
      await expect(tablePage.getByText(guestName)).toBeVisible();
    }

    // entering a visit number in the search box jumps to that visit
    {
      await tablePage.getByRole("searchbox").fill(visitNumber);
      await tablePage.getByRole("searchbox").press("Enter");
      await tablePage.waitForURL(`http://localhost:8080/ims/app/events/${eventName}/visits/${visitNumber}`);
      await tablePage.goto(`http://localhost:8080/ims/app/events/${eventName}/visits`);
      await expect(tablePage.getByText(guestName)).toBeVisible();
    }

    // override the arrival time, then set a departure time and see the visit
    // disappear from the default "Current" view of the visits table
    {
      // On mobile, flatpickr swaps in a native date picker that's harder to
      // drive, so skip date editing there (as in the incidents test).
      const onMobile = await visitPage.locator(".flatpickr-mobile").first().isVisible();
      if (!onMobile) {
        // fillDatetime fills a flatpickr-backed datetime field and commits it
        // with Tab, retrying until a save with the intended date is observed
        // (as with the datetime field in the incidents test).
        async function fillDatetime(field: Locator, value: string, jsonField: string): Promise<void> {
          await expect(field).toBeVisible();
          await expect(async (): Promise<void> => {
            await field.clear();
            await field.fill(value);
            const responsePromise = visitPage.waitForResponse(response =>
                response.url().includes(`/ims/api/events/${eventName}/visits/`)
                && response.request().method() === "POST"
                && response.ok()
                && (response.request().postData() ?? "").includes(`"${jsonField}":"2025-01-2`),
                {timeout: 3000},
            );
            // focus anywhere else, so that the field's onchange fires
            await field.press("Tab");
            await responsePromise;
          }).toPass();
        }

        // The departure time may not precede the arrival time, so move the
        // arrival time to the past first.
        await fillDatetime(visitPage.locator("#alt_arrival_time"), "Mon 2025-01-27 @ 10:00", "arrival_time");
        await fillDatetime(visitPage.locator("#alt_departure_time"), "Mon 2025-01-27 @ 11:11", "departure_time");
        await expect(visitPage.locator("#doc-title")).toHaveText(`Past Sanctuary Visit (${guestName})`);

        // the departed visit is hidden from the default "Current" view...
        await expect(tablePage.getByText(guestName)).toBeHidden();
        // ...but is shown under "All Statuses"
        await tablePage.getByRole("button", {name: "Current"}).click();
        await tablePage.getByRole("link", {name: "All Statuses"}).click();
        await expect(tablePage.getByText(guestName)).toBeVisible();
      }
    }

    await visitPage.close();
    await tablePage.close();
    await ctx.close();
  }
})
