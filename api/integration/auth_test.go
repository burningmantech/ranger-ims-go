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

package integration_test

import (
	"encoding/json"
	"github.com/burningmantech/ranger-ims-go/api"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestPostAuthAPIAuthorization(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisNotAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}

	// A user who doesn't exist gets s 401
	statusCode, body, token := apisNotAuthenticated.postAuth(ctx, api.PostAuthRequest{
		Identification: "Not a real user",
		Password:       "password123",
	})
	require.Equal(t, http.StatusUnauthorized, statusCode)
	require.Contains(t, body, "bad credentials")
	require.Empty(t, token)

	// A user with the correct password gets logged in and gets a JWT
	statusCode, _, token = apisNotAuthenticated.postAuth(ctx, api.PostAuthRequest{
		Identification: userAliceEmail,
		Password:       userAlicePassword,
	})
	require.Equal(t, http.StatusOK, statusCode)
	require.NotEmpty(t, token)

	// That same valid user can also log in by handle
	statusCode, _, token = apisNotAuthenticated.postAuth(ctx, api.PostAuthRequest{
		Identification: userAliceHandle,
		Password:       userAlicePassword,
	})
	require.Equal(t, http.StatusOK, statusCode)
	require.NotEmpty(t, token)

	// A valid user with the wrong password gets denied entry
	statusCode, body, token = apisNotAuthenticated.postAuth(ctx, api.PostAuthRequest{
		Identification: userAliceHandle,
		Password:       "not my password",
	})
	require.Equal(t, http.StatusUnauthorized, statusCode)
	require.Contains(t, body, "bad credentials")
	require.Empty(t, token)
}

func TestGetAuthAPIAuthorization(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	apisNotAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}

	// non-admin user can authenticate
	getAuth, resp := apisNonAdmin.getAuth(ctx, "")
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, api.GetAuthResponse{
		Authenticated: true,
		User:          userAliceHandle,
		Admin:         false,
	}, getAuth)
	require.NoError(t, resp.Body.Close())

	// admin user can authenticate
	getAuth, resp = apisAdmin.getAuth(ctx, "")
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, api.GetAuthResponse{
		Authenticated: true,
		User:          userAdminHandle,
		Admin:         true,
	}, getAuth)
	require.NoError(t, resp.Body.Close())

	// unauthenticated client cannot authenticate
	getAuth, resp = apisNotAuthenticated.getAuth(ctx, "someNonExistentEvent")
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, api.GetAuthResponse{
		Authenticated: false,
	}, getAuth)
	require.NoError(t, resp.Body.Close())
}

func TestGetAuthWithEvent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	// create event and give this user permissions on it
	eventName := rand.NonCryptoText()
	resp := apisAdmin.editEvent(ctx, imsjson.EditEventsRequest{
		Add: []string{eventName},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Readers: []imsjson.AccessRule{{
				Expression: "person:" + userAdminHandle,
				Validity:   "always",
			}},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	authResp, resp := apisAdmin.getAuth(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, api.GetAuthResponse{
		Authenticated: true,
		User:          userAdminHandle,
		Admin:         true,
		EventAccess: map[string]api.AccessForEvent{
			eventName: {
				ReadIncidents:     true,
				WriteIncidents:    false,
				WriteFieldReports: false,
				AttachFiles:       true,
			},
		},
	}, authResp)
	require.NoError(t, resp.Body.Close())
}

func TestGetAuthWithBadEventNames(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	// non-existent event case
	gar, httpResp := apisAdmin.getAuth(ctx, "ThisEventDoesNotExist")
	assert.Equal(t, http.StatusOK, httpResp.StatusCode)
	require.NoError(t, httpResp.Body.Close())
	assert.Contains(t, gar.EventAccess, "ThisEventDoesNotExist")
	assert.Equal(t, api.AccessForEvent{
		ReadIncidents:     false,
		WriteIncidents:    false,
		WriteFieldReports: false,
		AttachFiles:       false,
	}, gar.EventAccess["ThisEventDoesNotExist"])

	// bad event name (has spaces)
	gar, httpResp = apisAdmin.getAuth(ctx, "This event name is invalid")
	assert.Equal(t, http.StatusBadRequest, httpResp.StatusCode)
	require.NoError(t, httpResp.Body.Close())
	assert.Empty(t, gar.EventAccess)
}

func TestPostAuthMakesRefreshCookie(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisNotAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}

	// A user with the correct password can log in and get refresh and access tokens
	req := api.PostAuthRequest{
		Identification: userAliceEmail,
		Password:       userAlicePassword,
	}
	response := &api.PostAuthResponse{}
	resp := apisNotAuthenticated.imsPost(ctx, req, shared.serverURL.JoinPath("/ims/api/auth").String())
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	err = json.Unmarshal(b, &response)
	require.NoError(t, err)

	// check that the returned access token looks good
	jwter := authz.JWTer{SecretKey: shared.cfg.Core.JWTSecret}
	claims, err := jwter.AuthenticateJWT(response.Token)
	require.NoError(t, err)
	require.Equal(t, userAliceHandle, claims.RangerHandle())
	require.Greater(t, response.ExpiresUnixMs, time.Now().UnixMilli())

	// check that the refresh token was shipped over by cookie
	cookie, err := http.ParseSetCookie(resp.Header.Get("Set-Cookie"))
	require.NoError(t, err)
	require.True(t, cookie.HttpOnly)
	require.True(t, cookie.Secure)
	// and that it's valid
	claims, err = jwter.AuthenticateRefreshToken(cookie.Value)
	require.NoError(t, err)
	require.Equal(t, userAliceHandle, claims.RangerHandle())

	// now use the refresh token to get a fresh access token
	code, refreshResp := apisNotAuthenticated.refreshAccessToken(ctx, cookie)
	require.Equal(t, http.StatusOK, code)
	// and confirm the new access token's validity
	claims, err = jwter.AuthenticateJWT(refreshResp.Token)
	require.NoError(t, err)
	require.Equal(t, userAliceHandle, claims.RangerHandle())
	// this new token should expire no earlier than the old one
	require.GreaterOrEqual(t, refreshResp.ExpiresUnixMs, response.ExpiresUnixMs)
}
