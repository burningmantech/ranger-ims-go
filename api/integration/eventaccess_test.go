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
	"github.com/burningmantech/ranger-ims-go/api"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestEventAccessAPIAuthorization(t *testing.T) {
	s := httptest.NewServer(api.AddToMux(nil, shared.es, shared.cfg, shared.imsDB, nil))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)

	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForTestAdminRanger(t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForRealTestUser(t)}
	apisNotAuthenticated := ApiHelper{t: t, serverURL: serverURL, jwt: ""}

	_, resp := apisNotAuthenticated.getAccess()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_, resp = apisNonAdmin.getAccess()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	_, resp = apisAdmin.getAccess()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Only admins can hit the EditEvents endpoint
	// An unauthenticated client will get a 401
	// An unauthorized user will get a 403
	editAccessReq := imsjson.EventsAccess{}
	resp = apisNotAuthenticated.editAccess(editAccessReq)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp = apisNonAdmin.editAccess(editAccessReq)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp = apisAdmin.editAccess(editAccessReq)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

//func TestGetAndEditEvent(t *testing.T) {
//	s := httptest.NewServer(api.AddToMux(nil, shared.es, shared.cfg, shared.imsDB, nil))
//	defer s.Close()
//	serverURL, err := url.Parse(s.URL)
//	require.NoError(t, err)
//
//	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForTestAdminRanger(t)}
//
//	testEventName := "TestGetAndEditEvent"
//
//	editEventReq := imsjson.EditEventsRequest{
//		Add: []string{testEventName},
//	}
//
//	resp := apisAdmin.editEvent(editEventReq)
//	require.Equal(t, http.StatusNoContent, resp.StatusCode)
//
//	accessReq := imsjson.EventsAccess{
//		testEventName: {
//			Writers: []imsjson.AccessRule{
//				{
//					Expression: "person:" + userAdminHandle,
//					Validity:   "always",
//				},
//			},
//		},
//	}
//	resp = apisAdmin.editAccess(accessReq)
//	require.Equal(t, http.StatusNoContent, resp.StatusCode)
//
//	events, resp := apisAdmin.getEvents()
//	require.Equal(t, http.StatusOK, resp.StatusCode)
//	// The list may include events from other tests, and we can't be sure of this event's numeric ID.
//	// The best we can do is loop through the events and make sure there's one that matches.
//	var foundEvent *imsjson.Event
//	for _, event := range events {
//		if event.Name == testEventName {
//			foundEvent = &event
//		}
//	}
//	require.NotNil(t, foundEvent)
//	require.Equal(t, testEventName, foundEvent.Name)
//	require.NotZero(t, foundEvent.ID)
//}
//
//func TestEditEvent_errors(t *testing.T) {
//	s := httptest.NewServer(api.AddToMux(nil, shared.es, shared.cfg, shared.imsDB, nil))
//	defer s.Close()
//	serverURL, err := url.Parse(s.URL)
//	require.NoError(t, err)
//
//	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForTestAdminRanger(t)}
//
//	testEventName := "This name is ugly (has spaces and parentheses)"
//
//	editEventReq := imsjson.EditEventsRequest{
//		Add: []string{testEventName},
//	}
//
//	resp := apisAdmin.editEvent(editEventReq)
//	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
//	b, err := io.ReadAll(resp.Body)
//	defer resp.Body.Close()
//	require.NoError(t, err)
//	require.Contains(t, string(b), "names must match the pattern")
//}
