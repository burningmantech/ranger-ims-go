package integration_test

import (
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
	"time"
)

func sampleFieldReport1(eventName string) imsjson.FieldReport {
	return imsjson.FieldReport{
		Event:   eventName,
		Summary: ptr("my summary!"),
		ReportEntries: []imsjson.ReportEntry{
			{Text: "This is some report text lol"},
		},
	}
}

func TestCreateAndGetFieldReport(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user Reporter role on that event
	eventName := uuid.NewString()
	resp := apisAdmin.editEvent(ctx, imsjson.EditEventsRequest{Add: []string{eventName}})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addReporter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Use normal user to create a new Field Report
	fieldReportReq := sampleFieldReport1(eventName)
	entryReq := fieldReportReq.ReportEntries[0]
	num := apisNonAdmin.newFieldReportSuccess(ctx, fieldReportReq)
	fieldReportReq.Number = num

	{
		// Use normal user to fetch that Field Report from the API and check it over
		retrievedFieldReport, resp := apisNonAdmin.getFieldReport(ctx, eventName, num)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.NotNil(t, retrievedFieldReport)
		requireEqualFieldReport(t, fieldReportReq, retrievedFieldReport)
		require.Len(t, retrievedFieldReport.ReportEntries, 2)

		// The first report entry will be the system entry. The second should be the one we sent in the request
		retrievedUserEntry := retrievedFieldReport.ReportEntries[1]
		retrievedUserEntry.ID = 0
		require.WithinDuration(t, time.Now(), retrievedUserEntry.Created, 5*time.Minute)
		retrievedUserEntry.Created = time.Time{}
		entryReq.Author = userAliceHandle
		require.Equal(t, entryReq, retrievedUserEntry)
	}

	{
		// Now get the field report via the GetFieldReports (plural) endpoint, and repeat the validation
		retrievedFieldReports, resp := apisNonAdmin.getFieldReports(ctx, eventName)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
		require.NotNil(t, retrievedFieldReports)
		require.Len(t, retrievedFieldReports, 1)
		requireEqualFieldReport(t, fieldReportReq, retrievedFieldReports[0])
		require.Len(t, retrievedFieldReports[0].ReportEntries, 2)

		// The first report entry will be the system entry. The second should be the one we sent in the request
		retrievedUserEntry := retrievedFieldReports[0].ReportEntries[1]
		retrievedUserEntry.ID = 0
		require.WithinDuration(t, time.Now(), retrievedUserEntry.Created, 5*time.Minute)
		retrievedUserEntry.Created = time.Time{}
		entryReq.Author = userAliceHandle
		require.Equal(t, entryReq, retrievedUserEntry)
	}
}

func TestCreateAndUpdateFieldReport(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user Writer role on that event
	eventName := uuid.NewString()
	resp := apisAdmin.editEvent(ctx, imsjson.EditEventsRequest{Add: []string{eventName}})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Use normal user to create a new Field Report
	fieldReportReq := sampleFieldReport1(eventName)
	num := apisAlice.newFieldReportSuccess(ctx, fieldReportReq)
	fieldReportReq.Number = num

	retrievedNewFieldReport, resp := apisAlice.getFieldReport(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())

	// Now let's update the incident. First let's try just adding an incident number.
	updates := imsjson.FieldReport{
		Event:    fieldReportReq.Event,
		Number:   num,
		Incident: ptr(int32(12345)),
		ReportEntries: []imsjson.ReportEntry{
			{
				Text: "new details!",
			},
			{
				Text: "",
			},
		},
	}

	resp = apisAlice.updateFieldReport(ctx, eventName, num, updates)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	retrievedFieldReportAfterUpdate, resp := apisAlice.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	requireEqualFieldReport(t, retrievedNewFieldReport, retrievedFieldReportAfterUpdate)

	// now let's set all fields to empty
	updates = imsjson.FieldReport{
		Event:         fieldReportReq.Event,
		Number:        num,
		Summary:       ptr(""),
		Incident:      nil,
		ReportEntries: []imsjson.ReportEntry{},
	}
	resp = apisAlice.updateFieldReport(ctx, eventName, num, updates)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	// then check the result
	retrievedFieldReportAfterUpdate, resp = apisAlice.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	expected := imsjson.FieldReport{
		Event:    eventName,
		Number:   num,
		Summary:  nil,
		Incident: nil,
	}
	requireEqualFieldReport(t, expected, retrievedFieldReportAfterUpdate)

	// make an incident, then attach to it
	incidentNumber := apisAlice.newIncidentSuccess(ctx, imsjson.Incident{
		Event: eventName,
	})
	resp = apisAlice.attachFieldReportToIncident(ctx, eventName, num, incidentNumber)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// confirm it worked
	fieldReportAfterAttach, resp := apisAlice.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, incidentNumber, *fieldReportAfterAttach.Incident)

	// detach again
	resp = apisAlice.detachFieldReportFromIncident(ctx, eventName, num)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// confirm it worked
	fieldReportAfterDetach, resp := apisAlice.getFieldReport(ctx, eventName, num)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Nil(t, fieldReportAfterDetach.Incident)
}

func TestCreateAndAttachFileToFieldReport(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	// Use the admin JWT to create a new event,
	// then give the normal user Reporter role on that event
	eventName := uuid.NewString()
	resp := apisAdmin.editEvent(ctx, imsjson.EditEventsRequest{Add: []string{eventName}})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.addReporter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Use normal user to create a new Field Report
	fieldReportReq := sampleFieldReport1(eventName)
	num := apisNonAdmin.newFieldReportSuccess(ctx, fieldReportReq)
	fieldReportReq.Number = num

	// Now we'll upload an attachment. The "file" will just be this slice of bytes.
	fileBytes := []byte("This is a text file maybe?")
	reID, resp := apisNonAdmin.attachFileToFieldReport(ctx, eventName, num, fileBytes)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Now call to fetch the attachment and check that it's the same as what we sent.
	returnedAttachment, resp := apisNonAdmin.getFieldReportAttachment(ctx, eventName, num, reID)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, fileBytes, returnedAttachment)
}

// requireEqualIncident is a hacky way of checking two incident responses are the same.
// It does not consider ReportEntries.
func requireEqualFieldReport(t *testing.T, before, after imsjson.FieldReport) {
	t.Helper()
	// This field isn't in use in the client yet
	// require.Equal(t, before.EventID, after.EventID)
	require.Equal(t, before.Event, after.Event)
	require.Equal(t, before.Number, after.Number)

	// If the timestamp field was set before, then check it's the same. Otherwise
	// see if it was set to some reasonable time for when the test was running
	if !before.Created.IsZero() {
		require.Equal(t, before.Created, after.Created)
	} else {
		require.WithinDuration(t, time.Now(), after.Created, 20*time.Minute)
	}
	require.Equal(t, before.Summary, after.Summary)
	// these will always be different. Check them separately of this function
	// require.Equal(t, before.ReportEntries, after.ReportEntries)
}
