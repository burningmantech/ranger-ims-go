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
	"net/http"
	"testing"
	"time"
)

func sampleIncident1(eventName string) imsjson.Incident {
	return imsjson.Incident{
		Event:    eventName,
		State:    "new",
		Priority: 5,
		Summary:  ptr("my summary!"),
		Location: imsjson.Location{
			Name:         ptr("Zeroth Camp"),
			RadialHour:   ptr("10"),
			RadialMinute: ptr("5"),
			Description:  ptr("unknown"),
			Type:         "garett",
		},
		IncidentTypes: &[]string{"Admin", "Junk"},
		FieldReports:  &[]int32{},
		RangerHandles: &[]string{"SomeOne", "SomeTwo"},
		ReportEntries: []imsjson.ReportEntry{
			{Text: "This is some report text lol"},
		},
	}
}

func TestIncidentAPIAuthorization(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	adminUser := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	aliceUser := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	notAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}

	// Make an event to which no one has any access
	eventName := uuid.NewString()
	resp := adminUser.editEvent(ctx, imsjson.EditEventsRequest{Add: []string{eventName}})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Alright, now test hitting all the Incident endpoints

	// For the user who isn't authenticated at all (no JWT)
	_, resp = notAuthenticated.getIncidents(ctx, eventName)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = notAuthenticated.getIncident(ctx, eventName, 1)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = notAuthenticated.newIncident(ctx, imsjson.Incident{Event: eventName})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = notAuthenticated.updateIncident(ctx, eventName, 1, imsjson.Incident{})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// For a normal user without permissions on the event
	_, resp = aliceUser.getIncidents(ctx, eventName)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = aliceUser.getIncident(ctx, eventName, 1)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.updateIncident(ctx, eventName, 1, imsjson.Incident{})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.newIncident(ctx, imsjson.Incident{Event: eventName})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// For an admin user without permissions on the event
	// (an admin has no special privileges on each event)
	_, resp = adminUser.getIncidents(ctx, eventName)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = adminUser.getIncident(ctx, eventName, 1)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = adminUser.newIncident(ctx, imsjson.Incident{Event: eventName})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = adminUser.updateIncident(ctx, eventName, 1, imsjson.Incident{})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// make Alice a writer
	resp = adminUser.addWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Now Alice get access all the Incidents endpoints for this event
	_, resp = aliceUser.getIncidents(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = aliceUser.getIncident(ctx, eventName, 1)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.newIncident(ctx, imsjson.Incident{Event: eventName})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.updateIncident(ctx, eventName, 1, imsjson.Incident{})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestCreateAndGetIncident(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user Writer role on that event
	eventName := uuid.NewString()
	resp := apisAdmin.editEvent(ctx, imsjson.EditEventsRequest{Add: []string{eventName}})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Use normal user to create a new Incident
	incidentReq := sampleIncident1(eventName)
	entryReq := incidentReq.ReportEntries[0]
	num := apisNonAdmin.newIncidentSuccess(ctx, incidentReq)
	incidentReq.Number = num

	{
		// Use normal user to fetch that Incident from the API and check it over
		retrievedIncident, resp := apisNonAdmin.getIncident(ctx, eventName, num)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.NotNil(t, retrievedIncident)
		require.WithinDuration(t, time.Now(), retrievedIncident.Created, 5*time.Minute)
		require.WithinDuration(t, time.Now(), retrievedIncident.LastModified, 5*time.Minute)
		require.Len(t, retrievedIncident.ReportEntries, 2)

		// The first report entry will be the system entry. The second should be the one we sent in the request
		retrievedUserEntry := retrievedIncident.ReportEntries[1]
		retrievedUserEntry.ID = 0
		require.WithinDuration(t, time.Now(), retrievedUserEntry.Created, 5*time.Minute)
		retrievedUserEntry.Created = time.Time{}
		entryReq.Author = userAliceHandle
		require.Equal(t, entryReq, retrievedUserEntry)
		requireEqualIncident(t, incidentReq, retrievedIncident)
	}

	{
		// Now get the incident via the GetIncidents (plural) endpoint, and repeat the validation
		retrievedIncidents, resp := apisNonAdmin.getIncidents(ctx, eventName)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.Len(t, retrievedIncidents, 1)

		// The first entry will be the system entry. The second should be the one we sent in the request
		retrievedUserEntry := retrievedIncidents[0].ReportEntries[1]
		retrievedUserEntry.ID = 0
		require.WithinDuration(t, time.Now(), retrievedUserEntry.Created, 5*time.Minute)
		retrievedUserEntry.Created = time.Time{}
		entryReq.Author = userAliceHandle
		require.Equal(t, entryReq, retrievedUserEntry)
		requireEqualIncident(t, incidentReq, retrievedIncidents[0])
	}
}

func TestCreateAndUpdateIncident(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user Writer role on that event
	eventName := uuid.NewString()
	resp := apisAdmin.editEvent(ctx, imsjson.EditEventsRequest{Add: []string{eventName}})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Use normal user to create a new Incident.
	incidentReq := sampleIncident1(eventName)
	num := apisNonAdmin.newIncidentSuccess(ctx, incidentReq)
	incidentReq.Number = num

	retrievedNewIncident, resp := apisNonAdmin.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())

	// Now let's update the incident. First let's try changing nothing.
	updates := imsjson.Incident{
		Event:  incidentReq.Event,
		Number: num,
	}

	resp = apisNonAdmin.updateIncident(ctx, eventName, num, updates)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrievedIncidentAfterUpdate, resp := apisNonAdmin.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	requireEqualIncident(t, retrievedNewIncident, retrievedIncidentAfterUpdate)

	// now let's set all fields to empty
	updates = imsjson.Incident{
		Event:    incidentReq.Event,
		Number:   num,
		State:    "closed",
		Priority: 1,
		Summary:  ptr(""),
		Location: imsjson.Location{
			Name:         ptr(""),
			Concentric:   ptr(""),
			RadialHour:   ptr(""),
			RadialMinute: ptr(""),
			Description:  ptr(""),
			Type:         "garett",
		},
		IncidentTypes: &[]string{},
		FieldReports:  &[]int32{},
		RangerHandles: &[]string{},
		ReportEntries: []imsjson.ReportEntry{},
	}
	resp = apisNonAdmin.updateIncident(ctx, eventName, num, updates)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	// then check the result
	retrievedIncidentAfterUpdate, resp = apisNonAdmin.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	expected := imsjson.Incident{
		Event:    eventName,
		Number:   num,
		State:    "closed",
		Priority: 1,
		Location: imsjson.Location{
			Type: "garett",
		},
		IncidentTypes: &[]string{},
		FieldReports:  &[]int32{},
		RangerHandles: &[]string{},
	}
	requireEqualIncident(t, expected, retrievedIncidentAfterUpdate)
}

func TestCreateAndAttachFileToIncident(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user Writer role on that event
	eventName := uuid.NewString()
	resp := apisAdmin.editEvent(ctx, imsjson.EditEventsRequest{Add: []string{eventName}})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Use normal user to create a new Incident
	incidentReq := sampleIncident1(eventName)
	num := apisNonAdmin.newIncidentSuccess(ctx, incidentReq)
	incidentReq.Number = num

	// Now we'll upload an attachment. The "file" will just be this slice of bytes.
	fileBytes := []byte("This is a text file maybe?")
	reID, resp := apisNonAdmin.attachFileToIncident(ctx, eventName, num, fileBytes)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Now call to fetch the attachment and check that it's the same as what we sent.
	returnedAttachment, resp := apisNonAdmin.getIncidentAttachment(ctx, eventName, num, reID)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, fileBytes, returnedAttachment)
}

// requireEqualIncident is a hacky way of checking two incident responses are the same.
// It does not consider ReportEntries.
func requireEqualIncident(t *testing.T, before, after imsjson.Incident) {
	t.Helper()
	// This field isn't in use in the client yet
	// require.Equal(t, before.EventID, after.EventID)
	require.Equal(t, before.Event, after.Event)
	require.Equal(t, before.Number, after.Number)

	// If the timestamp field was set before, then check it's the same. Otherwise
	// see if it was set to some reasonable time for when the test was running
	if !before.Created.IsZero() {
		require.Equal(t, before.Created, after.Created)
	} else {
		require.WithinDuration(t, time.Now(), after.Created, 20*time.Minute)
	}
	require.WithinDuration(t, time.Now(), after.LastModified, 20*time.Minute)
	require.Equal(t, before.State, after.State)
	require.Equal(t, before.Priority, after.Priority)
	require.Equal(t, before.Summary, after.Summary)
	require.Equal(t, before.Location, after.Location)
	require.Equal(t, before.IncidentTypes, after.IncidentTypes)
	require.Equal(t, before.RangerHandles, after.RangerHandles)
	require.Equal(t, before.FieldReports, after.FieldReports)
	// these will always be different. Check them separately of this function
	// require.Equal(t, before.ReportEntries, after.ReportEntries)
}

func ptr[T any](s T) *T {
	return &s
}
