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

package authz

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// AccessTokenCookieName is the cookie through which the web client carries its
// access token. Non-browser clients send the same token in an Authorization
// header instead.
//
// Ideally we'd use a cookie prefix, but "__Host-" would require Path=/, and
// either prefix would make local development with Chrome more difficult :(.
//
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/Cookies#cookie_prefixes
// https://issues.chromium.org/issues/40202941
const AccessTokenCookieName = "ims_access_token"

// accessTokenCookiePath limits the cookie to the API. The web app's pages are
// static, and learn who's logged in by calling the API.
const accessTokenCookiePath = "/ims/api"

// legacyRefreshTokenCookieName is the cookie that held refresh tokens, back when
// IMS had them. It's only still named so that the server can expire it.
const legacyRefreshTokenCookieName = "refresh_token"

// AccessTokenCookie makes the cookie that carries a web client's access token,
// in response to req.
func AccessTokenCookie(req *http.Request, token string, lifetime time.Duration) *http.Cookie {
	// #nosec G124 // Secure is only off for loopback; see secureCookies
	return &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    token,
		Path:     accessTokenCookiePath,
		MaxAge:   int(lifetime / time.Second),
		HttpOnly: true,
		Secure:   secureCookies(req),
		// The cookie is only needed by the API calls a page makes on itself, and
		// never on a navigation from another site, so strict costs nothing.
		SameSite: http.SameSiteStrictMode,
	}
}

// ExpiredAccessTokenCookie makes a cookie that removes the access token cookie.
func ExpiredAccessTokenCookie(req *http.Request) *http.Cookie {
	c := AccessTokenCookie(req, "", 0) // #nosec G124 // See secureCookies
	c.MaxAge = -1
	return c
}

// ExpiredLegacyRefreshTokenCookie makes a cookie that removes the refresh token
// cookie set by older versions of IMS, which browsers would otherwise keep
// sending until it expired on its own.
func ExpiredLegacyRefreshTokenCookie(req *http.Request) *http.Cookie {
	// #nosec G124 // Secure is only off for loopback; see secureCookies
	return &http.Cookie{
		Name:     legacyRefreshTokenCookieName,
		MaxAge:   -1,
		Path:     "/",
		HttpOnly: true,
		Secure:   secureCookies(req),
		SameSite: http.SameSiteStrictMode,
	}
}

// secureCookies reports whether cookies set in response to req should be
// Secure, which is always, except for plain HTTP to a loopback host. WebKit
// won't store a Secure cookie sent over http://localhost, so local development
// in Safari (and Playwright's WebKit) couldn't log in at all. That exception
// can't weaken a real deployment: no browser sends a loopback Host to one, and
// a cookie set for localhost is never sent anywhere else.
func secureCookies(req *http.Request) bool {
	if req.TLS != nil {
		return true
	}
	host, _, err := net.SplitHostPort(req.Host)
	if err != nil {
		host = req.Host
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	addr, err := netip.ParseAddr(host)
	return err != nil || !addr.IsLoopback()
}
