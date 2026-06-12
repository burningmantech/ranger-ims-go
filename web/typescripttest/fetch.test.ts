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

import { expect, test } from "vitest";
import * as ims from "../typescript/ims.ts";
import { jsonResponse, mockFetch, problemResponse } from "./helpers.ts";

function requestHeaders(init?: RequestInit): Headers {
    return new Headers(init?.headers);
}

test("fetchNoThrow parses an application/json response", async (): Promise<void> => {
    const mock = mockFetch((url, _init) => {
        if (url === url_ping) {
            return jsonResponse({ hello: "world" });
        }
        return undefined;
    });

    const { resp, json, err } = await ims.fetchNoThrow<{ hello: string }>(url_ping, null);
    expect(err).toBeNull();
    expect(resp!.status).toBe(200);
    expect(json).toEqual({ hello: "world" });

    const headers = requestHeaders(mock.mock.calls[0]![1]);
    expect(headers.get("Accept")).toBe("application/json");
    expect(headers.get("Authorization")).toBeNull();
});

test("fetchNoThrow defaults to POST and JSON content type when a body is provided", async (): Promise<void> => {
    const mock = mockFetch(() => jsonResponse({}));

    await ims.fetchNoThrow(url_ping, { body: JSON.stringify({ a: 1 }) });

    const init = mock.mock.calls[0]![1]!;
    expect(init.method).toBe("POST");
    expect(requestHeaders(init).get("Content-Type")).toBe("application/json");
});

test("fetchNoThrow sends a Bearer token from localStorage", async (): Promise<void> => {
    ims.setAccessToken("token123");
    ims.setRefreshTokenBy(Date.now() + 60_000);
    const mock = mockFetch(() => jsonResponse({}));

    await ims.fetchNoThrow(url_ping, null);

    const headers = requestHeaders(mock.mock.calls[0]![1]);
    expect(headers.get("Authorization")).toBe("Bearer token123");
});

test("fetchNoThrow extracts the detail from an application/problem+json error", async (): Promise<void> => {
    mockFetch(() => problemResponse("event not found", 404));

    const { json, err } = await ims.fetchNoThrow(url_ping, null);
    expect(err).toBe("event not found (HTTP 404)");
    expect(json).toBeNull();
});

test("fetchNoThrow reports a thrown fetch error rather than throwing", async (): Promise<void> => {
    mockFetch(() => undefined);

    const { resp, json, err } = await ims.fetchNoThrow(url_ping, null);
    expect(resp).toBeNull();
    expect(json).toBeNull();
    expect(err).toContain("no mocked fetch route");
});

test("fetchNoThrow refreshes a stale access token before the real request", async (): Promise<void> => {
    ims.setAccessToken("staleToken");
    ims.setRefreshTokenBy(Date.now() - 1);
    const mock = mockFetch((url, _init) => {
        if (url === url_authRefresh) {
            return jsonResponse({ token: "freshToken", expires_unix_ms: Date.now() + 60_000 });
        }
        if (url === url_ping) {
            return jsonResponse({});
        }
        return undefined;
    });

    const { err } = await ims.fetchNoThrow(url_ping, null);
    expect(err).toBeNull();

    expect(mock.mock.calls[0]![0]).toBe(url_authRefresh);
    expect(mock.mock.calls[1]![0]).toBe(url_ping);
    expect(localStorage.getItem("access_token")).toBe("freshToken");
    const headers = requestHeaders(mock.mock.calls[1]![1]);
    expect(headers.get("Authorization")).toBe("Bearer freshToken");
});

test("fetchNoThrow clears stored credentials when the token refresh fails", async (): Promise<void> => {
    ims.setAccessToken("staleToken");
    ims.setRefreshTokenBy(Date.now() - 1);
    const mock = mockFetch((url, _init) => {
        if (url === url_authRefresh) {
            return problemResponse("refresh token expired", 401);
        }
        if (url === url_ping) {
            return jsonResponse({});
        }
        return undefined;
    });

    const { err } = await ims.fetchNoThrow(url_ping, null);
    expect(err).toBeNull();

    expect(localStorage.getItem("access_token")).toBeNull();
    expect(localStorage.getItem("access_token_refresh_after")).toBeNull();
    // The real request goes out unauthenticated.
    const headers = requestHeaders(mock.mock.calls[1]![1]);
    expect(headers.get("Authorization")).toBeNull();
});
