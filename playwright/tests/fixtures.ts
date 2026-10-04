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

// Shared fixtures for the Playwright suites. Each worker logs in once through
// the API, and every page a test opens starts out with that session's cookie,
// so tests needn't click through the login page. Setup and cleanup that
// aren't what a test is about go through the API too.

import {APIRequestContext, APIResponse, expect, test as base} from "@playwright/test";

export {expect};

export const baseURL = "http://localhost:8080";
export const username = "Hardware";

export function randomName(prefix: string): string {
  return `${prefix}-${crypto.randomUUID()}`;
}

type StorageState = Awaited<ReturnType<APIRequestContext["storageState"]>>;

export type AccessMode = "readers" | "writers" | "reporters" | "visit_writers";

async function expectOK(respPromise: Promise<APIResponse>): Promise<void> {
  const resp = await respPromise;
  expect(resp.ok(), `${resp.url()}: ${resp.status()} ${await resp.text()}`).toBeTruthy();
}

// TestEvents creates events for a test and deletes them again afterward,
// whether or not the test passed, so that test runs don't pile up events on
// the server.
export class TestEvents {
  private readonly names: string[] = [];

  constructor(private readonly api: APIRequestContext) {}

  // create makes a new event in which target has the given access.
  async create(mode: AccessMode, target: string): Promise<string> {
    const name = randomName("event");
    await expectOK(this.api.post("/ims/api/events", {data: {id: 0, name: name}}));
    this.names.push(name);
    await expectOK(this.api.post("/ims/api/access", {
      data: {[name]: {[mode]: [{expression: target, validity: "always"}]}},
    }));
    return name;
  }

  // track marks an event the test created some other way for cleanup.
  track(name: string): void {
    this.names.push(name);
  }

  async deleteAll(): Promise<void> {
    for (const name of this.names.splice(0)) {
      const resp = await this.api.delete(`/ims/api/events/${encodeURIComponent(name)}`);
      // The test may have deleted the event itself.
      expect(resp.ok() || resp.status() === 404, `deleting ${name}: ${resp.status()} ${await resp.text()}`).toBeTruthy();
    }
  }
}

export const test = base.extend<{events: TestEvents}, {api: APIRequestContext; authState: StorageState}>({
  api: [async ({playwright}, use): Promise<void> => {
    const api = await playwright.request.newContext({baseURL: baseURL});
    await expectOK(api.post("/ims/api/auth", {data: {identification: username, password: username}}));
    await use(api);
    await api.dispose();
  }, {scope: "worker"}],

  // For tests that open their own browser contexts, which don't pick up the
  // storageState fixture on their own.
  authState: [async ({api}, use): Promise<void> => {
    await use(await api.storageState());
  }, {scope: "worker"}],

  storageState: async ({authState}, use): Promise<void> => {
    await use(authState);
  },

  events: async ({api}, use): Promise<void> => {
    const events = new TestEvents(api);
    await use(events);
    await events.deleteAll();
  },
});
