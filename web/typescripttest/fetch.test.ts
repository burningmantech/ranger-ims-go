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
import { jsonResponse, mockFetch, mockXHR, problemResponse } from "./helpers.ts";

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

test("fetchNoThrow refuses a FormData body rather than JSONifying it", async (): Promise<void> => {
    mockFetch(() => jsonResponse({}));

    await expect(ims.fetchNoThrow(url_ping, { body: new FormData() })).rejects.toThrow(/uploadNoThrow/);
});

test("uploadNoThrow posts the form data with a Bearer token and no Content-Type", async (): Promise<void> => {
    ims.setAccessToken("token123");
    ims.setRefreshTokenBy(Date.now() + 60_000);
    const uploads = mockXHR(() => new Response(null, { status: 204 }));
    const body = new FormData();
    body.append("imsAttachment", new Blob(["file contents"]), "photo.jpg");

    const { err } = await ims.uploadNoThrow(url_ping, body, (): void => {});
    expect(err).toBeNull();

    expect(uploads.length).toBe(1);
    expect(uploads[0]!.method).toBe("POST");
    expect(uploads[0]!.url).toBe(url_ping);
    expect(uploads[0]!.body).toBe(body);
    expect(uploads[0]!.headers["Accept"]).toBe("application/json");
    expect(uploads[0]!.headers["Authorization"]).toBe("Bearer token123");
    // XHR sets the multipart Content-Type, with its boundary, itself.
    expect(uploads[0]!.headers["Content-Type"]).toBeUndefined();
});

test("uploadNoThrow reports progress as a percentage, then as pending once the body is sent", async (): Promise<void> => {
    mockXHR(() => new Response(null, { status: 204 }), { progress: [0, 0.5, 0.999, 1] });
    const progress: string[] = [];

    const { err } = await ims.uploadNoThrow(url_ping, new FormData(), (p: string): void => {
        progress.push(p);
    });
    expect(err).toBeNull();
    expect(progress).toEqual(["0%", "50%", "99%", "100%", "…"]);
});

test("uploadNoThrow reports a byte count when the upload size is unknown", async (): Promise<void> => {
    mockXHR(
        () => new Response(null, { status: 204 }),
        { progress: [0.5], lengthComputable: false },
    );
    const progress: string[] = [];

    await ims.uploadNoThrow(url_ping, new FormData(), (p: string): void => {
        progress.push(p);
    });
    // Half of the fake's 1000 byte total.
    expect(progress).toEqual(["0.0 MB", "…"]);
});

test("uploadNoThrow extracts the detail from an application/problem+json error", async (): Promise<void> => {
    mockXHR(() => problemResponse("attachment too large", 413));

    const { err } = await ims.uploadNoThrow(url_ping, new FormData(), (): void => {});
    expect(err).toBe("attachment too large (HTTP 413)");
});

test("uploadNoThrow falls back to the bare status for a non-problem error", async (): Promise<void> => {
    mockXHR(() => new Response("nope", { status: 500, statusText: "Internal Server Error" }));

    const { err } = await ims.uploadNoThrow(url_ping, new FormData(), (): void => {});
    expect(err).toBe("Internal Server Error (500)");
});

test("uploadNoThrow reports a failed request rather than throwing", async (): Promise<void> => {
    mockXHR(() => undefined);

    const { err } = await ims.uploadNoThrow(url_ping, new FormData(), (): void => {});
    expect(err).toBe("Upload failed");
});

test("uploadNoThrow refreshes a stale access token before uploading", async (): Promise<void> => {
    ims.setAccessToken("staleToken");
    ims.setRefreshTokenBy(Date.now() - 1);
    const mock = mockFetch((url, _init) => {
        if (url === url_authRefresh) {
            return jsonResponse({ token: "freshToken", expires_unix_ms: Date.now() + 60_000 });
        }
        return undefined;
    });
    const uploads = mockXHR(() => new Response(null, { status: 204 }));

    const { err } = await ims.uploadNoThrow(url_ping, new FormData(), (): void => {});
    expect(err).toBeNull();

    expect(mock.mock.calls[0]![0]).toBe(url_authRefresh);
    expect(uploads[0]!.headers["Authorization"]).toBe("Bearer freshToken");
});
