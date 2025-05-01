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
	"github.com/burningmantech/ranger-ims-go/api"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestIncidentTypesAPIAuthorization(t *testing.T) {
	s := httptest.NewServer(api.AddToMux(t.Context(), nil, shared.cfg, shared.imsDB, nil))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)

	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForTestAdminRanger(t)}
	apisNonAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForRealTestUser(t)}
	apisNotAuthenticated := ApiHelper{t: t, serverURL: serverURL, jwt: ""}

	// Any authenticated user can call GetIncidentTypes
	_, resp := apisNotAuthenticated.getTypes(false)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_, resp = apisNonAdmin.getTypes(false)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, resp = apisAdmin.getTypes(false)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Only admins can hit the EditIncidentTypes endpoint
	// An unauthenticated client will get a 401
	// An unauthorized user will get a 403
	editTypesReq := imsjson.EditIncidentTypesRequest{}
	resp = apisNotAuthenticated.editTypes(editTypesReq)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp = apisNonAdmin.editTypes(editTypesReq)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp = apisAdmin.editTypes(editTypesReq)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestCreateIncident(t *testing.T) {
	s := httptest.NewServer(api.AddToMux(t.Context(), nil, shared.cfg, shared.imsDB, nil))
	defer s.Close()
	serverURL, err := url.Parse(s.URL)
	require.NoError(t, err)

	apis := ApiHelper{t: t, serverURL: serverURL, jwt: jwtForTestAdminRanger(t)}

	// Make three new incident types
	typeA, typeB, typeC := "Cat", "Dog", "Emu"
	createTypes := imsjson.EditIncidentTypesRequest{
		Add:  imsjson.IncidentTypes{typeA, typeB, typeC},
		Hide: nil,
		Show: nil,
	}
	apis.editTypes(createTypes)

	// All three types should now be retrievable and non-hidden
	typesResp, resp := apis.getTypes(false)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, typesResp, typeA)
	require.Contains(t, typesResp, typeB)
	require.Contains(t, typesResp, typeC)

	// Hide one of those types
	hideOne := imsjson.EditIncidentTypesRequest{
		Hide: imsjson.IncidentTypes{typeA},
	}
	apis.editTypes(hideOne)

	// That type should no longer appear from the standard incident type query
	typesVisibleOnly, resp := apis.getTypes(false)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotContains(t, typesVisibleOnly, typeA)
	require.Contains(t, typesVisibleOnly, typeB)
	require.Contains(t, typesVisibleOnly, typeC)
	// but it will still appears when includeHidden=true
	typesIncludeHidden, resp := apis.getTypes(true)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, typesIncludeHidden, typeA)
	require.Contains(t, typesIncludeHidden, typeB)
	require.Contains(t, typesIncludeHidden, typeC)

	// Unhide that type we previously hid
	showItAgain := imsjson.EditIncidentTypesRequest{
		Show: imsjson.IncidentTypes{typeA, typeB},
	}
	apis.editTypes(showItAgain)
	// and see that it's back in the standard incident type query results
	typesVisibleOnly, resp = apis.getTypes(false)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, typesVisibleOnly, typeA)
	require.Contains(t, typesVisibleOnly, typeB)
	require.Contains(t, typesVisibleOnly, typeC)
}
