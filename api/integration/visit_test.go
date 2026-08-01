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
	"strings"
	"testing"
	"time"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/require"
)

func sampleVisit1(eventName string) imsjson.Visit {
	return imsjson.Visit{
		Event: eventName,
		// Incident

		GuestPreferredName:   new("Buffy"),
		GuestLegalName:       new("Jim"),
		GuestDescription:     new("Tall very large guy"),
		GuestActionPlan:      new("Return him to the herd"),
		GuestCampName:        new("Ranch Camp"),
		GuestCampAddress:     new("7:00 & A"),
		GuestCampDescription: new("Lots of bison out front"),
		GuestCampContacts:    new("705-555-1234, friend"),

		ArrivalTime:       new(time.Unix(1769599609, 0)),
		ArrivalMethod:     new("stomped in"),
		ArrivalState:      new("seemed angry"),
		ArrivalReason:     new("needed a place to chill"),
		ArrivalBelongings: new("bison costume"),

		DepartureTime:   new(time.Unix(1769617607, 0)),
		DepartureMethod: new("stomped out"),
		DepartureState:  new("happily eating some grass"),

		ResourceSitter:  new("Ranger Dude"),
		ResourceBedID:   new("Some bed"),
		ResourceRest:    new("slept in the quonset; needed literally all the space"),
		ResourceClothes: new("gave him some diapers"),
		ResourcePogs:    new("no, wasn't hungry"),
		ResourceFoodBev: new("ate a lot of our grass"),
		ResourceOther:   new("nothing else"),
		Rangers:         &[]imsjson.VisitRanger{{Handle: "SomeOne"}, {Handle: "SomeTwo"}},
		ReportEntries: []imsjson.ReportEntry{
			{Text: "This is some visit report text"},
			{Text: ""},
		},
	}
}

func TestVisitAPIAuthorization(t *testing.T) {
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

	// Alright, now test hitting all the Visit endpoints

	// For the user who isn't authenticated at all (no JWT)
	_, resp = notAuthenticated.getVisits(ctx, eventName)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = notAuthenticated.getVisit(ctx, eventName, 1)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = notAuthenticated.newVisit(ctx, imsjson.Visit{Event: eventName})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = notAuthenticated.updateVisit(ctx, eventName, 1, imsjson.Visit{})
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// For a normal user without permissions on the event
	_, resp = aliceUser.getVisits(ctx, eventName)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = aliceUser.getVisit(ctx, eventName, 1)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.updateVisit(ctx, eventName, 1, imsjson.Visit{})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.newVisit(ctx, imsjson.Visit{Event: eventName})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// For an admin user without permissions on the event
	// (an admin has no special privileges on each event)
	_, resp = adminUser.getVisits(ctx, eventName)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = adminUser.getVisit(ctx, eventName, 1)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = adminUser.newVisit(ctx, imsjson.Visit{Event: eventName})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = adminUser.updateVisit(ctx, eventName, 1, imsjson.Visit{})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// make Alice a writer
	resp = adminUser.addWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Now Alice get access all the Visit endpoints for this event
	_, resp = aliceUser.getVisits(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = aliceUser.getVisit(ctx, eventName, 1)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.newVisit(ctx, imsjson.Visit{Event: eventName})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.updateVisit(ctx, eventName, 1, imsjson.Visit{})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestCreateAndGetVisit(t *testing.T) {
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

	// Use normal user to create a new Visit
	visitReq := sampleVisit1(eventName)
	entryReq := visitReq.ReportEntries[0]
	num := apisNonAdmin.newVisitSuccess(ctx, visitReq)
	visitReq.Number = num
	for _, r := range *visitReq.Rangers {
		resp = apisNonAdmin.attachRangerToVisit(ctx, eventName, num, r.Handle)
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	}

	{
		// Use normal user to fetch that Visit from the API and check it over
		retrievedVisit, resp := apisNonAdmin.getVisit(ctx, eventName, num)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.NotNil(t, retrievedVisit)
		require.WithinDuration(t, time.Now(), retrievedVisit.Created, 5*time.Minute)
		require.WithinDuration(t, time.Now(), retrievedVisit.LastModified, 5*time.Minute)
		require.Len(t, retrievedVisit.ReportEntries, 4)

		// The first report entry will be the system entry. The second should be the one we sent in the request
		retrievedUserEntry := retrievedVisit.ReportEntries[1]
		retrievedUserEntry.ID = 0
		require.WithinDuration(t, time.Now(), retrievedUserEntry.Created, 5*time.Minute)
		retrievedUserEntry.Created = time.Time{}
		entryReq.Author = userAliceHandle
		entryReq.Stricken = new(false)
		require.Equal(t, entryReq, retrievedUserEntry)
		requireEqualVisit(t, visitReq, retrievedVisit)
	}

	{
		// Now get the visit via the GetVisits (plural) endpoint, and repeat the validation
		retrievedVisits, resp := apisNonAdmin.getVisits(ctx, eventName)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.Len(t, retrievedVisits, 1)

		// The first entry will be the system entry. The second should be the one we sent in the request
		retrievedUserEntry := retrievedVisits[0].ReportEntries[1]
		retrievedUserEntry.ID = 0
		require.WithinDuration(t, time.Now(), retrievedUserEntry.Created, 5*time.Minute)
		retrievedUserEntry.Created = time.Time{}
		entryReq.Author = userAliceHandle
		require.Equal(t, entryReq, retrievedUserEntry)
		requireEqualVisit(t, visitReq, retrievedVisits[0])
	}
}

func TestCreateAndUpdateVisit(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user WriteVisits role on that event
	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addVisitWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addWriter(ctx, eventName, userAdminHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Use normal user to create a new Visit.
	visitReq := sampleVisit1(eventName)
	num := apisNonAdmin.newVisitSuccess(ctx, visitReq)
	visitReq.Number = num

	retrievedNewVisit, resp := apisNonAdmin.getVisit(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())

	// Now let's update the visit. First let's try changing nothing.
	updates := imsjson.Visit{
		Event:  visitReq.Event,
		Number: num,
	}

	resp = apisNonAdmin.updateVisit(ctx, eventName, num, updates)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrievedVisitAfterUpdate, resp := apisNonAdmin.getVisit(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	requireEqualVisit(t, retrievedNewVisit, retrievedVisitAfterUpdate)

	// now let's set all fields to empty
	updates = imsjson.Visit{
		Event:                visitReq.Event,
		Number:               num,
		Incident:             new(int32(0)),
		GuestPreferredName:   new(""),
		GuestLegalName:       new(""),
		GuestDescription:     new(""),
		GuestActionPlan:      new(""),
		GuestCampName:        new(""),
		GuestCampAddress:     new(""),
		GuestCampDescription: new(""),
		GuestCampContacts:    new(""),
		ArrivalTime:          &time.Time{},
		ArrivalMethod:        new(""),
		ArrivalState:         new(""),
		ArrivalReason:        new(""),
		ArrivalBelongings:    new(""),
		DepartureTime:        &time.Time{},
		DepartureMethod:      new(""),
		DepartureState:       new(""),
		ResourceSitter:       new(""),
		ResourceBedID:        new(""),
		ResourceRest:         new(""),
		ResourceClothes:      new(""),
		ResourcePogs:         new(""),
		ResourceFoodBev:      new(""),
		ResourceOther:        new(""),
		Rangers:              nil,
		ReportEntries:        nil,
	}
	resp = apisNonAdmin.updateVisit(ctx, eventName, num, updates)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	// then check the result
	retrievedVisitAfterUpdate, resp = apisNonAdmin.getVisit(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	expected := imsjson.Visit{
		Event:   eventName,
		Number:  num,
		Rangers: &[]imsjson.VisitRanger{},
	}
	requireEqualVisit(t, expected, retrievedVisitAfterUpdate)

	// make an incident, then attach to it
	incidentNumber := apisAdmin.newIncidentSuccess(ctx, imsjson.Incident{
		Event: eventName,
	})
	resp = apisAdmin.updateIncident(ctx, eventName, num, imsjson.Incident{
		Event:  eventName,
		Number: incidentNumber,
		Visits: &[]int32{num},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// check it attached
	visitAfterAttach, resp := apisNonAdmin.getVisit(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, incidentNumber, *visitAfterAttach.Incident)

	// detach
	resp = apisAdmin.updateIncident(ctx, eventName, num, imsjson.Incident{
		Event:  eventName,
		Number: incidentNumber,
		Visits: &[]int32{},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// check it detached
	visitAfterDetach, resp := apisNonAdmin.getVisit(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Nil(t, visitAfterDetach.Incident)

	// attach a Ranger
	resp = apisNonAdmin.attachRangerToVisit(ctx, eventName, num, "Some Dude")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	retrievedVisitAfterUpdate, resp = apisNonAdmin.getVisit(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, *retrievedVisitAfterUpdate.Rangers, 1)
	require.Equal(t, "Some Dude", (*retrievedVisitAfterUpdate.Rangers)[0].Handle)

	// detach that Ranger
	resp = apisNonAdmin.detachRangerFromVisit(ctx, eventName, num, "Some Dude")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	retrievedVisitAfterUpdate, resp = apisNonAdmin.getVisit(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, *retrievedVisitAfterUpdate.Rangers)
}

func TestCreateAndAttachFileToVisit(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user VisitWriter role on that event
	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addVisitWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Use normal user to create a new Visit
	visitReq := sampleVisit1(eventName)
	num := apisNonAdmin.newVisitSuccess(ctx, visitReq)
	visitReq.Number = num

	// Now we'll upload an attachment. The "file" will just be this slice of bytes.
	fileBytes := []byte("This is a text file maybe?")
	reID, resp := apisNonAdmin.attachFileToVisit(ctx, eventName, num, fileBytes)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Now call to fetch the attachment and check that it's the same as what we sent.
	returnedAttachment, resp := apisNonAdmin.getVisitAttachment(ctx, eventName, num, reID)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, fileBytes, returnedAttachment)

	// Try to send something too large
	fileBytes = []byte(strings.Repeat("a", int(shared.cfg.Core.MaxRequestBytes+1)))
	_, resp = apisNonAdmin.attachFileToVisit(ctx, eventName, num, fileBytes)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
}

// requireEqualVisit checks that two visit responses are the same.
// It does not consider ReportEntries.
func requireEqualVisit(t *testing.T, before, after imsjson.Visit) {
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
	before.Version, after.Version = 0, 0
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

// The stored sample visit's arrival and departure times, which the tests below
// move relative to.
var (
	sampleArrival   = time.Unix(1769599609, 0)
	sampleDeparture = time.Unix(1769617607, 0)
)

// A visit can't have the guest arriving after they left. This guards the
// arrival side: the new arrival time is compared against the stored departure.
func TestVisitArrivalAfterStoredDepartureIsRejected(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)
	num := apisAlice.newVisitSuccess(ctx, sampleVisit1(eventName))

	tooLate := sampleDeparture.Add(time.Hour)
	resp := apisAlice.updateVisit(ctx, eventName, num, imsjson.Visit{ArrivalTime: &tooLate})
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Contains(t, string(body), "Arrival time cannot be after departure time")

	// The rejected edit didn't partially apply.
	visit, resp := apisAlice.getVisit(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	requireEqualishTimePtr(t, &sampleArrival, visit.ArrivalTime)
}

// The mirror of the above, guarding the departure side.
func TestVisitDepartureBeforeStoredArrivalIsRejected(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)
	num := apisAlice.newVisitSuccess(ctx, sampleVisit1(eventName))

	tooEarly := sampleArrival.Add(-time.Hour)
	resp := apisAlice.updateVisit(ctx, eventName, num, imsjson.Visit{DepartureTime: &tooEarly})
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Contains(t, string(body), "Departure time cannot be before arrival time")

	visit, resp := apisAlice.getVisit(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	requireEqualishTimePtr(t, &sampleDeparture, visit.DepartureTime)
}

// Both times in one request are applied arrival-first, so an out-of-order pair
// is caught by the departure check even though neither is being compared
// against a stored value.
func TestVisitArrivalAndDepartureSentTogetherOutOfOrderIsRejected(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)
	num := apisAlice.newVisitSuccess(ctx, sampleVisit1(eventName))

	// Both are before the stored departure, so only the departure-side check
	// can reject this pair.
	newArrival := sampleArrival.Add(time.Minute)
	newDeparture := sampleArrival.Add(-time.Minute)
	resp := apisAlice.updateVisit(ctx, eventName, num, imsjson.Visit{
		ArrivalTime:   &newArrival,
		DepartureTime: &newDeparture,
	})
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Contains(t, string(body), "Departure time cannot be before arrival time")

	visit, resp := apisAlice.getVisit(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	requireEqualishTimePtr(t, &sampleArrival, visit.ArrivalTime)
	requireEqualishTimePtr(t, &sampleDeparture, visit.DepartureTime)
}

// The comparison is strict, so a guest who left the same instant they arrived
// is allowed. Zero-length visits are odd but not invalid.
func TestVisitDepartureEqualToArrivalIsAllowed(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)
	num := apisAlice.newVisitSuccess(ctx, sampleVisit1(eventName))

	resp := apisAlice.updateVisit(ctx, eventName, num, imsjson.Visit{DepartureTime: &sampleArrival})
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	visit, resp := apisAlice.getVisit(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	requireEqualishTimePtr(t, &sampleArrival, visit.DepartureTime)
}

// A later arrival that stays before the stored departure is an ordinary edit.
func TestVisitArrivalMovedWithinRangeIsAccepted(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)
	num := apisAlice.newVisitSuccess(ctx, sampleVisit1(eventName))

	later := sampleArrival.Add(time.Hour)
	resp := apisAlice.updateVisit(ctx, eventName, num, imsjson.Visit{ArrivalTime: &later})
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	visit, resp := apisAlice.getVisit(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	requireEqualishTimePtr(t, &later, visit.ArrivalTime)
}
