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

package authz_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/stretchr/testify/require"
)

func TestDevCookiesAreSecureExceptForPlainHTTPToLoopback(t *testing.T) {
	t.Parallel()
	dev := authz.TokenCookies{Dev: true}

	// A real host, over plain HTTP (e.g. behind a TLS-terminating load
	// balancer), still gets a Secure, prefixed cookie
	req := httptest.NewRequest(http.MethodPost, "http://ims.example.org/ims/api/auth", nil)
	c := dev.AccessToken(req, "tok", time.Hour)
	require.True(t, c.Secure)
	require.Equal(t, "__Host-ims_access_token", c.Name)
	require.True(t, dev.ExpiredAccessToken(req).Secure)
	require.True(t, dev.ExpiredLegacyRefreshToken(req).Secure)

	// So does a real host with a port
	req = httptest.NewRequest(http.MethodPost, "http://ims.example.org:8080/ims/api/auth", nil)
	require.True(t, dev.AccessToken(req, "tok", time.Hour).Secure)

	// So does a host that merely starts with "localhost"
	req = httptest.NewRequest(http.MethodPost, "http://localhost.example.org/ims/api/auth", nil)
	require.True(t, dev.AccessToken(req, "tok", time.Hour).Secure)

	// Plain HTTP to loopback doesn't, since WebKit would drop the cookie, and
	// so it can't have the prefix either
	req = httptest.NewRequest(http.MethodPost, "http://localhost:8080/ims/api/auth", nil)
	c = dev.AccessToken(req, "tok", time.Hour)
	require.False(t, c.Secure)
	require.Equal(t, "ims_access_token", c.Name)
	require.Equal(t, "/", c.Path)
	expired := dev.ExpiredAccessToken(req)
	require.False(t, expired.Secure)
	require.Equal(t, "ims_access_token", expired.Name)
	require.False(t, dev.ExpiredLegacyRefreshToken(req).Secure)

	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/ims/api/auth", nil)
	require.False(t, dev.AccessToken(req, "tok", time.Hour).Secure)

	req = httptest.NewRequest(http.MethodPost, "http://[::1]:8080/ims/api/auth", nil)
	require.False(t, dev.AccessToken(req, "tok", time.Hour).Secure)

	// Loopback over TLS gets a Secure cookie after all
	req = httptest.NewRequest(http.MethodPost, "https://localhost:8443/ims/api/auth", nil)
	require.NotNil(t, req.TLS)
	require.True(t, dev.AccessToken(req, "tok", time.Hour).Secure)
}

func TestNonDevCookiesAreAlwaysSecure(t *testing.T) {
	t.Parallel()
	prod := authz.TokenCookies{Dev: false}

	// The prefix forces Path=/ and no Domain
	req := httptest.NewRequest(http.MethodPost, "http://ims.example.org/ims/api/auth", nil)
	c := prod.AccessToken(req, "tok", time.Hour)
	require.True(t, c.Secure)
	require.Equal(t, "__Host-ims_access_token", c.Name)
	require.Equal(t, "/", c.Path)
	require.Empty(t, c.Domain)
	expired := prod.ExpiredAccessToken(req)
	require.True(t, expired.Secure)
	require.Equal(t, "__Host-ims_access_token", expired.Name)
	require.Equal(t, "/", expired.Path)
	require.True(t, prod.ExpiredLegacyRefreshToken(req).Secure)

	// A reverse proxy that passes along its upstream's loopback address as the
	// Host, as nginx does by default, doesn't make the cookie any weaker
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/ims/api/auth", nil)
	c = prod.AccessToken(req, "tok", time.Hour)
	require.True(t, c.Secure)
	require.Equal(t, "__Host-ims_access_token", c.Name)
	require.True(t, prod.ExpiredLegacyRefreshToken(req).Secure)

	req = httptest.NewRequest(http.MethodPost, "http://localhost:8080/ims/api/auth", nil)
	require.True(t, prod.AccessToken(req, "tok", time.Hour).Secure)

	req = httptest.NewRequest(http.MethodPost, "http://[::1]:8080/ims/api/auth", nil)
	require.True(t, prod.AccessToken(req, "tok", time.Hour).Secure)
}

func TestAccessTokenFromCookie(t *testing.T) {
	t.Parallel()
	dev := authz.TokenCookies{Dev: true}
	prod := authz.TokenCookies{Dev: false}

	// No cookie, no token
	req := httptest.NewRequest(http.MethodGet, "http://ims.example.org/ims/api/events", nil)
	token, err := prod.AccessTokenFrom(req)
	require.NoError(t, err)
	require.Empty(t, token)

	// The prefixed cookie carries the token
	req = httptest.NewRequest(http.MethodGet, "http://ims.example.org/ims/api/events", nil)
	req.AddCookie(requestCookie("__Host-ims_access_token", "tok"))
	token, err = prod.AccessTokenFrom(req)
	require.NoError(t, err)
	require.Equal(t, "tok", token)

	// An unprefixed cookie is ignored, since any subdomain could have set it
	req = httptest.NewRequest(http.MethodGet, "http://ims.example.org/ims/api/events", nil)
	req.AddCookie(requestCookie("ims_access_token", "planted"))
	token, err = prod.AccessTokenFrom(req)
	require.NoError(t, err)
	require.Empty(t, token)

	// That includes when a proxy has made the Host look like loopback
	req = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080/ims/api/events", nil)
	req.AddCookie(requestCookie("ims_access_token", "planted"))
	token, err = prod.AccessTokenFrom(req)
	require.NoError(t, err)
	require.Empty(t, token)

	// Two cookies by that name mean one is a lookalike, and neither is trusted
	req = httptest.NewRequest(http.MethodGet, "http://ims.example.org/ims/api/events", nil)
	req.AddCookie(requestCookie("__Host-ims_access_token", "planted"))
	req.AddCookie(requestCookie("__Host-ims_access_token", "tok"))
	token, err = prod.AccessTokenFrom(req)
	require.ErrorIs(t, err, authz.ErrMultipleTokenCookies)
	require.Empty(t, token)

	// A dev deployment on plain HTTP to loopback uses the unprefixed cookie
	req = httptest.NewRequest(http.MethodGet, "http://localhost:8080/ims/api/events", nil)
	req.AddCookie(requestCookie("ims_access_token", "tok"))
	token, err = dev.AccessTokenFrom(req)
	require.NoError(t, err)
	require.Equal(t, "tok", token)
}

// requestCookie is a cookie as a browser sends it, which is only a name and value.
func requestCookie(name, value string) *http.Cookie {
	// #nosec G124 // A request cookie has no Secure, HttpOnly, or SameSite
	return &http.Cookie{Name: name, Value: value}
}
