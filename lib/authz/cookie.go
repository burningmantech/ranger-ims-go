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
	"errors"
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
// Browsers only accept a "__Host-" cookie that's Secure, has Path=/, and has no
// Domain, which means it can only have been set by this exact host. Without the
// prefix, a page on a sibling subdomain could plant a cookie carrying its own
// valid token, and silently sign a Ranger in as someone else.
//
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/Cookies#cookie_prefixes
const AccessTokenCookieName = "__Host-ims_access_token" // #nosec G101 // A name, not a credential

// LoopbackAccessTokenCookieName is the access token cookie's name for plain
// HTTP to a loopback host in a dev deployment. That cookie can't be Secure (see
// TokenCookies.secure), so browsers wouldn't accept the prefix.
//
// https://issues.chromium.org/issues/40202941
const LoopbackAccessTokenCookieName = "ims_access_token"

// ErrMultipleTokenCookies means a request carried more than one access token
// cookie. IMS never sets more than one, so something else set the others.
var ErrMultipleTokenCookies = errors.New("multiple access token cookies")

// legacyRefreshTokenCookieName is the cookie that held refresh tokens, back when
// IMS had them. It's only still named so that the server can expire it.
const legacyRefreshTokenCookieName = "refresh_token"

// TokenCookies makes and reads the cookies that carry a web client's access token.
type TokenCookies struct {
	// Dev is whether this is a dev deployment, the only kind that may use a
	// cookie that isn't Secure.
	Dev bool
}

// AccessToken makes the cookie that carries a web client's access token, in
// response to req.
func (c TokenCookies) AccessToken(req *http.Request, token string, lifetime time.Duration) *http.Cookie {
	// #nosec G124 // Secure is only off for dev loopback; see secure
	return &http.Cookie{
		Name:     c.accessTokenName(req),
		Value:    token,
		Path:     "/",
		MaxAge:   int(lifetime / time.Second),
		HttpOnly: true,
		Secure:   c.secure(req),
		// The cookie is only needed by the API calls a page makes on itself, and
		// never on a navigation from another site, so strict costs nothing.
		SameSite: http.SameSiteStrictMode,
	}
}

// ExpiredAccessToken makes a cookie that removes the access token cookie.
func (c TokenCookies) ExpiredAccessToken(req *http.Request) *http.Cookie {
	cookie := c.AccessToken(req, "", 0) // #nosec G124 // See secure
	cookie.MaxAge = -1
	return cookie
}

// ExpiredLegacyRefreshToken makes a cookie that removes the refresh token cookie
// set by older versions of IMS, which browsers would otherwise keep sending
// until it expired on its own.
func (c TokenCookies) ExpiredLegacyRefreshToken(req *http.Request) *http.Cookie {
	// #nosec G124 // Secure is only off for dev loopback; see secure
	return &http.Cookie{
		Name:     legacyRefreshTokenCookieName,
		MaxAge:   -1,
		Path:     "/",
		HttpOnly: true,
		Secure:   c.secure(req),
		SameSite: http.SameSiteStrictMode,
	}
}

// AccessTokenFrom returns the token in req's access token cookie, or "" if
// there isn't one.
func (c TokenCookies) AccessTokenFrom(req *http.Request) (string, error) {
	cookies := req.CookiesNamed(c.accessTokenName(req))
	switch len(cookies) {
	case 0:
		return "", nil
	case 1:
		return cookies[0].Value, nil
	default:
		// A prefixed cookie is unique per host, but some browsers could be
		// tricked into sending a lookalike: a nameless cookie, set from a sibling
		// subdomain, whose value begins "__Host-ims_access_token=".
		return "", ErrMultipleTokenCookies
	}
}

func (c TokenCookies) accessTokenName(req *http.Request) string {
	if c.secure(req) {
		return AccessTokenCookieName
	}
	return LoopbackAccessTokenCookieName
}

// secure reports whether cookies set in response to req should be Secure,
// which is always, except for plain HTTP to a loopback host in a dev
// deployment. WebKit won't store a Secure cookie sent over http://localhost, so
// local development in Safari (and Playwright's WebKit) couldn't log in at all.
//
// Other deployments ignore the Host header, since a reverse proxy commonly
// rewrites it to the loopback address IMS listens on (e.g. nginx's proxy_pass
// without "proxy_set_header Host $host"), while browsers still reach the proxy
// over HTTPS.
func (c TokenCookies) secure(req *http.Request) bool {
	if !c.Dev || req.TLS != nil {
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
