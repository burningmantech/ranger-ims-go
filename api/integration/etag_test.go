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
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/burningmantech/ranger-ims-go/api"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newEventWithWriter makes a fresh event and gives Alice the Writer role on it.
func newEventWithWriter(t *testing.T, apisAdmin ApiHelper) (eventName string) {
	t.Helper()
	ctx := t.Context()
	eventName = rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	return eventName
}

func TestIncidentETagLifecycle(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	// Creation goes through the guarded update path, so a new incident's
	// version is 2 (insert at 1, then one bump), and the 201 carries its ETag.
	resp := apis.newIncident(ctx, sampleIncident1(eventName))
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.Equal(t, `"2"`, resp.Header.Get("ETag"))
	numStr := resp.Header.Get("IMS-Incident-Number")
	require.NoError(t, resp.Body.Close())
	num := parseInt32Required(t, numStr)

	// A single GET returns the same ETag, and the version in the body.
	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, `"2"`, resp.Header.Get("ETag"))
	require.Equal(t, int32(2), retrieved.Version)

	// An edit carrying the current ETag succeeds, and the 204 carries the new ETag.
	resp = apis.updateIncidentIfMatch(ctx, eventName, num, imsjson.Incident{
		Event:   eventName,
		Number:  num,
		Summary: new("an updated summary"),
	}, `"2"`)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, `"3"`, resp.Header.Get("ETag"))
	require.NoError(t, resp.Body.Close())

	retrieved, resp = apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int32(3), retrieved.Version)
	require.Equal(t, "an updated summary", *retrieved.Summary)
}

func TestIncidentEditWithStaleIfMatchIsRejected(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	num := apis.newIncidentSuccess(ctx, sampleIncident1(eventName))

	// Two dispatchers read the incident at the same version...
	_, resp := apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	etag := resp.Header.Get("ETag")
	require.NotEmpty(t, etag)

	// ...the first one's edit lands...
	resp = apis.updateIncidentIfMatch(ctx, eventName, num, imsjson.Incident{
		Event:   eventName,
		Number:  num,
		Summary: new("the first edit wins"),
	}, etag)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// ...and the second one's edit, still carrying the now-stale ETag, is
	// rejected with a 412 problem response instead of clobbering the first.
	resp = apis.updateIncidentIfMatch(ctx, eventName, num, imsjson.Incident{
		Event:   eventName,
		Number:  num,
		Summary: new("the second edit must not clobber the first"),
	}, etag)
	require.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)
	require.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
	require.NoError(t, resp.Body.Close())

	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "the first edit wins", *retrieved.Summary)
}

func TestIncidentEditWithoutIfMatchStillSucceeds(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	num := apis.newIncidentSuccess(ctx, sampleIncident1(eventName))

	// A client that predates If-Match keeps working: the server does its own
	// compare-and-swap internally instead of requiring the header.
	resp := apis.updateIncident(ctx, eventName, num, imsjson.Incident{
		Event:   eventName,
		Number:  num,
		Summary: new("edited without an If-Match header"),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NotEmpty(t, resp.Header.Get("ETag"))
	require.NoError(t, resp.Body.Close())

	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "edited without an If-Match header", *retrieved.Summary)
}

func TestIncidentEditIfMatchOnMissingIncidentIs404(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	// A version check against a nonexistent incident is a 404, not a 412.
	resp := apis.updateIncidentIfMatch(ctx, eventName, 12345, imsjson.Incident{
		Event:   eventName,
		Number:  12345,
		Summary: new("does not exist"),
	}, `"1"`)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestReportEntryAppendDoesNotBumpIncidentETag(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	num := apis.newIncidentSuccess(ctx, sampleIncident1(eventName))

	_, resp := apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	etagBefore := resp.Header.Get("ETag")
	require.NotEmpty(t, etagBefore)

	// Appending a note can't lose data, so it must not move the version and
	// thereby make other clients' edits spuriously conflict.
	resp = apis.updateIncident(ctx, eventName, num, imsjson.Incident{
		Event:         eventName,
		Number:        num,
		ReportEntries: []imsjson.ReportEntry{{Text: "just a note", ID: -1}},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, etagBefore, resp.Header.Get("ETag"))
	require.NoError(t, resp.Body.Close())

	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, etagBefore, resp.Header.Get("ETag"))
	lastEntry := retrieved.ReportEntries[len(retrieved.ReportEntries)-1]
	require.Equal(t, "just a note", lastEntry.Text)

	// A field edit carrying the pre-note ETag still succeeds, because the note
	// didn't change the version.
	resp = apis.updateIncidentIfMatch(ctx, eventName, num, imsjson.Incident{
		Event:   eventName,
		Number:  num,
		Summary: new("edited after the note"),
	}, etagBefore)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestRangerAttachBumpsIncidentETag(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	num := apis.newIncidentSuccess(ctx, sampleIncident1(eventName))

	_, resp := apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	etagBefore := resp.Header.Get("ETag")
	require.NotEmpty(t, etagBefore)

	// A roster change moves the incident's version, and the response carries
	// the new ETag so the client can keep its cached value fresh.
	resp = apis.attachRangerToIncident(ctx, eventName, num, "Some Dude")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	etagAfterAttach := resp.Header.Get("ETag")
	require.NotEmpty(t, etagAfterAttach)
	require.NotEqual(t, etagBefore, etagAfterAttach)
	require.NoError(t, resp.Body.Close())

	// An edit still carrying the pre-attach ETag is rejected.
	resp = apis.updateIncidentIfMatch(ctx, eventName, num, imsjson.Incident{
		Event:   eventName,
		Number:  num,
		Summary: new("stale after roster change"),
	}, etagBefore)
	require.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Detaching moves it again.
	resp = apis.detachRangerFromIncident(ctx, eventName, num, "Some Dude")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	etagAfterDetach := resp.Header.Get("ETag")
	require.NotEmpty(t, etagAfterDetach)
	require.NotEqual(t, etagAfterAttach, etagAfterDetach)
	require.NoError(t, resp.Body.Close())
}

func TestFieldReportETagAndLinkBumpsBothVersions(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	// A field report is created directly, so it starts at version 1.
	resp := apis.newFieldReport(ctx, imsjson.FieldReport{Event: eventName, Summary: new("an FR")})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.Equal(t, `"1"`, resp.Header.Get("ETag"))
	frNum := parseInt32Required(t, resp.Header.Get("IMS-Field-Report-Number"))
	require.NoError(t, resp.Body.Close())

	fr, resp := apis.getFieldReport(ctx, eventName, frNum)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, `"1"`, resp.Header.Get("ETag"))
	require.Equal(t, int32(1), fr.Version)

	// A stale If-Match on a field report edit is rejected.
	resp = apis.updateFieldReportIfMatch(ctx, eventName, frNum, imsjson.FieldReport{
		Event:   eventName,
		Number:  frNum,
		Summary: new("edited summary"),
	}, `"1"`)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, `"2"`, resp.Header.Get("ETag"))
	require.NoError(t, resp.Body.Close())
	resp = apis.updateFieldReportIfMatch(ctx, eventName, frNum, imsjson.FieldReport{
		Event:   eventName,
		Number:  frNum,
		Summary: new("stale edit"),
	}, `"1"`)
	require.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Attaching the field report to an incident bumps both records' versions.
	incidentNum := apis.newIncidentSuccess(ctx, sampleIncident1(eventName))
	incidentBefore, resp := apis.getIncident(ctx, eventName, incidentNum)
	require.NoError(t, resp.Body.Close())

	resp = apis.attachFieldReportToIncident(ctx, eventName, frNum, incidentNum)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	frAfter, resp := apis.getFieldReport(ctx, eventName, frNum)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Greater(t, frAfter.Version, fr.Version)

	incidentAfter, resp := apis.getIncident(ctx, eventName, incidentNum)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Greater(t, incidentAfter.Version, incidentBefore.Version)
}

func TestVisitETagAndReassignmentBumpsIncidents(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	resp := apisAdmin.addVisitWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addWriter(ctx, eventName, userAdminHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Creation goes through the guarded update path, so a new visit's version
	// is 2, matching incidents.
	resp = apis.newVisit(ctx, imsjson.Visit{
		Event:              eventName,
		GuestPreferredName: new("A. Guest"),
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.Equal(t, `"2"`, resp.Header.Get("ETag"))
	visitNum := parseInt32Required(t, resp.Header.Get("IMS-Visit-Number"))
	require.NoError(t, resp.Body.Close())

	// An edit with the current ETag succeeds; repeating it with the stale one fails.
	resp = apis.updateVisitIfMatch(ctx, eventName, visitNum, imsjson.Visit{
		Event:            eventName,
		Number:           visitNum,
		GuestDescription: new("wearing a big hat"),
	}, `"2"`)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, `"3"`, resp.Header.Get("ETag"))
	require.NoError(t, resp.Body.Close())
	resp = apis.updateVisitIfMatch(ctx, eventName, visitNum, imsjson.Visit{
		Event:            eventName,
		Number:           visitNum,
		GuestDescription: new("stale edit"),
	}, `"2"`)
	require.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Assigning the visit to an incident bumps the incident's version too.
	// Incident operations use the admin here: granting Alice VisitWriter above
	// replaced her person-expression access rules, including her Writer role.
	incidentNum := apisAdmin.newIncidentSuccess(ctx, sampleIncident1(eventName))
	incidentBefore, resp := apisAdmin.getIncident(ctx, eventName, incidentNum)
	require.NoError(t, resp.Body.Close())

	resp = apis.updateVisit(ctx, eventName, visitNum, imsjson.Visit{
		Event:    eventName,
		Number:   visitNum,
		Incident: &incidentNum,
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	incidentAfter, resp := apisAdmin.getIncident(ctx, eventName, incidentNum)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Greater(t, incidentAfter.Version, incidentBefore.Version)
}

// casInterceptor wraps the sqlc Querier so a test can act in a race window:
// between an edit's read of the stored record and its version-guarded UPDATE
// (the interleaving where a concurrent writer causes a CAS conflict), or
// between a creation's number allocation and its INSERT (the interleaving
// where a concurrent creator claims the same number). A nil hook leaves the
// corresponding query untouched.
type casInterceptor struct {
	imsdb.Querier

	beforeUpdateIncident    func(ctx context.Context, arg imsdb.UpdateIncidentParams)
	beforeUpdateFieldReport func(ctx context.Context, arg imsdb.UpdateFieldReportParams)
	beforeUpdateVisit       func(ctx context.Context, arg imsdb.UpdateVisitParams)
	beforeCreateIncident    func(ctx context.Context, arg imsdb.CreateIncidentParams)
	beforeCreateFieldReport func(ctx context.Context, arg imsdb.CreateFieldReportParams)
	beforeCreateVisit       func(ctx context.Context, arg imsdb.CreateVisitParams)
}

func (q casInterceptor) UpdateIncident(ctx context.Context, db imsdb.DBTX, arg imsdb.UpdateIncidentParams) (int64, error) {
	if q.beforeUpdateIncident != nil {
		q.beforeUpdateIncident(ctx, arg)
	}
	return q.Querier.UpdateIncident(ctx, db, arg)
}

func (q casInterceptor) UpdateFieldReport(ctx context.Context, db imsdb.DBTX, arg imsdb.UpdateFieldReportParams) (int64, error) {
	if q.beforeUpdateFieldReport != nil {
		q.beforeUpdateFieldReport(ctx, arg)
	}
	return q.Querier.UpdateFieldReport(ctx, db, arg)
}

func (q casInterceptor) UpdateVisit(ctx context.Context, db imsdb.DBTX, arg imsdb.UpdateVisitParams) (int64, error) {
	if q.beforeUpdateVisit != nil {
		q.beforeUpdateVisit(ctx, arg)
	}
	return q.Querier.UpdateVisit(ctx, db, arg)
}

// interceptedServer starts a second in-process IMS server on the shared
// database, identical to the shared one except for the interceptor's hooks.
func interceptedServer(t *testing.T, interceptor casInterceptor) *url.URL {
	t.Helper()
	interceptor.Querier = imsdb.New()
	dbq := store.NewDBQ(shared.imsDBQ.DB, interceptor)
	server := httptest.NewServer(
		api.AddToMux(nil, shared.es, shared.cfg, dbq, shared.userStore, nil, shared.actionLogger),
	)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	return serverURL
}

// The bump helpers below commit a version bump for the record being updated,
// simulating another writer's edit landing first. They assert rather than
// require, since they run on the server's request goroutine.

func bumpIncidentVersion(ctx context.Context, t *testing.T, event, number int32) {
	t.Helper()
	err := shared.imsDBQ.BumpIncidentVersion(ctx, shared.imsDBQ, imsdb.BumpIncidentVersionParams{
		Event:  event,
		Number: number,
	})
	assert.NoError(t, err)
}

func bumpFieldReportVersion(ctx context.Context, t *testing.T, event, number int32) {
	t.Helper()
	// No production code path needs a bare field report bump, so there's no
	// sqlc query for it; do the competing write directly.
	_, err := shared.imsDBQ.ExecContext(ctx,
		"update FIELD_REPORT set VERSION = VERSION + 1 where EVENT = ? and NUMBER = ?", event, number)
	assert.NoError(t, err)
}

func bumpVisitVersion(ctx context.Context, t *testing.T, event, number int32) {
	t.Helper()
	err := shared.imsDBQ.BumpVisitVersion(ctx, shared.imsDBQ, imsdb.BumpVisitVersionParams{
		Event:  event,
		Number: number,
	})
	assert.NoError(t, err)
}

func TestIncidentEditRetriesPastConcurrentWrite(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	num := apis.newIncidentSuccess(ctx, sampleIncident1(eventName))

	// Another writer's edit commits after this edit has read the incident, but
	// before its guarded UPDATE runs, so the first attempt is a CAS conflict.
	var updateAttempts atomic.Int32
	hookedURL := interceptedServer(t, casInterceptor{
		beforeUpdateIncident: func(ctx context.Context, arg imsdb.UpdateIncidentParams) {
			if updateAttempts.Add(1) == 1 {
				bumpIncidentVersion(ctx, t, arg.Event, arg.Number)
			}
		},
	})
	hookedApis := ApiHelper{t: t, serverURL: hookedURL, jwt: apis.jwt}

	// Without an If-Match header, the server retries internally and the edit
	// still lands, on the second attempt.
	resp := hookedApis.updateIncident(ctx, eventName, num, imsjson.Incident{
		Event:   eventName,
		Number:  num,
		Summary: new("landed on the retry"),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	// Version 2 at creation, 3 after the competing write, 4 after this edit.
	require.Equal(t, `"4"`, resp.Header.Get("ETag"))
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int32(2), updateAttempts.Load())

	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "landed on the retry", *retrieved.Summary)
	require.Equal(t, int32(4), retrieved.Version)
}

func TestIncidentEditGivesUpAfterRepeatedConcurrentWrites(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	num := apis.newIncidentSuccess(ctx, sampleIncident1(eventName))

	// A competing write lands inside the race window on every attempt.
	var updateAttempts atomic.Int32
	hookedURL := interceptedServer(t, casInterceptor{
		beforeUpdateIncident: func(ctx context.Context, arg imsdb.UpdateIncidentParams) {
			updateAttempts.Add(1)
			bumpIncidentVersion(ctx, t, arg.Event, arg.Number)
		},
	})
	hookedApis := ApiHelper{t: t, serverURL: hookedURL, jwt: apis.jwt}

	resp := hookedApis.updateIncident(ctx, eventName, num, imsjson.Incident{
		Event:   eventName,
		Number:  num,
		Summary: new("this edit must not land"),
	})
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	require.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
	require.NoError(t, resp.Body.Close())
	// The server exhausts its CAS retry budget, then gives up.
	require.Equal(t, int32(3), updateAttempts.Load())

	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, *sampleIncident1(eventName).Summary, *retrieved.Summary)
}

func TestIncidentEditIfMatchLosingRaceIs412(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	num := apis.newIncidentSuccess(ctx, sampleIncident1(eventName))

	_, resp := apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	etag := resp.Header.Get("ETag")
	require.NotEmpty(t, etag)

	// The If-Match precondition passes against the version this edit read, but
	// a competing write commits before the guarded UPDATE runs.
	var updateAttempts atomic.Int32
	hookedURL := interceptedServer(t, casInterceptor{
		beforeUpdateIncident: func(ctx context.Context, arg imsdb.UpdateIncidentParams) {
			if updateAttempts.Add(1) == 1 {
				bumpIncidentVersion(ctx, t, arg.Event, arg.Number)
			}
		},
	})
	hookedApis := ApiHelper{t: t, serverURL: hookedURL, jwt: apis.jwt}

	// An edit carrying If-Match gets no retries: the conflict must be reported
	// back as a 412 so the client can re-fetch and re-apply its change.
	resp = hookedApis.updateIncidentIfMatch(ctx, eventName, num, imsjson.Incident{
		Event:   eventName,
		Number:  num,
		Summary: new("this edit must not land"),
	}, etag)
	require.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)
	require.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int32(1), updateAttempts.Load())

	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, *sampleIncident1(eventName).Summary, *retrieved.Summary)
}

func TestFieldReportEditRetriesPastConcurrentWrite(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	num := apis.newFieldReportSuccess(ctx, imsjson.FieldReport{Event: eventName, Summary: new("original summary")})

	// Another writer's edit commits after this edit has read the field report,
	// but before its guarded UPDATE runs, so the first attempt is a CAS conflict.
	var updateAttempts atomic.Int32
	hookedURL := interceptedServer(t, casInterceptor{
		beforeUpdateFieldReport: func(ctx context.Context, arg imsdb.UpdateFieldReportParams) {
			if updateAttempts.Add(1) == 1 {
				bumpFieldReportVersion(ctx, t, arg.Event, arg.Number)
			}
		},
	})
	hookedApis := ApiHelper{t: t, serverURL: hookedURL, jwt: apis.jwt}

	// Without an If-Match header, the server retries internally and the edit
	// still lands, on the second attempt.
	resp := hookedApis.updateFieldReport(ctx, eventName, num, imsjson.FieldReport{
		Event:   eventName,
		Number:  num,
		Summary: new("landed on the retry"),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	// Version 1 at creation, 2 after the competing write, 3 after this edit.
	require.Equal(t, `"3"`, resp.Header.Get("ETag"))
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int32(2), updateAttempts.Load())

	retrieved, resp := apis.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "landed on the retry", *retrieved.Summary)
	require.Equal(t, int32(3), retrieved.Version)
}

func TestFieldReportEditGivesUpAfterRepeatedConcurrentWrites(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	num := apis.newFieldReportSuccess(ctx, imsjson.FieldReport{Event: eventName, Summary: new("original summary")})

	// A competing write lands inside the race window on every attempt.
	var updateAttempts atomic.Int32
	hookedURL := interceptedServer(t, casInterceptor{
		beforeUpdateFieldReport: func(ctx context.Context, arg imsdb.UpdateFieldReportParams) {
			updateAttempts.Add(1)
			bumpFieldReportVersion(ctx, t, arg.Event, arg.Number)
		},
	})
	hookedApis := ApiHelper{t: t, serverURL: hookedURL, jwt: apis.jwt}

	resp := hookedApis.updateFieldReport(ctx, eventName, num, imsjson.FieldReport{
		Event:   eventName,
		Number:  num,
		Summary: new("this edit must not land"),
	})
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	require.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
	require.NoError(t, resp.Body.Close())
	// The server exhausts its CAS retry budget, then gives up.
	require.Equal(t, int32(3), updateAttempts.Load())

	retrieved, resp := apis.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "original summary", *retrieved.Summary)
}

func TestFieldReportEditIfMatchLosingRaceIs412(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	num := apis.newFieldReportSuccess(ctx, imsjson.FieldReport{Event: eventName, Summary: new("original summary")})

	_, resp := apis.getFieldReport(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	etag := resp.Header.Get("ETag")
	require.NotEmpty(t, etag)

	// The If-Match precondition passes against the version this edit read, but
	// a competing write commits before the guarded UPDATE runs.
	var updateAttempts atomic.Int32
	hookedURL := interceptedServer(t, casInterceptor{
		beforeUpdateFieldReport: func(ctx context.Context, arg imsdb.UpdateFieldReportParams) {
			if updateAttempts.Add(1) == 1 {
				bumpFieldReportVersion(ctx, t, arg.Event, arg.Number)
			}
		},
	})
	hookedApis := ApiHelper{t: t, serverURL: hookedURL, jwt: apis.jwt}

	// An edit carrying If-Match gets no retries: the conflict must be reported
	// back as a 412 so the client can re-fetch and re-apply its change.
	resp = hookedApis.updateFieldReportIfMatch(ctx, eventName, num, imsjson.FieldReport{
		Event:   eventName,
		Number:  num,
		Summary: new("this edit must not land"),
	}, etag)
	require.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)
	require.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int32(1), updateAttempts.Load())

	retrieved, resp := apis.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "original summary", *retrieved.Summary)
}

// newEventWithVisitWriter makes a fresh event and gives Alice the VisitWriter
// role on it. That role is granted instead of Writer, not in addition: setting
// it replaces Alice's person-expression access rules for the event.
func newEventWithVisitWriter(t *testing.T, apisAdmin ApiHelper) (eventName string) {
	t.Helper()
	ctx := t.Context()
	eventName = rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addVisitWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	return eventName
}

func TestVisitEditRetriesPastConcurrentWrite(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithVisitWriter(t, apisAdmin)
	num := apis.newVisitSuccess(ctx, imsjson.Visit{
		Event:            eventName,
		GuestDescription: new("original description"),
	})

	// Another writer's edit commits after this edit has read the visit, but
	// before its guarded UPDATE runs, so the first attempt is a CAS conflict.
	var updateAttempts atomic.Int32
	hookedURL := interceptedServer(t, casInterceptor{
		beforeUpdateVisit: func(ctx context.Context, arg imsdb.UpdateVisitParams) {
			if updateAttempts.Add(1) == 1 {
				bumpVisitVersion(ctx, t, arg.Event, arg.Number)
			}
		},
	})
	hookedApis := ApiHelper{t: t, serverURL: hookedURL, jwt: apis.jwt}

	// Without an If-Match header, the server retries internally and the edit
	// still lands, on the second attempt.
	resp := hookedApis.updateVisit(ctx, eventName, num, imsjson.Visit{
		Event:            eventName,
		Number:           num,
		GuestDescription: new("landed on the retry"),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	// Version 2 at creation, 3 after the competing write, 4 after this edit.
	require.Equal(t, `"4"`, resp.Header.Get("ETag"))
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int32(2), updateAttempts.Load())

	retrieved, resp := apis.getVisit(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "landed on the retry", *retrieved.GuestDescription)
	require.Equal(t, int32(4), retrieved.Version)
}

func TestVisitEditGivesUpAfterRepeatedConcurrentWrites(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithVisitWriter(t, apisAdmin)
	num := apis.newVisitSuccess(ctx, imsjson.Visit{
		Event:            eventName,
		GuestDescription: new("original description"),
	})

	// A competing write lands inside the race window on every attempt.
	var updateAttempts atomic.Int32
	hookedURL := interceptedServer(t, casInterceptor{
		beforeUpdateVisit: func(ctx context.Context, arg imsdb.UpdateVisitParams) {
			updateAttempts.Add(1)
			bumpVisitVersion(ctx, t, arg.Event, arg.Number)
		},
	})
	hookedApis := ApiHelper{t: t, serverURL: hookedURL, jwt: apis.jwt}

	resp := hookedApis.updateVisit(ctx, eventName, num, imsjson.Visit{
		Event:            eventName,
		Number:           num,
		GuestDescription: new("this edit must not land"),
	})
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	require.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
	require.NoError(t, resp.Body.Close())
	// The server exhausts its CAS retry budget, then gives up.
	require.Equal(t, int32(3), updateAttempts.Load())

	retrieved, resp := apis.getVisit(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "original description", *retrieved.GuestDescription)
}

func TestVisitEditIfMatchLosingRaceIs412(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithVisitWriter(t, apisAdmin)
	num := apis.newVisitSuccess(ctx, imsjson.Visit{
		Event:            eventName,
		GuestDescription: new("original description"),
	})

	_, resp := apis.getVisit(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	etag := resp.Header.Get("ETag")
	require.NotEmpty(t, etag)

	// The If-Match precondition passes against the version this edit read, but
	// a competing write commits before the guarded UPDATE runs.
	var updateAttempts atomic.Int32
	hookedURL := interceptedServer(t, casInterceptor{
		beforeUpdateVisit: func(ctx context.Context, arg imsdb.UpdateVisitParams) {
			if updateAttempts.Add(1) == 1 {
				bumpVisitVersion(ctx, t, arg.Event, arg.Number)
			}
		},
	})
	hookedApis := ApiHelper{t: t, serverURL: hookedURL, jwt: apis.jwt}

	// An edit carrying If-Match gets no retries: the conflict must be reported
	// back as a 412 so the client can re-fetch and re-apply its change.
	resp = hookedApis.updateVisitIfMatch(ctx, eventName, num, imsjson.Visit{
		Event:            eventName,
		Number:           num,
		GuestDescription: new("this edit must not land"),
	}, etag)
	require.Equal(t, http.StatusPreconditionFailed, resp.StatusCode)
	require.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int32(1), updateAttempts.Load())

	retrieved, resp := apis.getVisit(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "original description", *retrieved.GuestDescription)
}

func parseInt32Required(t *testing.T, s string) int32 {
	t.Helper()
	require.NotEmpty(t, s)
	num, err := conv.ParseInt32(s)
	require.NoError(t, err)
	require.Positive(t, num)
	return num
}
