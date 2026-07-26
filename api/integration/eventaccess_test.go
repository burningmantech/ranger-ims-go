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

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventAccessTODO(t *testing.T) {
	t.Parallel()
}

// TestEventAccessDescription checks that a grant's description round-trips
// through the API: every rule posted with a description reads back with it.
func TestEventAccessDescription(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	testEventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &testEventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Two readers share one grant's description; a writer has a different one.
	accessReq := imsjson.EventsAccess{
		testEventName: {
			Readers: []imsjson.AccessRule{
				{Expression: "position:Leads-A", Validity: "always", Description: "Sanctuary leads"},
				{Expression: "position:Leads-B", Validity: "always", Description: "Sanctuary leads"},
			},
			Writers: []imsjson.AccessRule{
				{Expression: "team:Comms", Validity: "always", Description: "Comms team"},
			},
		},
	}
	resp = apisAdmin.editAccess(ctx, accessReq)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	accessResult, httpResp := apisAdmin.getAccess(ctx)
	require.Equal(t, http.StatusOK, httpResp.StatusCode)
	require.NoError(t, httpResp.Body.Close())

	got := accessResult[testEventName]
	readerDescriptions := make(map[string]string)
	for _, r := range got.Readers {
		readerDescriptions[r.Expression] = r.Description
	}
	assert.Equal(t, "Sanctuary leads", readerDescriptions["position:Leads-A"])
	assert.Equal(t, "Sanctuary leads", readerDescriptions["position:Leads-B"])

	require.Len(t, got.Writers, 1)
	assert.Equal(t, "Comms team", got.Writers[0].Description)
}

// TestEventAccessOneExpressionManyModes checks that one expression can hold
// several access modes on the same event at once, and that the holder gets the
// union of those modes' permissions.
func TestEventAccessOneExpressionManyModes(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	apisAlice := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAlice(t, ctx)}

	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	aliceExpr := "person:" + userAliceHandle

	// Make Alice a reader.
	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Readers: []imsjson.AccessRule{{Expression: aliceExpr, Validity: "always"}},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// A reader can read incidents and visits, but can't write visits.
	auth, resp := apisAlice.getAuth(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.True(t, auth.EventAccess[eventName].ReadIncidents)
	assert.True(t, auth.EventAccess[eventName].ReadVisits)
	assert.False(t, auth.EventAccess[eventName].WriteVisits)

	// Now also make her a visit writer, posting only that mode. This is the
	// behavior under test: the reader rule for the same expression survives.
	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			VisitWriters: []imsjson.AccessRule{{Expression: aliceExpr, Validity: "always"}},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Both rules are stored, one per mode.
	accessResult, resp := apisAdmin.getAccess(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	got := accessResult[eventName]
	require.Len(t, got.Readers, 1)
	assert.Equal(t, aliceExpr, got.Readers[0].Expression)
	require.Len(t, got.VisitWriters, 1)
	assert.Equal(t, aliceExpr, got.VisitWriters[0].Expression)
	assert.Empty(t, got.Writers)
	assert.Empty(t, got.Reporters)

	// Alice now holds the union: reader's incident access plus the visit
	// writer's write access. No single role grants that combination.
	auth, resp = apisAlice.getAuth(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.True(t, auth.EventAccess[eventName].ReadIncidents)
	assert.True(t, auth.EventAccess[eventName].ReadVisits)
	assert.True(t, auth.EventAccess[eventName].WriteVisits)
	assert.False(t, auth.EventAccess[eventName].WriteIncidents)

	// Clearing one mode with an empty (non-nil) list leaves the other alone.
	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Readers: []imsjson.AccessRule{},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	accessResult, resp = apisAdmin.getAccess(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	got = accessResult[eventName]
	assert.Empty(t, got.Readers)
	require.Len(t, got.VisitWriters, 1)

	auth, resp = apisAlice.getAuth(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.False(t, auth.EventAccess[eventName].ReadIncidents)
	assert.True(t, auth.EventAccess[eventName].WriteVisits)
}

// TestEventAccessModesAreIndependent checks that rewriting one mode's rules
// doesn't disturb the other three, even when they name the same expression.
func TestEventAccessModesAreIndependent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// One expression in all four modes.
	expr := "position:Nooperator"
	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Readers:      []imsjson.AccessRule{{Expression: expr, Validity: "always"}},
			Writers:      []imsjson.AccessRule{{Expression: expr, Validity: "always"}},
			Reporters:    []imsjson.AccessRule{{Expression: expr, Validity: "always"}},
			VisitWriters: []imsjson.AccessRule{{Expression: expr, Validity: "always"}},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	accessResult, resp := apisAdmin.getAccess(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	got := accessResult[eventName]
	require.Len(t, got.Readers, 1)
	require.Len(t, got.Writers, 1)
	require.Len(t, got.Reporters, 1)
	require.Len(t, got.VisitWriters, 1)

	// Rewrite just the writers, to a different expression. The other three
	// modes keep their rule for the original expression.
	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Writers: []imsjson.AccessRule{{Expression: "team:Brown Dot", Validity: "always"}},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	accessResult, resp = apisAdmin.getAccess(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	got = accessResult[eventName]
	require.Len(t, got.Writers, 1)
	assert.Equal(t, "team:Brown Dot", got.Writers[0].Expression)
	require.Len(t, got.Readers, 1)
	assert.Equal(t, expr, got.Readers[0].Expression)
	require.Len(t, got.Reporters, 1)
	assert.Equal(t, expr, got.Reporters[0].Expression)
	require.Len(t, got.VisitWriters, 1)
	assert.Equal(t, expr, got.VisitWriters[0].Expression)

	// A mode the caller doesn't mention (a nil list) is left alone entirely.
	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Reporters: []imsjson.AccessRule{},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	accessResult, resp = apisAdmin.getAccess(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	got = accessResult[eventName]
	assert.Empty(t, got.Reporters)
	require.Len(t, got.Readers, 1)
	require.Len(t, got.Writers, 1)
	require.Len(t, got.VisitWriters, 1)
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
