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
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/burningmantech/ranger-ims-go/api"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostAuthAPIAuthorization(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)

	apisNotAuthenticated := srv.unauthed()

	// A user who doesn't exist gets s 401
	statusCode, body, token := apisNotAuthenticated.postAuth(ctx, api.PostAuthRequest{
		Identification: "Not a real user",
		Password:       "password123",
	})
	require.Equal(t, http.StatusUnauthorized, statusCode)
	require.Contains(t, body, "bad credentials")
	require.Empty(t, token)

	// A user with the correct password gets logged in and gets a JWT
	statusCode, _, token = apisNotAuthenticated.postAuth(ctx,
		api.PostAuthRequest{
			Identification: userAliceEmail,
			Password:       userAlicePassword,
		},
	)
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
	srv := newServer(t)

	apisAdmin := srv.admin(ctx)
	apisNonAdmin := srv.alice(ctx)
	apisNotAuthenticated := srv.unauthed()

	// non-admin user can authenticate
	getAuth, resp := apisNonAdmin.getAuth(ctx, "")
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, api.GetAuthResponse{
		Authenticated: true,
		User:          userAliceHandle,
		Admin:         false,
		// The test server is configured with a Burning Man API key. This says
		// nothing about whether this user may use it.
		PlacesImportAllowed: true,
	}, getAuth)
	require.NoError(t, resp.Body.Close())

	// admin user can authenticate
	getAuth, resp = apisAdmin.getAuth(ctx, "")
	require.NotNil(t, resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, api.GetAuthResponse{
		Authenticated:       true,
		User:                userAdminHandle,
		Admin:               true,
		PlacesImportAllowed: true,
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
	srv := newServer(t)

	apisAdmin := srv.admin(ctx)

	// create event and give this user permissions on it
	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{
		Name: &eventName,
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
	eventID := authResp.EventAccess[eventName].EventID
	require.NotZero(t, eventID)
	require.Equal(t, api.GetAuthResponse{
		Authenticated:       true,
		User:                userAdminHandle,
		Admin:               true,
		PlacesImportAllowed: true,
		EventAccess: map[string]api.AccessForEvent{
			eventName: {
				EventID:           eventID,
				ReadIncidents:     true,
				WriteIncidents:    false,
				WriteFieldReports: false,
				ReadVisits:        true,
				WriteVisits:       false,
				AttachFiles:       true,
			},
		},
	}, authResp)
	require.NoError(t, resp.Body.Close())
}

func TestGetAuthWithBadEventNames(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)

	apisAdmin := srv.admin(ctx)

	// non-existent event case
	gar, httpResp := apisAdmin.getAuth(ctx, "ThisEventDoesNotExist")
	assert.Equal(t, http.StatusOK, httpResp.StatusCode)
	require.NoError(t, httpResp.Body.Close())
	assert.Contains(t, gar.EventAccess, "ThisEventDoesNotExist")
	assert.Equal(t, api.AccessForEvent{
		ReadIncidents:     false,
		WriteIncidents:    false,
		WriteFieldReports: false,
		ReadVisits:        false,
		WriteVisits:       false,
		AttachFiles:       false,
	}, gar.EventAccess["ThisEventDoesNotExist"])

	// bad event name (has spaces)
	gar, httpResp = apisAdmin.getAuth(ctx, "This event name is invalid")
	assert.Equal(t, http.StatusBadRequest, httpResp.StatusCode)
	require.NoError(t, httpResp.Body.Close())
	assert.Empty(t, gar.EventAccess)
}

// sendRaw sends a request built by the test itself, for when the request needs
// a cookie or browser headers that the ApiHelper methods don't send.
func sendRaw(t *testing.T, srv testServer, req *http.Request) *http.Response {
	t.Helper()
	// #nosec G704 // SSRF via taint analysis. We control the URLs.
	resp, err := srv.client.Do(req)
	require.NoError(t, err)
	return resp
}

// tokenCookie is the cookie a browser would send back after logging in. Only the
// name and value go out with a request, so the other attributes don't matter.
func tokenCookie(token string) *http.Cookie {
	// #nosec G124 // A request cookie has no Secure, HttpOnly, or SameSite
	return &http.Cookie{Name: authz.AccessTokenCookieName, Value: token}
}

// authCookie finds the access token cookie among a response's Set-Cookies.
func authCookie(resp *http.Response) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == authz.AccessTokenCookieName {
			return c
		}
	}
	return nil
}

func TestPostAuthSetsCookie(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)

	// Log in the way the web client does, without asking for the token in the body
	// #nosec G117 // Test credentials
	loginBody, err := json.Marshal(api.PostAuthRequest{
		Identification: userAliceEmail,
		Password:       userAlicePassword,
	})
	require.NoError(t, err)
	loginReq, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.url.JoinPath("/ims/api/auth").String(), bytes.NewReader(loginBody))
	require.NoError(t, err)
	loginReq.Header.Set("Content-Type", "application/json")
	resp := sendRaw(t, srv, loginReq)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// The body gives the expiration, but not the token itself
	response := api.PostAuthResponse{}
	require.NoError(t, json.Unmarshal(b, &response))
	require.Empty(t, response.Token)
	require.NotContains(t, string(b), `"token"`)
	require.InDelta(t, time.Now().Add(shared.cfg.Core.TokenLifetime).UnixMilli(), response.ExpiresUnixMs, float64(time.Minute.Milliseconds()))

	// The token comes in a locked-down cookie, scoped to the API
	cookie := authCookie(resp)
	require.NotNil(t, cookie)
	require.True(t, cookie.HttpOnly)
	require.True(t, cookie.Secure)
	require.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
	require.Equal(t, "/ims/api", cookie.Path)
	require.Equal(t, int(shared.cfg.Core.TokenLifetime/time.Second), cookie.MaxAge)
	jwter := authz.JWTer{SecretKey: shared.cfg.Core.JWTSecret}
	claims, err := jwter.AuthenticateJWT(cookie.Value)
	require.NoError(t, err)
	require.Equal(t, userAliceHandle, claims.RangerHandle())

	// Any refresh token cookie left over from older versions of IMS gets expired
	var legacy *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "refresh_token" {
			legacy = c
		}
	}
	require.NotNil(t, legacy)
	require.Negative(t, legacy.MaxAge)

	// The cookie alone authenticates API requests
	getAuthReq, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.url.JoinPath("/ims/api/auth").String(), nil)
	require.NoError(t, err)
	getAuthReq.AddCookie(cookie)
	resp = sendRaw(t, srv, getAuthReq)
	b, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	authResp := api.GetAuthResponse{}
	require.NoError(t, json.Unmarshal(b, &authResp))
	require.True(t, authResp.Authenticated)
	require.Equal(t, userAliceHandle, authResp.User)

	eventsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.url.JoinPath("/ims/api/events").String(), nil)
	require.NoError(t, err)
	eventsReq.AddCookie(cookie)
	resp = sendRaw(t, srv, eventsReq)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPostAuthTokenInBody(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)

	// #nosec G117 // Test credentials
	loginBody, err := json.Marshal(api.PostAuthRequest{
		Identification: userAliceEmail,
		Password:       userAlicePassword,
		TokenInBody:    true,
	})
	require.NoError(t, err)
	loginReq, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.url.JoinPath("/ims/api/auth").String(), bytes.NewReader(loginBody))
	require.NoError(t, err)
	loginReq.Header.Set("Content-Type", "application/json")
	resp := sendRaw(t, srv, loginReq)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// The token is in the body, and no cookie carries it
	response := api.PostAuthResponse{}
	require.NoError(t, json.Unmarshal(b, &response))
	jwter := authz.JWTer{SecretKey: shared.cfg.Core.JWTSecret}
	claims, err := jwter.AuthenticateJWT(response.Token)
	require.NoError(t, err)
	require.Equal(t, userAliceHandle, claims.RangerHandle())
	require.Greater(t, response.ExpiresUnixMs, time.Now().UnixMilli())
	require.Nil(t, authCookie(resp))

	// and that token works as a bearer token
	code := apiCall(t, MethodURL{http.MethodGet, "/ims/api/events"}, srv.withJWT(response.Token))
	require.Equal(t, http.StatusOK, code)
}

func TestAuthorizationHeaderTakesPrecedenceOverCookie(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)

	jwter := authz.JWTer{SecretKey: shared.cfg.Core.JWTSecret}
	goodToken := srv.login(ctx, userAliceEmail, userAlicePassword)
	expiredToken, err := jwter.CreateAccessToken(userAliceHandle, 1, time.Now().Add(-time.Hour))
	require.NoError(t, err)

	// A bad header isn't rescued by a good cookie
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.url.JoinPath("/ims/api/events").String(), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	req.AddCookie(tokenCookie(goodToken))
	resp := sendRaw(t, srv, req)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// A good header works despite a bad cookie
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, srv.url.JoinPath("/ims/api/events").String(), nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+goodToken)
	req.AddCookie(tokenCookie(expiredToken))
	resp = sendRaw(t, srv, req)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestCrossOriginRequestsAreRejected covers the CSRF defense that cookie auth
// depends on: a browser attaches the cookie to a request no matter which site
// initiated it, but it also says where the request came from.
func TestCrossOriginRequestsAreRejected(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)

	cookie := tokenCookie(srv.login(ctx, userAdminEmail, userAdminPassword))
	typesURL := srv.url.JoinPath("/ims/api/incident_types").String()
	newType := func() *http.Request {
		body, err := json.Marshal(imsjson.IncidentType{Name: new(rand.NonCryptoText())})
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, typesURL, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(cookie)
		return req
	}

	// A modern browser marks a request from another site
	req := newType()
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp := sendRaw(t, srv, req)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// That includes a sibling subdomain, which SameSite cookies don't guard against
	req = newType()
	req.Header.Set("Sec-Fetch-Site", "same-site")
	resp = sendRaw(t, srv, req)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// An older browser only sends an Origin, which must match the host
	req = newType()
	req.Header.Set("Origin", "https://evil.example.com")
	resp = sendRaw(t, srv, req)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// A request from IMS's own pages goes through
	req = newType()
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	resp = sendRaw(t, srv, req)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// So does one from a non-browser client, which sends neither header
	resp = sendRaw(t, srv, newType())
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Login is covered too, so another site can't swap in its own session
	// #nosec G117 // Test credentials
	loginBody, err := json.Marshal(api.PostAuthRequest{
		Identification: userAliceEmail,
		Password:       userAlicePassword,
	})
	require.NoError(t, err)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, srv.url.JoinPath("/ims/api/auth").String(), bytes.NewReader(loginBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp = sendRaw(t, srv, req)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.Nil(t, authCookie(resp))

	// Reads aren't state-changing, so a cross-site GET gets through to authentication
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, typesURL, nil)
	require.NoError(t, err)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.AddCookie(cookie)
	resp = sendRaw(t, srv, req)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestPostAuthRejectsCrossSiteFormPost guards against login CSRF. A page on another
// origin can't run JavaScript against IMS (no CORS headers) or read our responses,
// but it can make a browser submit an HTML form to us. Such a form can only send a
// text/plain, urlencoded, or multipart body, so IMS requires application/json on any
// request body. Without that, an attacker could silently log a Ranger's browser into
// the attacker's own account, and then harvest whatever the Ranger typed next.
func TestPostAuthRejectsCrossSiteFormPost(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)

	// This is what a cross-site form POST looks like on the wire: real credentials
	// in a JSON body, but a Content-Type that a form (and only a form) would send.
	// #nosec G117 // Test credentials
	postBody, err := json.Marshal(api.PostAuthRequest{
		Identification: userAliceEmail,
		Password:       userAlicePassword,
	})
	require.NoError(t, err)
	authURL := srv.url.JoinPath("/ims/api/auth").String()

	formPost := func(contentType string) *http.Response {
		httpPost, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, bytes.NewReader(postBody))
		require.NoError(t, err)
		httpPost.Header.Set("Content-Type", contentType)
		// #nosec G704 // SSRF via taint analysis. We control the URL.
		resp, err := srv.client.Do(httpPost)
		require.NoError(t, err)
		return resp
	}

	// The text/plain form encoding is rejected, and hands out no cookie.
	resp := formPost("text/plain;charset=UTF-8")
	require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("Set-Cookie"))
	require.NoError(t, resp.Body.Close())

	// So is the urlencoded form encoding.
	resp = formPost("application/x-www-form-urlencoded")
	require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("Set-Cookie"))
	require.NoError(t, resp.Body.Close())

	// So is the multipart form encoding.
	resp = formPost("multipart/form-data; boundary=whatever")
	require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
	assert.Empty(t, resp.Header.Get("Set-Cookie"))
	require.NoError(t, resp.Body.Close())

	// The same credentials sent as JSON still work.
	statusCode, _, token := srv.unauthed().postAuth(ctx,
		api.PostAuthRequest{
			Identification: userAliceEmail,
			Password:       userAlicePassword,
		},
	)
	require.Equal(t, http.StatusOK, statusCode)
	require.NotEmpty(t, token)
}
