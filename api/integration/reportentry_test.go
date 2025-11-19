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
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestEditIncidentReportEntry(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user Writer role on that event
	eventName := rand.NonCryptoText()
	resp := apisAdmin.editEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Use normal user to create a new Incident
	incidentReq := sampleIncident1(eventName)
	entryReq := incidentReq.ReportEntries[0]
	num := apisNonAdmin.newIncidentSuccess(ctx, incidentReq)
	incidentReq.Number = num

	// Use normal user to fetch that Incident from the API
	retrievedIncident, resp := apisNonAdmin.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, retrievedIncident)
	require.Len(t, retrievedIncident.ReportEntries, 2)
	reportEntry := retrievedIncident.ReportEntries[1]
	require.Equal(t, entryReq.Text, reportEntry.Text)

	// Strike that report entry
	reportEntry.Stricken = ptr(true)
	resp = apisNonAdmin.updateIncidentReportEntry(ctx, eventName, num, reportEntry)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Check that the striking worked
	retrievedIncident, resp = apisNonAdmin.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, retrievedIncident)
	reportEntry = retrievedIncident.ReportEntries[1]
	require.True(t, *reportEntry.Stricken)

	// Unstrike that report entry
	reportEntry.Stricken = ptr(false)
	resp = apisNonAdmin.updateIncidentReportEntry(ctx, eventName, num, reportEntry)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Check that the unstriking worked
	retrievedIncident, resp = apisNonAdmin.getIncident(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, retrievedIncident)
	reportEntry = retrievedIncident.ReportEntries[1]
	require.False(t, *reportEntry.Stricken)

	// If no Stricken value is provided, nothing happens
	reportEntry.Stricken = nil
	resp = apisNonAdmin.updateIncidentReportEntry(ctx, eventName, num, reportEntry)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestEditFieldReportReportEntry(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user Writer role on that event
	eventName := rand.NonCryptoText()
	resp := apisAdmin.editEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Use normal user to create a new FR
	frReq := sampleFieldReport1(eventName)
	entryReq := frReq.ReportEntries[0]
	num := apisNonAdmin.newFieldReportSuccess(ctx, frReq)
	frReq.Number = num

	// Use normal user to fetch that FR from the API
	retrievedFR, resp := apisNonAdmin.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, retrievedFR)
	require.Len(t, retrievedFR.ReportEntries, 2)
	reportEntry := retrievedFR.ReportEntries[1]
	require.Equal(t, entryReq.Text, reportEntry.Text)

	// Strike that report entry
	reportEntry.Stricken = ptr(true)
	resp = apisNonAdmin.updateFieldReportReportEntry(ctx, eventName, num, reportEntry)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Check that the striking worked
	retrievedFR, resp = apisNonAdmin.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, retrievedFR)
	reportEntry = retrievedFR.ReportEntries[1]
	require.True(t, *reportEntry.Stricken)

	// Unstrike that report entry
	reportEntry.Stricken = ptr(false)
	resp = apisNonAdmin.updateFieldReportReportEntry(ctx, eventName, num, reportEntry)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Check that the unstriking worked
	retrievedFR, resp = apisNonAdmin.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, retrievedFR)
	reportEntry = retrievedFR.ReportEntries[1]
	require.False(t, *reportEntry.Stricken)

	// If no Stricken value is provided, nothing happens
	reportEntry.Stricken = nil
	resp = apisNonAdmin.updateFieldReportReportEntry(ctx, eventName, num, reportEntry)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}
