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

// incidentVersion reads an Incident's optimistic-concurrency version, which the
// API reports in the record body.
func incidentVersion(ctx context.Context, t *testing.T, apis ApiHelper, eventName string, number int32) int32 {
	t.Helper()
	retrieved, resp := apis.getIncident(ctx, eventName, number)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	return retrieved.Version
}

func fieldReportVersion(ctx context.Context, t *testing.T, apis ApiHelper, eventName string, number int32) int32 {
	t.Helper()
	retrieved, resp := apis.getFieldReport(ctx, eventName, number)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	return retrieved.Version
}

func TestIncidentVersionLifecycle(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	// Creation goes through the guarded update path, so a new incident's
	// version is 2: the insert lands at 1, then one bump.
	num := apis.newIncidentSuccess(ctx, sampleIncident1(eventName))
	require.Equal(t, int32(2), incidentVersion(ctx, t, apis, eventName, num))

	// A field edit moves it.
	resp := apis.updateIncident(ctx, eventName, num, imsjson.Incident{
		Event:   eventName,
		Number:  num,
		Summary: new("an updated summary"),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int32(3), retrieved.Version)
	require.Equal(t, "an updated summary", *retrieved.Summary)
}

func TestReportEntryAppendDoesNotBumpIncidentVersion(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	num := apis.newIncidentSuccess(ctx, sampleIncident1(eventName))
	versionBefore := incidentVersion(ctx, t, apis, eventName, num)

	// Appending a note can't lose data, so it must not move the version and
	// thereby make a concurrent field edit retry for nothing.
	resp := apis.updateIncident(ctx, eventName, num, imsjson.Incident{
		Event:         eventName,
		Number:        num,
		ReportEntries: []imsjson.ReportEntry{{Text: "just a note", ID: -1}},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, versionBefore, retrieved.Version)
	lastEntry := retrieved.ReportEntries[len(retrieved.ReportEntries)-1]
	require.Equal(t, "just a note", lastEntry.Text)
}

func TestRangerRosterDoesNotBumpIncidentVersion(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	num := apis.newIncidentSuccess(ctx, sampleIncident1(eventName))
	versionBefore := incidentVersion(ctx, t, apis, eventName, num)

	// The roster lives in its own table, so no field edit can clobber it. Moving
	// the version would only make a concurrent field edit retry for nothing.
	resp := apis.attachRangerToIncident(ctx, eventName, num, "Some Dude")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, versionBefore, incidentVersion(ctx, t, apis, eventName, num))

	resp = apis.detachRangerFromIncident(ctx, eventName, num, "Some Dude")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, versionBefore, incidentVersion(ctx, t, apis, eventName, num))

	// The roster changes themselves still landed.
	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, *retrieved.Rangers)
}

func TestFieldReportVersionLifecycle(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	// A field report is created directly, so it starts at version 1.
	frNum := apis.newFieldReportSuccess(ctx, imsjson.FieldReport{Event: eventName, Summary: new("an FR")})
	require.Equal(t, int32(1), fieldReportVersion(ctx, t, apis, eventName, frNum))

	resp := apis.updateFieldReport(ctx, eventName, frNum, imsjson.FieldReport{
		Event:   eventName,
		Number:  frNum,
		Summary: new("edited summary"),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int32(2), fieldReportVersion(ctx, t, apis, eventName, frNum))
}

func TestFieldReportAttachDoesNotBumpIncidentVersion(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	frNum := apis.newFieldReportSuccess(ctx, imsjson.FieldReport{Event: eventName, Summary: new("an FR")})
	incidentNum := apis.newIncidentSuccess(ctx, sampleIncident1(eventName))
	incidentBefore := incidentVersion(ctx, t, apis, eventName, incidentNum)

	// The attachment is stored on the Field Report, so the Incident's own
	// columns don't change and its version stays put.
	resp := apis.attachFieldReportToIncident(ctx, eventName, frNum, incidentNum)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, incidentBefore, incidentVersion(ctx, t, apis, eventName, incidentNum))

	retrieved, resp := apis.getIncident(ctx, eventName, incidentNum)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []int32{frNum}, *retrieved.FieldReports)
}

func TestIncidentTypeAttachDoesNotBumpIncidentVersion(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	num := apis.newIncidentSuccess(ctx, typelessIncident(eventName))
	versionBefore := incidentVersion(ctx, t, apis, eventName, num)

	// Type membership lives in its own table, so a field edit can't clobber it
	// and the version has no reason to move.
	resp := apis.attachTypeToIncident(ctx, eventName, num, 1)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, versionBefore, incidentVersion(ctx, t, apis, eventName, num))

	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []int32{1}, *retrieved.IncidentTypeIDs)

	// A repeated attach is a no-op, as is a detach of an absent type. That is
	// what makes these requests safe to retry.
	resp = apis.attachTypeToIncident(ctx, eventName, num, 1)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apis.detachTypeFromIncident(ctx, eventName, num, 1)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apis.detachTypeFromIncident(ctx, eventName, num, 1)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, versionBefore, incidentVersion(ctx, t, apis, eventName, num))
}

func TestIncidentLinkEndpointDoesNotBumpEitherVersion(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)

	num1 := apis.newIncidentSuccess(ctx, typelessIncident(eventName))
	num2 := apis.newIncidentSuccess(ctx, typelessIncident(eventName))
	before1 := incidentVersion(ctx, t, apis, eventName, num1)
	before2 := incidentVersion(ctx, t, apis, eventName, num2)

	// A link is stored in LINKED_INCIDENT, so neither Incident row changes and
	// neither version moves.
	resp := apis.linkIncident(ctx, eventName, num1, eventName, num2)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, before1, incidentVersion(ctx, t, apis, eventName, num1))
	require.Equal(t, before2, incidentVersion(ctx, t, apis, eventName, num2))

	retrieved, resp := apis.getIncident(ctx, eventName, num1)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, *retrieved.LinkedIncidents, 1)
	require.Equal(t, num2, (*retrieved.LinkedIncidents)[0].Number)

	// A repeated link is a no-op on both, and unlinking leaves the versions
	// alone too.
	resp = apis.linkIncident(ctx, eventName, num1, eventName, num2)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	resp = apis.unlinkIncident(ctx, eventName, num1, eventName, num2)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, before1, incidentVersion(ctx, t, apis, eventName, num1))
	require.Equal(t, before2, incidentVersion(ctx, t, apis, eventName, num2))
}

func TestVisitVersionLifecycleAndReassignment(t *testing.T) {
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
	visitNum := apis.newVisitSuccess(ctx, imsjson.Visit{
		Event:              eventName,
		GuestPreferredName: new("A. Guest"),
	})
	retrieved, resp := apis.getVisit(ctx, eventName, visitNum)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int32(2), retrieved.Version)

	resp = apis.updateVisit(ctx, eventName, visitNum, imsjson.Visit{
		Event:            eventName,
		Number:           visitNum,
		GuestDescription: new("wearing a big hat"),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	retrieved, resp = apis.getVisit(ctx, eventName, visitNum)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int32(3), retrieved.Version)

	// Assigning the visit to an incident is stored on the visit, so the
	// incident's version stays put. Incident operations use the admin here:
	// granting Alice VisitWriter above replaced her person-expression access
	// rules, including her Writer role.
	incidentNum := apisAdmin.newIncidentSuccess(ctx, sampleIncident1(eventName))
	incidentBefore := incidentVersion(ctx, t, apisAdmin, eventName, incidentNum)

	resp = apis.updateVisit(ctx, eventName, visitNum, imsjson.Visit{
		Event:    eventName,
		Number:   visitNum,
		Incident: &incidentNum,
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	require.Equal(t, incidentBefore, incidentVersion(ctx, t, apisAdmin, eventName, incidentNum))

	retrievedIncident, resp := apisAdmin.getIncident(ctx, eventName, incidentNum)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []int32{visitNum}, *retrievedIncident.Visits)
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
		api.AddToMux(nil, shared.es, shared.cfg, dbq, shared.userStore, nil, shared.actionLogger, shared.errorLogger),
	)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	return serverURL
}

// The bump helpers below commit a version bump for the record being updated,
// simulating another writer's edit landing first. No production code path bumps
// a version on its own — only the guarded UPDATEs move it — so there are no sqlc
// queries for these; the competing writes go out directly. They assert rather
// than require, since they run on the server's request goroutine.

func bumpIncidentVersion(ctx context.Context, t *testing.T, event, number int32) {
	t.Helper()
	_, err := shared.imsDBQ.ExecContext(ctx,
		"update INCIDENT set VERSION = VERSION + 1 where EVENT = ? and NUMBER = ?", event, number)
	assert.NoError(t, err)
}

func bumpFieldReportVersion(ctx context.Context, t *testing.T, event, number int32) {
	t.Helper()
	_, err := shared.imsDBQ.ExecContext(ctx,
		"update FIELD_REPORT set VERSION = VERSION + 1 where EVENT = ? and NUMBER = ?", event, number)
	assert.NoError(t, err)
}

func bumpVisitVersion(ctx context.Context, t *testing.T, event, number int32) {
	t.Helper()
	_, err := shared.imsDBQ.ExecContext(ctx,
		"update VISIT set VERSION = VERSION + 1 where EVENT = ? and NUMBER = ?", event, number)
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

	// The server retries the read-merge-write internally, so the edit still
	// lands, on the second attempt.
	resp := hookedApis.updateIncident(ctx, eventName, num, imsjson.Incident{
		Event:   eventName,
		Number:  num,
		Summary: new("landed on the retry"),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
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

	// The server retries the read-merge-write internally, so the edit still
	// lands, on the second attempt.
	resp := hookedApis.updateFieldReport(ctx, eventName, num, imsjson.FieldReport{
		Event:   eventName,
		Number:  num,
		Summary: new("landed on the retry"),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
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

	// The server retries the read-merge-write internally, so the edit still
	// lands, on the second attempt.
	resp := hookedApis.updateVisit(ctx, eventName, num, imsjson.Visit{
		Event:            eventName,
		Number:           num,
		GuestDescription: new("landed on the retry"),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
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
