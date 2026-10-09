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
	"net/url"
	"testing"
	"time"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireIncidentSearch runs an Incidents list search and checks which
// Incidents it returned.
func requireIncidentSearch(ctx context.Context, t *testing.T, apis ApiHelper, eventName, q string, want ...int32) {
	t.Helper()
	items, resp := apis.queryIncidents(ctx, eventName, url.Values{"q": {q}})
	require.Equal(t, http.StatusOK, resp.StatusCode, q)
	require.NoError(t, resp.Body.Close())
	var got []int32
	for _, item := range items {
		got = append(got, item.Number)
	}
	require.ElementsMatch(t, want, got, q)
}

// requireFieldReportSearch is requireIncidentSearch for Field Reports.
func requireFieldReportSearch(ctx context.Context, t *testing.T, apis ApiHelper, eventName, q string, want ...int32) {
	t.Helper()
	items, resp := apis.queryFieldReports(ctx, eventName, url.Values{"q": {q}})
	require.Equal(t, http.StatusOK, resp.StatusCode, q)
	require.NoError(t, resp.Body.Close())
	var got []int32
	for _, item := range items {
		got = append(got, item.Number)
	}
	require.ElementsMatch(t, want, got, q)
}

func TestIncidentListOmitsReportEntries(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	apisAdmin := srv.admin(ctx)
	apis := srv.alice(ctx)
	eventName := newEventWithWriter(t, apisAdmin)

	num := apis.newIncidentSuccess(ctx, imsjson.Incident{
		Event: eventName,
		ReportEntries: []imsjson.ReportEntry{
			{Text: "\nThe first line of the text\nThe second line"},
			{Text: "Some later text"},
		},
	})

	path := apis.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents").String()
	body, resp := apis.imsGetBodyBytes(ctx, path)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotContains(t, string(body), "report_entries")
	require.NotContains(t, string(body), "The second line")
	require.NotContains(t, string(body), "Some later text")

	// With no summary of its own, the Incident's summary comes from the first
	// line of its first non-system entry.
	items, resp := apis.getIncidents(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, items, 1)
	require.Equal(t, num, items[0].Number)
	require.Equal(t, "The first line of the text", items[0].Summary)
}

func TestIncidentListNumberFilter(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	apisAdmin := srv.admin(ctx)
	apis := srv.alice(ctx)
	eventName := newEventWithWriter(t, apisAdmin)

	apis.newIncidentSuccess(ctx, imsjson.Incident{Event: eventName, Summary: new("one")})
	num2 := apis.newIncidentSuccess(ctx, imsjson.Incident{Event: eventName, Summary: new("two")})

	items, resp := apis.queryIncidents(ctx, eventName, url.Values{"number": {conv.FormatInt(num2)}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, items, 1)
	require.Equal(t, num2, items[0].Number)
	require.Equal(t, "two", items[0].Summary)

	_, resp = apis.queryIncidents(ctx, eventName, url.Values{"number": {"999"}})
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	_, resp = apis.queryIncidents(ctx, eventName, url.Values{"number": {"not a number"}})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	_, resp = apis.queryIncidents(ctx, eventName, url.Values{"number": {"1"}, "q": {"one"}})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestIncidentListSearch(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	apisAdmin := srv.admin(ctx)
	apis := srv.alice(ctx)
	eventName := newEventWithWriter(t, apisAdmin)

	fire := apis.newIncidentSuccess(ctx, imsjson.Incident{
		Event:   eventName,
		Summary: new("Burn barrel"),
		ReportEntries: []imsjson.ReportEntry{
			{Text: "Smoke seen near the Café, crew en route"},
		},
	})
	lost := apis.newIncidentSuccess(ctx, imsjson.Incident{
		Event:    eventName,
		Summary:  new("Lost child"),
		Location: imsjson.Location{Name: new("Dusty Camp"), Address: new("7:30 & E")},
	})
	resp := apis.attachRangerToIncident(ctx, eventName, lost, "Tuba")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Words match anywhere in the Incident, in any order, ignoring case
	requireIncidentSearch(ctx, t, apis, eventName, "crew BARREL", fire)
	// Diacritics are ignored
	requireIncidentSearch(ctx, t, apis, eventName, "cafe", fire)
	// Every word has to match
	requireIncidentSearch(ctx, t, apis, eventName, "crew child")
	// A quoted phrase has to match as a whole
	requireIncidentSearch(ctx, t, apis, eventName, `"seen near"`, fire)
	requireIncidentSearch(ctx, t, apis, eventName, `"near seen"`)
	// A negated word excludes
	requireIncidentSearch(ctx, t, apis, eventName, "!smoke", lost)
	// A slash-wrapped query is a regex
	requireIncidentSearch(ctx, t, apis, eventName, "/b.rn|l.st/", fire, lost)
	// Location, Rangers, and the Incident number are all searchable
	requireIncidentSearch(ctx, t, apis, eventName, "dusty 7:30", lost)
	requireIncidentSearch(ctx, t, apis, eventName, "tuba", lost)
	requireIncidentSearch(ctx, t, apis, eventName, conv.FormatInt(fire)+" smoke", fire)

	_, resp = apis.queryIncidents(ctx, eventName, url.Values{"q": {"/(unclosed/"}})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestIncidentListSearchesTypesAndAttachments(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	apisAdmin := srv.admin(ctx)
	apis := srv.alice(ctx)
	eventName := newEventWithWriter(t, apisAdmin)

	incident := apis.newIncidentSuccess(ctx, imsjson.Incident{Event: eventName, Summary: new("Plain")})
	other := apis.newIncidentSuccess(ctx, imsjson.Incident{Event: eventName, Summary: new("Other")})

	typeName := "Type" + rand.NonCryptoText()
	typeID, resp := apisAdmin.editType(ctx, imsjson.IncidentType{Name: &typeName, Hidden: new(false)})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apis.attachTypeToIncident(ctx, eventName, incident, *typeID)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	requireIncidentSearch(ctx, t, apis, eventName, typeName, incident)

	fr := apis.newFieldReportSuccess(ctx, imsjson.FieldReport{
		Event:         eventName,
		ReportEntries: []imsjson.ReportEntry{{Text: "an armadillo in the report"}},
	})
	requireIncidentSearch(ctx, t, apis, eventName, "armadillo")
	resp = apis.attachFieldReportToIncident(ctx, eventName, fr, incident)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	requireIncidentSearch(ctx, t, apis, eventName, "armadillo", incident)

	visit := apis.newVisitSuccess(ctx, imsjson.Visit{
		Event:              eventName,
		GuestPreferredName: new("Pangolin"),
		ReportEntries:      []imsjson.ReportEntry{{Text: "brought a narwhal"}},
	})
	resp = apis.updateVisit(ctx, eventName, visit, imsjson.Visit{Event: eventName, Number: visit, Incident: &other})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	requireIncidentSearch(ctx, t, apis, eventName, "pangolin", other)
	requireIncidentSearch(ctx, t, apis, eventName, "narwhal", other)
	// Words can match across the Incident and its attachments
	requireIncidentSearch(ctx, t, apis, eventName, "armadillo plain", incident)
}

func TestFieldReportListSearch(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	apisAdmin := srv.admin(ctx)
	apisAlice := srv.alice(ctx)
	eventName := newEventWithReporter(t, apisAdmin)

	alices := apisAlice.newFieldReportSuccess(ctx, imsjson.FieldReport{
		Event:         eventName,
		ReportEntries: []imsjson.ReportEntry{{Text: "A shared word, and alpaca"}},
	})
	admins := apisAdmin.newFieldReportSuccess(ctx, imsjson.FieldReport{
		Event:         eventName,
		Summary:       new("The admin's report"),
		ReportEntries: []imsjson.ReportEntry{{Text: "A shared word, and llama"}},
	})

	requireFieldReportSearch(ctx, t, apisAdmin, eventName, "shared", alices, admins)
	requireFieldReportSearch(ctx, t, apisAdmin, eventName, "llama", admins)
	requireFieldReportSearch(ctx, t, apisAdmin, eventName, "admin's", admins)
	// The author is searchable
	requireFieldReportSearch(ctx, t, apisAdmin, eventName, userAliceHandle+" shared", alices)

	// A Reporter's search only ever returns their own Field Reports
	requireFieldReportSearch(ctx, t, apisAlice, eventName, "shared", alices)
	requireFieldReportSearch(ctx, t, apisAlice, eventName, "llama")

	// Nor can a Reporter get someone else's by number
	_, resp := apisAlice.queryFieldReports(ctx, eventName, url.Values{"number": {conv.FormatInt(admins)}})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	items, resp := apisAlice.queryFieldReports(ctx, eventName, url.Values{"number": {conv.FormatInt(alices)}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, items, 1)
	require.Equal(t, "A shared word, and alpaca", items[0].Summary)
	require.Equal(t, userAliceHandle, items[0].Author)
}

func TestVisitList(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	apisAdmin := srv.admin(ctx)
	apis := srv.alice(ctx)
	eventName := newEventWithWriter(t, apisAdmin)

	named := apis.newVisitSuccess(ctx, imsjson.Visit{
		Event:              eventName,
		GuestPreferredName: new("Sunny"),
		GuestLegalName:     new("Legal Name One"),
	})
	unnamed := apis.newVisitSuccess(ctx, imsjson.Visit{
		Event:          eventName,
		GuestLegalName: new("Legal Name Two"),
	})

	// The legal name only comes along when there's no preferred name to show
	items, resp := apis.queryVisits(ctx, eventName, url.Values{"number": {conv.FormatInt(named)}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, items, 1)
	require.Equal(t, "Sunny", *items[0].GuestPreferredName)
	require.Nil(t, items[0].GuestLegalName)

	items, resp = apis.queryVisits(ctx, eventName, url.Values{"number": {conv.FormatInt(unnamed)}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, items, 1)
	require.Equal(t, "Legal Name Two", *items[0].GuestLegalName)

	// The Visits page searches its table on the client
	_, resp = apis.queryVisits(ctx, eventName, url.Values{"q": {"Sunny"}})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestListAndRecordReadsAreAudited(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	apisAdmin := srv.admin(ctx)
	referrer := "TestListAndRecordReadsAreAudited" + rand.NonCryptoText()
	apis := srv.alice(ctx).withReferrer(referrer)
	eventName := newEventWithWriter(t, apisAdmin)

	num := apis.newIncidentSuccess(ctx, imsjson.Incident{Event: eventName})

	_, resp := apis.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = apis.getIncidents(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = apis.queryIncidents(ctx, eventName, url.Values{"q": {"a secret search"}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	longAgo := time.Now().Add(-500 * time.Hour).UnixMilli()
	longFromNow := time.Now().Add(500 * time.Hour).UnixMilli()
	logs, resp := apisAdmin.getActionLogsForPage(ctx, conv.FormatInt(longAgo), conv.FormatInt(longFromNow), referrer)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	listPath := "/ims/api/events/" + eventName + "/incidents"
	var recordReads, listReads []imsjson.ActionLog
	for _, al := range logs {
		if al.Method != http.MethodGet {
			continue
		}
		switch al.Path {
		case listPath + "/" + conv.FormatInt(num):
			recordReads = append(recordReads, al)
		case listPath:
			listReads = append(listReads, al)
		}
	}
	// Reading the Incident itself is logged
	require.Len(t, recordReads, 1)
	assert.Equal(t, userAliceHandle, recordReads[0].UserName)
	// A search is logged with its query, but a plain list fetch isn't logged
	require.Len(t, listReads, 1)
	assert.Contains(t, listReads[0].RequestBody, "a secret search")
}

// The full lists build their items from report entries fetched without most
// of their text, so check that they come out the same as the number= lookup,
// which builds from the whole entries.
func TestIncidentListMatchesNumberLookup(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	apisAdmin := srv.admin(ctx)
	apis := srv.alice(ctx)
	eventName := newEventWithWriter(t, apisAdmin)

	// No summary, and a first entry with only blank lines, so the summary
	// comes from the second entry.
	noSummary := apis.newIncidentSuccess(ctx, imsjson.Incident{
		Event: eventName,
		ReportEntries: []imsjson.ReportEntry{
			{Text: "\r\n\n"},
			{Text: "  Indented first line\nsecond line"},
		},
	})
	// A summary change adds a system entry, which moves LastModified.
	withSummary := apis.newIncidentSuccess(ctx, imsjson.Incident{
		Event:         eventName,
		Summary:       new("Own summary"),
		ReportEntries: []imsjson.ReportEntry{{Text: "Not the summary"}},
	})
	noEntries := apis.newIncidentSuccess(ctx, imsjson.Incident{Event: eventName})

	items, resp := apis.getIncidents(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, items, 3)

	byNumber := make(map[int32]imsjson.IncidentListItem)
	for _, item := range items {
		byNumber[item.Number] = item
	}
	require.Contains(t, byNumber[noSummary].Summary, "Indented first line")
	require.Equal(t, "Own summary", byNumber[withSummary].Summary)
	require.Empty(t, byNumber[noEntries].Summary)

	for _, num := range []int32{noSummary, withSummary, noEntries} {
		single, resp := apis.queryIncidents(ctx, eventName, url.Values{"number": {conv.FormatInt(num)}})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.Len(t, single, 1)
		require.Equal(t, single[0], byNumber[num])
	}
}

func TestFieldReportListMatchesNumberLookup(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	apisAdmin := srv.admin(ctx)
	apis := srv.alice(ctx)
	eventName := newEventWithWriter(t, apisAdmin)

	noSummary := apis.newFieldReportSuccess(ctx, imsjson.FieldReport{
		Event: eventName,
		ReportEntries: []imsjson.ReportEntry{
			{Text: "\n"},
			{Text: "The real first line\nand more"},
		},
	})
	withSummary := apis.newFieldReportSuccess(ctx, sampleFieldReport1(eventName))

	items, resp := apis.getFieldReports(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, items, 2)

	byNumber := make(map[int32]imsjson.FieldReportListItem)
	for _, item := range items {
		byNumber[item.Number] = item
	}
	require.Equal(t, "The real first line", byNumber[noSummary].Summary)
	require.Equal(t, "my summary!", byNumber[withSummary].Summary)
	require.Equal(t, userAliceHandle, byNumber[noSummary].Author)

	for _, num := range []int32{noSummary, withSummary} {
		single, resp := apis.queryFieldReports(ctx, eventName, url.Values{"number": {conv.FormatInt(num)}})
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.Len(t, single, 1)
		require.Equal(t, single[0], byNumber[num])
	}
}

func TestVisitListMatchesNumberLookup(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	apisAdmin := srv.admin(ctx)
	apis := srv.alice(ctx)
	eventName := newEventWithVisitWriter(t, apisAdmin)

	num := apis.newVisitSuccess(ctx, imsjson.Visit{
		Event:         eventName,
		ReportEntries: []imsjson.ReportEntry{{Text: "Some entry"}},
	})

	items, resp := apis.getVisits(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, items, 1)

	single, resp := apis.queryVisits(ctx, eventName, url.Values{"number": {conv.FormatInt(num)}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, single, 1)
	require.Equal(t, single[0], items[0])
}
