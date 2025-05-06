import js from "@eslint/js";
import globals from "globals";
import tseslint from "typescript-eslint";
import json from "@eslint/json";
import css from "@eslint/css";
import nounsanitized from "eslint-plugin-no-unsanitized";
import {defineConfig, globalIgnores} from "eslint/config";


export default defineConfig([
    [
        globalIgnores([
            // external files we don't control
            "web/static/ext/",
            // files generated from TS files
            "web/static/*.js",
        ])
    ],
    {
        files: [
            "**/*.{js,mjs,cjs,ts}",
        ],
        plugins: {js},
        extends: ["js/recommended"],
    },
    {
        files: [
            "**/*.{js,mjs,cjs,ts}",
        ],
        languageOptions: {globals: globals.browser},
    },
    ...tseslint.config({
        extends: [...tseslint.configs.recommended],
        rules: {
            "@typescript-eslint/no-unused-vars": [
                "error",
                {
                    argsIgnorePattern: "^_",
                    varsIgnorePattern: "^_"
                }
            ]
        }
    }),
    {
        files: ["**/*.json"],
        ignores: ["**/tsconfig.json"],
        plugins: {json},
        language: "json/json",
        extends: ["json/recommended"],
    },
    {
        files: [
            "**/*.jsonc",
            "**/tsconfig.json",
        ],
        plugins: {json},
        language: "json/jsonc",
        extends: ["json/recommended"],
    },
    {
        files: ["**/*.css"],
        plugins: {css},
        language: "css/css",
        extends: ["css/recommended"],
    },
    nounsanitized.configs.recommended,
    {
        files: ["**/*.{js,mjs,cjs,ts}"],
        plugins: { "no-unsanitized": nounsanitized },
        rules: {
            "no-unsanitized/method": "error",
            "no-unsanitized/property": "error",
        },
    },
]);
