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
	"io"
	"net/http"
	"testing"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/require"
)

func TestGetAndEditEvent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	testEventName := rand.NonCryptoText()

	editEventReq := imsjson.Event{
		Name: &testEventName,
	}

	eventID, resp := apisAdmin.createEvent(ctx, editEventReq)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

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
	require.NoError(t, resp.Body.Close())

	expectedAccessResult := imsjson.EventAccess{
		Writers:   accessReq[testEventName].Writers,
		Readers:   []imsjson.AccessRule{},
		Reporters: []imsjson.AccessRule{},
	}
	expectedAccessResult.Writers[0].DebugInfo.MatchesUsers = []string{userAdminHandle}
	expectedAccessResult.Writers[0].DebugInfo.KnownTarget = true
	accessResult, httpResp := apisAdmin.getAccess(ctx)
	require.Equal(t, http.StatusOK, httpResp.StatusCode)
	require.Equal(t, expectedAccessResult, accessResult[testEventName])
	require.NoError(t, httpResp.Body.Close())

	events, resp := apisAdmin.getEvents(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	// The list may include events from other tests
	var foundEvent *imsjson.Event
	for _, event := range events {
		if event.ID == eventID {
			foundEvent = &event
		}
	}
	require.NotNil(t, foundEvent)
	require.NotNil(t, foundEvent.Name)
	require.Equal(t, testEventName, *foundEvent.Name)
	require.NotZero(t, foundEvent.ID)
}

func TestEditEvent_errors(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	testEventName := "This name is ugly (has spaces and parentheses)"

	editEventReq := imsjson.Event{
		Name: &testEventName,
	}

	resp := apisAdmin.editEvent(ctx, editEventReq)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, string(b), "names must match the pattern")
}
