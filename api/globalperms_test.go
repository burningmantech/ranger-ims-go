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

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/stretchr/testify/require"
)

// These tests cover the global permission gates on the read-only endpoints.
//
// They're unit tests rather than integration tests because the gates can't be
// reached over HTTP as things stand: authentication rejects a token with no
// Ranger handle, and every handle that does authenticate is granted all three
// of these permissions via AnyAuthenticatedUser. The gates are defense in
// depth, and they become live the moment RolesToGlobalPerms stops handing one
// of these out to everyone -- which is plausible for personnel data. Testing
// them here pins the intended behaviour for that day.

// emptyDirectorySource is a directory.Source with no users, positions or teams.
// The global permission lookup only needs the positions and teams, and only to
// map the caller's claimed IDs to names.
type emptyDirectorySource struct{}

func (emptyDirectorySource) FetchUsers(context.Context) (map[int64]*directory.User, error) {
	return map[int64]*directory.User{}, nil
}

func (emptyDirectorySource) FetchPositions(context.Context) (map[int64]string, error) {
	return map[int64]string{}, nil
}

func (emptyDirectorySource) FetchTeams(context.Context) (map[int64]string, error) {
	return map[int64]string{}, nil
}

// requestWithClaims builds a GET carrying the JWT context that RequireAuthN
// would have attached. A nil store.DBQ is fine here: computing global (non
// event) permissions never queries the database.
func requestWithClaims(t *testing.T, claims authz.IMSClaims) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	return req.WithContext(context.WithValue(req.Context(), JWTContextKey, JWTContext{
		Claims: &claims,
	}))
}

func testUserStore() *directory.UserStore {
	return directory.NewUserStore(emptyDirectorySource{}, time.Minute)
}

func TestGetEventsForbiddenWithoutGlobalListEvents(t *testing.T) {
	t.Parallel()

	// A caller with no handle holds no roles at all, so no global permissions.
	action := GetEvents{nil, testUserStore(), nil, time.Minute}

	_, errHTTP := action.getEvents(requestWithClaims(t, authz.IMSClaims{}))

	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusForbidden, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "GlobalListEvents")
}

func TestGetIncidentTypesForbiddenWithoutGlobalReadIncidentTypes(t *testing.T) {
	t.Parallel()

	action := GetIncidentTypes{nil, testUserStore(), nil, time.Minute}

	_, errHTTP := action.getIncidentTypes(requestWithClaims(t, authz.IMSClaims{}))

	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusForbidden, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "GlobalReadIncidentTypes")
}

func TestGetPersonnelForbiddenWithoutGlobalReadPersonnel(t *testing.T) {
	t.Parallel()

	action := GetPersonnel{nil, testUserStore(), nil, time.Minute}

	_, errHTTP := action.getPersonnel(requestWithClaims(t, authz.IMSClaims{}))

	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusForbidden, errHTTP.Code)
	require.Contains(t, errHTTP.ResponseMessage, "GlobalReadPersonnel")
}

// The gates above only fire for a caller with no roles. Any handle at all picks
// up AnyAuthenticatedUser, which carries all three permissions -- this is the
// invariant that makes those branches unreachable over HTTP today.
func TestAnyHandleCarriesTheGlobalReadPermissions(t *testing.T) {
	t.Parallel()

	_, globalPerms := authz.ManyEventPermissions(
		nil, nil, "SomeRanger", false, nil, nil, "",
	)

	require.NotZero(t, globalPerms&authz.GlobalListEvents)
	require.NotZero(t, globalPerms&authz.GlobalReadIncidentTypes)
	require.NotZero(t, globalPerms&authz.GlobalReadPersonnel)

	// ...and an empty handle carries none of them.
	_, noPerms := authz.ManyEventPermissions(nil, nil, "", false, nil, nil, "")
	require.Equal(t, authz.GlobalNoPermissions, noPerms)
}
