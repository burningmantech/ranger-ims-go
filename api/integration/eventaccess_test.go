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
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventAccessTODO(t *testing.T) {
	t.Parallel()
}

func TestGetAccessTargets(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// An unauthenticated user gets a 401
	apisNotAuthenticated := ApiHelper{t: t, serverURL: shared.serverURL, jwt: ""}
	_, resp := apisNotAuthenticated.getAccessTargets(ctx)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// A non-admin user gets a 403
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}
	_, resp = apisAlice.getAccessTargets(ctx)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// An admin gets all the persons, positions, and teams from the directory
	// (these values come from clubhousedb_test_seed.sql)
	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	targets, resp := apisAdmin.getAccessTargets(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, []string{userAdminHandle, userAliceHandle}, targets.Persons)
	assert.Equal(t, []string{"Nooperator"}, targets.Positions)
	assert.Equal(t, []string{"Brown Dot"}, targets.Teams)
}
