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

func TestCookiesAreSecureByDefault(t *testing.T) {
	t.Parallel()
	cookies := authz.TokenCookies{}

	// The prefix forces Path=/ and no Domain
	req := httptest.NewRequest(http.MethodPost, "http://ims.example.org/ims/api/auth", nil)
	c := cookies.AccessToken(req, "tok", time.Hour)
	require.True(t, c.Secure)
	require.Equal(t, "__Host-ims_access_token", c.Name)
	require.Equal(t, "/", c.Path)
	require.Empty(t, c.Domain)
	expired := cookies.ExpiredAccessToken(req)
	require.True(t, expired.Secure)
	require.Equal(t, "__Host-ims_access_token", expired.Name)
	require.Equal(t, "/", expired.Path)
	require.True(t, cookies.ExpiredLegacyRefreshToken(req).Secure)

	// Plain HTTP to loopback, whether from a local browser or a reverse proxy
	// that passes along its upstream's address as the Host, changes nothing
	req = httptest.NewRequest(http.MethodPost, "http://localhost:8080/ims/api/auth", nil)
	c = cookies.AccessToken(req, "tok", time.Hour)
	require.True(t, c.Secure)
	require.Equal(t, "__Host-ims_access_token", c.Name)
	require.True(t, cookies.ExpiredLegacyRefreshToken(req).Secure)
}

func TestInsecureCookiesOverPlainHTTP(t *testing.T) {
	t.Parallel()
	cookies := authz.TokenCookies{Insecure: true}

	// Plain HTTP gets a cookie that isn't Secure, and so can't have the prefix
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/ims/api/auth", nil)
	c := cookies.AccessToken(req, "tok", time.Hour)
	require.False(t, c.Secure)
	require.Equal(t, "ims_access_token", c.Name)
	require.Equal(t, "/", c.Path)
	expired := cookies.ExpiredAccessToken(req)
	require.False(t, expired.Secure)
	require.Equal(t, "ims_access_token", expired.Name)
	require.False(t, cookies.ExpiredLegacyRefreshToken(req).Secure)

	// TLS still gets a Secure, prefixed cookie
	req = httptest.NewRequest(http.MethodPost, "https://localhost:8443/ims/api/auth", nil)
	require.NotNil(t, req.TLS)
	c = cookies.AccessToken(req, "tok", time.Hour)
	require.True(t, c.Secure)
	require.Equal(t, "__Host-ims_access_token", c.Name)
	require.True(t, cookies.ExpiredLegacyRefreshToken(req).Secure)
}

func TestAccessTokenFromCookie(t *testing.T) {
	t.Parallel()
	insecure := authz.TokenCookies{Insecure: true}
	prod := authz.TokenCookies{}

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

	// Two cookies by that name mean one is a lookalike, and neither is trusted
	req = httptest.NewRequest(http.MethodGet, "http://ims.example.org/ims/api/events", nil)
	req.AddCookie(requestCookie("__Host-ims_access_token", "planted"))
	req.AddCookie(requestCookie("__Host-ims_access_token", "tok"))
	token, err = prod.AccessTokenFrom(req)
	require.ErrorIs(t, err, authz.ErrMultipleTokenCookies)
	require.Empty(t, token)

	// With insecure cookies, plain HTTP uses the unprefixed cookie
	req = httptest.NewRequest(http.MethodGet, "http://localhost:8080/ims/api/events", nil)
	req.AddCookie(requestCookie("ims_access_token", "tok"))
	token, err = insecure.AccessTokenFrom(req)
	require.NoError(t, err)
	require.Equal(t, "tok", token)
}

// requestCookie is a cookie as a browser sends it, which is only a name and value.
func requestCookie(name, value string) *http.Cookie {
	// #nosec G124 // A request cookie has no Secure, HttpOnly, or SameSite
	return &http.Cookie{Name: name, Value: value}
}
