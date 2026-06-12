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

// Vitest setup file, run once before each test file (see vitest.config.ts).

import { readFileSync } from "node:fs";
import { join } from "node:path";
import process from "node:process";
import { beforeEach } from "vitest";
import { MockEventSource, MockFlatpickr } from "./helpers.ts";

// In production, urls.js is loaded as a classic (non-module) script, so its
// top-level "const url_*" declarations are globals that the page modules
// reference as bare identifiers. Extract them from the TypeScript source and
// install them as actual globals. (Vitest runs with the repo root as cwd.)
const urlsSource = readFileSync(join(process.cwd(), "web", "typescript", "urls.ts"), "utf8");
for (const match of urlsSource.matchAll(/^const (url_\w+)\s*=\s*"([^"]*)"/gm)) {
    (globalThis as Record<string, any>)[match[1]!] = match[2]!;
}

Object.assign(globalThis, {
    // Bootstrap and DataTables are loaded as classic scripts in head.templ.
    // Stub the small surface that ims.ts touches.
    bootstrap: {
        Modal: class {
            show(): void {}
            hide(): void {}
            toggle(): void {}
        },
    },
    DataTable: {
        render: {
            text: () => ({
                display: (s: string): string => s,
            }),
        },
    },
    // happy-dom doesn't implement EventSource.
    EventSource: MockEventSource,
    // flatpickr is loaded as a classic script in head.templ.
    flatpickr: (selector: string | Node, opts: ConstructorParameters<typeof MockFlatpickr>[1]): MockFlatpickr =>
        new MockFlatpickr(selector, opts),
});

beforeEach((): void => {
    localStorage.clear();
    sessionStorage.clear();
    MockEventSource.instances.length = 0;
    MockFlatpickr.instances.length = 0;
});
