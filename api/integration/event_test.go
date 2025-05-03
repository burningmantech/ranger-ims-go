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
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"testing"
)

func TestEventAPIAuthorization(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForTestAdminRanger(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForRealTestUser(t, ctx)}
	apisNotAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}

	// Any authenticated user can call GetEvents
	_, resp := apisNotAuthenticated.getEvents(ctx)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_, resp = apisNonAdmin.getEvents(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, resp = apisAdmin.getEvents(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Only admins can hit the EditEvents endpoint
	// An unauthenticated client will get a 401
	// An unauthorized user will get a 403
	editEventReq := imsjson.EditEventsRequest{}
	resp = apisNotAuthenticated.editEvent(ctx, editEventReq)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp = apisNonAdmin.editEvent(ctx, editEventReq)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp = apisAdmin.editEvent(ctx, editEventReq)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestGetAndEditEvent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForTestAdminRanger(ctx, t)}

	testEventName := uuid.New().String()

	editEventReq := imsjson.EditEventsRequest{
		Add: []string{testEventName},
	}

	resp := apisAdmin.editEvent(ctx, editEventReq)
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
	resp = apisAdmin.editAccess(ctx, accessReq)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	expectedAccessResult := imsjson.EventAccess{
		Writers:   accessReq[testEventName].Writers,
		Readers:   []imsjson.AccessRule{},
		Reporters: []imsjson.AccessRule{},
	}
	accessResult, httpResp := apisAdmin.getAccess(ctx)
	require.Equal(t, http.StatusOK, httpResp.StatusCode)
	require.Equal(t, expectedAccessResult, accessResult[testEventName])

	events, resp := apisAdmin.getEvents(ctx)
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
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForTestAdminRanger(ctx, t)}

	testEventName := "This name is ugly (has spaces and parentheses)"

	editEventReq := imsjson.EditEventsRequest{
		Add: []string{testEventName},
	}

	resp := apisAdmin.editEvent(ctx, editEventReq)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, string(b), "names must match the pattern")
}
