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
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/burningmantech/ranger-ims-go/api"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/argon2id"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	dirAdminHandle = "DirectoryAdmin"
	dirAdminEmail  = "directoryadmin@example.com"
	// #nosec G101 // test-only credential
	dirAdminPassword = "dir-admin-password"
)

// newIMSDirectoryServer starts a second IMS server, one that uses the
// IMS-native directory (backed by the shared IMS DB container) rather than
// the Clubhouse directory. It also bootstraps an admin user in the
// DIRECTORY_PERSON table, the way the add-user CLI command would.
func newIMSDirectoryServer(t *testing.T, ctx context.Context) *url.URL {
	t.Helper()

	cfg := *shared.cfg
	cfg.Directory.Directory = conf.DirectoryTypeIMS
	cfg.Core.Admins = []string{dirAdminHandle}

	hashed := argon2id.CreateHash(dirAdminPassword, argon2id.DevelopmentParams)
	_, err := shared.imsDBQ.DirectoryCreatePerson(ctx, shared.imsDBQ,
		imsdb.DirectoryCreatePersonParams{
			Handle:   dirAdminHandle,
			Email:    sql.NullString{String: dirAdminEmail, Valid: true},
			Password: hashed,
			Active:   true,
			Onsite:   true,
		})
	require.NoError(t, err)

	userStore := directory.NewUserStore(
		directory.NewIMSSource(shared.imsDBQ),
		cfg.Directory.InMemoryCacheTTL,
	)
	server := httptest.NewServer(
		api.AddToMux(nil, api.NewEventSourcerer(), &cfg, shared.imsDBQ, userStore, nil, shared.actionLogger),
	)
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	return serverURL
}

func TestIMSNativeDirectory(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	serverURL := newIMSDirectoryServer(t, ctx)

	// The bootstrapped admin can log in through the normal auth flow,
	// with users coming from the IMS-native directory.
	unauthed := ApiHelper{t: t, serverURL: serverURL, jwt: ""}
	statusCode, _, adminJWT := unauthed.postAuth(ctx, api.PostAuthRequest{
		Identification: dirAdminHandle,
		Password:       dirAdminPassword,
	})
	require.Equal(t, http.StatusOK, statusCode)
	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: adminJWT}

	// The admin can read the directory, which contains the admin itself.
	dir, resp := apisAdmin.getDirectory(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, dir.Persons, 1)
	require.Equal(t, dirAdminHandle, *dir.Persons[0].Handle)
	require.Equal(t, dirAdminEmail, *dir.Persons[0].Email)
	require.True(t, *dir.Persons[0].Active)
	require.True(t, *dir.Persons[0].Onsite)

	// Create a team and a position.
	teamID, resp := apisAdmin.editDirectoryGroup(ctx, "teams", imsjson.DirectoryGroup{
		Title: new("Night Shift"),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, teamID)
	positionID, resp := apisAdmin.editDirectoryGroup(ctx, "positions", imsjson.DirectoryGroup{
		Title: new("Khaki"),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, positionID)

	// Create a person on that team and position.
	frodoHandle := "Frodo"
	frodoEmail := "frodo@example.com"
	personID, resp := apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		Handle:      &frodoHandle,
		Email:       &frodoEmail,
		Onsite:      new(true),
		TeamIDs:     &[]int64{*teamID},
		PositionIDs: &[]int64{*positionID},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, personID)

	// The new person can't log in yet: they have no password.
	statusCode, _, _ = unauthed.postAuth(ctx, api.PostAuthRequest{
		Identification: frodoHandle,
		Password:       "literally anything",
	})
	require.Equal(t, http.StatusUnauthorized, statusCode)

	// Set the person's password, and then they can log in,
	// by handle or by email.
	frodoPassword := "frodo-password-" + rand.NonCryptoText()
	resp = apisAdmin.setDirectoryPersonPassword(ctx, *personID, frodoPassword)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	statusCode, _, frodoJWT := unauthed.postAuth(ctx, api.PostAuthRequest{
		Identification: frodoHandle,
		Password:       frodoPassword,
	})
	require.Equal(t, http.StatusOK, statusCode)
	statusCode, _, _ = unauthed.postAuth(ctx, api.PostAuthRequest{
		Identification: frodoEmail,
		Password:       frodoPassword,
	})
	require.Equal(t, http.StatusOK, statusCode)
	apisFrodo := ApiHelper{t: t, serverURL: serverURL, jwt: frodoJWT}

	// The new person shows up in the personnel API, with the fixed
	// "active" status that all IMS-native directory users have.
	personnel, resp := apisFrodo.getPersonnel(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Len(t, personnel, 2)
	handles := []string{personnel[0].Handle, personnel[1].Handle}
	require.ElementsMatch(t, []string{dirAdminHandle, frodoHandle}, handles)
	for _, p := range personnel {
		require.Equal(t, "active", p.Status)
	}

	// Team-based access rules work with IMS-native directory teams:
	// grant write access on a new event to the Night Shift team, and
	// Frodo (a member) gets it.
	eventName := "dir-event-" + rand.NonCryptoText()
	_, resp = apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Writers: []imsjson.AccessRule{{
				Expression: "team:Night Shift",
				Validity:   "always",
			}},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	frodoAuth, resp := apisFrodo.getAuth(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.True(t, frodoAuth.EventAccess[eventName].WriteIncidents)

	// A non-admin cannot read or edit the directory.
	_, resp = apisFrodo.getDirectory(ctx)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = apisFrodo.editDirectoryPerson(ctx, imsjson.DirectoryPerson{Handle: new("Mallory")})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Deactivating a person locks them out.
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		ID:     *personID,
		Active: new(false),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	statusCode, _, _ = unauthed.postAuth(ctx, api.PostAuthRequest{
		Identification: frodoHandle,
		Password:       frodoPassword,
	})
	require.Equal(t, http.StatusUnauthorized, statusCode)

	// Deleting a person removes them from the directory entirely.
	bilboHandle := "Bilbo"
	bilboID, resp := apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{Handle: &bilboHandle})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, bilboID)
	resp = apisAdmin.deleteDirectoryPerson(ctx, *bilboID)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	dir, resp = apisAdmin.getDirectory(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	for _, p := range dir.Persons {
		assert.NotEqual(t, bilboHandle, *p.Handle)
	}
}

func TestDirectoryAPIDisabledOnClubhouseDeployments(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// On the main test server (which uses the Clubhouse directory), even an
	// admin gets a 403 from the directory admin API.
	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}
	_, resp := apisAdmin.getDirectory(ctx)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{Handle: new("Sneaky")})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func (a ApiHelper) getDirectory(ctx context.Context) (imsjson.Directory, *http.Response) {
	a.t.Helper()
	bod, resp := a.imsGet(ctx, a.serverURL.JoinPath("/ims/api/directory").String(), &imsjson.Directory{})
	return *bod.(*imsjson.Directory), resp
}

func (a ApiHelper) getPersonnel(ctx context.Context) ([]imsjson.Person, *http.Response) {
	a.t.Helper()
	bod, resp := a.imsGet(ctx, a.serverURL.JoinPath("/ims/api/personnel").String(), &[]imsjson.Person{})
	return *bod.(*[]imsjson.Person), resp
}

func (a ApiHelper) editDirectoryPerson(ctx context.Context, req imsjson.DirectoryPerson) (*int64, *http.Response) {
	a.t.Helper()
	resp := a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/directory/persons").String())
	idStr := resp.Header.Get("IMS-Directory-Person-ID")
	if idStr == "" {
		return nil, resp
	}
	id, err := conv.ParseInt64(idStr)
	require.NoError(a.t, err)
	return &id, resp
}

// editDirectoryGroup creates or updates a team or position.
// groupKind must be "teams" or "positions".
func (a ApiHelper) editDirectoryGroup(ctx context.Context, groupKind string, req imsjson.DirectoryGroup) (*int64, *http.Response) {
	a.t.Helper()
	resp := a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/directory/", groupKind).String())
	header := "IMS-Directory-Team-ID"
	if groupKind == "positions" {
		header = "IMS-Directory-Position-ID"
	}
	idStr := resp.Header.Get(header)
	if idStr == "" {
		return nil, resp
	}
	id, err := conv.ParseInt64(idStr)
	require.NoError(a.t, err)
	return &id, resp
}

func (a ApiHelper) setDirectoryPersonPassword(ctx context.Context, personID int64, password string) *http.Response {
	a.t.Helper()
	req := imsjson.DirectoryPersonPassword{Password: password}
	return a.imsPost(ctx, req, a.serverURL.JoinPath("/ims/api/directory/persons/", conv.FormatInt(personID), "/password").String())
}

func (a ApiHelper) deleteDirectoryPerson(ctx context.Context, personID int64) *http.Response {
	a.t.Helper()
	_, resp := a.imsDelete(ctx, a.serverURL.JoinPath("/ims/api/directory/persons/", conv.FormatInt(personID)).String(), nil)
	return resp
}
