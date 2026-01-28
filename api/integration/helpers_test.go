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
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
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
	bod, resp := a.imsGet(ctx, path, &api.GetAuthResponse{})
	return *bod.(*api.GetAuthResponse), resp
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
	bod, resp := a.imsGet(ctx, path, &imsjson.IncidentTypes{})
	return *bod.(*imsjson.IncidentTypes), resp
}

func (a ApiHelper) editDestinations(ctx context.Context, eventName string, req imsjson.Destinations) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/destinations").String())
}

func (a ApiHelper) getDestinations(ctx context.Context, eventName string) (imsjson.Destinations, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/destinations").String()
	bod, resp := a.imsGet(ctx, path, &imsjson.Destinations{})
	return *bod.(*imsjson.Destinations), resp
}

func (a ApiHelper) editStreets(ctx context.Context, req imsjson.EventsStreets) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/streets").String())
}

func (a ApiHelper) getStreets(ctx context.Context, eventID int32) (imsjson.EventsStreets, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/streets").String()
	if eventID != 0 {
		path += fmt.Sprintf("?event_id=%d", eventID)
	}
	bod, resp := a.imsGet(ctx, path, &imsjson.EventsStreets{})
	return *bod.(*imsjson.EventsStreets), resp
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
	bod, resp := a.imsGet(ctx, path, &imsjson.FieldReport{})
	return *bod.(*imsjson.FieldReport), resp
}

func (a ApiHelper) getFieldReports(ctx context.Context, eventName string) (imsjson.FieldReports, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath(fmt.Sprint("/ims/api/events/", eventName, "/field_reports")).String()
	bod, resp := a.imsGet(ctx, path, &imsjson.FieldReports{})
	return *bod.(*imsjson.FieldReports), resp
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

func (a ApiHelper) newStay(ctx context.Context, req imsjson.Stay) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/"+req.Event+"/stays").String())
}

func (a ApiHelper) newStaySuccess(ctx context.Context, stayReq imsjson.Stay) (stayNumber int32) {
	a.t.Helper()
	resp := a.newStay(ctx, stayReq)
	require.Equal(a.t, http.StatusCreated, resp.StatusCode)
	numStr := resp.Header.Get("IMS-Stay-Number")
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
	bod, resp := a.imsGet(ctx, path, &imsjson.Incident{})
	return *bod.(*imsjson.Incident), resp
}

func (a ApiHelper) getStay(ctx context.Context, eventName string, stay int32) (imsjson.Stay, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/stays/", strconv.Itoa(int(stay))).String()
	bod, resp := a.imsGet(ctx, path, &imsjson.Stay{})
	return *bod.(*imsjson.Stay), resp
}

func (a ApiHelper) updateIncident(ctx context.Context, eventName string, incident int32, req imsjson.Incident) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents/", strconv.Itoa(int(incident))).String())
}

func (a ApiHelper) updateStay(ctx context.Context, eventName string, stay int32, req imsjson.Stay) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/stays/", strconv.Itoa(int(stay))).String())
}

func (a ApiHelper) attachRangerToIncident(ctx context.Context, eventName string, incident int32, handle string) *http.Response {
	a.t.Helper()
	req := imsjson.IncidentRanger{Handle: handle}
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents/", strconv.Itoa(int(incident)), "/rangers/", handle).String())
}

func (a ApiHelper) attachRangerToStay(ctx context.Context, eventName string, stay int32, handle string) *http.Response {
	a.t.Helper()
	req := imsjson.StayRanger{Handle: handle}
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/stays/", strconv.Itoa(int(stay)), "/rangers/", handle).String())
}

func (a ApiHelper) detachRangerFromIncident(ctx context.Context, eventName string, incident int32, handle string) *http.Response {
	a.t.Helper()
	_, resp := a.imsDelete(ctx, a.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents/", strconv.Itoa(int(incident)), "/rangers/", handle).String(), nil)
	return resp
}

func (a ApiHelper) detachRangerFromStay(ctx context.Context, eventName string, stay int32, handle string) *http.Response {
	a.t.Helper()
	_, resp := a.imsDelete(ctx, a.serverURL.JoinPath("/ims/api/events/", eventName, "/stays/", strconv.Itoa(int(stay)), "/rangers/", handle).String(), nil)
	return resp
}

func (a ApiHelper) getIncidents(ctx context.Context, eventName string) (imsjson.Incidents, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath(fmt.Sprint("/ims/api/events/", eventName, "/incidents")).String()
	bod, resp := a.imsGet(ctx, path, &imsjson.Incidents{})
	return *bod.(*imsjson.Incidents), resp
}

func (a ApiHelper) getStays(ctx context.Context, eventName string) (imsjson.Stays, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath(fmt.Sprint("/ims/api/events/", eventName, "/stays")).String()
	bod, resp := a.imsGet(ctx, path, &imsjson.Stays{})
	return *bod.(*imsjson.Stays), resp
}

func (a ApiHelper) updateIncidentReportEntry(ctx context.Context, eventName string, incident int32, req imsjson.ReportEntry) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents/", conv.FormatInt(incident), "/report_entries/", conv.FormatInt(req.ID)).String())
}

func (a ApiHelper) updateFieldReportReportEntry(ctx context.Context, eventName string, fieldReport int32, req imsjson.ReportEntry) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/field_reports/", conv.FormatInt(fieldReport), "/report_entries/", conv.FormatInt(req.ID)).String())
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

func (a ApiHelper) getEvents(ctx context.Context) (imsjson.Events, *http.Response) {
	a.t.Helper()
	bod, resp := a.imsGet(ctx, a.serverURL.JoinPath("/ims/api/events").String(), &imsjson.Events{})
	return *bod.(*imsjson.Events), resp
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

func (a ApiHelper) addStayWriter(ctx context.Context, eventName, handle string) *http.Response {
	a.t.Helper()
	return a.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			StayWriters: []imsjson.AccessRule{{
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
	bod, resp := a.imsGet(ctx, a.serverURL.JoinPath("/ims/api/access").String(), &imsjson.EventsAccess{})
	return *bod.(*imsjson.EventsAccess), resp
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
	resp, err := client.Do(httpPost)
	require.NoError(a.t, err)

	reID, _ := conv.ParseInt32(resp.Header.Get("IMS-Report-Entry-Number"))

	return reID, resp
}

func (a ApiHelper) attachFileToStay(ctx context.Context, eventName string, stay int32, fileBytes []byte) (int32, *http.Response) {
	a.t.Helper()

	path := a.serverURL.JoinPath("/ims/api/events", eventName, "stays", conv.FormatInt(stay), "attachments")

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

func (a ApiHelper) getStayAttachment(ctx context.Context, eventName string, stay, reID int32) ([]byte, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events", eventName, "stays", conv.FormatInt(stay), "attachments", conv.FormatInt(reID)).String()
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

func (a ApiHelper) imsPost(ctx context.Context, body any, path string) *http.Response {
	a.t.Helper()
	postBody, err := json.Marshal(body)
	require.NoError(a.t, err)
	httpPost, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(postBody))
	require.NoError(a.t, err)
	if a.jwt != "" {
		httpPost.Header.Set("Authorization", "Bearer "+a.jwt)
	}
	if a.referrer != "" {
		httpPost.Header.Set("Referer", a.referrer)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(httpPost)
	require.NoError(a.t, err)
	return resp
}

func (a ApiHelper) imsGetBodyBytes(ctx context.Context, path string) ([]byte, *http.Response) {
	a.t.Helper()
	outBytes, httpResp := a.imsGet(ctx, path, nil)
	return outBytes.([]byte), httpResp
}

func (a ApiHelper) imsDelete(ctx context.Context, path string, resp any) (any, *http.Response) {
	a.t.Helper()
	return a.imsDoNoReqBody(ctx, http.MethodDelete, path, resp)
}

func (a ApiHelper) imsGet(ctx context.Context, path string, resp any) (any, *http.Response) {
	a.t.Helper()
	return a.imsDoNoReqBody(ctx, http.MethodGet, path, resp)
}

func (a ApiHelper) imsDoNoReqBody(ctx context.Context, method, path string, resp any) (any, *http.Response) {
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
	get, err := client.Do(httpReq)
	require.NoError(a.t, err)
	b, err := io.ReadAll(get.Body)
	require.NoError(a.t, err)
	require.NoError(a.t, get.Body.Close())
	if resp == nil {
		return b, get
	}
	err = json.Unmarshal(b, &resp)
	if err != nil && get.StatusCode != http.StatusOK {
		return resp, get
	}
	require.NoError(a.t, err)
	return resp, get
}

func (a ApiHelper) getActionLogs(ctx context.Context, minTime, maxTime string) (imsjson.ActionLogs, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/actionlogs")
	q := path.Query()
	q.Set("minTimeUnixMs", minTime)
	q.Set("maxTimeUnixMs", maxTime)
	path.RawQuery = q.Encode()

	bod, resp := a.imsGet(ctx, path.String(), &imsjson.ActionLogs{})
	return *bod.(*imsjson.ActionLogs), resp
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
