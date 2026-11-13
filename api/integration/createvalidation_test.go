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
	"github.com/stretchr/testify/require"
)

func TestReporterCanUseAFieldReportCreatedEmpty(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	admin := srv.admin(ctx)
	alice := srv.alice(ctx)
	eventName := newEventWithReporter(t, admin)

	// This is what the Field Report page sends when a new Field Report's first
	// action is attaching a file.
	num := alice.newFieldReportSuccess(ctx, imsjson.FieldReport{Event: eventName})

	fr, resp := alice.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, fr.ReportEntries, 1)
	require.Equal(t, userAliceHandle, fr.ReportEntries[0].Author)
	require.True(t, fr.ReportEntries[0].SystemEntry)

	_, resp = alice.attachFileToFieldReport(ctx, eventName, num, []byte("a file"))
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestOverlongIncidentTextIsRejected(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	admin := srv.admin(ctx)
	eventName := newEventWithReporter(t, admin)
	tooLong := strings.Repeat("é", 1025)

	// A refused create leaves no Incident behind.
	resp := admin.newIncident(ctx, imsjson.Incident{Event: eventName, Summary: &tooLong})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	incidents, resp := admin.getIncidents(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, incidents)

	// The limit is in characters, not bytes.
	justFits := strings.Repeat("é", 1024)
	num := admin.newIncidentSuccess(ctx, imsjson.Incident{Event: eventName, Summary: &justFits})

	resp = admin.updateIncident(ctx, eventName, num, imsjson.Incident{
		Location: imsjson.Location{Name: &tooLong},
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestOverlongFieldReportSummaryIsRejected(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	admin := srv.admin(ctx)
	eventName := newEventWithReporter(t, admin)
	tooLong := strings.Repeat("a", 1025)

	resp := admin.newFieldReport(ctx, imsjson.FieldReport{Event: eventName, Summary: &tooLong})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	num := admin.newFieldReportSuccess(ctx, imsjson.FieldReport{Event: eventName})
	resp = admin.updateFieldReport(ctx, eventName, num, imsjson.FieldReport{Number: num, Summary: &tooLong})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestRefusedVisitCreateLeavesNoVisit(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	admin := srv.admin(ctx)
	eventName := newEventWithReporter(t, admin)

	tooLong := strings.Repeat("a", 257)
	resp := admin.newVisit(ctx, imsjson.Visit{Event: eventName, GuestDescription: &tooLong})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	arrival := time.Now()
	departure := arrival.Add(-time.Hour)
	resp = admin.newVisit(ctx, imsjson.Visit{Event: eventName, ArrivalTime: &arrival, DepartureTime: &departure})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	visits, resp := admin.getVisits(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, visits)

	num := admin.newVisitSuccess(ctx, imsjson.Visit{Event: eventName})
	resp = admin.updateVisit(ctx, eventName, num, imsjson.Visit{GuestDescription: &tooLong})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestFailedPlacesUpdateKeepsTheOldPlaces(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	admin := srv.admin(ctx)
	eventName := newEventWithReporter(t, admin)

	resp := admin.editPlaces(ctx, eventName, imsjson.Places{
		"camp": {{Name: "Camp One", LocationString: "3:00 & A"}, {Name: "Camp Two", LocationString: "4:00 & B"}},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// The second place can't be stored, by which point the old ones are gone
	// and the first new one is in.
	resp = admin.editPlaces(ctx, eventName, imsjson.Places{
		"camp": {{Name: "Camp Three", LocationString: "5:00 & C"}, {Name: strings.Repeat("a", 1025), LocationString: "6:00 & D"}},
	})
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	places, resp := admin.getPlaces(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, places["camp"], 2)
	require.Equal(t, "Camp One", places["camp"][0].Name)
	require.Equal(t, "Camp Two", places["camp"][1].Name)
}
