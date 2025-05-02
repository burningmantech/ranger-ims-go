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

package integration

import (
	"encoding/json"
	"github.com/burningmantech/ranger-ims-go/api"
	"github.com/burningmantech/ranger-ims-go/auth"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestPostAuthAPIAuthorization(t *testing.T) {
	s := httptest.NewServer(api.AddToMux(nil, shared.es, shared.cfg, shared.imsDB, shared.userStore))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)

	apisNotAuthenticated := ApiHelper{t: t, serverURL: serverURL, jwt: ""}

	// A user who doesn't exist gets s 401
	statusCode, body, token := apisNotAuthenticated.postAuth(api.PostAuthRequest{
		Identification: "Not a real user",
		Password:       "password123",
	})
	require.Equal(t, http.StatusUnauthorized, statusCode)
	require.Contains(t, body, "bad credentials")
	require.Empty(t, token)

	// A user with the correct password gets logged in and gets a JWT
	statusCode, _, token = apisNotAuthenticated.postAuth(api.PostAuthRequest{
		Identification: userAliceEmail,
		Password:       userAlicePassword,
	})
	require.Equal(t, http.StatusOK, statusCode)
	require.NotEmpty(t, token)

	// That same valid user can also log in by handle
	statusCode, _, token = apisNotAuthenticated.postAuth(api.PostAuthRequest{
		Identification: userAliceHandle,
		Password:       userAlicePassword,
	})
	require.Equal(t, http.StatusOK, statusCode)
	require.NotEmpty(t, token)

	// A valid user with the wrong password gets denied entry
	statusCode, body, token = apisNotAuthenticated.postAuth(api.PostAuthRequest{
		Identification: userAliceHandle,
		Password:       "not my password",
	})
	require.Equal(t, http.StatusUnauthorized, statusCode)
	require.Contains(t, body, "bad credentials")
	require.Empty(t, token)
}

func TestGetAuthAPIAuthorization(t *testing.T) {
	s := httptest.NewServer(api.AddToMux(nil, shared.es, shared.cfg, shared.imsDB, shared.userStore))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)

	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForTestAdminRanger(t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForRealTestUser(t)}
	apisNotAuthenticated := ApiHelper{t: t, serverURL: serverURL, jwt: ""}

	// non-admin user can authenticate
	getAuth, resp := apisNonAdmin.getAuth("")
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, api.GetAuthResponse{
		Authenticated: true,
		User:          userAliceHandle,
		Admin:         false,
	}, getAuth)

	// admin user can authenticate
	getAuth, resp = apisAdmin.getAuth("")
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, api.GetAuthResponse{
		Authenticated: true,
		User:          userAdminHandle,
		Admin:         true,
	}, getAuth)

	// unauthenticated client cannot authenticate
	getAuth, resp = apisNotAuthenticated.getAuth("someNonExistentEvent")
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, api.GetAuthResponse{
		Authenticated: false,
	}, getAuth)
}

func TestGetAuthWithEvent(t *testing.T) {
	s := httptest.NewServer(api.AddToMux(nil, shared.es, shared.cfg, shared.imsDB, shared.userStore))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)

	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForTestAdminRanger(t)}

	// create event and give this user permissions on it
	eventName := "TestGetAuthWithEvent"
	resp := apisAdmin.editEvent(imsjson.EditEventsRequest{
		Add: []string{eventName},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp = apisAdmin.editAccess(imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Readers: []imsjson.AccessRule{imsjson.AccessRule{
				Expression: "person:" + userAdminHandle,
				Validity:   "always",
			}},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	auth, resp := apisAdmin.getAuth(eventName)
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
				AttachFiles:       false,
			},
		},
	}, auth)
}

func TestPostAuthMakesRefreshCookie(t *testing.T) {
	s := httptest.NewServer(api.AddToMux(nil, shared.es, shared.cfg, shared.imsDB, shared.userStore))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)

	apisNotAuthenticated := ApiHelper{t: t, serverURL: serverURL, jwt: ""}

	// A user with the correct password can log in and get refresh and access tokens
	req := api.PostAuthRequest{
		Identification: userAliceEmail,
		Password:       userAlicePassword,
	}
	response := &api.PostAuthResponse{}
	resp := apisNotAuthenticated.imsPost(req, serverURL.JoinPath("/ims/api/auth").String())
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	err = json.Unmarshal(b, &response)
	require.NoError(t, err)

	// check that the returned access token looks good
	jwter := auth.JWTer{SecretKey: shared.cfg.Core.JWTSecret}
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
	code, refreshResp := apisNotAuthenticated.refreshAccessToken(cookie)
	require.Equal(t, http.StatusOK, code)
	// and confirm the new access token's validity
	claims, err = jwter.AuthenticateJWT(refreshResp.Token)
	require.NoError(t, err)
	require.Equal(t, userAliceHandle, claims.RangerHandle())
	// this new token should expire no earlier than the old one
	require.GreaterOrEqual(t, refreshResp.ExpiresUnixMs, response.ExpiresUnixMs)
}
