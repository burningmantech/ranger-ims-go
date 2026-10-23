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
	"net/url"
	"testing"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newJunkTestEvent makes an Event on which Alice is a writer.
func newJunkTestEvent(t *testing.T, admin ApiHelper) string {
	t.Helper()
	eventName := rand.NonCryptoText()
	_, resp := admin.createEvent(t.Context(), imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = admin.addWriter(t.Context(), eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	return eventName
}

func systemEntryTexts(entries []imsjson.ReportEntry) []string {
	var texts []string
	for _, entry := range entries {
		if entry.SystemEntry {
			texts = append(texts, entry.Text)
		}
	}
	return texts
}

func TestJunkIncident(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	aliceUser := srv.alice(ctx)
	eventName := newJunkTestEvent(t, srv.admin(ctx))

	num := aliceUser.newIncidentSuccess(ctx, imsjson.Incident{Event: eventName, State: "new"})

	// A new Incident isn't junk.
	incident, resp := aliceUser.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, incident.Junk)
	assert.False(t, *incident.Junk)
	versionBefore := incident.Version

	resp = aliceUser.updateIncident(ctx, eventName, num, imsjson.Incident{Junk: new(true)})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	incident, resp = aliceUser.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, incident.Junk)
	assert.True(t, *incident.Junk)
	assert.Equal(t, versionBefore+1, incident.Version)
	assert.Contains(t, systemEntryTexts(incident.ReportEntries), "Marked as junk")

	// The list still returns it, flagged, for the page to filter.
	items, resp := aliceUser.queryIncidents(ctx, eventName, url.Values{"number": {conv.FormatInt(num)}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, items, 1)
	assert.True(t, items[0].Junk)

	// An edit that doesn't mention junk leaves it alone.
	resp = aliceUser.updateIncident(ctx, eventName, num, imsjson.Incident{Summary: new("still junk")})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	incident, resp = aliceUser.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.True(t, *incident.Junk)

	resp = aliceUser.updateIncident(ctx, eventName, num, imsjson.Incident{Junk: new(false)})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	incident, resp = aliceUser.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.False(t, *incident.Junk)
	assert.Contains(t, systemEntryTexts(incident.ReportEntries), "Marked as not junk")
}

func TestJunkFieldReport(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	aliceUser := srv.alice(ctx)
	eventName := newJunkTestEvent(t, srv.admin(ctx))

	num := aliceUser.newFieldReportSuccess(ctx, imsjson.FieldReport{Event: eventName, Summary: new("oops")})

	fieldReport, resp := aliceUser.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, fieldReport.Junk)
	assert.False(t, *fieldReport.Junk)
	versionBefore := fieldReport.Version

	resp = aliceUser.updateFieldReport(ctx, eventName, num, imsjson.FieldReport{Number: num, Junk: new(true)})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	fieldReport, resp = aliceUser.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, fieldReport.Junk)
	assert.True(t, *fieldReport.Junk)
	assert.Equal(t, versionBefore+1, fieldReport.Version)
	assert.Equal(t, "oops", *fieldReport.Summary)
	assert.Contains(t, systemEntryTexts(fieldReport.ReportEntries), "Marked as junk")

	items, resp := aliceUser.queryFieldReports(ctx, eventName, url.Values{"number": {conv.FormatInt(num)}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, items, 1)
	assert.True(t, items[0].Junk)

	resp = aliceUser.updateFieldReport(ctx, eventName, num, imsjson.FieldReport{Number: num, Junk: new(false)})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	fieldReport, resp = aliceUser.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.False(t, *fieldReport.Junk)
	assert.Contains(t, systemEntryTexts(fieldReport.ReportEntries), "Marked as not junk")
}

func TestJunkVisit(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	aliceUser := srv.alice(ctx)
	eventName := newJunkTestEvent(t, srv.admin(ctx))

	num := aliceUser.newVisitSuccess(ctx, imsjson.Visit{Event: eventName, GuestPreferredName: new("Dupe")})

	visit, resp := aliceUser.getVisit(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, visit.Junk)
	assert.False(t, *visit.Junk)
	versionBefore := visit.Version

	resp = aliceUser.updateVisit(ctx, eventName, num, imsjson.Visit{Junk: new(true)})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	visit, resp = aliceUser.getVisit(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, visit.Junk)
	assert.True(t, *visit.Junk)
	assert.Equal(t, versionBefore+1, visit.Version)
	assert.Equal(t, "Dupe", *visit.GuestPreferredName)
	assert.Contains(t, systemEntryTexts(visit.ReportEntries), "Marked as junk")

	items, resp := aliceUser.queryVisits(ctx, eventName, url.Values{"number": {conv.FormatInt(num)}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, items, 1)
	assert.True(t, items[0].Junk)

	resp = aliceUser.updateVisit(ctx, eventName, num, imsjson.Visit{Junk: new(false)})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	visit, resp = aliceUser.getVisit(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.False(t, *visit.Junk)
	assert.Contains(t, systemEntryTexts(visit.ReportEntries), "Marked as not junk")
}

func TestSearchLeavesOutJunk(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	aliceUser := srv.alice(ctx)
	eventName := newJunkTestEvent(t, srv.admin(ctx))

	token := "junktok" + rand.NonCryptoText()

	incidentNumber := aliceUser.newIncidentSuccess(ctx, imsjson.Incident{
		Event:   eventName,
		State:   "new",
		Summary: new("Incident " + token),
	})
	fieldReportNumber := aliceUser.newFieldReportSuccess(ctx, imsjson.FieldReport{
		Event:   eventName,
		Summary: new("Field Report " + token),
	})
	visitNumber := aliceUser.newVisitSuccess(ctx, imsjson.Visit{
		Event:              eventName,
		GuestPreferredName: new("Guest " + token),
	})

	results, resp := aliceUser.search(ctx, url.Values{"q": {token}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Len(t, results.Hits, 3)

	resp = aliceUser.updateIncident(ctx, eventName, incidentNumber, imsjson.Incident{Junk: new(true)})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.updateFieldReport(ctx, eventName, fieldReportNumber, imsjson.FieldReport{Number: fieldReportNumber, Junk: new(true)})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = aliceUser.updateVisit(ctx, eventName, visitNumber, imsjson.Visit{Junk: new(true)})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Junk is left out by default.
	results, resp = aliceUser.search(ctx, url.Values{"q": {token}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Empty(t, results.Hits)

	// junk=true brings it back, flagged.
	results, resp = aliceUser.search(ctx, url.Values{"q": {token}, "junk": {"true"}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, results.Hits, 3)
	for _, hit := range results.Hits {
		assert.True(t, hit.Junk, hit.Resource)
	}

	_, resp = aliceUser.search(ctx, url.Values{"q": {token}, "junk": {"maybe"}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}
