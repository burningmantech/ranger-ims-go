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
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestCreateStreets(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	// Make an event
	eventName := rand.NonCryptoText()
	resp := apis.editEvent(ctx, imsjson.EditEventsRequest{Add: []string{eventName}})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	events, resp := apis.getEvents(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	var event imsjson.Event
	for _, e := range events {
		if e.Name == eventName {
			event = e
			break
		}
	}
	require.NotZero(t, event.ID)

	// Set some streets on that event
	createStreets := imsjson.EventsStreets{
		event.ID: imsjson.EventStreets{
			"1": "Esplanade",
			"5": "Emu St",
		},
	}
	resp = apis.editStreets(ctx, createStreets)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Try add one more, and try to modify one of those streets (the update won't work,
	// as that's not currently supported).
	editStreets := imsjson.EventsStreets{
		event.ID: imsjson.EventStreets{
			"1": "A Street",
			"2": "Cat St",
		},
	}
	resp = apis.editStreets(ctx, editStreets)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	expected := imsjson.EventsStreets{
		event.ID: imsjson.EventStreets{
			"1": "Esplanade",
			"2": "Cat St",
			"5": "Emu St",
		},
	}

	// Get the streets from the API, make sure they match what we sent
	resultStreets, resp := apis.getStreets(ctx, event.ID)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, expected, resultStreets)

	// Get the streets again from the API, without specifying the event ID.
	// We'll get back streets for every event.
	resultStreets, resp = apis.getStreets(ctx, 0)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, expected[event.ID], resultStreets[event.ID])
}
