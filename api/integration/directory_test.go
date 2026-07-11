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
	"strings"
	"sync"
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

// dirAdminOnce guards the DIRECTORY_PERSON admin bootstrap: the IMS DB (and
// its DIRECTORY_* tables) is shared by every test in this package, and the
// admin's handle is unique-constrained, so only the first caller may insert it.
var (
	dirAdminOnce sync.Once
	dirAdminErr  error //nolint:errname // This isn't a sentinel error
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

	dirAdminOnce.Do(func() {
		hashed := argon2id.CreateHash(dirAdminPassword, argon2id.DevelopmentParams)
		_, dirAdminErr = shared.imsDBQ.DirectoryCreatePerson(ctx, shared.imsDBQ,
			imsdb.DirectoryCreatePersonParams{
				Handle:   dirAdminHandle,
				Email:    sql.NullString{String: dirAdminEmail, Valid: true},
				Password: hashed,
				Active:   true,
				Onsite:   true,
			})
	})
	require.NoError(t, dirAdminErr)

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

// dirAdminJWT logs in as the bootstrapped IMS-native directory admin.
func dirAdminJWT(t *testing.T, ctx context.Context, serverURL *url.URL) string {
	t.Helper()
	unauthed := ApiHelper{t: t, serverURL: serverURL, jwt: ""}
	statusCode, _, jwt := unauthed.postAuth(ctx, api.PostAuthRequest{
		Identification: dirAdminHandle,
		Password:       dirAdminPassword,
	})
	require.Equal(t, http.StatusOK, statusCode)
	return jwt
}

// The DIRECTORY_* tables are shared by all tests in this package, so lookups
// must find rows by ID rather than assuming counts or indexes.

func findDirectoryPerson(dir imsjson.Directory, id int64) *imsjson.DirectoryPerson {
	for _, p := range dir.Persons {
		if p.ID == id {
			return &p
		}
	}
	return nil
}

func findDirectoryGroup(groups []imsjson.DirectoryGroup, id int64) *imsjson.DirectoryGroup {
	for _, g := range groups {
		if g.ID == id {
			return &g
		}
	}
	return nil
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
	// (Other parallel tests may have added more persons to the shared
	// DIRECTORY_* tables, so find the admin by handle.)
	dir, resp := apisAdmin.getDirectory(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	var admin *imsjson.DirectoryPerson
	for _, p := range dir.Persons {
		if *p.Handle == dirAdminHandle {
			admin = &p
			break
		}
	}
	require.NotNil(t, admin)
	require.Equal(t, dirAdminEmail, *admin.Email)
	require.True(t, *admin.Active)
	require.True(t, *admin.Onsite)

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
	handles := make([]string, 0, len(personnel))
	for _, p := range personnel {
		handles = append(handles, p.Handle)
		require.Equal(t, "active", p.Status)
	}
	require.Contains(t, handles, dirAdminHandle)
	require.Contains(t, handles, frodoHandle)

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

func TestDirectoryPersonValidation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	serverURL := newIMSDirectoryServer(t, ctx)
	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: dirAdminJWT(t, ctx, serverURL)}

	// An unauthenticated request is rejected outright.
	unauthed := ApiHelper{t: t, serverURL: serverURL, jwt: ""}
	_, resp := unauthed.getDirectory(ctx)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// A new person must have a handle.
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		Email: new("nohandle@example.com"),
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// A handle must not be empty.
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{Handle: new("")})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// A handle must not be too long.
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		Handle: new(strings.Repeat("h", 129)),
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// An email must not be too long.
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		Handle: new("LongEmail-" + rand.NonCryptoText()),
		Email:  new(strings.Repeat("e", 257)),
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Create a valid person to test uniqueness against.
	handle := "Unique-" + rand.NonCryptoText()
	email := handle + "@example.com"
	personID, resp := apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		Handle: &handle,
		Email:  &email,
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, personID)

	// Handles must be unique.
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{Handle: &handle})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Emails must be unique.
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		Handle: new("OtherHandle-" + rand.NonCryptoText()),
		Email:  &email,
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Updating a person that doesn't exist is a 404.
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		ID:     999999999,
		Handle: new("Ghost"),
	})
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Memberships must reference a team that exists.
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		ID:      *personID,
		TeamIDs: &[]int64{999999999},
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// ...and a position that exists.
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		ID:          *personID,
		PositionIDs: &[]int64{999999999},
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// A non-numeric person ID in the path is a 400, for delete and
	// for password-setting.
	_, resp = apisAdmin.imsDelete(ctx, serverURL.JoinPath("/ims/api/directory/persons/pippin").String(), nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.imsPost(ctx, imsjson.DirectoryPersonPassword{Password: "irrelevant"},
		serverURL.JoinPath("/ims/api/directory/persons/pippin/password").String())
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Deleting a person that doesn't exist succeeds (the delete is idempotent).
	resp = apisAdmin.deleteDirectoryPerson(ctx, 999999999)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestDirectoryPersonPartialUpdate(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	serverURL := newIMSDirectoryServer(t, ctx)
	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: dirAdminJWT(t, ctx, serverURL)}

	teamA, resp := apisAdmin.editDirectoryGroup(ctx, "teams", imsjson.DirectoryGroup{
		Title: new("TeamA-" + rand.NonCryptoText()),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	teamB, resp := apisAdmin.editDirectoryGroup(ctx, "teams", imsjson.DirectoryGroup{
		Title: new("TeamB-" + rand.NonCryptoText()),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	position, resp := apisAdmin.editDirectoryGroup(ctx, "positions", imsjson.DirectoryGroup{
		Title: new("Position-" + rand.NonCryptoText()),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	handle := "Partial-" + rand.NonCryptoText()
	email := handle + "@example.com"
	personID, resp := apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		Handle:      &handle,
		Email:       &email,
		Onsite:      new(true),
		TeamIDs:     &[]int64{*teamA},
		PositionIDs: &[]int64{*position},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, personID)

	// The person comes back from the directory with all fields set,
	// active by default.
	dir, resp := apisAdmin.getDirectory(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	person := findDirectoryPerson(dir, *personID)
	require.NotNil(t, person)
	require.Equal(t, handle, *person.Handle)
	require.Equal(t, email, *person.Email)
	require.True(t, *person.Active)
	require.True(t, *person.Onsite)
	require.Equal(t, []int64{*teamA}, *person.TeamIDs)
	require.Equal(t, []int64{*position}, *person.PositionIDs)

	// Updating only "active" leaves every other field untouched,
	// including memberships (nil membership lists mean "no change").
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		ID:     *personID,
		Active: new(false),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	dir, resp = apisAdmin.getDirectory(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	person = findDirectoryPerson(dir, *personID)
	require.NotNil(t, person)
	require.Equal(t, handle, *person.Handle)
	require.Equal(t, email, *person.Email)
	require.False(t, *person.Active)
	require.True(t, *person.Onsite)
	require.Equal(t, []int64{*teamA}, *person.TeamIDs)
	require.Equal(t, []int64{*position}, *person.PositionIDs)

	// Handle and email can be changed, again without touching memberships.
	handle2 := "Partial2-" + rand.NonCryptoText()
	email2 := handle2 + "@example.com"
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		ID:     *personID,
		Handle: &handle2,
		Email:  &email2,
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	dir, resp = apisAdmin.getDirectory(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	person = findDirectoryPerson(dir, *personID)
	require.NotNil(t, person)
	require.Equal(t, handle2, *person.Handle)
	require.Equal(t, email2, *person.Email)
	require.Equal(t, []int64{*teamA}, *person.TeamIDs)

	// A non-nil team list replaces the memberships wholesale.
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		ID:      *personID,
		TeamIDs: &[]int64{*teamB},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	dir, resp = apisAdmin.getDirectory(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	person = findDirectoryPerson(dir, *personID)
	require.NotNil(t, person)
	require.Equal(t, []int64{*teamB}, *person.TeamIDs)
	require.Equal(t, []int64{*position}, *person.PositionIDs)

	// Empty (but non-nil) lists clear the memberships.
	_, resp = apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		ID:          *personID,
		TeamIDs:     &[]int64{},
		PositionIDs: &[]int64{},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	dir, resp = apisAdmin.getDirectory(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	person = findDirectoryPerson(dir, *personID)
	require.NotNil(t, person)
	require.Empty(t, *person.TeamIDs)
	require.Empty(t, *person.PositionIDs)
}

func TestDirectoryPersonPassword(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	serverURL := newIMSDirectoryServer(t, ctx)
	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: dirAdminJWT(t, ctx, serverURL)}
	unauthed := ApiHelper{t: t, serverURL: serverURL, jwt: ""}

	handle := "PwPerson-" + rand.NonCryptoText()
	personID, resp := apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{Handle: &handle})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, personID)

	// A password must not be empty.
	resp = apisAdmin.setDirectoryPersonPassword(ctx, *personID, "")
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// A password must not be too long.
	resp = apisAdmin.setDirectoryPersonPassword(ctx, *personID, strings.Repeat("p", 257))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Setting a password for a nonexistent person is a 404.
	resp = apisAdmin.setDirectoryPersonPassword(ctx, 999999999, "a fine password")
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Set a first password and log in with it.
	password1 := "pw1-" + rand.NonCryptoText()
	resp = apisAdmin.setDirectoryPersonPassword(ctx, *personID, password1)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	statusCode, _, _ := unauthed.postAuth(ctx, api.PostAuthRequest{
		Identification: handle,
		Password:       password1,
	})
	require.Equal(t, http.StatusOK, statusCode)

	// Change the password: the old one stops working and the new one works.
	password2 := "pw2-" + rand.NonCryptoText()
	resp = apisAdmin.setDirectoryPersonPassword(ctx, *personID, password2)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	statusCode, _, _ = unauthed.postAuth(ctx, api.PostAuthRequest{
		Identification: handle,
		Password:       password1,
	})
	require.Equal(t, http.StatusUnauthorized, statusCode)
	statusCode, _, _ = unauthed.postAuth(ctx, api.PostAuthRequest{
		Identification: handle,
		Password:       password2,
	})
	require.Equal(t, http.StatusOK, statusCode)
}

func TestDirectoryGroupValidationAndUpdate(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	serverURL := newIMSDirectoryServer(t, ctx)
	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: dirAdminJWT(t, ctx, serverURL)}

	// A new team must have a title.
	_, resp := apisAdmin.editDirectoryGroup(ctx, "teams", imsjson.DirectoryGroup{})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// A title must not be empty.
	_, resp = apisAdmin.editDirectoryGroup(ctx, "teams", imsjson.DirectoryGroup{Title: new("")})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// A title must not be too long.
	_, resp = apisAdmin.editDirectoryGroup(ctx, "teams", imsjson.DirectoryGroup{
		Title: new(strings.Repeat("t", 129)),
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Team titles must be unique.
	teamTitle := "Team-" + rand.NonCryptoText()
	teamID, resp := apisAdmin.editDirectoryGroup(ctx, "teams", imsjson.DirectoryGroup{Title: &teamTitle})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, teamID)
	_, resp = apisAdmin.editDirectoryGroup(ctx, "teams", imsjson.DirectoryGroup{Title: &teamTitle})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Updating a nonexistent team or position is a 404.
	_, resp = apisAdmin.editDirectoryGroup(ctx, "teams", imsjson.DirectoryGroup{
		ID:    999999999,
		Title: new("Ghost Team"),
	})
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = apisAdmin.editDirectoryGroup(ctx, "positions", imsjson.DirectoryGroup{
		ID:    999999999,
		Title: new("Ghost Position"),
	})
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Updating only the title leaves "active" untouched...
	teamTitle2 := "Team2-" + rand.NonCryptoText()
	_, resp = apisAdmin.editDirectoryGroup(ctx, "teams", imsjson.DirectoryGroup{
		ID:    *teamID,
		Title: &teamTitle2,
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	dir, resp := apisAdmin.getDirectory(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	team := findDirectoryGroup(dir.Teams, *teamID)
	require.NotNil(t, team)
	require.Equal(t, teamTitle2, *team.Title)
	require.True(t, *team.Active)

	// ...and updating only "active" leaves the title untouched.
	_, resp = apisAdmin.editDirectoryGroup(ctx, "teams", imsjson.DirectoryGroup{
		ID:     *teamID,
		Active: new(false),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	dir, resp = apisAdmin.getDirectory(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	team = findDirectoryGroup(dir.Teams, *teamID)
	require.NotNil(t, team)
	require.Equal(t, teamTitle2, *team.Title)
	require.False(t, *team.Active)

	// Positions update the same way.
	positionTitle := "Position-" + rand.NonCryptoText()
	positionID, resp := apisAdmin.editDirectoryGroup(ctx, "positions", imsjson.DirectoryGroup{Title: &positionTitle})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, positionID)
	positionTitle2 := "Position2-" + rand.NonCryptoText()
	_, resp = apisAdmin.editDirectoryGroup(ctx, "positions", imsjson.DirectoryGroup{
		ID:    *positionID,
		Title: &positionTitle2,
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	dir, resp = apisAdmin.getDirectory(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	position := findDirectoryGroup(dir.Positions, *positionID)
	require.NotNil(t, position)
	require.Equal(t, positionTitle2, *position.Title)
	require.True(t, *position.Active)
}

func TestDirectoryGroupDelete(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	serverURL := newIMSDirectoryServer(t, ctx)
	apisAdmin := ApiHelper{t: t, serverURL: serverURL, jwt: dirAdminJWT(t, ctx, serverURL)}

	teamID, resp := apisAdmin.editDirectoryGroup(ctx, "teams", imsjson.DirectoryGroup{
		Title: new("DoomedTeam-" + rand.NonCryptoText()),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, teamID)
	positionID, resp := apisAdmin.editDirectoryGroup(ctx, "positions", imsjson.DirectoryGroup{
		Title: new("DoomedPosition-" + rand.NonCryptoText()),
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, positionID)

	// A person belongs to the doomed team and position.
	handle := "Member-" + rand.NonCryptoText()
	personID, resp := apisAdmin.editDirectoryPerson(ctx, imsjson.DirectoryPerson{
		Handle:      &handle,
		TeamIDs:     &[]int64{*teamID},
		PositionIDs: &[]int64{*positionID},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, personID)

	// Delete the team and the position.
	resp = apisAdmin.deleteDirectoryTeam(ctx, *teamID)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.deleteDirectoryPosition(ctx, *positionID)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// They're gone from the directory, and the person's memberships in
	// them were removed too.
	dir, resp := apisAdmin.getDirectory(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Nil(t, findDirectoryGroup(dir.Teams, *teamID))
	require.Nil(t, findDirectoryGroup(dir.Positions, *positionID))
	person := findDirectoryPerson(dir, *personID)
	require.NotNil(t, person)
	require.Empty(t, *person.TeamIDs)
	require.Empty(t, *person.PositionIDs)

	// A non-numeric team or position ID in the path is a 400.
	_, resp = apisAdmin.imsDelete(ctx, serverURL.JoinPath("/ims/api/directory/teams/legolas").String(), nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	_, resp = apisAdmin.imsDelete(ctx, serverURL.JoinPath("/ims/api/directory/positions/gimli").String(), nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Deleting a team or position that doesn't exist succeeds
	// (the delete is idempotent).
	resp = apisAdmin.deleteDirectoryTeam(ctx, 999999999)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.deleteDirectoryPosition(ctx, 999999999)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
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

func (a ApiHelper) deleteDirectoryTeam(ctx context.Context, teamID int64) *http.Response {
	a.t.Helper()
	_, resp := a.imsDelete(ctx, a.serverURL.JoinPath("/ims/api/directory/teams/", conv.FormatInt(teamID)).String(), nil)
	return resp
}

func (a ApiHelper) deleteDirectoryPosition(ctx context.Context, positionID int64) *http.Response {
	a.t.Helper()
	_, resp := a.imsDelete(ctx, a.serverURL.JoinPath("/ims/api/directory/positions/", conv.FormatInt(positionID)).String(), nil)
	return resp
}
