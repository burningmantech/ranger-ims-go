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

package integration

import (
	"bytes"
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

func (a ApiHelper) postAuth(req api.PostAuthRequest) (statusCode int, body, validJWT string) {
	response := &api.PostAuthResponse{}
	resp := a.imsPost(req, a.serverURL.JoinPath("/ims/api/auth").String())
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

func (a ApiHelper) refreshAccessToken(refreshCookie *http.Cookie) (statusCode int, result *api.RefreshAccessTokenResponse) {
	response := &api.RefreshAccessTokenResponse{}
	postBody, err := json.Marshal(struct{}{})
	require.NoError(a.t, err)
	httpPost, err := http.NewRequest("POST", a.serverURL.JoinPath("/ims/api/auth/refresh").String(), bytes.NewReader(postBody))
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

func (a ApiHelper) getAuth(eventName string) (api.GetAuthResponse, *http.Response) {
	path := a.serverURL.JoinPath("/ims/api/auth").String()
	if eventName != "" {
		path = path + "?event_id=" + eventName
	}
	bod, resp := a.imsGet(path, &api.GetAuthResponse{})
	return *bod.(*api.GetAuthResponse), resp
}

func (a ApiHelper) editTypes(req imsjson.EditIncidentTypesRequest) *http.Response {
	return a.imsPost(req, a.serverURL.JoinPath("/ims/api/incident_types").String())
}

func (a ApiHelper) getTypes(includeHidden bool) (imsjson.IncidentTypes, *http.Response) {
	path := a.serverURL.JoinPath("/ims/api/incident_types").String()
	if includeHidden {
		path = path + "?hidden=true"
	}
	bod, resp := a.imsGet(path, &imsjson.IncidentTypes{})
	return *bod.(*imsjson.IncidentTypes), resp
}

func (a ApiHelper) newIncident(req imsjson.Incident) *http.Response {
	return a.imsPost(req, a.serverURL.JoinPath("/ims/api/events/"+req.Event+"/incidents").String())
}

func (a ApiHelper) newIncidentSuccess(incidentReq imsjson.Incident) (incidentNumber int32) {
	resp := a.newIncident(incidentReq)
	require.Equal(a.t, http.StatusCreated, resp.StatusCode)
	numStr := resp.Header.Get("X-IMS-Incident-Number")
	require.NotEmpty(a.t, numStr)
	num, err := strconv.ParseInt(numStr, 10, 32)
	require.NoError(a.t, err)
	require.Greater(a.t, num, int64(0))
	return int32(num)
}

func (a ApiHelper) getIncident(eventName string, incident int32) (imsjson.Incident, *http.Response) {
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents/", fmt.Sprint(incident)).String()
	bod, resp := a.imsGet(path, &imsjson.Incident{})
	return *bod.(*imsjson.Incident), resp
}

func (a ApiHelper) updateIncident(eventName string, incident int32, req imsjson.Incident) *http.Response {
	return a.imsPost(req, a.serverURL.JoinPath("/ims/api/events/", eventName, "/incidents/", fmt.Sprint(incident)).String())
}

func (a ApiHelper) getIncidents(eventName string) (imsjson.Incidents, *http.Response) {
	path := a.serverURL.JoinPath(fmt.Sprint("/ims/api/events/", eventName, "/incidents")).String()
	bod, resp := a.imsGet(path, &imsjson.Incidents{})
	return *bod.(*imsjson.Incidents), resp
}

func (a ApiHelper) editEvent(req imsjson.EditEventsRequest) *http.Response {
	return a.imsPost(req, a.serverURL.JoinPath("/ims/api/events").String())
}

func (a ApiHelper) getEvents() (imsjson.Events, *http.Response) {
	bod, resp := a.imsGet(a.serverURL.JoinPath("/ims/api/events").String(), &imsjson.Events{})
	return *bod.(*imsjson.Events), resp
}

func (a ApiHelper) addWriter(eventName, handle string) *http.Response {
	return a.editAccess(imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Writers: []imsjson.AccessRule{{
				Expression: "person:" + handle,
				Validity:   "always",
			}},
		},
	})
}

func (a ApiHelper) editAccess(req imsjson.EventsAccess) *http.Response {
	return a.imsPost(req, a.serverURL.JoinPath("/ims/api/access").String())
}

func (a ApiHelper) getAccess() (imsjson.EventsAccess, *http.Response) {
	bod, resp := a.imsGet(a.serverURL.JoinPath("/ims/api/access").String(), &imsjson.EventsAccess{})
	return *bod.(*imsjson.EventsAccess), resp
}

func (a ApiHelper) imsPost(body any, path string) *http.Response {
	postBody, err := json.Marshal(body)
	require.NoError(a.t, err)
	httpPost, err := http.NewRequest("POST", path, bytes.NewReader(postBody))
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

func (a ApiHelper) imsGet(path string, resp any) (any, *http.Response) {
	httpReq, err := http.NewRequest("GET", path, nil)
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

func jwtForRealTestUser(t *testing.T) string {
	apisNotAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}
	statusCode, _, token := apisNotAuthenticated.postAuth(api.PostAuthRequest{
		Identification: userAliceEmail,
		Password:       userAlicePassword,
	})
	require.Equal(t, http.StatusOK, statusCode)
	return token
}

func jwtForTestAdminRanger(t *testing.T) string {
	apisNotAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}
	statusCode, _, token := apisNotAuthenticated.postAuth(api.PostAuthRequest{
		Identification: userAdminEmail,
		Password:       userAdminPassword,
	})
	require.Equal(t, http.StatusOK, statusCode)
	return token
}
