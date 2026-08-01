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
	"sync"
	"testing"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/stretchr/testify/require"
)

// TestConcurrentIncidentRangerRosterWrites is the regression test for the lock
// ordering in bumpRosterVersion. Concurrent roster writes against one Incident
// all take its row exclusively before touching INCIDENT__RANGER; with the two
// writes in the other order they deadlocked in MariaDB and some of these
// requests came back 500.
func TestConcurrentIncidentRangerRosterWrites(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	num := apis.newIncidentSuccess(ctx, typelessIncident(eventName))

	attachAll := func(handles []string) []int {
		t.Helper()
		codes := make([]int, len(handles))
		var wg sync.WaitGroup
		for i, handle := range handles {
			wg.Go(func() {
				resp := apis.attachRangerToIncident(ctx, eventName, num, handle)
				codes[i] = resp.StatusCode
				_ = resp.Body.Close()
			})
		}
		wg.Wait()
		return codes
	}

	// Several requests naming different Rangers at once. Each writes only its own
	// row, so all of them stick.
	handles := []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "November", "Oscar", "Papa", "Quebec", "Romeo", "Sierra"}
	for _, code := range attachAll(handles) {
		require.Equal(t, http.StatusNoContent, code)
	}

	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	attached := make([]string, 0, len(handles))
	for _, ranger := range *retrieved.Rangers {
		attached = append(attached, ranger.Handle)
	}
	require.ElementsMatch(t, handles, attached)

	// The same Ranger from several requests at once. Each of these either lands
	// the Ranger on the roster or finds them already there with the same role, so
	// how many report entries they leave depends on the interleaving. What they
	// must not do is leave the Ranger on the roster twice, or fail.
	//
	// This runs over several Incidents because the deadlock it guards against was
	// only lost about a third of the time on any one of them, and a regression
	// test that catches a third of regressions isn't one.
	repeated := []string{"Golf", "Golf", "Golf", "Golf", "Golf", "Golf", "Golf", "Golf"}
	for range 8 {
		attempt := apis.newIncidentSuccess(ctx, typelessIncident(eventName))
		before, resp := apis.getIncident(ctx, eventName, attempt)
		require.NoError(t, resp.Body.Close())

		codes := make([]int, len(repeated))
		var wg sync.WaitGroup
		for i, handle := range repeated {
			wg.Go(func() {
				resp := apis.attachRangerToIncident(ctx, eventName, attempt, handle)
				codes[i] = resp.StatusCode
				_ = resp.Body.Close()
			})
		}
		wg.Wait()
		for _, code := range codes {
			require.Equal(t, http.StatusNoContent, code)
		}

		attempted, resp := apis.getIncident(ctx, eventName, attempt)
		require.NoError(t, resp.Body.Close())
		require.Len(t, *attempted.Rangers, 1)
		require.Equal(t, "Golf", (*attempted.Rangers)[0].Handle)
		added := len(attempted.ReportEntries) - len(before.ReportEntries)
		require.GreaterOrEqual(t, added, 1)
		require.LessOrEqual(t, added, len(repeated))
	}

	resp = apis.attachRangerToIncident(ctx, eventName, num, "Golf")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Adds and removals at the same time, which is the shape two Rangers editing
	// one roster actually make. Each request names a different Ranger, so the
	// result doesn't depend on which of them wins the race: the arrivals are on
	// the roster afterwards and the departures aren't.
	arrivals := []string{"Tango", "Uniform", "Victor", "Whiskey", "Xray", "Yankee"}
	codes := make([]int, len(handles)+len(arrivals))
	var wg sync.WaitGroup
	for i, handle := range handles {
		wg.Go(func() {
			resp := apis.detachRangerFromIncident(ctx, eventName, num, handle)
			codes[i] = resp.StatusCode
			_ = resp.Body.Close()
		})
	}
	for i, handle := range arrivals {
		wg.Go(func() {
			resp := apis.attachRangerToIncident(ctx, eventName, num, handle)
			codes[len(handles)+i] = resp.StatusCode
			_ = resp.Body.Close()
		})
	}
	wg.Wait()
	for _, code := range codes {
		require.Equal(t, http.StatusNoContent, code)
	}

	retrieved, resp = apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	attached = attached[:0]
	for _, ranger := range *retrieved.Rangers {
		attached = append(attached, ranger.Handle)
	}
	require.ElementsMatch(t, append([]string{"Golf"}, arrivals...), attached)
}

// lastVisitReportEntryText is lastReportEntryText for a Visit.
func lastVisitReportEntryText(t *testing.T, visit imsjson.Visit) string {
	t.Helper()
	require.NotEmpty(t, visit.ReportEntries)
	return visit.ReportEntries[len(visit.ReportEntries)-1].Text
}

// The attach endpoint both adds a Ranger to the roster and sets the role of one
// who's already on it, and the change log has to say which of those happened.
func TestIncidentRangerRoleChangeLog(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithWriter(t, apisAdmin)
	num := apis.newIncidentSuccess(ctx, typelessIncident(eventName))

	// Added with no role.
	resp := apis.attachRangerToIncident(ctx, eventName, num, "Hardware")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp := apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "Added Ranger: Hardware", lastReportEntryText(t, retrieved))

	// Given a role, which is a change to a Ranger who's already on the roster.
	resp = apis.setIncidentRangerRole(ctx, eventName, num, "Hardware", new("Driver"))
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp = apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Len(t, *retrieved.Rangers, 1)
	require.Equal(t, "Driver", *(*retrieved.Rangers)[0].Role)
	require.Equal(t, "Set role for Hardware: Driver", lastReportEntryText(t, retrieved))
	entriesAfterRole := len(retrieved.ReportEntries)

	// Setting the same role again changes nothing, so it writes nothing.
	resp = apis.setIncidentRangerRole(ctx, eventName, num, "Hardware", new("Driver"))
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp = apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Len(t, retrieved.ReportEntries, entriesAfterRole)

	// Clearing the role, which the UI sends as an empty string.
	resp = apis.setIncidentRangerRole(ctx, eventName, num, "Hardware", new(""))
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp = apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Len(t, *retrieved.Rangers, 1)
	require.Nil(t, (*retrieved.Rangers)[0].Role)
	require.Equal(t, "Removed role for Hardware", lastReportEntryText(t, retrieved))
	entriesAfterClear := len(retrieved.ReportEntries)

	// Clearing it again is the no-op case for a Ranger who never had a role.
	resp = apis.setIncidentRangerRole(ctx, eventName, num, "Hardware", new(""))
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp = apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Len(t, retrieved.ReportEntries, entriesAfterClear)

	// A Ranger who arrives with a role in hand is one line, not two.
	resp = apis.setIncidentRangerRole(ctx, eventName, num, "Software", new("Driver"))
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp = apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Len(t, *retrieved.Rangers, 2)
	require.Equal(t, "Added Ranger: Software (role: Driver)", lastReportEntryText(t, retrieved))

	// Removal still says so, whatever role the Ranger held.
	resp = apis.detachRangerFromIncident(ctx, eventName, num, "Software")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp = apis.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Len(t, *retrieved.Rangers, 1)
	require.Equal(t, "Removed Ranger: Software", lastReportEntryText(t, retrieved))
}

// Visits share the roster code with Incidents, so their change log says the same
// things.
func TestVisitRangerRoleChangeLog(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithVisitWriter(t, apisAdmin)
	num := apis.newVisitSuccess(ctx, sampleVisit1(eventName))

	resp := apis.attachRangerToVisit(ctx, eventName, num, "Hardware")
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp := apis.getVisit(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "Added Ranger: Hardware", lastVisitReportEntryText(t, retrieved))

	resp = apis.setVisitRangerRole(ctx, eventName, num, "Hardware", new("Sitter"))
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrieved, resp = apis.getVisit(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Len(t, *retrieved.Rangers, 1)
	require.Equal(t, "Sitter", *(*retrieved.Rangers)[0].Role)
	require.Equal(t, "Set role for Hardware: Sitter", lastVisitReportEntryText(t, retrieved))
}

// The roster code is shared between Incidents and Visits through rangerRoster,
// so the same fix has to hold on a Visit's roster, which hangs off the VISIT
// table by the same kind of foreign key.
func TestConcurrentVisitRangerRosterWrites(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	eventName := newEventWithVisitWriter(t, apisAdmin)
	num := apis.newVisitSuccess(ctx, sampleVisit1(eventName))

	handles := []string{"Hotel", "India", "Juliett", "Kilo", "Lima", "Mike", "Tango", "Uniform", "Victor", "Whiskey", "Xray", "Yankee"}
	codes := make([]int, len(handles))
	var wg sync.WaitGroup
	for i, handle := range handles {
		wg.Go(func() {
			resp := apis.attachRangerToVisit(ctx, eventName, num, handle)
			codes[i] = resp.StatusCode
			_ = resp.Body.Close()
		})
	}
	wg.Wait()
	for _, code := range codes {
		require.Equal(t, http.StatusNoContent, code)
	}

	retrieved, resp := apis.getVisit(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	attached := make([]string, 0, len(handles))
	for _, ranger := range *retrieved.Rangers {
		attached = append(attached, ranger.Handle)
	}
	require.ElementsMatch(t, handles, attached)
}
