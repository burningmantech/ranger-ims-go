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

package cmd

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDirectoryQuerier implements just the queries add-user makes, recording
// what they were called with. Any other query panics on the nil embedded Querier.
type fakeDirectoryQuerier struct {
	imsdb.Querier

	existing    *imsdb.DirectoryPersonByHandleRow
	lookupErr   error
	createErr   error
	updateErr   error
	passwordErr error

	created     *imsdb.DirectoryCreatePersonParams
	updated     *imsdb.DirectoryUpdatePersonParams
	passwordSet *imsdb.DirectorySetPersonPasswordParams
}

func (f *fakeDirectoryQuerier) DirectoryPersonByHandle(
	_ context.Context, _ imsdb.DBTX, handle string,
) (imsdb.DirectoryPersonByHandleRow, error) {
	if f.lookupErr != nil {
		return imsdb.DirectoryPersonByHandleRow{}, f.lookupErr
	}
	if f.existing == nil || f.existing.Handle != handle {
		return imsdb.DirectoryPersonByHandleRow{}, sql.ErrNoRows
	}
	return *f.existing, nil
}

func (f *fakeDirectoryQuerier) DirectoryCreatePerson(
	_ context.Context, _ imsdb.DBTX, arg imsdb.DirectoryCreatePersonParams,
) (int64, error) {
	f.created = &arg
	return 1, f.createErr
}

func (f *fakeDirectoryQuerier) DirectoryUpdatePerson(
	_ context.Context, _ imsdb.DBTX, arg imsdb.DirectoryUpdatePersonParams,
) error {
	f.updated = &arg
	return f.updateErr
}

func (f *fakeDirectoryQuerier) DirectorySetPersonPassword(
	_ context.Context, _ imsdb.DBTX, arg imsdb.DirectorySetPersonPasswordParams,
) error {
	f.passwordSet = &arg
	return f.passwordErr
}

func TestUpsertDirectoryUserCreate(t *testing.T) {
	t.Parallel()
	fake := &fakeDirectoryQuerier{}

	created, err := upsertDirectoryUser(t.Context(), store.NewDBQ(nil, fake),
		"Newbie", "newbie@example.com", new(true), "hashed")
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, &imsdb.DirectoryCreatePersonParams{
		Handle:   "Newbie",
		Email:    sql.NullString{String: "newbie@example.com", Valid: true},
		Password: "hashed",
		Active:   true,
		Onsite:   true,
	}, fake.created)
	assert.Nil(t, fake.updated)
	assert.Nil(t, fake.passwordSet)
}

func TestUpsertDirectoryUserCreateWithoutEmailOrOnsite(t *testing.T) {
	t.Parallel()
	fake := &fakeDirectoryQuerier{}

	created, err := upsertDirectoryUser(t.Context(), store.NewDBQ(nil, fake),
		"Newbie", "", nil, "hashed")
	require.NoError(t, err)
	assert.True(t, created)
	require.NotNil(t, fake.created)
	assert.False(t, fake.created.Email.Valid)
	assert.False(t, fake.created.Onsite)
}

func TestUpsertDirectoryUserUpdate(t *testing.T) {
	t.Parallel()
	fake := &fakeDirectoryQuerier{
		existing: &imsdb.DirectoryPersonByHandleRow{
			ID:     42,
			Handle: "Oldtimer",
			Email:  sql.NullString{String: "old@example.com", Valid: true},
			Active: false,
			Onsite: true,
		},
	}

	created, err := upsertDirectoryUser(t.Context(), store.NewDBQ(nil, fake),
		"Oldtimer", "new@example.com", new(false), "hashed")
	require.NoError(t, err)
	assert.False(t, created)
	assert.Nil(t, fake.created)
	assert.Equal(t, &imsdb.DirectoryUpdatePersonParams{
		Handle: "Oldtimer",
		Email:  sql.NullString{String: "new@example.com", Valid: true},
		Active: true,
		Onsite: false,
		ID:     42,
	}, fake.updated)
	assert.Equal(t, &imsdb.DirectorySetPersonPasswordParams{
		Password: "hashed",
		ID:       42,
	}, fake.passwordSet)
}

func TestUpsertDirectoryUserUpdateKeepsUnsetFields(t *testing.T) {
	t.Parallel()
	fake := &fakeDirectoryQuerier{
		existing: &imsdb.DirectoryPersonByHandleRow{
			ID:     42,
			Handle: "Oldtimer",
			Email:  sql.NullString{String: "old@example.com", Valid: true},
			Active: false,
			Onsite: true,
		},
	}

	created, err := upsertDirectoryUser(t.Context(), store.NewDBQ(nil, fake),
		"Oldtimer", "", nil, "hashed")
	require.NoError(t, err)
	assert.False(t, created)
	require.NotNil(t, fake.updated)
	assert.Equal(t, sql.NullString{String: "old@example.com", Valid: true}, fake.updated.Email)
	assert.True(t, fake.updated.Onsite)
	assert.True(t, fake.updated.Active)
}

func TestUpsertDirectoryUserErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	existing := &imsdb.DirectoryPersonByHandleRow{ID: 42, Handle: "Oldtimer"}

	lookupFails := &fakeDirectoryQuerier{lookupErr: boom}
	_, err := upsertDirectoryUser(t.Context(), store.NewDBQ(nil, lookupFails), "Oldtimer", "", nil, "hashed")
	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "[DirectoryPersonByHandle]")
	assert.Nil(t, lookupFails.created)
	assert.Nil(t, lookupFails.updated)

	createFails := &fakeDirectoryQuerier{createErr: boom}
	_, err = upsertDirectoryUser(t.Context(), store.NewDBQ(nil, createFails), "Newbie", "", nil, "hashed")
	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "[DirectoryCreatePerson]")

	updateFails := &fakeDirectoryQuerier{existing: existing, updateErr: boom}
	_, err = upsertDirectoryUser(t.Context(), store.NewDBQ(nil, updateFails), "Oldtimer", "", nil, "hashed")
	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "[DirectoryUpdatePerson]")
	// The password must not be changed if the rest of the update failed.
	assert.Nil(t, updateFails.passwordSet)

	passwordFails := &fakeDirectoryQuerier{existing: existing, passwordErr: boom}
	_, err = upsertDirectoryUser(t.Context(), store.NewDBQ(nil, passwordFails), "Oldtimer", "", nil, "hashed")
	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "[DirectorySetPersonPassword]")
}

func TestCheckAddUserConfig(t *testing.T) {
	t.Parallel()

	good := conf.DefaultIMS()
	good.Directory.Directory = conf.DirectoryTypeIMS
	good.Store.Type = conf.DBStoreTypeMaria
	require.NoError(t, checkAddUserConfig(good))

	clubhouse := conf.DefaultIMS()
	clubhouse.Directory.Directory = conf.DirectoryTypeClubhouseDB
	clubhouse.Store.Type = conf.DBStoreTypeMaria
	err := checkAddUserConfig(clubhouse)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IMS_DIRECTORY")

	noopStore := conf.DefaultIMS()
	noopStore.Directory.Directory = conf.DirectoryTypeIMS
	noopStore.Store.Type = conf.DBStoreTypeNoOp
	err = checkAddUserConfig(noopStore)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MariaDB")
}

func TestReadPasswordFromStdin(t *testing.T) {
	t.Parallel()

	pw, err := readPassword(true, strings.NewReader("hunter2\n"))
	require.NoError(t, err)
	assert.Equal(t, "hunter2", pw)

	pw, err = readPassword(true, strings.NewReader("hunter2\r\n"))
	require.NoError(t, err)
	assert.Equal(t, "hunter2", pw)

	// A final line with no newline still counts
	pw, err = readPassword(true, strings.NewReader("hunter2"))
	require.NoError(t, err)
	assert.Equal(t, "hunter2", pw)

	// Only the first line is the password
	pw, err = readPassword(true, strings.NewReader("hunter2\nsomething else\n"))
	require.NoError(t, err)
	assert.Equal(t, "hunter2", pw)

	// Leading and trailing spaces are part of the password
	pw, err = readPassword(true, strings.NewReader(" hunter2 \n"))
	require.NoError(t, err)
	assert.Equal(t, " hunter2 ", pw)

	_, err = readPassword(true, strings.NewReader(""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read password from stdin")

	_, err = readPassword(true, strings.NewReader("\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty password")
}
