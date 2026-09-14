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

func TestCookiesAreSecureExceptForPlainHTTPToLoopback(t *testing.T) {
	t.Parallel()

	// A real deployment's host, over plain HTTP (e.g. behind a TLS-terminating
	// load balancer), still gets a Secure cookie
	req := httptest.NewRequest(http.MethodPost, "http://ims.example.org/ims/api/auth", nil)
	require.True(t, authz.AccessTokenCookie(req, "tok", time.Hour).Secure)
	require.True(t, authz.ExpiredAccessTokenCookie(req).Secure)
	require.True(t, authz.ExpiredLegacyRefreshTokenCookie(req).Secure)

	// So does a real deployment's host with a port
	req = httptest.NewRequest(http.MethodPost, "http://ims.example.org:8080/ims/api/auth", nil)
	require.True(t, authz.AccessTokenCookie(req, "tok", time.Hour).Secure)

	// So does a host that merely starts with "localhost"
	req = httptest.NewRequest(http.MethodPost, "http://localhost.example.org/ims/api/auth", nil)
	require.True(t, authz.AccessTokenCookie(req, "tok", time.Hour).Secure)

	// Local development over plain HTTP doesn't, since WebKit would drop the cookie
	req = httptest.NewRequest(http.MethodPost, "http://localhost:8080/ims/api/auth", nil)
	require.False(t, authz.AccessTokenCookie(req, "tok", time.Hour).Secure)
	require.False(t, authz.ExpiredAccessTokenCookie(req).Secure)
	require.False(t, authz.ExpiredLegacyRefreshTokenCookie(req).Secure)

	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/ims/api/auth", nil)
	require.False(t, authz.AccessTokenCookie(req, "tok", time.Hour).Secure)

	req = httptest.NewRequest(http.MethodPost, "http://[::1]:8080/ims/api/auth", nil)
	require.False(t, authz.AccessTokenCookie(req, "tok", time.Hour).Secure)

	// Loopback over TLS gets a Secure cookie after all
	req = httptest.NewRequest(http.MethodPost, "https://localhost:8443/ims/api/auth", nil)
	require.NotNil(t, req.TLS)
	require.True(t, authz.AccessTokenCookie(req, "tok", time.Hour).Secure)
}
