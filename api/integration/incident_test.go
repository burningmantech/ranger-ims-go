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
	"net/http"
	"strings"
	"testing"
	"time"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/require"
)

func sampleIncident1(eventName string) imsjson.Incident {
	return imsjson.Incident{
		Event:    eventName,
		State:    "new",
		Priority: 5,
		Summary:  new("my summary!"),
		Location: imsjson.Location{
			Name:         new("Zeroth Camp"),
			Address:      new("10:05 & W"),
			RadialHour:   new("10"),
			RadialMinute: new("5"),
			Description:  new("unknown"),
		},
		IncidentTypeIDs: &[]int32{1, 2},
		FieldReports:    &[]int32{},
		Stays:           &[]int32{},
		Rangers:         &[]imsjson.IncidentRanger{{Handle: "SomeOne"}, {Handle: "SomeTwo"}},
		ReportEntries: []imsjson.ReportEntry{
			{Text: "This is some report text lol"},
			{Text: ""},
		},
		LinkedIncidents: &[]imsjson.LinkedIncident{},
	}
}

func TestIncidentAPIAuthorization(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	adminUser := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	aliceUser := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	notAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}

	// Make an event to which no one has any access
	eventName := rand.NonCryptoText()
	_, resp := adminUser.createEvent(ctx, imsjson.Event{Name: &eventName})
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
	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
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
	for _, r := range *incidentReq.Rangers {
		resp = apisNonAdmin.attachRangerToIncident(ctx, eventName, num, r.Handle)
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	}

	{
		// Use normal user to fetch that Incident from the API and check it over
		retrievedIncident, resp := apisNonAdmin.getIncident(ctx, eventName, num)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.NotNil(t, retrievedIncident)
		require.WithinDuration(t, time.Now(), retrievedIncident.Created, 5*time.Minute)
		require.WithinDuration(t, time.Now(), retrievedIncident.Started, 5*time.Minute)
		require.WithinDuration(t, time.Now(), retrievedIncident.LastModified, 5*time.Minute)
		require.Len(t, retrievedIncident.ReportEntries, 4)

		// The first report entry will be the system entry. The second should be the one we sent in the request
		retrievedUserEntry := retrievedIncident.ReportEntries[1]
		retrievedUserEntry.ID = 0
		require.WithinDuration(t, time.Now(), retrievedUserEntry.Created, 5*time.Minute)
		retrievedUserEntry.Created = time.Time{}
		entryReq.Author = userAliceHandle
		entryReq.Stricken = new(false)
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
	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
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
		Event:  incidentReq.Event,
		Number: num,
		State:  "closed",
		// need to send some time for this other than zero for the time to update
		Started:  time.UnixMilli(1),
		Priority: 1,
		Summary:  new(""),
		Location: imsjson.Location{
			Name:         new(""),
			Address:      new(""),
			Concentric:   new(""),
			RadialHour:   new(""),
			RadialMinute: new(""),
			Description:  new(""),
		},
		IncidentTypeIDs: &[]int32{},
		FieldReports:    &[]int32{},
		Rangers:         &[]imsjson.IncidentRanger{},
		ReportEntries:   []imsjson.ReportEntry{},
	}
	resp = apisNonAdmin.updateIncident(ctx, eventName, num, updates)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	// then check the result
	retrievedIncidentAfterUpdate, resp = apisNonAdmin.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	expected := imsjson.Incident{
		Event:           eventName,
		Number:          num,
		State:           "closed",
		Closed:          time.Now(),
		Priority:        1,
		Started:         time.UnixMilli(1),
		Location:        imsjson.Location{},
		IncidentTypeIDs: &[]int32{},
		FieldReports:    &[]int32{},
		Stays:           &[]int32{},
		Rangers:         &[]imsjson.IncidentRanger{},
		LinkedIncidents: &[]imsjson.LinkedIncident{},
	}
	requireEqualIncident(t, expected, retrievedIncidentAfterUpdate)

	// attach a Ranger
	resp = apisNonAdmin.attachRangerToIncident(ctx, eventName, num, "Some Dude")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	retrievedIncidentAfterUpdate, resp = apisNonAdmin.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, *retrievedIncidentAfterUpdate.Rangers, 1)
	require.Equal(t, "Some Dude", (*retrievedIncidentAfterUpdate.Rangers)[0].Handle)

	// detach that Ranger
	resp = apisNonAdmin.detachRangerFromIncident(ctx, eventName, num, "Some Dude")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	retrievedIncidentAfterUpdate, resp = apisNonAdmin.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, *retrievedIncidentAfterUpdate.Rangers)
}

func TestCreateAndAttachFileToIncident(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user Writer role on that event
	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
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

	// Try to send something too large
	fileBytes = []byte(strings.Repeat("a", int(shared.cfg.Core.MaxRequestBytes+1)))
	_, resp = apisNonAdmin.attachFileToIncident(ctx, eventName, num, fileBytes)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

func TestCreateAndLinkIncidents(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user Writer role on that event
	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Use normal user to create two new Incidents
	incidentReq1 := sampleIncident1(eventName)
	num1 := apisNonAdmin.newIncidentSuccess(ctx, incidentReq1)
	incidentReq1.Number = num1
	incidentReq2 := sampleIncident1(eventName)
	num2 := apisNonAdmin.newIncidentSuccess(ctx, incidentReq2)
	incidentReq2.Number = num2

	// Link one incident to the other
	retrievedNewIncident1, resp := apisNonAdmin.getIncident(ctx, eventName, num1)
	require.NoError(t, resp.Body.Close())
	eventID := retrievedNewIncident1.EventID
	retrievedNewIncident2, resp := apisNonAdmin.getIncident(ctx, eventName, num2)
	require.NoError(t, resp.Body.Close())
	*retrievedNewIncident1.LinkedIncidents = append(*retrievedNewIncident1.LinkedIncidents, imsjson.LinkedIncident{
		EventID: eventID,
		Number:  num2,
	})
	resp = apisNonAdmin.updateIncident(ctx, eventName, num1, retrievedNewIncident1)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Now each incident is linked to the other
	{
		retrievedNewIncident1, resp = apisNonAdmin.getIncident(ctx, eventName, num1)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.Len(t, *retrievedNewIncident1.LinkedIncidents, 1)
		linkedIncident := (*retrievedNewIncident1.LinkedIncidents)[0]
		require.Equal(t, eventID, linkedIncident.EventID)
		require.Equal(t, num2, linkedIncident.Number)
		require.Equal(t, eventName, linkedIncident.EventName)
		require.Equal(t, *retrievedNewIncident2.Summary, linkedIncident.Summary)
	}
	{
		retrievedNewIncident2, resp = apisNonAdmin.getIncident(ctx, eventName, num2)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.Len(t, *retrievedNewIncident2.LinkedIncidents, 1)
		linkedIncident := (*retrievedNewIncident2.LinkedIncidents)[0]
		require.Equal(t, eventID, linkedIncident.EventID)
		require.Equal(t, num1, linkedIncident.Number)
		require.Equal(t, eventName, linkedIncident.EventName)
		require.Equal(t, *retrievedNewIncident1.Summary, linkedIncident.Summary)
	}

	retrievedNewIncident2.LinkedIncidents = &[]imsjson.LinkedIncident{}
	resp = apisNonAdmin.updateIncident(ctx, eventName, num2, retrievedNewIncident2)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	{
		retrievedNewIncident1, resp = apisNonAdmin.getIncident(ctx, eventName, num1)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.Empty(t, *retrievedNewIncident1.LinkedIncidents)
	}
	{
		retrievedNewIncident2, resp = apisNonAdmin.getIncident(ctx, eventName, num2)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.Empty(t, *retrievedNewIncident2.LinkedIncidents)
	}
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
	if !before.Started.IsZero() {
		require.WithinDuration(t, before.Started, after.Started, 1*time.Millisecond)
	} else {
		require.WithinDuration(t, time.Now(), after.Started, 20*time.Minute)
	}
	require.WithinDuration(t, before.Closed, after.Closed, 1*time.Minute)

	before.EventID, after.EventID = 0, 0
	before.Created, after.Created = time.Time{}, time.Time{}
	before.Started, after.Started = time.Time{}, time.Time{}
	before.Closed, after.Closed = time.Time{}, time.Time{}
	before.LastModified, after.LastModified = time.Time{}, time.Time{}
	before.ReportEntries, after.ReportEntries = nil, nil

	require.Equal(t, before, after)
}
