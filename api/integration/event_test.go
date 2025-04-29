package integration

import (
	"github.com/burningmantech/ranger-ims-go/api"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestEventAPIAuthorization(t *testing.T) {
	s := httptest.NewServer(api.AddToMux(nil, shared.cfg, shared.imsDB, nil))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)

	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForTestAdminRanger(t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForRealTestUser(t)}
	apisNotAuthenticated := ApiHelper{t: t, serverURL: serverURL, jwt: ""}

	// Any authenticated user can call GetEvents
	_, resp := apisNotAuthenticated.getEvents()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_, resp = apisNonAdmin.getEvents()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, resp = apisAdmin.getEvents()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Only admins can hit the EditEvents endpoint
	// An unauthenticated client will get a 401
	// An unauthorized user will get a 403
	editEventReq := imsjson.EditEventsRequest{}
	resp = apisNotAuthenticated.editEvent(editEventReq)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp = apisNonAdmin.editEvent(editEventReq)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp = apisAdmin.editEvent(editEventReq)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestGetAndEditEvent(t *testing.T) {
	s := httptest.NewServer(api.AddToMux(nil, shared.cfg, shared.imsDB, nil))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)

	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForTestAdminRanger(t)}

	testEventName := "TestGetAndEditEvent"

	editEventReq := imsjson.EditEventsRequest{
		Add: []string{testEventName},
	}

	resp := apisAdmin.editEvent(editEventReq)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	accessReq := imsjson.EventsAccess{
		testEventName: {
			Writers: []imsjson.AccessRule{
				{
					Expression: "person:" + userAdminHandle,
					Validity:   "always",
				},
			},
		},
	}
	resp = apisAdmin.editAccess(accessReq)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	events, resp := apisAdmin.getEvents()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	// The list may include events from other tests, and we can't be sure of this event's numeric ID.
	// The best we can do is loop through the events and make sure there's one that matches.
	var foundEvent *imsjson.Event
	for _, event := range events {
		if event.Name == testEventName {
			foundEvent = &event
		}
	}
	require.NotNil(t, foundEvent)
	require.Equal(t, testEventName, foundEvent.Name)
	require.NotZero(t, foundEvent.ID)
}

func TestEditEvent_errors(t *testing.T) {
	s := httptest.NewServer(api.AddToMux(nil, shared.cfg, shared.imsDB, nil))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)

	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForTestAdminRanger(t)}

	testEventName := "This name is ugly (has spaces and parentheses)"

	editEventReq := imsjson.EditEventsRequest{
		Add: []string{testEventName},
	}

	resp := apisAdmin.editEvent(editEventReq)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	require.NoError(t, err)
	require.Contains(t, string(b), "names must match the pattern")
}
