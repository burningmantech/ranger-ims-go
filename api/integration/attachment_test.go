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
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/burningmantech/ranger-ims-go/api"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/require"
)

// newEventWithWriterID is newEventWithWriter, but it also returns the Event's
// numeric ID, which SSE pushes are keyed on.
func newEventWithWriterID(t *testing.T, admin ApiHelper) (eventName string, eventID int32) {
	t.Helper()
	ctx := t.Context()
	eventName = rand.NonCryptoText()
	eventID, resp := admin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = admin.addWriter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	return eventName, eventID
}

// newEventWithReporter creates a fresh Event, grants Alice the Reporter role on it,
// and gives the admin the Writer role so the admin can author entities to test
// against. A Reporter has EventReadOwnFieldReports but not EventReadAllFieldReports,
// so the handlers treat them as "limited access" and check report entry authorship.
//
// The admin needs an explicit grant here: being an IMS admin confers global
// permissions only, not per-event ones.
func newEventWithReporter(t *testing.T, admin ApiHelper) (eventName string) {
	t.Helper()
	ctx := t.Context()
	eventName = rand.NonCryptoText()
	_, resp := admin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = admin.addReporter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = admin.addWriter(ctx, eventName, userAdminHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	return eventName
}

// newEventWithReporterAndReader creates a fresh Event on which Alice is both a
// Reporter and a Reader, and the admin is a Writer. That combination — may read
// every Field Report, may write only her own — is what a Ranger ends up with
// from a blanket "report" rule plus a "read" rule for their position, and it's
// the case that tells a handler gating on EventWriteAllFieldReports apart from
// one wrongly gating on EventReadAllFieldReports.
func newEventWithReporterAndReader(t *testing.T, admin ApiHelper) (eventName string) {
	t.Helper()
	ctx := t.Context()
	eventName = rand.NonCryptoText()
	_, resp := admin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	alice := []imsjson.AccessRule{{Expression: "person:" + userAliceHandle, Validity: "always"}}
	resp = admin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Readers:   alice,
			Reporters: alice,
			Writers:   []imsjson.AccessRule{{Expression: "person:" + userAdminHandle, Validity: "always"}},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	return eventName
}

// Attaching a file adds a Report Entry, so it's an edit, and it has to be held
// to the write permission. A Ranger who may read every Field Report but write
// only her own must not be able to attach to someone else's.
func TestAttachToFieldReportDeniedForReaderWhoMayOnlyWriteOwn(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithReporterAndReader(t, apisAdmin)
	num := apisAdmin.newFieldReportSuccess(ctx, sampleFieldReport1(eventName))

	_, resp := apisAlice.attachFileToFieldReport(ctx, eventName, num, []byte("Alice was here"))
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// Alice can read that same Field Report, which is what makes the 403 above
	// about the write permission rather than the read one.
	_, resp = apisAlice.getFieldReport(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// A Reporter may only read Field Reports they authored. Reading an attachment on
// someone else's Field Report must be refused, even though the Reporter can read
// Field Reports on this Event in general.
func TestGetFieldReportAttachmentDeniedForNonAuthoringReporter(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithReporter(t, apisAdmin)

	// The admin authors the Field Report and its attachment, so Alice authored nothing on it.
	num := apisAdmin.newFieldReportSuccess(ctx, sampleFieldReport1(eventName))
	reID, resp := apisAdmin.attachFileToFieldReport(ctx, eventName, num, []byte("admin's private notes"))
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	_, resp = apisAlice.getFieldReportAttachment(ctx, eventName, num, reID)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	// The admin can still read it, confirming the 403 above is about authorship
	// rather than a broken attachment.
	body, resp := apisAdmin.getFieldReportAttachment(ctx, eventName, num, reID)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, []byte("admin's private notes"), body)
}

// The same authorship check guards uploads, not just reads.
func TestAttachToFieldReportDeniedForNonAuthoringReporter(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithReporter(t, apisAdmin)
	num := apisAdmin.newFieldReportSuccess(ctx, sampleFieldReport1(eventName))

	_, resp := apisAlice.attachFileToFieldReport(ctx, eventName, num, []byte("Alice was here"))
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// A Reporter reading an attachment on their *own* Field Report is allowed. This is
// the other side of the authorship check above.
func TestGetFieldReportAttachmentAllowedForAuthoringReporter(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithReporter(t, apisAdmin)
	num := apisAlice.newFieldReportSuccess(ctx, sampleFieldReport1(eventName))

	fileBytes := []byte("Alice's own report attachment")
	reID, resp := apisAlice.attachFileToFieldReport(ctx, eventName, num, fileBytes)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	body, resp := apisAlice.getFieldReportAttachment(ctx, eventName, num, reID)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, fileBytes, body)
}

// Asking for an attachment by the ID of a report entry that carries no file is a
// 404, not a 500 or an empty 200.
func TestGetIncidentAttachmentForEntryWithNoFileIs404(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)
	num := apisAlice.newIncidentSuccess(ctx, sampleIncident1(eventName))

	// The sample incident carries plain report entries with no attached file.
	incident, resp := apisAlice.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotEmpty(t, incident.ReportEntries)
	entryWithoutFile := incident.ReportEntries[0]
	require.Empty(t, entryWithoutFile.Attachment.Name)

	_, resp = apisAlice.getIncidentAttachment(ctx, eventName, num, entryWithoutFile.ID)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// An attachment number that matches no report entry at all is likewise a 404.
func TestGetIncidentAttachmentForUnknownEntryIs404(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)
	num := apisAlice.newIncidentSuccess(ctx, sampleIncident1(eventName))

	_, resp := apisAlice.getIncidentAttachment(ctx, eventName, num, 999999)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// A POST that isn't a well-formed multipart upload is a client error, not a panic
// or a 500.
func TestAttachToIncidentWithMalformedBodyIs400(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)
	num := apisAlice.newIncidentSuccess(ctx, sampleIncident1(eventName))

	path := shared.serverURL.JoinPath(
		"/ims/api/events", eventName, "incidents", conv.FormatInt(num), "attachments",
	).String()
	// multipart/form-data with no boundary parameter, and a JSON body behind it.
	resp := apisAlice.imsPostContentType(ctx, path, "multipart/form-data")
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAttachToFieldReportWithMalformedBodyIs400(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)
	num := apisAlice.newFieldReportSuccess(ctx, sampleFieldReport1(eventName))

	path := shared.serverURL.JoinPath(
		"/ims/api/events", eventName, "field_reports", conv.FormatInt(num), "attachments",
	).String()
	resp := apisAlice.imsPostContentType(ctx, path, "multipart/form-data")
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestAttachToVisitWithMalformedBodyIs400(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)
	num := apisAlice.newVisitSuccess(ctx, sampleVisit1(eventName))

	path := shared.serverURL.JoinPath(
		"/ims/api/events", eventName, "visits", conv.FormatInt(num), "attachments",
	).String()
	resp := apisAlice.imsPostContentType(ctx, path, "multipart/form-data")
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Non-numeric path segments must be rejected as client errors rather than
// reaching the store.
func TestIncidentAttachmentWithUnparseableNumbersIs400(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)

	badIncident := shared.serverURL.JoinPath(
		"/ims/api/events", eventName, "incidents", "not-a-number", "attachments", "1",
	).String()
	_, resp := apisAlice.imsGetBodyBytes(ctx, badIncident)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	badAttachment := shared.serverURL.JoinPath(
		"/ims/api/events", eventName, "incidents", "1", "attachments", "not-a-number",
	).String()
	_, resp = apisAlice.imsGetBodyBytes(ctx, badAttachment)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	badUpload := shared.serverURL.JoinPath(
		"/ims/api/events", eventName, "incidents", "not-a-number", "attachments",
	).String()
	resp = apisAlice.imsPostContentType(ctx, badUpload, "multipart/form-data")
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestFieldReportAttachmentWithUnparseableNumbersIs400(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)

	badFieldReport := shared.serverURL.JoinPath(
		"/ims/api/events", eventName, "field_reports", "not-a-number", "attachments", "1",
	).String()
	_, resp := apisAlice.imsGetBodyBytes(ctx, badFieldReport)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	badAttachment := shared.serverURL.JoinPath(
		"/ims/api/events", eventName, "field_reports", "1", "attachments", "not-a-number",
	).String()
	_, resp = apisAlice.imsGetBodyBytes(ctx, badAttachment)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	badUpload := shared.serverURL.JoinPath(
		"/ims/api/events", eventName, "field_reports", "not-a-number", "attachments",
	).String()
	resp = apisAlice.imsPostContentType(ctx, badUpload, "multipart/form-data")
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestVisitAttachmentWithUnparseableNumbersIs400(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)

	badVisit := shared.serverURL.JoinPath(
		"/ims/api/events", eventName, "visits", "not-a-number", "attachments", "1",
	).String()
	_, resp := apisAlice.imsGetBodyBytes(ctx, badVisit)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	badAttachment := shared.serverURL.JoinPath(
		"/ims/api/events", eventName, "visits", "1", "attachments", "not-a-number",
	).String()
	_, resp = apisAlice.imsGetBodyBytes(ctx, badAttachment)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	badUpload := shared.serverURL.JoinPath(
		"/ims/api/events", eventName, "visits", "not-a-number", "attachments",
	).String()
	resp = apisAlice.imsPostContentType(ctx, badUpload, "multipart/form-data")
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// Attachments are served back in the same origin as IMS, so an uploaded HTML
// document must not come back as text/html — otherwise the browser renders it and
// we have stored XSS.
func TestUploadedHTMLIsNotServedAsHTML(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)
	num := apisAlice.newIncidentSuccess(ctx, sampleIncident1(eventName))

	html := []byte("<html><body><script>alert(1)</script></body></html>")
	reID, resp := apisAlice.attachFileToIncident(ctx, eventName, num, html)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	body, resp := apisAlice.getIncidentAttachment(ctx, eventName, num, reID)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, html, body)

	contentType := resp.Header.Get("Content-Type")
	require.NotContains(t, contentType, "text/html")
	require.Contains(t, contentType, "text/plain")
}

// SVG is scriptable, and isn't on the preview allowlist, so it must be served as
// an opaque download.
func TestUploadedSVGIsServedAsOctetStream(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)
	num := apisAlice.newIncidentSuccess(ctx, sampleIncident1(eventName))

	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	reID, resp := apisAlice.attachFileToIncident(ctx, eventName, num, svg)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	_, resp = apisAlice.getIncidentAttachment(ctx, eventName, num, reID)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)

	contentType := resp.Header.Get("Content-Type")
	require.NotContains(t, contentType, "svg")
	require.Contains(t, contentType, "application/octet-stream")
}

// A PNG is on the allowlist, so it keeps its real media type and stays previewable.
func TestUploadedPNGKeepsItsContentType(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := newEventWithWriter(t, apisAdmin)
	num := apisAlice.newIncidentSuccess(ctx, sampleIncident1(eventName))

	reID, resp := apisAlice.attachFileToIncident(ctx, eventName, num, onePixelPNG)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	_, resp = apisAlice.getIncidentAttachment(ctx, eventName, num, reID)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "image/png")

	// The entity's report entry advertises the attachment as previewable.
	incident, resp := apisAlice.getIncident(ctx, eventName, num)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var found bool
	for _, re := range incident.ReportEntries {
		if re.ID == reID {
			found = true
			require.True(t, re.Attachment.Previewable)
		}
	}
	require.True(t, found, "did not find the report entry for the uploaded attachment")
}

// Attaching a file to a Field Report that is linked to an Incident must notify
// watchers of the parent Incident too, since the Incident page renders the Field
// Report's entries.
func TestAttachToLinkedFieldReportNotifiesParentIncident(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName, eventID := newEventWithWriterID(t, apisAdmin)
	incidentNum := apisAlice.newIncidentSuccess(ctx, sampleIncident1(eventName))
	frNum := apisAlice.newFieldReportSuccess(ctx, sampleFieldReport1(eventName))

	resp := apisAlice.attachFieldReportToIncident(ctx, eventName, frNum, incidentNum)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	events := subscribeToEventSource(ctx, t)

	_, resp = apisAlice.attachFileToFieldReport(ctx, eventName, frNum, []byte("some evidence"))
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// The SSE channel is global and other parallel tests publish to it, so match on
	// this test's own Event ID rather than taking whatever arrives first.
	wantIncident := api.IMSEventData{EventID: eventID, IncidentNumber: incidentNum}
	wantFieldReport := api.IMSEventData{EventID: eventID, FieldReportNumber: frNum}
	require.True(t, events.await(wantFieldReport), "no SSE push for the updated Field Report")
	require.True(t, events.await(wantIncident), "no SSE push for the parent Incident")
}

// onePixelPNG is a minimal valid PNG, used to exercise the previewable-content-type
// path with a real image rather than a sniffed text file.
var onePixelPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T',
	0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05,
	0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00,
	0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

// sseWatcher collects Server-Sent Events published after it was created.
type sseWatcher struct {
	t    *testing.T
	seen chan api.IMSEventData
}

// subscribeToEventSource opens a streaming connection to the SSE endpoint and reads
// pushes into a channel until the test ends.
func subscribeToEventSource(ctx context.Context, t *testing.T) *sseWatcher {
	t.Helper()

	path := shared.serverURL.JoinPath("ims/api/eventsource").String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	require.NoError(t, err)
	// #nosec G704 // SSRF via taint analysis. We control the URLs.
	resp, err := (&http.Client{}).Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	t.Cleanup(func() { _ = resp.Body.Close() })

	w := &sseWatcher{t: t, seen: make(chan api.IMSEventData, 128)}
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			data, ok := strings.CutPrefix(scanner.Text(), "data: ")
			if !ok {
				continue
			}
			var parsed api.IMSEventData
			err := json.Unmarshal([]byte(data), &parsed)
			if err != nil {
				continue
			}
			select {
			case w.seen <- parsed:
			default:
			}
		}
	}()
	return w
}

// await reports whether want shows up on the channel before a short deadline,
// discarding unrelated pushes from other parallel tests along the way.
func (w *sseWatcher) await(want api.IMSEventData) bool {
	w.t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case got := <-w.seen:
			if got == want {
				return true
			}
		case <-deadline:
			return false
		}
	}
}
