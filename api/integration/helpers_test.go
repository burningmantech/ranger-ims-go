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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/burningmantech/ranger-ims-go/api"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/require"
)

type ApiHelper struct {
	t         *testing.T
	serverURL *url.URL
	jwt       string
	referrer  string
}

func (a ApiHelper) postAuth(ctx context.Context, req api.PostAuthRequest) (statusCode int, body, validJWT string) {
	a.t.Helper()
	response := &api.PostAuthResponse{}
	resp := a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/auth").String())
	b, err := io.ReadAll(resp.Body)
	require.NoError(a.t, resp.Body.Close())
	require.NoError(a.t, err)
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, string(b), ""
	}
	err = json.Unmarshal(b, &response)
	require.NoError(a.t, err)
	return resp.StatusCode, string(b), response.Token
}

func (a ApiHelper) refreshAccessToken(ctx context.Context, refreshCookie *http.Cookie) (statusCode int, result *api.RefreshAccessTokenResponse) {
	a.t.Helper()
	response := &api.RefreshAccessTokenResponse{}
	postBody, err := json.Marshal(struct{}{})
	require.NoError(a.t, err)
	httpPost, err := http.NewRequestWithContext(ctx, http.MethodPost, a.serverURL.JoinPath("/ims/api/auth/refresh").String(), bytes.NewReader(postBody))
	require.NoError(a.t, err)
	if a.jwt != "" {
		httpPost.Header.Set("Authorization", "Bearer "+a.jwt)
	}
	httpPost.AddCookie(refreshCookie)
	// #nosec G704 // SSRF via taint analysis. We control the URLs.
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	// #nosec G704 // SSRF via taint analysis.
	resp, err := client.Do(httpPost)
	require.NoError(a.t, err)

	b, err := io.ReadAll(resp.Body)
	require.NoError(a.t, err)
	require.NoError(a.t, resp.Body.Close())
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil
	}
	err = json.Unmarshal(b, &response)
	require.NoError(a.t, err)
	return resp.StatusCode, response
}

func (a ApiHelper) getAuth(ctx context.Context, eventName string) (api.GetAuthResponse, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/auth").String()
	if eventName != "" {
		path = path + "?event_id=" + eventName
	}
	return a.imsGet[api.GetAuthResponse](ctx, path)
}

func (a ApiHelper) editType(ctx context.Context, req imsjson.IncidentType) (*int32, *http.Response) {
	a.t.Helper()
	httpResp := a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/incident_types").String())
	numStr := httpResp.Header.Get("IMS-Incident-Type-ID")
	require.NoError(a.t, httpResp.Body.Close())
	if numStr == "" {
		return nil, httpResp
	}
	num, err := conv.ParseInt32(numStr)
	require.NoError(a.t, err)
	return &num, httpResp
}

func (a ApiHelper) getTypes(ctx context.Context) (imsjson.IncidentTypes, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/incident_types").String()
	return a.imsGet[imsjson.IncidentTypes](ctx, path)
}

func (a ApiHelper) editPlaces(ctx context.Context, eventName string, req imsjson.Places) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/places").String())
}

func (a ApiHelper) importPlaces(ctx context.Context, eventName, placeType, year string) *http.Response {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/places/import")
	q := path.Query()
	q.Set("place_type", placeType)
	q.Set("year", year)
	path.RawQuery = q.Encode()
	return a.imsPost[any](ctx, nil, path.String())
}

func (a ApiHelper) getPlaces(ctx context.Context, eventName string) (imsjson.Places, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/places").String()
	return a.imsGet[imsjson.Places](ctx, path)
}

func (a ApiHelper) getPlacesExcludingExternalData(ctx context.Context, eventName string) (imsjson.Places, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/places")
	q := path.Query()
	q.Set("exclude_external_data", "true")
	path.RawQuery = q.Encode()
	return a.imsGet[imsjson.Places](ctx, path.String())
}

func (a ApiHelper) newFieldReport(ctx context.Context, req imsjson.FieldReport) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/"+req.Event+"/field_reports").String())
}

func (a ApiHelper) newFieldReportSuccess(ctx context.Context, fieldReportReq imsjson.FieldReport) (fieldReport int32) {
	a.t.Helper()
	httpResp := a.newFieldReport(ctx, fieldReportReq)
	require.Equal(a.t, http.StatusCreated, httpResp.StatusCode)
	numStr := httpResp.Header.Get("IMS-Field-Report-Number")
	require.NoError(a.t, httpResp.Body.Close())
	require.NotEmpty(a.t, numStr)
	num, err := conv.ParseInt32(numStr)
	require.NoError(a.t, err)
	require.Positive(a.t, num)
	return num
}

func (a ApiHelper) getFieldReport(ctx context.Context, eventName string, fieldReport int32) (imsjson.FieldReport, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/field_reports/", strconv.Itoa(int(fieldReport))).String()
	return a.imsGet[imsjson.FieldReport](ctx, path)
}

func (a ApiHelper) getFieldReports(ctx context.Context, eventName string) (imsjson.FieldReports, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath(fmt.Sprint("/ims/api/events/", eventName, "/field_reports")).String()
	return a.imsGet[imsjson.FieldReports](ctx, path)
}

func (a ApiHelper) updateFieldReport(ctx context.Context, eventName string, fieldReport int32, req imsjson.FieldReport) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/field_reports/", conv.FormatInt(fieldReport)).String())
}

func (a ApiHelper) attachFieldReportToIncident(ctx context.Context, eventName string, fieldReport int32, incident int32) *http.Response {
	a.t.Helper()
	req := imsjson.FieldReport{}
	params := "?action=attach&incident=" + conv.FormatInt(incident)
	return a.imsPost(ctx, req,
		a.serverURL.JoinPath("/ims/api/events/", eventName, "/field_reports/",
			conv.FormatInt(fieldReport)).String()+params)
}

func (a ApiHelper) detachFieldReportFromIncident(ctx context.Context, eventName string, fieldReport int32) *http.Response {
	a.t.Helper()
	req := imsjson.FieldReport{}
	params := "?action=detach"
	return a.imsPost(ctx, req,
		a.serverURL.JoinPath("/ims/api/events/", eventName, "/field_reports/",
			conv.FormatInt(fieldReport)).String()+params)
}

func (a ApiHelper) newIncident(ctx context.Context, req imsjson.Incident) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/"+req.Event+"/incidents").String())
}

func (a ApiHelper) newIncidentSuccess(ctx context.Context, incidentReq imsjson.Incident) (incidentNumber int32) {
	a.t.Helper()
	resp := a.newIncident(ctx, incidentReq)
	require.Equal(a.t, http.StatusCreated, resp.StatusCode)
	numStr := resp.Header.Get("IMS-Incident-Number")
	require.NoError(a.t, resp.Body.Close())
	require.NotEmpty(a.t, numStr)
	num, err := conv.ParseInt32(numStr)
	require.NoError(a.t, err)
	require.Positive(a.t, num)
	return num
}

func (a ApiHelper) newVisit(ctx context.Context, req imsjson.Visit) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/"+req.Event+"/visits").String())
}

func (a ApiHelper) newVisitSuccess(ctx context.Context, visitReq imsjson.Visit) (visitNumber int32) {
	a.t.Helper()
	resp := a.newVisit(ctx, visitReq)
	require.Equal(a.t, http.StatusCreated, resp.StatusCode)
	numStr := resp.Header.Get("IMS-Visit-Number")
	require.NoError(a.t, resp.Body.Close())
	require.NotEmpty(a.t, numStr)
	num, err := conv.ParseInt32(numStr)
	require.NoError(a.t, err)
	require.Positive(a.t, num)
	return num
}

func (a ApiHelper) getIncident(ctx context.Context, eventName string, incident int32) (imsjson.Incident, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents/", strconv.Itoa(int(incident))).String()
	return a.imsGet[imsjson.Incident](ctx, path)
}

func (a ApiHelper) getVisit(ctx context.Context, eventName string, visit int32) (imsjson.Visit, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/visits/", strconv.Itoa(int(visit))).String()
	return a.imsGet[imsjson.Visit](ctx, path)
}

func (a ApiHelper) updateIncident(ctx context.Context, eventName string, incident int32, req imsjson.Incident) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents/", strconv.Itoa(int(incident))).String())
}

func (a ApiHelper) updateVisit(ctx context.Context, eventName string, visit int32, req imsjson.Visit) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/visits/", strconv.Itoa(int(visit))).String())
}

func (a ApiHelper) attachRangerToIncident(ctx context.Context, eventName string, incident int32, handle string) *http.Response {
	a.t.Helper()
	return a.setIncidentRangerRole(ctx, eventName, incident, handle, nil)
}

func (a ApiHelper) setIncidentRangerRole(
	ctx context.Context, eventName string, incident int32, handle string, role *string,
) *http.Response {
	a.t.Helper()
	req := imsjson.IncidentRanger{Handle: handle, Role: role}
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents/", strconv.Itoa(int(incident)), "/rangers/", handle).String())
}

func (a ApiHelper) attachRangerToVisit(ctx context.Context, eventName string, visit int32, handle string) *http.Response {
	a.t.Helper()
	return a.setVisitRangerRole(ctx, eventName, visit, handle, nil)
}

func (a ApiHelper) setVisitRangerRole(
	ctx context.Context, eventName string, visit int32, handle string, role *string,
) *http.Response {
	a.t.Helper()
	req := imsjson.VisitRanger{Handle: handle, Role: role}
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/visits/", strconv.Itoa(int(visit)), "/rangers/", handle).String())
}

func (a ApiHelper) attachTypeToIncident(ctx context.Context, eventName string, incident, incidentTypeID int32) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, struct{}{}, a.incidentTypePath(eventName, incident, incidentTypeID))
}

func (a ApiHelper) detachTypeFromIncident(ctx context.Context, eventName string, incident, incidentTypeID int32) *http.Response {
	a.t.Helper()
	return a.imsDelete(ctx, a.incidentTypePath(eventName, incident, incidentTypeID))
}

func (a ApiHelper) incidentTypePath(eventName string, incident, incidentTypeID int32) string {
	a.t.Helper()
	return a.serverURL.JoinPath(
		"/ims/api/events/", eventName, "/incidents/", conv.FormatInt(incident),
		"/incident_types/", conv.FormatInt(incidentTypeID),
	).String()
}

func (a ApiHelper) linkIncident(
	ctx context.Context, eventName string, incident int32, linkedEventName string, linkedIncident int32,
) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, struct{}{}, a.linkedIncidentPath(eventName, incident, linkedEventName, linkedIncident))
}

func (a ApiHelper) unlinkIncident(
	ctx context.Context, eventName string, incident int32, linkedEventName string, linkedIncident int32,
) *http.Response {
	a.t.Helper()
	return a.imsDelete(ctx, a.linkedIncidentPath(eventName, incident, linkedEventName, linkedIncident))
}

func (a ApiHelper) linkedIncidentPath(
	eventName string, incident int32, linkedEventName string, linkedIncident int32,
) string {
	a.t.Helper()
	return a.serverURL.JoinPath(
		"/ims/api/events/", eventName, "/incidents/", conv.FormatInt(incident),
		"/linked_incidents/", linkedEventName, conv.FormatInt(linkedIncident),
	).String()
}

func (a ApiHelper) detachRangerFromIncident(ctx context.Context, eventName string, incident int32, handle string) *http.Response {
	a.t.Helper()
	return a.imsDelete(ctx, a.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents/", strconv.Itoa(int(incident)), "/rangers/", handle).String())
}

func (a ApiHelper) detachRangerFromVisit(ctx context.Context, eventName string, visit int32, handle string) *http.Response {
	a.t.Helper()
	return a.imsDelete(ctx, a.serverURL.JoinPath("/ims/api/events/", eventName, "/visits/", strconv.Itoa(int(visit)), "/rangers/", handle).String())
}

func (a ApiHelper) getIncidents(ctx context.Context, eventName string) (imsjson.Incidents, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath(fmt.Sprint("/ims/api/events/", eventName, "/incidents")).String()
	return a.imsGet[imsjson.Incidents](ctx, path)
}

func (a ApiHelper) getVisits(ctx context.Context, eventName string) (imsjson.Visits, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath(fmt.Sprint("/ims/api/events/", eventName, "/visits")).String()
	return a.imsGet[imsjson.Visits](ctx, path)
}

func (a ApiHelper) updateIncidentReportEntry(ctx context.Context, eventName string, incident int32, req imsjson.ReportEntry) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents/", conv.FormatInt(incident), "/report_entries/", conv.FormatInt(req.ID)).String())
}

func (a ApiHelper) updateFieldReportReportEntry(ctx context.Context, eventName string, fieldReport int32, req imsjson.ReportEntry) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/field_reports/", conv.FormatInt(fieldReport), "/report_entries/", conv.FormatInt(req.ID)).String())
}

func (a ApiHelper) updateVisitReportEntry(ctx context.Context, eventName string, visit int32, req imsjson.ReportEntry) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/visits/", conv.FormatInt(visit), "/report_entries/", conv.FormatInt(req.ID)).String())
}

func (a ApiHelper) search(ctx context.Context, params url.Values) (imsjson.SearchResults, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/search")
	path.RawQuery = params.Encode()
	return a.imsGet[imsjson.SearchResults](ctx, path.String())
}

func (a ApiHelper) editEvent(ctx context.Context, req imsjson.Event) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events").String())
}

func (a ApiHelper) createEvent(ctx context.Context, req imsjson.Event) (eventID int32, resp *http.Response) {
	a.t.Helper()
	resp = a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events").String())
	var err error
	eventID, err = conv.ParseInt32(resp.Header.Get("IMS-Event-ID"))
	require.NoError(a.t, err)
	return eventID, resp
}

func (a ApiHelper) deleteEvent(ctx context.Context, eventName string) (resp *http.Response) {
	a.t.Helper()
	return a.imsDelete(ctx, a.serverURL.JoinPath("/ims/api/events/", eventName).String())
}

func (a ApiHelper) getEvents(ctx context.Context) (imsjson.Events, *http.Response) {
	a.t.Helper()
	return a.imsGet[imsjson.Events](ctx, a.serverURL.JoinPath("/ims/api/events").String())
}

func (a ApiHelper) getEventsIncludingGroups(ctx context.Context) (imsjson.Events, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events")
	q := path.Query()
	q.Set("include_groups", "true")
	path.RawQuery = q.Encode()
	return a.imsGet[imsjson.Events](ctx, path.String())
}

func (a ApiHelper) addWriter(ctx context.Context, eventName, handle string) *http.Response {
	a.t.Helper()
	return a.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Writers: []imsjson.AccessRule{{
				Expression: "person:" + handle,
				Validity:   "always",
			}},
		},
	})
}

func (a ApiHelper) addReporter(ctx context.Context, eventName, handle string) *http.Response {
	a.t.Helper()
	return a.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Reporters: []imsjson.AccessRule{{
				Expression: "person:" + handle,
				Validity:   "always",
			}},
		},
	})
}

func (a ApiHelper) addVisitWriter(ctx context.Context, eventName, handle string) *http.Response {
	a.t.Helper()
	return a.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			VisitWriters: []imsjson.AccessRule{{
				Expression: "person:" + handle,
				Validity:   "always",
			}},
		},
	})
}

func (a ApiHelper) editAccess(ctx context.Context, req imsjson.EventsAccess) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/access").String())
}

func (a ApiHelper) getAccess(ctx context.Context) (imsjson.EventsAccess, *http.Response) {
	a.t.Helper()
	return a.imsGet[imsjson.EventsAccess](ctx, a.serverURL.JoinPath("/ims/api/access").String())
}

func (a ApiHelper) getAccessTargets(ctx context.Context) (imsjson.AccessTargets, *http.Response) {
	a.t.Helper()
	return a.imsGet[imsjson.AccessTargets](ctx, a.serverURL.JoinPath("/ims/api/access_targets").String())
}

func (a ApiHelper) attachFileToIncident(ctx context.Context, eventName string, incident int32, fileBytes []byte) (int32, *http.Response) {
	a.t.Helper()

	path := a.serverURL.JoinPath("/ims/api/events", eventName, "incidents", conv.FormatInt(incident), "attachments")

	// Create a `multipart/form-data`-encoded request, with a single form file inside
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	part, err := writer.CreateFormFile(api.IMSAttachmentFormKey, "irrelevant-filename-"+rand.NonCryptoText())
	require.NoError(a.t, err)
	_, err = part.Write(fileBytes)
	require.NoError(a.t, err)
	require.NoError(a.t, writer.Close())

	httpPost, err := http.NewRequestWithContext(ctx, http.MethodPost, path.String(), &requestBody)
	require.NoError(a.t, err)
	if a.jwt != "" {
		httpPost.Header.Set("Authorization", "Bearer "+a.jwt)
	}
	httpPost.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	// #nosec G704 // SSRF via taint analysis. We control the URLs.
	resp, err := client.Do(httpPost)
	require.NoError(a.t, err)

	reID, _ := conv.ParseInt32(resp.Header.Get("IMS-Report-Entry-Number"))

	return reID, resp
}

func (a ApiHelper) attachFileToVisit(ctx context.Context, eventName string, visit int32, fileBytes []byte) (int32, *http.Response) {
	a.t.Helper()

	path := a.serverURL.JoinPath("/ims/api/events", eventName, "visits", conv.FormatInt(visit), "attachments")

	// Create a `multipart/form-data`-encoded request, with a single form file inside
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	part, err := writer.CreateFormFile(api.IMSAttachmentFormKey, "irrelevant-filename-"+rand.NonCryptoText())
	require.NoError(a.t, err)
	_, err = part.Write(fileBytes)
	require.NoError(a.t, err)
	require.NoError(a.t, writer.Close())

	httpPost, err := http.NewRequestWithContext(ctx, http.MethodPost, path.String(), &requestBody)
	require.NoError(a.t, err)
	if a.jwt != "" {
		httpPost.Header.Set("Authorization", "Bearer "+a.jwt)
	}
	httpPost.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	// #nosec G704 // SSRF via taint analysis.
	resp, err := client.Do(httpPost)
	require.NoError(a.t, err)

	reID, _ := conv.ParseInt32(resp.Header.Get("IMS-Report-Entry-Number"))

	return reID, resp
}

func (a ApiHelper) getIncidentAttachment(ctx context.Context, eventName string, incident, reID int32) ([]byte, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events", eventName, "incidents", conv.FormatInt(incident), "attachments", conv.FormatInt(reID)).String()
	return a.imsGetBodyBytes(ctx, path)
}

func (a ApiHelper) getVisitAttachment(ctx context.Context, eventName string, visit, reID int32) ([]byte, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events", eventName, "visits", conv.FormatInt(visit), "attachments", conv.FormatInt(reID)).String()
	return a.imsGetBodyBytes(ctx, path)
}

func (a ApiHelper) attachFileToFieldReport(ctx context.Context, eventName string, fieldReport int32, fileBytes []byte) (int32, *http.Response) {
	a.t.Helper()

	path := a.serverURL.JoinPath("/ims/api/events", eventName, "field_reports", conv.FormatInt(fieldReport), "attachments")

	// Create a `multipart/form-data`-encoded request, with a single form file inside
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	part, err := writer.CreateFormFile(api.IMSAttachmentFormKey, "irrelevant-filename-"+rand.NonCryptoText())
	require.NoError(a.t, err)
	_, err = part.Write(fileBytes)
	require.NoError(a.t, err)
	require.NoError(a.t, writer.Close())

	httpPost, err := http.NewRequestWithContext(ctx, http.MethodPost, path.String(), &requestBody)
	require.NoError(a.t, err)
	if a.jwt != "" {
		httpPost.Header.Set("Authorization", "Bearer "+a.jwt)
	}
	httpPost.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	// #nosec G704 // SSRF via taint analysis.
	resp, err := client.Do(httpPost)
	require.NoError(a.t, err)

	reID, _ := conv.ParseInt32(resp.Header.Get("IMS-Report-Entry-Number"))

	return reID, resp
}

func (a ApiHelper) getFieldReportAttachment(ctx context.Context, eventName string, fieldReport, reID int32) ([]byte, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events", eventName, "field_reports", conv.FormatInt(fieldReport), "attachments", conv.FormatInt(reID)).String()
	return a.imsGetBodyBytes(ctx, path)
}

func (a ApiHelper) imsPost[T any](ctx context.Context, body T, path string) *http.Response {
	a.t.Helper()
	postBody, err := json.Marshal(body)
	require.NoError(a.t, err)
	httpPost, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(postBody))
	require.NoError(a.t, err)
	httpPost.Header.Set("Content-Type", "application/json")
	if a.jwt != "" {
		httpPost.Header.Set("Authorization", "Bearer "+a.jwt)
	}
	if a.referrer != "" {
		httpPost.Header.Set("Referer", a.referrer)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	// #nosec G704 // SSRF via taint analysis.
	resp, err := client.Do(httpPost)
	require.NoError(a.t, err)
	return resp
}

// imsPostContentType sends a POST with the given Content-Type, or with none at
// all if contentType is empty. Only the endpoints that reject a non-JSON
// Content-Type need this; imsPost always sends application/json.
func (a ApiHelper) imsPostContentType(ctx context.Context, path, contentType string) *http.Response {
	a.t.Helper()
	httpPost, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader([]byte("{}")))
	require.NoError(a.t, err)
	if contentType != "" {
		httpPost.Header.Set("Content-Type", contentType)
	}
	if a.jwt != "" {
		httpPost.Header.Set("Authorization", "Bearer "+a.jwt)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	// #nosec G704 // SSRF via taint analysis. We control the URLs.
	resp, err := client.Do(httpPost)
	require.NoError(a.t, err)
	return resp
}

func (a ApiHelper) imsGetBodyBytes(ctx context.Context, path string) ([]byte, *http.Response) {
	a.t.Helper()
	_, b, err := a.imsDoNoReqBody[any](ctx, http.MethodGet, path)
	return b, err
}

func (a ApiHelper) imsDelete(ctx context.Context, path string) *http.Response {
	a.t.Helper()
	_, _, resp := a.imsDoNoReqBody[any](ctx, http.MethodDelete, path)
	return resp
}

func (a ApiHelper) imsGet[V any](ctx context.Context, path string) (V, *http.Response) {
	a.t.Helper()
	parsed, _, err := a.imsDoNoReqBody[V](ctx, http.MethodGet, path)
	return parsed, err
}

func (a ApiHelper) imsDoNoReqBody[V any](ctx context.Context, method, path string) (V, []byte, *http.Response) {
	a.t.Helper()
	httpReq, err := http.NewRequestWithContext(ctx, method, path, nil)
	require.NoError(a.t, err)
	if a.jwt != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.jwt)
	}
	if a.referrer != "" {
		httpReq.Header.Set("Referer", a.referrer)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	// #nosec G704 // SSRF via taint analysis.
	get, err := client.Do(httpReq)
	require.NoError(a.t, err)
	b, err := io.ReadAll(get.Body)
	require.NoError(a.t, err)
	require.NoError(a.t, get.Body.Close())
	var resp V
	if get.Header["Content-Type"][0] == "application/json" {
		err = json.Unmarshal(b, &resp)
		if err != nil && get.StatusCode != http.StatusOK {
			return resp, b, get
		}
		require.NoError(a.t, err)
	}
	return resp, b, get
}

func (a ApiHelper) getActionLogs(ctx context.Context, minTime, maxTime string) (imsjson.ActionLogs, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/actionlogs")
	q := path.Query()
	q.Set("minTimeUnixMs", minTime)
	q.Set("maxTimeUnixMs", maxTime)
	path.RawQuery = q.Encode()

	return a.imsGet[imsjson.ActionLogs](ctx, path.String())
}

func (a ApiHelper) getErrorLogs(ctx context.Context, minTime, maxTime string) (imsjson.ErrorLogs, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/errorlogs")
	q := path.Query()
	q.Set("minTimeUnixMs", minTime)
	q.Set("maxTimeUnixMs", maxTime)
	path.RawQuery = q.Encode()

	return a.imsGet[imsjson.ErrorLogs](ctx, path.String())
}

func jwtForAlice(t *testing.T, ctx context.Context) string {
	t.Helper()
	apisNotAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}
	statusCode, _, token := apisNotAuthenticated.postAuth(ctx, api.PostAuthRequest{
		Identification: userAliceEmail,
		Password:       userAlicePassword,
	})
	require.Equal(t, http.StatusOK, statusCode)
	return token
}

func jwtForAdmin(ctx context.Context, t *testing.T) string {
	t.Helper()
	apisNotAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}
	statusCode, _, token := apisNotAuthenticated.postAuth(ctx, api.PostAuthRequest{
		Identification: userAdminEmail,
		Password:       userAdminPassword,
	})
	require.Equal(t, http.StatusOK, statusCode)
	return token
}
