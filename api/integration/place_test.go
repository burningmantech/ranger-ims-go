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
	"net/http"
	"testing"
	"time"

	"github.com/burningmantech/ranger-ims-go/api"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePlace(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	// Make an event
	eventName := rand.NonCryptoText()
	_, resp := apis.createEvent(ctx, imsjson.Event{
		Name: &eventName,
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	events, resp := apis.getEvents(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	var event imsjson.Event
	for _, e := range events {
		if *e.Name == eventName {
			event = e
			break
		}
	}
	require.NotZero(t, event.ID)

	dests := imsjson.Places{
		"camp": {
			{
				Name:           "Camp Fun Times",
				LocationString: "4:15 & E",
				ExternalData: map[string]any{
					"some_json_field": "some field value",
				},
			},
		},
	}

	resp = apis.editPlaces(ctx, eventName, dests)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	places, resp := apis.getPlaces(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, dests, places)
}

// TestPlaceLocationEmbargo checks that camp and art locations are withheld from
// non-admins until the event's release times arrive, and that mutant vehicle
// locations, which Burning Man doesn't embargo, are never withheld.
func TestPlaceLocationEmbargo(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := rand.NonCryptoText()
	eventID, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Readers: []imsjson.AccessRule{{Expression: "person:" + userAliceHandle, Validity: "always"}},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apisAdmin.editPlaces(ctx, eventName, imsjson.Places{
		"camp": {{
			Name:           "Camp Fun Times",
			LocationString: "4:15 & E",
			ExternalData: map[string]any{
				"name":            "Camp Fun Times",
				"location_string": "4:15 & E",
				"location":        map[string]any{"frontage": "E", "intersection": "4:15"},
			},
		}},
		"art": {{
			Name:           "Big Sculpture",
			LocationString: "9:00 500'",
			ExternalData: map[string]any{
				"name":            "Big Sculpture",
				"location_string": "9:00 500'",
				"location":        map[string]any{"gps_latitude": 40.786, "gps_longitude": -119.203},
			},
		}},
		"mv": {{
			Name:         "The Bus",
			ExternalData: map[string]any{"name": "The Bus"},
		}},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// With no release times set, Alice sees every location.
	places, resp := apisAlice.getPlaces(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, "4:15 & E", places["camp"][0].LocationString)
	assert.Contains(t, places["camp"][0].ExternalData, "location")
	assert.Equal(t, "9:00 500'", places["art"][0].LocationString)
	assert.Contains(t, places["art"][0].ExternalData, "location")

	// Embargo camp locations until next year, and release art locations as of
	// last year.
	future := time.Now().AddDate(1, 0, 0)
	past := time.Now().AddDate(-1, 0, 0)
	resp = apisAdmin.editEvent(ctx, imsjson.Event{
		ID:                   eventID,
		CampLocationsRelease: &future,
		ArtLocationsRelease:  &past,
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	places, resp = apisAlice.getPlaces(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// The camp keeps its name, but loses every form of its location.
	require.Len(t, places["camp"], 1)
	assert.Equal(t, "Camp Fun Times", places["camp"][0].Name)
	assert.Empty(t, places["camp"][0].LocationString)
	assert.NotContains(t, places["camp"][0].ExternalData, "location")
	assert.NotContains(t, places["camp"][0].ExternalData, "location_string")
	assert.Contains(t, places["camp"][0].ExternalData, "name")

	// The art was released last year, so it's unaffected.
	require.Len(t, places["art"], 1)
	assert.Equal(t, "9:00 500'", places["art"][0].LocationString)
	assert.Contains(t, places["art"][0].ExternalData, "location")

	// Mutant vehicles come through untouched.
	require.Len(t, places["mv"], 1)
	assert.Equal(t, "The Bus", places["mv"][0].Name)

	// An admin sees the embargoed camp location.
	places, resp = apisAdmin.getPlaces(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, "4:15 & E", places["camp"][0].LocationString)
	assert.Contains(t, places["camp"][0].ExternalData, "location")

	// Clearing the release time lifts the embargo for Alice too.
	resp = apisAdmin.editEvent(ctx, imsjson.Event{
		ID:                   eventID,
		CampLocationsRelease: &time.Time{},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	places, resp = apisAlice.getPlaces(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, "4:15 & E", places["camp"][0].LocationString)
	assert.Contains(t, places["camp"][0].ExternalData, "location")

	events, resp := apisAdmin.getEvents(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	event := findEvent(events, eventID)
	require.NotNil(t, event)
	assert.Nil(t, event.CampLocationsRelease)
	require.NotNil(t, event.ArtLocationsRelease)
	assert.WithinDuration(t, past, *event.ArtLocationsRelease, time.Second)
}

// TestPlaceLocationEmbargoExcludingExternalData checks the embargo on the
// slimmed-down response that the incident address autocomplete fetches.
func TestPlaceLocationEmbargoExcludingExternalData(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := rand.NonCryptoText()
	eventID, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Readers: []imsjson.AccessRule{{Expression: "person:" + userAliceHandle, Validity: "always"}},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apisAdmin.editPlaces(ctx, eventName, imsjson.Places{
		"camp": {{
			Name:           "Camp Fun Times",
			LocationString: "4:15 & E",
			ExternalData:   map[string]any{"location": map[string]any{"frontage": "E"}},
		}},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	future := time.Now().AddDate(1, 0, 0)
	resp = apisAdmin.editEvent(ctx, imsjson.Event{ID: eventID, CampLocationsRelease: &future})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	places, resp := apisAlice.getPlacesExcludingExternalData(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, places["camp"], 1)
	assert.Equal(t, "Camp Fun Times", places["camp"][0].Name)
	assert.Empty(t, places["camp"][0].LocationString)
	assert.Nil(t, places["camp"][0].ExternalData)
}

// TestImportPlacesFromAPI covers the Places admin page's "Set from API"
// buttons, which have the server fetch a place type from the Burning Man API
// and store it, rather than an admin pasting the response in by hand. The
// Burning Man API is stood in for by fakeBMAPI.
func TestImportPlacesFromAPI(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	eventName := rand.NonCryptoText()
	_, resp := apis.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apis.importPlaces(ctx, eventName, "camp", "2025")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var imported api.ImportPlacesResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&imported))
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, 2, imported.Count)

	places, resp := apis.getPlaces(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, places["camp"], 2)
	assert.Equal(t, "camp One", places["camp"][0].Name)
	assert.Equal(t, "3:00 & A", places["camp"][0].LocationString)
	// The whole upstream object is kept as the Place's external data.
	assert.Equal(t, map[string]any{
		"uid": "camp-1", "name": "camp One", "location_string": "3:00 & A", "year": float64(2025),
	}, places["camp"][0].ExternalData)

	// Importing one type leaves the others alone.
	resp = apis.importPlaces(ctx, eventName, "art", "2025")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	places, resp = apis.getPlaces(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Len(t, places["camp"], 2)
	require.Len(t, places["art"], 2)
	assert.Equal(t, "art One", places["art"][0].Name)
}

// A year the Burning Man API has no data for must not wipe out what the event
// already has, since that's an easy typo to make in the year input.
func TestImportPlacesEmptyResponseChangesNothing(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	eventName := rand.NonCryptoText()
	_, resp := apis.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apis.editPlaces(ctx, eventName, imsjson.Places{
		"camp": {{Name: "Camp Fun Times", LocationString: "4:15 & E"}},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apis.importPlaces(ctx, eventName, "camp", bmAPIYearNoData)
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	places, resp := apis.getPlaces(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, places["camp"], 1)
	assert.Equal(t, "Camp Fun Times", places["camp"][0].Name)
}

func TestImportPlacesUpstreamFailure(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	eventName := rand.NonCryptoText()
	_, resp := apis.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apis.importPlaces(ctx, eventName, "camp", bmAPIYearBroken)
	assert.Equal(t, http.StatusBadGateway, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	places, resp := apis.getPlaces(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Empty(t, places["camp"])
}

func TestImportPlacesBadRequests(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	eventName := rand.NonCryptoText()
	_, resp := apis.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// "Other" places are hand-written in IMS, with no upstream API to pull from.
	resp = apis.importPlaces(ctx, eventName, "other", "2025")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apis.importPlaces(ctx, eventName, "camp", "not a year")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apis.importPlaces(ctx, "no-such-event", "camp", "2025")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

// Importing places rewrites an event's data wholesale, so it takes the same
// admin-only permission that saving them by hand does.
func TestImportPlacesRequiresAdmin(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Writers: []imsjson.AccessRule{{Expression: "person:" + userAliceHandle, Validity: "always"}},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apisAlice.importPlaces(ctx, eventName, "camp", "2025")
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}
