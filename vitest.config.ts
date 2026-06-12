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

import { defineConfig } from "vitest/config";

export default defineConfig({
    test: {
        environment: "happy-dom",
        include: ["web/typescripttest/**/*.test.ts"],
        setupFiles: ["web/typescripttest/setup.ts"],
        restoreMocks: true,
        unstubGlobals: true,
        coverage: {
            // Report on the real TypeScript sources, not the test helpers.
            include: ["web/typescript/**"],
        },
        environmentOptions: {
            happyDOM: {
                // A secure context, like the real IMS deployments.
                url: "https://localhost/ims/app/",
                settings: {
                    // Fixture pages contain <script src> and <link> tags for
                    // assets that don't exist in tests. Tests import the real
                    // TypeScript modules themselves instead.
                    disableJavaScriptFileLoading: true,
                    disableJavaScriptEvaluation: true,
                    disableCSSFileLoading: true,
                    handleDisabledFileLoadingAsSuccess: true,
                },
            },
        },
    },
});
