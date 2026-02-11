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

func sampleStay1(eventName string) imsjson.Stay {
	return imsjson.Stay{
		Event: eventName,
		// Incident

		GuestPreferredName:   new("Buffy"),
		GuestLegalName:       new("Jim"),
		GuestDescription:     new("Tall very large guy"),
		GuestCampName:        new("Ranch Camp"),
		GuestCampAddress:     new("7:00 & A"),
		GuestCampDescription: new("Lots of bison out front"),

		ArrivalTime:       new(time.Unix(1769599609, 0)),
		ArrivalMethod:     new("stomped in"),
		ArrivalState:      new("seemed angry"),
		ArrivalReason:     new("needed a place to chill"),
		ArrivalBelongings: new("bison costume"),

		DepartureTime:   new(time.Unix(1769617607, 0)),
		DepartureMethod: new("stomped out"),
		DepartureState:  new("happily eating some grass"),

		ResourceRest:    new("slept in the quonset; needed literally all the space"),
		ResourceClothes: new("gave him some diapers"),
		ResourcePogs:    new("no, wasn't hungry"),
		ResourceFoodBev: new("ate a lot of our grass"),
		ResourceOther:   new("nothing else"),
		Rangers:         &[]imsjson.StayRanger{{Handle: "SomeOne"}, {Handle: "SomeTwo"}},
		ReportEntries: []imsjson.ReportEntry{
			{Text: "This is some stay report text"},
			{Text: ""},
		},
	}
}

func TestStayAPIAuthorization(t *testing.T) {
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

	// Alright, now test hitting all the Stay endpoints

	// For the user who isn't authenticated at all (no JWT)
	_, resp = notAuthenticated.getStays(ctx, eventName)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = notAuthenticated.getStay(ctx, eventName, 1)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = notAuthenticated.newStay(ctx, imsjson.Stay{Event: eventName})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = notAuthenticated.updateStay(ctx, eventName, 1, imsjson.Stay{})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// For a normal user without permissions on the event
	_, resp = aliceUser.getStays(ctx, eventName)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = aliceUser.getStay(ctx, eventName, 1)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.updateStay(ctx, eventName, 1, imsjson.Stay{})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.newStay(ctx, imsjson.Stay{Event: eventName})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// For an admin user without permissions on the event
	// (an admin has no special privileges on each event)
	_, resp = adminUser.getStays(ctx, eventName)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = adminUser.getStay(ctx, eventName, 1)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = adminUser.newStay(ctx, imsjson.Stay{Event: eventName})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = adminUser.updateStay(ctx, eventName, 1, imsjson.Stay{})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// make Alice a writer
	resp = adminUser.addWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Now Alice get access all the Stay endpoints for this event
	_, resp = aliceUser.getStays(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = aliceUser.getStay(ctx, eventName, 1)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.newStay(ctx, imsjson.Stay{Event: eventName})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.updateStay(ctx, eventName, 1, imsjson.Stay{})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestCreateAndGetStay(t *testing.T) {
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
	stayReq := sampleStay1(eventName)
	entryReq := stayReq.ReportEntries[0]
	num := apisNonAdmin.newStaySuccess(ctx, stayReq)
	stayReq.Number = num
	for _, r := range *stayReq.Rangers {
		resp = apisNonAdmin.attachRangerToStay(ctx, eventName, num, r.Handle)
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	}

	{
		// Use normal user to fetch that Incident from the API and check it over
		retrievedStay, resp := apisNonAdmin.getStay(ctx, eventName, num)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.NotNil(t, retrievedStay)
		require.WithinDuration(t, time.Now(), retrievedStay.Created, 5*time.Minute)
		require.WithinDuration(t, time.Now(), retrievedStay.LastModified, 5*time.Minute)
		require.Len(t, retrievedStay.ReportEntries, 4)

		// The first report entry will be the system entry. The second should be the one we sent in the request
		retrievedUserEntry := retrievedStay.ReportEntries[1]
		retrievedUserEntry.ID = 0
		require.WithinDuration(t, time.Now(), retrievedUserEntry.Created, 5*time.Minute)
		retrievedUserEntry.Created = time.Time{}
		entryReq.Author = userAliceHandle
		entryReq.Stricken = new(false)
		require.Equal(t, entryReq, retrievedUserEntry)
		requireEqualStay(t, stayReq, retrievedStay)
	}

	{
		// Now get the stay via the GetStays (plural) endpoint, and repeat the validation
		retrievedStays, resp := apisNonAdmin.getStays(ctx, eventName)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.Len(t, retrievedStays, 1)

		// The first entry will be the system entry. The second should be the one we sent in the request
		retrievedUserEntry := retrievedStays[0].ReportEntries[1]
		retrievedUserEntry.ID = 0
		require.WithinDuration(t, time.Now(), retrievedUserEntry.Created, 5*time.Minute)
		retrievedUserEntry.Created = time.Time{}
		entryReq.Author = userAliceHandle
		require.Equal(t, entryReq, retrievedUserEntry)
		requireEqualStay(t, stayReq, retrievedStays[0])
	}
}

func TestCreateAndUpdateStay(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user WriteStays role on that event
	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addStayWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addWriter(ctx, eventName, userAdminHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Use normal user to create a new Stay.
	stayReq := sampleStay1(eventName)
	num := apisNonAdmin.newStaySuccess(ctx, stayReq)
	stayReq.Number = num

	retrievedNewStay, resp := apisNonAdmin.getStay(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())

	// Now let's update the stay. First let's try changing nothing.
	updates := imsjson.Stay{
		Event:  stayReq.Event,
		Number: num,
	}

	resp = apisNonAdmin.updateStay(ctx, eventName, num, updates)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrievedStayAfterUpdate, resp := apisNonAdmin.getStay(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	requireEqualStay(t, retrievedNewStay, retrievedStayAfterUpdate)

	// now let's set all fields to empty
	updates = imsjson.Stay{
		Event:                stayReq.Event,
		Number:               num,
		Incident:             new(int32(0)),
		GuestPreferredName:   new(""),
		GuestLegalName:       new(""),
		GuestDescription:     new(""),
		GuestCampName:        new(""),
		GuestCampAddress:     new(""),
		GuestCampDescription: new(""),
		ArrivalTime:          &time.Time{},
		ArrivalMethod:        new(""),
		ArrivalState:         new(""),
		ArrivalReason:        new(""),
		ArrivalBelongings:    new(""),
		DepartureTime:        &time.Time{},
		DepartureMethod:      new(""),
		DepartureState:       new(""),
		ResourceRest:         new(""),
		ResourceClothes:      new(""),
		ResourcePogs:         new(""),
		ResourceFoodBev:      new(""),
		ResourceOther:        new(""),
		Rangers:              nil,
		ReportEntries:        nil,
	}
	resp = apisNonAdmin.updateStay(ctx, eventName, num, updates)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	// then check the result
	retrievedStayAfterUpdate, resp = apisNonAdmin.getStay(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	expected := imsjson.Stay{
		Event:   eventName,
		Number:  num,
		Rangers: &[]imsjson.StayRanger{},
	}
	requireEqualStay(t, expected, retrievedStayAfterUpdate)

	// make an incident, then attach to it
	incidentNumber := apisAdmin.newIncidentSuccess(ctx, imsjson.Incident{
		Event: eventName,
	})
	resp = apisAdmin.updateIncident(ctx, eventName, num, imsjson.Incident{
		Event:  eventName,
		Number: incidentNumber,
		Stays:  &[]int32{num},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// check it attached
	stayAfterAttach, resp := apisNonAdmin.getStay(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, incidentNumber, *stayAfterAttach.Incident)

	// detach
	resp = apisAdmin.updateIncident(ctx, eventName, num, imsjson.Incident{
		Event:  eventName,
		Number: incidentNumber,
		Stays:  &[]int32{},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// check it detached
	stayAfterDetach, resp := apisNonAdmin.getStay(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Nil(t, stayAfterDetach.Incident)

	// attach a Ranger
	resp = apisNonAdmin.attachRangerToStay(ctx, eventName, num, "Some Dude")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	retrievedStayAfterUpdate, resp = apisNonAdmin.getStay(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, *retrievedStayAfterUpdate.Rangers, 1)
	require.Equal(t, "Some Dude", (*retrievedStayAfterUpdate.Rangers)[0].Handle)

	// detach that Ranger
	resp = apisNonAdmin.detachRangerFromStay(ctx, eventName, num, "Some Dude")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	retrievedStayAfterUpdate, resp = apisNonAdmin.getStay(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, *retrievedStayAfterUpdate.Rangers)
}

func TestCreateAndAttachFileToStay(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user StayWriter role on that event
	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addStayWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Use normal user to create a new Stay
	stayReq := sampleStay1(eventName)
	num := apisNonAdmin.newStaySuccess(ctx, stayReq)
	stayReq.Number = num

	// Now we'll upload an attachment. The "file" will just be this slice of bytes.
	fileBytes := []byte("This is a text file maybe?")
	reID, resp := apisNonAdmin.attachFileToStay(ctx, eventName, num, fileBytes)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Now call to fetch the attachment and check that it's the same as what we sent.
	returnedAttachment, resp := apisNonAdmin.getStayAttachment(ctx, eventName, num, reID)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, fileBytes, returnedAttachment)

	// Try to send something too large
	fileBytes = []byte(strings.Repeat("a", int(shared.cfg.Core.MaxRequestBytes+1)))
	_, resp = apisNonAdmin.attachFileToStay(ctx, eventName, num, fileBytes)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

// requireEqualStay checks that two stay responses are the same.
// It does not consider ReportEntries.
func requireEqualStay(t *testing.T, before, after imsjson.Stay) {
	t.Helper()

	// If the timestamp field was set before, then check it's the same. Otherwise
	// see if it was set to some reasonable time for when the test was running
	if !before.Created.IsZero() {
		require.Equal(t, before.Created, after.Created)
	} else {
		require.WithinDuration(t, time.Now(), after.Created, 20*time.Minute)
	}
	requireEqualishTimePtr(t, before.ArrivalTime, after.ArrivalTime)
	requireEqualishTimePtr(t, before.DepartureTime, after.DepartureTime)

	before.EventID, after.EventID = 0, 0
	before.Created, after.Created = time.Time{}, time.Time{}
	before.LastModified, after.LastModified = time.Time{}, time.Time{}
	before.ArrivalTime, after.ArrivalTime = &time.Time{}, &time.Time{}
	before.DepartureTime, after.DepartureTime = &time.Time{}, &time.Time{}
	before.ReportEntries, after.ReportEntries = nil, nil

	require.Equal(t, before, after)
}

func requireEqualishTimePtr(t *testing.T, before, after *time.Time) {
	t.Helper()
	if before == nil {
		require.Nil(t, after)
		return
	}
	require.NotNil(t, after)
	require.WithinDuration(t, *before, *after, 1*time.Millisecond)
}
