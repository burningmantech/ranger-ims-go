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
	"fmt"
	"net/http"
	"sync"
	"testing"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/require"
)

// TestIncidentRelationPOSTsRequireJSONContentType covers the CSRF defense on the
// two POST endpoints. They read no request body, so they never reach the
// Content-Type check that readBodyAs performs for the handlers that do, and
// without a check of their own a cross-origin HTML form could forge them. The
// DELETE endpoints need no such check, since a form can't issue a DELETE.
func TestIncidentRelationPOSTsRequireJSONContentType(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	num1 := apis.newIncidentSuccess(ctx, typelessIncident(eventName))
	num2 := apis.newIncidentSuccess(ctx, typelessIncident(eventName))
	typePath := apis.incidentTypePath(eventName, num1, 1)
	linkPath := apis.linkedIncidentPath(eventName, num1, eventName, num2)

	// text/plain is the Content-Type a cross-site HTML form would use.
	resp := apis.imsPostContentType(ctx, typePath, "text/plain")
	require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apis.imsPostContentType(ctx, linkPath, "text/plain")
	require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// So are the form encodings.
	resp = apis.imsPostContentType(ctx, typePath, "application/x-www-form-urlencoded")
	require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apis.imsPostContentType(ctx, linkPath, "multipart/form-data; boundary=xyz")
	require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// A missing Content-Type is rejected too.
	resp = apis.imsPostContentType(ctx, typePath, "")
	require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apis.imsPostContentType(ctx, linkPath, "")
	require.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// None of those rejected requests changed anything.
	retrieved, resp := apis.getIncident(ctx, eventName, num1)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, *retrieved.IncidentTypeIDs)
	require.Empty(t, *retrieved.LinkedIncidents)

	// The DELETEs carry no body and no Content-Type, and are accepted.
	resp = apis.detachTypeFromIncident(ctx, eventName, num1, 1)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apis.unlinkIncident(ctx, eventName, num1, eventName, num2)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// The same POSTs with a JSON Content-Type do apply, so it's the header that
	// the requests above were turned away for.
	resp = apis.imsPostContentType(ctx, typePath, "application/json")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apis.imsPostContentType(ctx, linkPath, "Application/JSON; charset=utf-8")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp = apis.getIncident(ctx, eventName, num1)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []int32{1}, *retrieved.IncidentTypeIDs)
	require.Len(t, *retrieved.LinkedIncidents, 1)
}

// A path value that isn't a number is the client's mistake, so it's a 400 rather
// than a 404 for the Incident, Incident Type or link it can't name.
func TestIncidentRelationEndpointsRejectBadPathValues(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	num := apis.newIncidentSuccess(ctx, typelessIncident(eventName))

	requireBadRequest := func(path string) {
		t.Helper()
		resp := apis.imsPost(ctx, struct{}{}, path)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		_, resp = apis.imsDelete(ctx, path, nil)
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	}

	// A non-numeric Incident number, on each relation.
	requireBadRequest(shared.serverURL.JoinPath(
		"/ims/api/events/", eventName, "/incidents/", "not-a-number", "/incident_types/1",
	).String())
	requireBadRequest(shared.serverURL.JoinPath(
		"/ims/api/events/", eventName, "/incidents/", "not-a-number", "/linked_incidents/", eventName, conv.FormatInt(num),
	).String())

	// A non-numeric Incident Type id.
	requireBadRequest(shared.serverURL.JoinPath(
		"/ims/api/events/", eventName, "/incidents/", conv.FormatInt(num), "/incident_types/", "Junk",
	).String())

	// A non-numeric linked Incident number.
	requireBadRequest(shared.serverURL.JoinPath(
		"/ims/api/events/", eventName, "/incidents/", conv.FormatInt(num), "/linked_incidents/", eventName, "the-other-one",
	).String())

	// An Event that doesn't exist is a 404 instead, on both relations. The Event
	// in the path is resolved before anything else, so this answer comes back
	// ahead of any judgement on the rest of the request.
	requireNotFound := func(path string) {
		t.Helper()
		resp := apis.imsPost(ctx, struct{}{}, path)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		_, resp = apis.imsDelete(ctx, path, nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	}
	requireNotFound(apis.incidentTypePath("no-such-event-"+rand.NonCryptoText(), num, 1))
	requireNotFound(apis.linkedIncidentPath("no-such-event-"+rand.NonCryptoText(), num, eventName, num))
}

// The Incident named in the path has to exist, on all four endpoints. A missing
// one is a 404, even where the request would otherwise have been a no-op.
func TestIncidentRelationEndpointsOnMissingIncident(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	num := apis.newIncidentSuccess(ctx, typelessIncident(eventName))

	const missing = 9999999

	resp := apis.attachTypeToIncident(ctx, eventName, missing, 1)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apis.detachTypeFromIncident(ctx, eventName, missing, 1)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apis.linkIncident(ctx, eventName, missing, eventName, num)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apis.unlinkIncident(ctx, eventName, missing, eventName, num)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// A missing Incident on the other end of a link is different. Linking to it
	// is a 400, since the link can't be made, but unlinking from it is the same
	// no-op as unlinking anything else that isn't linked: the Incident in the
	// path already has the membership the request asked for.
	before, resp := apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())

	resp = apis.linkIncident(ctx, eventName, num, eventName, missing)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apis.unlinkIncident(ctx, eventName, num, eventName, missing)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	after, resp := apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, before.Version, after.Version)
	require.Len(t, after.ReportEntries, len(before.ReportEntries))
}

// A hidden Incident Type can still be attached and detached one at a time, which
// is what the whole-list field on the Incident API allows too. Hiding a type
// keeps it out of the picker for new use; it doesn't freeze the Incidents that
// already carry it, or stop someone from taking it off one.
func TestAttachHiddenIncidentType(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	typeName := rand.NonCryptoText()
	typeID, resp := apisAdmin.editType(ctx, imsjson.IncidentType{Name: &typeName})
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, typeID)
	_, resp = apisAdmin.editType(ctx, imsjson.IncidentType{ID: *typeID, Hidden: new(true)})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	num := apis.newIncidentSuccess(ctx, typelessIncident(eventName))

	resp = apis.attachTypeToIncident(ctx, eventName, num, *typeID)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []int32{*typeID}, *retrieved.IncidentTypeIDs)
	require.Equal(t, "Added type: "+typeName, lastReportEntryText(t, retrieved))

	resp = apis.detachTypeFromIncident(ctx, eventName, num, *typeID)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp = apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, *retrieved.IncidentTypeIDs)
	require.Equal(t, "Removed type: "+typeName, lastReportEntryText(t, retrieved))
}

// TestConcurrentIncidentTypeAttach is the regression test for setIncidentType
// under concurrency. These requests only ever take shared foreign-key locks on
// the Incident row, so there's nothing for them to deadlock on; an earlier
// version bumped the Incident's version too, and the shared-to-exclusive upgrade
// that took deadlocked in MariaDB, returning 500 for some of these requests.
// Both shapes below reproduced that reliably at this width.
func TestConcurrentIncidentTypeAttach(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	num := apis.newIncidentSuccess(ctx, typelessIncident(eventName))

	before, resp := apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())

	attachAll := func(typeIDs []int32) []int {
		t.Helper()
		codes := make([]int, len(typeIDs))
		var wg sync.WaitGroup
		for i, typeID := range typeIDs {
			wg.Go(func() {
				resp := apis.attachTypeToIncident(ctx, eventName, num, typeID)
				codes[i] = resp.StatusCode
				_ = resp.Body.Close()
			})
		}
		wg.Wait()
		return codes
	}

	// Several requests for the same type at once. They all pass the "already has
	// this membership" check, since none of them has committed yet, and then all
	// try to insert; the losers get a duplicate-key error, which is the state
	// they asked for and so is the same no-op a repeated request gets.
	for _, code := range attachAll([]int32{1, 1, 1, 1, 1, 1}) {
		require.Equal(t, http.StatusNoContent, code)
	}

	// The type is attached once, and one report entry says so: whichever requests
	// lost the race wrote nothing at all. None of them moved the Incident's
	// version, which guards only the Incident row's own columns.
	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []int32{1}, *retrieved.IncidentTypeIDs)
	require.Len(t, retrieved.ReportEntries, len(before.ReportEntries)+1)
	require.Equal(t, before.Version, retrieved.Version)

	// Concurrent requests for *different* types all stick, which is the whole
	// point of these endpoints: each request names only its own member, so
	// there's no whole list computed from a stale view and no way for one request
	// to revert another.
	typeIDs := []int32{2}
	for range 3 {
		typeName := rand.NonCryptoText()
		typeID, resp := apisAdmin.editType(ctx, imsjson.IncidentType{Name: &typeName})
		require.NoError(t, resp.Body.Close())
		require.NotNil(t, typeID)
		typeIDs = append(typeIDs, *typeID)
	}
	for _, code := range attachAll(typeIDs) {
		require.Equal(t, http.StatusNoContent, code)
	}

	retrieved, resp = apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.ElementsMatch(t, append([]int32{1}, typeIDs...), *retrieved.IncidentTypeIDs)
}

// TestConcurrentIncidentLinkAndUnlink covers requests coming in from opposite
// ends of the same pair at once. Each takes only shared foreign-key locks on the
// two Incident rows, which are compatible with each other, so the order the two
// rows are reached in doesn't matter.
func TestConcurrentIncidentLinkAndUnlink(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	num1 := apis.newIncidentSuccess(ctx, typelessIncident(eventName))
	num2 := apis.newIncidentSuccess(ctx, typelessIncident(eventName))

	// The same link, asked for from both ends at once.
	codes := make([]int, 6)
	var wg sync.WaitGroup
	for i := range codes {
		wg.Go(func() {
			from, to := num1, num2
			if i%2 == 0 {
				from, to = num2, num1
			}
			resp := apis.linkIncident(ctx, eventName, from, eventName, to)
			codes[i] = resp.StatusCode
			_ = resp.Body.Close()
		})
	}
	wg.Wait()
	for _, code := range codes {
		require.Equal(t, http.StatusNoContent, code)
	}

	// One link, recorded once on each side.
	incident1, resp := apis.getIncident(ctx, eventName, num1)
	require.NoError(t, resp.Body.Close())
	require.Len(t, *incident1.LinkedIncidents, 1)
	require.Equal(t, num2, (*incident1.LinkedIncidents)[0].Number)
	incident2, resp := apis.getIncident(ctx, eventName, num2)
	require.NoError(t, resp.Body.Close())
	require.Len(t, *incident2.LinkedIncidents, 1)

	// And taking it apart from both ends at once leaves it apart.
	wg = sync.WaitGroup{}
	for i := range codes {
		wg.Go(func() {
			from, to := num1, num2
			if i%2 == 0 {
				from, to = num2, num1
			}
			resp := apis.unlinkIncident(ctx, eventName, from, eventName, to)
			codes[i] = resp.StatusCode
			_ = resp.Body.Close()
		})
	}
	wg.Wait()
	for _, code := range codes {
		require.Equal(t, http.StatusNoContent, code)
	}

	incident1, resp = apis.getIncident(ctx, eventName, num1)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, *incident1.LinkedIncidents)
	incident2, resp = apis.getIncident(ctx, eventName, num2)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, *incident2.LinkedIncidents)
}

// Only the Event in the path is permission-checked, which matches what the
// whole-list field on the Incident API does. So a writer on one Event can link
// an Incident there to an Incident in an Event they can't even read, and the
// change lands in that other Incident's change log under their handle.
func TestLinkIncidentChecksOnlyThePathEventPermissions(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Alice is a writer on the first Event and has no access to the second. Even
	// the IMS admin needs the write grant on the second one: being an admin
	// carries the global permissions, not the per-Event ones.
	writableEvent := newEventWithWriter(t, apisAdmin)
	closedEvent := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &closedEvent})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addWriter(ctx, closedEvent, userAdminHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	writableNum := apis.newIncidentSuccess(ctx, typelessIncident(writableEvent))
	closedNum := apisAdmin.newIncidentSuccess(ctx, typelessIncident(closedEvent))

	_, resp = apis.getIncident(ctx, closedEvent, closedNum)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apis.linkIncident(ctx, writableEvent, writableNum, closedEvent, closedNum)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	linked, resp := apisAdmin.getIncident(ctx, closedEvent, closedNum)
	require.NoError(t, resp.Body.Close())
	require.Len(t, *linked.LinkedIncidents, 1)
	require.Equal(t, writableNum, (*linked.LinkedIncidents)[0].Number)
	require.Equal(t,
		fmt.Sprintf("Incident linked: %v #%v", writableEvent, writableNum),
		lastReportEntryText(t, linked),
	)

	// The same asymmetry applies in reverse: she can't reach the endpoint at all
	// with the Event she has no access to in the path, even to unlink.
	resp = apis.unlinkIncident(ctx, closedEvent, closedNum, writableEvent, writableNum)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// ...but she can unlink from her own side, and both ends come apart.
	resp = apis.unlinkIncident(ctx, writableEvent, writableNum, closedEvent, closedNum)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	linked, resp = apisAdmin.getIncident(ctx, closedEvent, closedNum)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, *linked.LinkedIncidents)
}
