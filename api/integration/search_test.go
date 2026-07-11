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
	"strings"
	"testing"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchAcrossEvents(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	adminUser := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	aliceUser := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// A search term that won't collide with data from any other test.
	token := "srchtok" + rand.NonCryptoText()

	// Two events on which Alice is a writer.
	eventA := rand.NonCryptoText()
	eventB := rand.NonCryptoText()
	for _, eventName := range []string{eventA, eventB} {
		_, resp := adminUser.createEvent(ctx, imsjson.Event{Name: &eventName})
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		resp = adminUser.addWriter(ctx, eventName, userAliceHandle)
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	}

	// An Incident in event A matching on its summary.
	incidentANumber := aliceUser.newIncidentSuccess(ctx, imsjson.Incident{
		Event:   eventA,
		State:   "new",
		Summary: new("Lost bicycle " + token),
	})

	// An Incident in event B matching only via report entry text.
	incidentBNumber := aliceUser.newIncidentSuccess(ctx, imsjson.Incident{
		Event: eventB,
		State: "new",
		ReportEntries: []imsjson.ReportEntry{
			{Text: "the guest said " + token + " and wandered off"},
		},
	})

	// A Field Report in event B matching via report entry text.
	fieldReportNumber := aliceUser.newFieldReportSuccess(ctx, imsjson.FieldReport{
		Event:   eventB,
		Summary: new("An FR summary"),
		ReportEntries: []imsjson.ReportEntry{
			{Text: "field report about " + token},
		},
	})

	// A Visit in event A matching on the guest's preferred name.
	visitNumber := aliceUser.newVisitSuccess(ctx, imsjson.Visit{
		Event:              eventA,
		GuestPreferredName: new("Guesty " + token),
	})

	// Alice finds all four records, across both events and all three kinds.
	results, resp := aliceUser.search(ctx, url.Values{"q": []string{token}})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Len(t, results.Hits, 4)
	assert.False(t, results.Truncated)

	byKind := make(map[string]imsjson.SearchResult)
	for _, hit := range results.Hits {
		byKind[hit.Kind] = hit
	}

	// Both incidents came back; spot-check the one that matched via report
	// entry text, which must carry a snippet showing the match.
	var incidentHits []imsjson.SearchResult
	for _, hit := range results.Hits {
		if hit.Kind == imsjson.SearchResultKindIncident {
			incidentHits = append(incidentHits, hit)
		}
	}
	assert.Len(t, incidentHits, 2)
	for _, hit := range incidentHits {
		switch hit.Event {
		case eventA:
			assert.Equal(t, incidentANumber, hit.Number)
			assert.Equal(t, "Lost bicycle "+token, hit.Summary)
		case eventB:
			assert.Equal(t, incidentBNumber, hit.Number)
			assert.Contains(t, hit.Snippet, token)
		default:
			t.Fatalf("unexpected event %v in incident hit", hit.Event)
		}
	}

	frHit := byKind[imsjson.SearchResultKindFieldReport]
	assert.Equal(t, eventB, frHit.Event)
	assert.Equal(t, fieldReportNumber, frHit.Number)
	assert.Contains(t, frHit.Snippet, token)

	visitHit := byKind[imsjson.SearchResultKindVisit]
	assert.Equal(t, eventA, visitHit.Event)
	assert.Equal(t, visitNumber, visitHit.Number)
	assert.Equal(t, "Guesty "+token, visitHit.Summary)

	// Results come back sorted by creation time, newest first.
	for i := 1; i < len(results.Hits); i++ {
		assert.False(t, results.Hits[i-1].Created.Before(results.Hits[i].Created))
	}

	// The kinds filter restricts which record types are searched.
	results, resp = aliceUser.search(ctx, url.Values{
		"q":     []string{token},
		"kinds": []string{imsjson.SearchResultKindIncident},
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Len(t, results.Hits, 2)
	for _, hit := range results.Hits {
		assert.Equal(t, imsjson.SearchResultKindIncident, hit.Kind)
	}

	// A small limit truncates results and says so.
	results, resp = aliceUser.search(ctx, url.Values{
		"q":     []string{token},
		"kinds": []string{imsjson.SearchResultKindIncident},
		"limit": []string{"1"},
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Len(t, results.Hits, 1)
	assert.True(t, results.Truncated)

	// The LIKE special characters in a query are matched literally rather
	// than as wildcards, so "%" doesn't match everything.
	results, resp = aliceUser.search(ctx, url.Values{"q": []string{"%" + token}})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Empty(t, results.Hits)
}

func TestSearchRegexp(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	adminUser := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	aliceUser := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// A search term that won't collide with data from any other test.
	token := "rgxtok" + rand.NonCryptoText()

	eventName := rand.NonCryptoText()
	_, resp := adminUser.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = adminUser.addWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// An Incident matching on its summary, and a Field Report matching only
	// via report entry text.
	incidentNumber := aliceUser.newIncidentSuccess(ctx, imsjson.Incident{
		Event:   eventName,
		State:   "new",
		Summary: new("Lost bicycle " + token),
	})
	fieldReportNumber := aliceUser.newFieldReportSuccess(ctx, imsjson.FieldReport{
		Event:   eventName,
		Summary: new("An FR summary"),
		ReportEntries: []imsjson.ReportEntry{
			{Text: "field report about " + token},
		},
	})

	// A pattern with a "." wildcard finds both records. The pattern itself is
	// not a substring of anything stored, so this only works if the query is
	// matched as a regexp.
	pattern := "rgxt.k" + strings.TrimPrefix(token, "rgxtok")
	results, resp := aliceUser.search(ctx, url.Values{"q": []string{pattern}, "regex": []string{"true"}})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, results.Hits, 2)
	for _, hit := range results.Hits {
		switch hit.Kind {
		case imsjson.SearchResultKindIncident:
			assert.Equal(t, incidentNumber, hit.Number)
			assert.Equal(t, "Lost bicycle "+token, hit.Summary)
		case imsjson.SearchResultKindFieldReport:
			assert.Equal(t, fieldReportNumber, hit.Number)
			// The snippet comes from the matched report entry, located by the
			// regexp rather than by literal search.
			assert.Contains(t, hit.Snippet, token)
		default:
			t.Fatalf("unexpected kind %v in hit", hit.Kind)
		}
	}

	// The same string searched literally matches nothing.
	results, resp = aliceUser.search(ctx, url.Values{"q": []string{pattern}})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Empty(t, results.Hits)

	// Regexp matching is case-insensitive, like literal matching.
	results, resp = aliceUser.search(ctx, url.Values{"q": []string{strings.ToUpper(pattern)}, "regex": []string{"true"}})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Len(t, results.Hits, 2)

	// Anchors are honored: nothing stored *starts* with the token.
	results, resp = aliceUser.search(ctx, url.Values{"q": []string{"^" + token}, "regex": []string{"true"}})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Empty(t, results.Hits)

	// Alternation matches too.
	results, resp = aliceUser.search(ctx, url.Values{
		"q":     []string{"(zebra|bicycle) " + token},
		"regex": []string{"true"},
		"kinds": []string{imsjson.SearchResultKindIncident},
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Len(t, results.Hits, 1)
}

func TestSearchAuthorization(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	adminUser := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	aliceUser := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	notAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}

	token := "srchtok" + rand.NonCryptoText()

	// An event on which Alice is only a reporter, and the admin has no access.
	eventName := rand.NonCryptoText()
	_, resp := adminUser.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = adminUser.addReporter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	aliceUser.newFieldReportSuccess(ctx, imsjson.FieldReport{
		Event:   eventName,
		Summary: new("FR " + token),
	})

	// Searching requires authentication.
	_, resp = notAuthenticated.search(ctx, url.Values{"q": []string{token}})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Cross-event search never includes Field Reports from events where the
	// user can only read their own reports -- not even reports they authored,
	// as Alice did here.
	results, resp := aliceUser.search(ctx, url.Values{"q": []string{token}})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Empty(t, results.Hits)

	// Being an admin grants no per-event read access, so the admin finds
	// nothing either.
	results, resp = adminUser.search(ctx, url.Values{"q": []string{token}})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Empty(t, results.Hits)
}

func TestSearchBadRequests(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	aliceUser := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Query too short
	_, resp := aliceUser.search(ctx, url.Values{"q": []string{"x"}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Missing query
	_, resp = aliceUser.search(ctx, url.Values{})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Invalid kind
	_, resp = aliceUser.search(ctx, url.Values{"q": []string{"abcd"}, "kinds": []string{"sandwich"}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Invalid limits
	_, resp = aliceUser.search(ctx, url.Values{"q": []string{"abcd"}, "limit": []string{"0"}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = aliceUser.search(ctx, url.Values{"q": []string{"abcd"}, "limit": []string{"1001"}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = aliceUser.search(ctx, url.Values{"q": []string{"abcd"}, "limit": []string{"NaN"}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Invalid regular expression
	_, resp = aliceUser.search(ctx, url.Values{"q": []string{"ab(c"}, "regex": []string{"true"}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Invalid regex parameter
	_, resp = aliceUser.search(ctx, url.Values{"q": []string{"abcd"}, "regex": []string{"banana"}})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}
