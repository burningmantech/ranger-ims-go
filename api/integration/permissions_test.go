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
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"slices"
	"testing"
	"time"
)

type MethodURL struct {
	Method string
	Path   string
}

func TestAdminOnlyEndpoints(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	apisNotAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}

	adminOnly := []MethodURL{
		{http.MethodGet, "/ims/api/access"},
		{http.MethodPost, "/ims/api/access"},
		{http.MethodPost, "/ims/api/events"},
		{http.MethodPost, "/ims/api/streets"},
		{http.MethodPost, "/ims/api/incident_types"},
	}

	for _, api := range adminOnly {
		// admin is allowed in
		code := apiCall(t, api, apisAdmin)
		require.True(t, permitted(code), "%v %v wanted non-401/403 status code, got %v", api.Method, api.Path, code)

		// nonadmin is forbidden
		code = apiCall(t, api, apisNonAdmin)
		require.True(t, forbidden(code), "%v %v wanted 403 status code, got %v", api.Method, api.Path, code)

		// unauthenticated is unauthorized
		code = apiCall(t, api, apisNotAuthenticated)
		require.True(t, unauthorized(code), "%v %v wanted 401 status code, got %v", api.Method, api.Path, code)
	}
}

func TestAnyUnauthenticatedUserEndpoints(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	apisNotAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}

	anyAuthenticatedUserEndpoints := []MethodURL{
		{http.MethodGet, "/ims/api/streets"},
		{http.MethodGet, "/ims/api/personnel"},
		{http.MethodGet, "/ims/api/incident_types"},
		{http.MethodGet, "/ims/api/events"},
	}

	for _, api := range anyAuthenticatedUserEndpoints {
		// admin is allowed in
		code := apiCall(t, api, apisAdmin)
		require.True(t, permitted(code), "%v %v wanted non-401/403 status code, got %v", api.Method, api.Path, code)

		// nonadmin is allowed in
		code = apiCall(t, api, apisNonAdmin)
		require.True(t, permitted(code), "%v %v wanted non-401/403 status code, got %v", api.Method, api.Path, code)

		// unauthenticated is unauthorized
		code = apiCall(t, api, apisNotAuthenticated)
		require.True(t, unauthorized(code), "%v %v wanted 401 status code, got %v", api.Method, api.Path, code)
	}
}

func TestEventEndpoints_ForNoEventPerms(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisNotAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}

	eventName := uuid.NewString()
	resp := apisAdmin.editEvent(ctx, imsjson.EditEventsRequest{Add: []string{eventName}})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	eventPath := "/ims/api/events/" + eventName
	getIncidents := MethodURL{http.MethodGet, eventPath + "/incidents"}
	getIncident := MethodURL{http.MethodGet, eventPath + "/incidents/1"}
	getIncidentAttachment := MethodURL{http.MethodGet, eventPath + "/incidents/1/attachments/1"}
	postIncident := MethodURL{http.MethodPost, eventPath + "/incidents/1"}
	postIncidentAttachment := MethodURL{http.MethodPost, eventPath + "/incidents/1/attachments"}
	postIncidentRE := MethodURL{http.MethodPost, eventPath + "/incidents/1/report_entries/2"}
	getFieldReports := MethodURL{http.MethodGet, eventPath + "/field_reports"}
	getFieldReport := MethodURL{http.MethodGet, eventPath + "/field_reports/1"}
	getFieldReportAttachment := MethodURL{http.MethodGet, eventPath + "/field_reports/1/attachments/1"}
	postFieldReport := MethodURL{http.MethodPost, eventPath + "/field_reports/1"}
	postFieldReportAttachment := MethodURL{http.MethodPost, eventPath + "/field_reports/1/attachments"}
	postFieldReportRE := MethodURL{http.MethodPost, eventPath + "/field_reports/1/report_entries/2"}

	allPerms := []MethodURL{
		getIncidents,
		getIncident,
		getIncidentAttachment,
		postIncident,
		postIncidentAttachment,
		postIncidentRE,
		getFieldReports,
		getFieldReport,
		getFieldReportAttachment,
		postFieldReport,
		postFieldReportAttachment,
		postFieldReportRE,
	}
	reporter := []MethodURL{
		getFieldReports,
		getFieldReport,
		getFieldReportAttachment,
		postFieldReport,
		postFieldReportAttachment,
		postFieldReportRE,
	}
	reader := []MethodURL{
		getIncidents,
		getIncident,
		getIncidentAttachment,
		getFieldReports,
		getFieldReport,
		getFieldReportAttachment,
	}
	writer := slices.Clone(allPerms)

	for _, api := range allPerms {
		// unauthenticated is unauthorized
		code := apiCall(t, api, apisNotAuthenticated)
		require.True(t, unauthorized(code), "%v %v wanted 401 status code, got %v", api.Method, api.Path, code)
	}

	// to begin, the user has no permission
	for _, api := range allPerms {
		// forbidden
		code := apiCall(t, api, apisAdmin)
		require.True(t, forbidden(code), "%v %v wanted 403 status code, got %v", api.Method, api.Path, code)
	}

	// make the user a reporter
	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Reporters: []imsjson.AccessRule{{
				Expression: "person:" + userAdminHandle,
				Validity:   "always",
			}},
		}},
	)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// now the user can hit some more endpoints
	for _, api := range allPerms {
		switch {
		case api == postFieldReport:
			// the user won't be able to write to an FR for which they're not an author,
			// e.g. the one in this dummy call, so we should expect a 403, but we
			// can confirm they got the right error message
			code := apiCall(t, api, apisAdmin)
			require.True(t, forbidden(code), "%v %v wanted 403 status code, got %v", api.Method, api.Path, code)
		case slices.Contains(reporter, api):
			// permitted
			code := apiCall(t, api, apisAdmin)
			require.True(t, permitted(code), "%v %v wanted non-401/403 status code, got %v", api.Method, api.Path, code)
		default:
			// forbidden
			code := apiCall(t, api, apisAdmin)
			require.True(t, forbidden(code), "%v %v wanted 403 status code, got %v", api.Method, api.Path, code)
		}
	}

	// make the user a reader
	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Readers: []imsjson.AccessRule{{
				Expression: "person:" + userAdminHandle,
				Validity:   "always",
			}},
		}},
	)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// now the user can hit some other endpoints
	for _, api := range allPerms {
		if slices.Contains(reader, api) {
			// permitted
			code := apiCall(t, api, apisAdmin)
			require.True(t, permitted(code), "%v %v wanted non-401/403 status code, got %v", api.Method, api.Path, code)
		} else {
			// forbidden
			code := apiCall(t, api, apisAdmin)
			require.True(t, forbidden(code), "%v %v wanted 403 status code, got %v", api.Method, api.Path, code)
		}
	}

	// finally, make the user a writer
	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Writers: []imsjson.AccessRule{{
				Expression: "person:" + userAdminHandle,
				Validity:   "always",
			}},
		}},
	)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// now the user can hit many more endpoints
	for _, api := range allPerms {
		if slices.Contains(writer, api) {
			// permitted
			code := apiCall(t, api, apisAdmin)
			require.True(t, permitted(code), "%v %v wanted non-401/403 status code, got %v", api.Method, api.Path, code)
		} else {
			// forbidden
			code := apiCall(t, api, apisAdmin)
			require.True(t, forbidden(code), "%v %v wanted 403 status code, got %v", api.Method, api.Path, code)
		}
	}
}

func TestPublicAPIs_RequireNoAuthn(t *testing.T) {
	t.Parallel()
	public := []MethodURL{
		{http.MethodGet, "/ims/api/ping"},
		{http.MethodGet, "/ims/api/debug/buildinfo"},
	}
	apisNotAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}
	for _, api := range public {
		code := apiCall(t, api, apisNotAuthenticated)
		require.Equalf(t, http.StatusOK, code, "Got status code %v for %v %v", code, api.Method, api.Path)
	}
}

func TestEventSource_RequiresNoAuthn(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	path := shared.serverURL.JoinPath("ims/api/eventsource")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path.String(), nil)
	require.NoError(t, err)
	client := http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// The response body will keep streaming until the test ends, so we can just read
	// a prefix of the expect response to know that things look good.
	expectedFirstBytes := []byte("id: 0\nevent: InitialEvent")
	buf := make([]byte, len(expectedFirstBytes))
	_, err = io.ReadFull(resp.Body, buf)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
}

func apiCall(t *testing.T, api MethodURL, user ApiHelper) (statusCode int) {
	t.Helper()
	ctx := t.Context()
	var httpResp *http.Response
	switch api.Method {
	case http.MethodGet:
		_, httpResp = user.imsGetBodyBytes(ctx, user.serverURL.JoinPath(api.Path).String())
	case http.MethodPost:
		httpResp = user.imsPost(ctx, map[string]any{}, user.serverURL.JoinPath(api.Path).String())
	}
	require.NotNil(t, httpResp)
	require.NoError(t, httpResp.Body.Close())
	return httpResp.StatusCode
}

func permitted(status int) bool {
	return !unauthorized(status) && !forbidden(status)
}

func unauthorized(status int) bool {
	return status == http.StatusUnauthorized
}

func forbidden(status int) bool {
	return status == http.StatusForbidden
}
