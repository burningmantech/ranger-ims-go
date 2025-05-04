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
	"github.com/burningmantech/ranger-ims-go/api"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"
)

type ApiHelper struct {
	t         *testing.T
	serverURL *url.URL
	jwt       string
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

func (a ApiHelper) editTypes(ctx context.Context, req imsjson.EditIncidentTypesRequest) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/incident_types").String())
}

func (a ApiHelper) getTypes(ctx context.Context, includeHidden bool) (imsjson.IncidentTypes, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/incident_types").String()
	if includeHidden {
		path += "?hidden=true"
	}
	bod, resp := a.imsGet(ctx, path, &imsjson.IncidentTypes{})
	return *bod.(*imsjson.IncidentTypes), resp
}

func (a ApiHelper) newFieldReport(ctx context.Context, req imsjson.FieldReport) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/"+req.Event+"/field_reports").String())
}

func (a ApiHelper) newFieldReportSuccess(ctx context.Context, fieldReportReq imsjson.FieldReport) (fieldReport int32) {
	a.t.Helper()
	httpResp := a.newFieldReport(ctx, fieldReportReq)
	require.Equal(a.t, http.StatusCreated, httpResp.StatusCode)
	numStr := httpResp.Header.Get("X-IMS-Field-Report-Number")
	require.NoError(a.t, httpResp.Body.Close())
	require.NotEmpty(a.t, numStr)
	num, err := strconv.ParseInt(numStr, 10, 32)
	require.NoError(a.t, err)
	require.Positive(a.t, num)
	return int32(num)
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
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/field_reports/", strconv.Itoa(int(fieldReport))).String())
}

func (a ApiHelper) newIncident(ctx context.Context, req imsjson.Incident) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/"+req.Event+"/incidents").String())
}

func (a ApiHelper) newIncidentSuccess(ctx context.Context, incidentReq imsjson.Incident) (incidentNumber int32) {
	a.t.Helper()
	resp := a.newIncident(ctx, incidentReq)
	require.Equal(a.t, http.StatusCreated, resp.StatusCode)
	numStr := resp.Header.Get("X-IMS-Incident-Number")
	require.NoError(a.t, resp.Body.Close())
	require.NotEmpty(a.t, numStr)
	num, err := strconv.ParseInt(numStr, 10, 32)
	require.NoError(a.t, err)
	require.Positive(a.t, num)
	return int32(num)
}

func (a ApiHelper) getIncident(ctx context.Context, eventName string, incident int32) (imsjson.Incident, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents/", strconv.Itoa(int(incident))).String()
	bod, resp := a.imsGet(ctx, path, &imsjson.Incident{})
	return *bod.(*imsjson.Incident), resp
}

func (a ApiHelper) updateIncident(ctx context.Context, eventName string, incident int32, req imsjson.Incident) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents/", strconv.Itoa(int(incident))).String())
}

func (a ApiHelper) getIncidents(ctx context.Context, eventName string) (imsjson.Incidents, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath(fmt.Sprint("/ims/api/events/", eventName, "/incidents")).String()
	bod, resp := a.imsGet(ctx, path, &imsjson.Incidents{})
	return *bod.(*imsjson.Incidents), resp
}

func (a ApiHelper) editEvent(ctx context.Context, req imsjson.EditEventsRequest) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/events").String())
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

func (a ApiHelper) editAccess(ctx context.Context, req imsjson.EventsAccess) *http.Response {
	a.t.Helper()
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/access").String())
}

func (a ApiHelper) getAccess(ctx context.Context) (imsjson.EventsAccess, *http.Response) {
	a.t.Helper()
	bod, resp := a.imsGet(ctx, a.serverURL.JoinPath("/ims/api/access").String(), &imsjson.EventsAccess{})
	return *bod.(*imsjson.EventsAccess), resp
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
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(httpPost)
	require.NoError(a.t, err)
	return resp
	// require.Equal(a.t, http.StatusNoContent, resp.StatusCode)
}

func (a ApiHelper) imsGet(ctx context.Context, path string, resp any) (any, *http.Response) {
	a.t.Helper()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	require.NoError(a.t, err)
	if a.jwt != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.jwt)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	get, err := client.Do(httpReq)
	require.NoError(a.t, err)
	b, err := io.ReadAll(get.Body)
	require.NoError(a.t, err)
	require.NoError(a.t, get.Body.Close())
	if get.StatusCode != http.StatusOK {
		return resp, get
	}
	err = json.Unmarshal(b, &resp)
	require.NoError(a.t, err)
	return resp, get
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
