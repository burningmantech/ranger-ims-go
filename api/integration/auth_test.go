package integration

import (
	"github.com/burningmantech/ranger-ims-go/api"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestPostAuthAPIAuthorization(t *testing.T) {
	s := httptest.NewServer(api.AddToMux(nil, shared.cfg, shared.imsDB, shared.userStore))
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
	statusCode, body, token = apisNotAuthenticated.postAuth(api.PostAuthRequest{
		Identification: userAliceEmail,
		Password:       userAlicePassword,
	})
	require.Equal(t, http.StatusOK, statusCode)
	require.NotEmpty(t, token)

	// That same valid user can also log in by handle
	statusCode, body, token = apisNotAuthenticated.postAuth(api.PostAuthRequest{
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
	s := httptest.NewServer(api.AddToMux(nil, shared.cfg, shared.imsDB, shared.userStore))
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
	s := httptest.NewServer(api.AddToMux(nil, shared.cfg, shared.imsDB, shared.userStore))
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
