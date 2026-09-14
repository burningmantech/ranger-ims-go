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

package authz

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/require"
)

// fakeAccessQuerier serves EventAndParentAccess from memory. Any other Querier
// method panics on the nil embedded interface, which is what we want: the
// permission lookup has no business calling anything else.
type fakeAccessQuerier struct {
	imsdb.Querier

	rows       []imsdb.EventAndParentAccessRow
	err        error
	calls      int
	gotEventID int32
}

func (q *fakeAccessQuerier) EventAndParentAccess(
	_ context.Context, _ imsdb.DBTX, arg imsdb.EventAndParentAccessParams,
) ([]imsdb.EventAndParentAccessRow, error) {
	q.calls++
	q.gotEventID = arg.EventID
	return q.rows, q.err
}

func (q *fakeAccessQuerier) addRow(eventID int32, expr string, mode imsdb.EventAccessMode, validity imsdb.EventAccessValidity) {
	q.rows = append(q.rows, imsdb.EventAndParentAccessRow{
		EventAccess: imsdb.EventAccess{
			Event:      eventID,
			Expression: expr,
			Mode:       mode,
			Validity:   validity,
		},
	})
}

type fakeDirectorySource struct {
	users map[int64]*directory.User
	err   error
	// If nonzero, FetchUsers succeeds this many times and then returns err.
	failAfter int
	fetches   int
}

func (s *fakeDirectorySource) FetchUsers(context.Context) (map[int64]*directory.User, error) {
	s.fetches++
	if s.err != nil && s.fetches > s.failAfter {
		return nil, s.err
	}
	return s.users, nil
}

func (s *fakeDirectorySource) FetchPositions(context.Context) (map[int64]string, error) {
	return map[int64]string{}, nil
}

func (s *fakeDirectorySource) FetchTeams(context.Context) (map[int64]string, error) {
	return map[int64]string{}, nil
}

func newDBQ(q *fakeAccessQuerier) *store.DBQ {
	return store.NewDBQ(nil, q)
}

func newUserStore(src *fakeDirectorySource) *directory.UserStore {
	return directory.NewUserStore(src, time.Hour)
}

func claimsFor(handle, directoryID string) IMSClaims {
	return IMSClaims{}.WithRangerHandle(handle).WithSubject(directoryID)
}

func TestEventPermissions_noEventSkipsAccessQuery(t *testing.T) {
	t.Parallel()
	querier := &fakeAccessQuerier{}
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware"},
	}}

	eventPerms, globalPerms, err := EventPermissions(
		t.Context(), nil, newDBQ(querier), newUserStore(src), testAdmins, claimsFor("Hardware", "1"),
	)
	require.NoError(t, err)
	require.Equal(t, 0, querier.calls)
	require.Empty(t, eventPerms)
	require.Equal(t, authenticatedUserPerms, globalPerms)
}

func TestEventPermissions_adminGlobalPerms(t *testing.T) {
	t.Parallel()
	querier := &fakeAccessQuerier{}
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		1: {ID: 1, Handle: "AdminCat"},
	}}

	eventPerms, globalPerms, err := EventPermissions(
		t.Context(), nil, newDBQ(querier), newUserStore(src), testAdmins, claimsFor("AdminCat", "1"),
	)
	require.NoError(t, err)
	require.Empty(t, eventPerms)
	require.Equal(t, authenticatedUserPerms|adminGlobalPerms, globalPerms)

	// Being an admin grants nothing on an event by itself.
	eventID := int32(123)
	querier.addRow(eventID, "person:SomeoneElse", modeWrite, validityAlways)
	eventPerms, globalPerms, err = EventPermissions(
		t.Context(), &eventID, newDBQ(querier), newUserStore(src), testAdmins, claimsFor("AdminCat", "1"),
	)
	require.NoError(t, err)
	require.Equal(t, EventNoPermissions, eventPerms[eventID])
	require.Equal(t, authenticatedUserPerms|adminGlobalPerms, globalPerms)
}

func TestEventPermissions_noHandleGetsNoGlobalPerms(t *testing.T) {
	t.Parallel()
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	querier.addRow(eventID, "*", modeRead, validityAlways)
	src := &fakeDirectorySource{users: map[int64]*directory.User{}}

	eventPerms, globalPerms, err := EventPermissions(
		t.Context(), &eventID, newDBQ(querier), newUserStore(src), testAdmins, IMSClaims{},
	)
	require.NoError(t, err)
	require.Equal(t, GlobalNoPermissions, globalPerms)
	// The wildcard matches on expression alone, so it still applies here.
	// Authentication is what keeps handleless tokens out.
	require.Equal(t, readerPerm, eventPerms[eventID])
}

func TestEventPermissions_personRule(t *testing.T) {
	t.Parallel()
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	querier.addRow(eventID, "person:Hardware", modeWrite, validityAlways)
	querier.addRow(eventID, "person:Software", modeRead, validityAlways)
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware"},
		2: {ID: 2, Handle: "Software"},
		3: {ID: 3, Handle: "Firmware"},
	}}
	dbq := newDBQ(querier)
	users := newUserStore(src)

	eventPerms, globalPerms, err := EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Hardware", "1"))
	require.NoError(t, err)
	require.Equal(t, eventID, querier.gotEventID)
	require.Equal(t, writerPerm, eventPerms[eventID])
	require.Equal(t, authenticatedUserPerms, globalPerms)

	eventPerms, _, err = EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Software", "2"))
	require.NoError(t, err)
	require.Equal(t, readerPerm, eventPerms[eventID])

	eventPerms, _, err = EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Firmware", "3"))
	require.NoError(t, err)
	require.Equal(t, EventNoPermissions, eventPerms[eventID])
}

func TestEventPermissions_eventWithNoAccessRows(t *testing.T) {
	t.Parallel()
	// This is also what an event group looks like, since the query returns no
	// rows for one.
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware", Onsite: true, PositionNames: []string{"Dirt"}},
	}}

	eventPerms, globalPerms, err := EventPermissions(
		t.Context(), &eventID, newDBQ(querier), newUserStore(src), testAdmins, claimsFor("Hardware", "1"),
	)
	require.NoError(t, err)
	require.Equal(t, 1, querier.calls)
	require.Equal(t, EventNoPermissions, eventPerms[eventID])
	require.Equal(t, authenticatedUserPerms, globalPerms)
}

func TestEventPermissions_parentGroupRowsApplyToEvent(t *testing.T) {
	t.Parallel()
	const parentGroupID int32 = 10
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	querier.addRow(eventID, "person:Hardware", modeReport, validityAlways)
	querier.addRow(parentGroupID, "person:Hardware", modeWriteVisits, validityAlways)
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware"},
	}}

	eventPerms, _, err := EventPermissions(
		t.Context(), &eventID, newDBQ(querier), newUserStore(src), testAdmins, claimsFor("Hardware", "1"),
	)
	require.NoError(t, err)
	require.Equal(t, reporterPerm|visitWriterPerm, eventPerms[eventID])
	require.NotContains(t, eventPerms, parentGroupID)
}

func TestEventPermissions_positionFromDirectory(t *testing.T) {
	t.Parallel()
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	querier.addRow(eventID, "position:Dirt", modeRead, validityAlways)
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware", PositionIDs: []int64{7, 8}, PositionNames: []string{"Dirt", "Green Dot"}},
		2: {ID: 2, Handle: "Software", PositionIDs: []int64{8}, PositionNames: []string{"Green Dot"}},
	}}
	dbq := newDBQ(querier)
	users := newUserStore(src)

	eventPerms, _, err := EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Hardware", "1"))
	require.NoError(t, err)
	require.Equal(t, readerPerm, eventPerms[eventID])

	eventPerms, _, err = EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Software", "2"))
	require.NoError(t, err)
	require.Equal(t, EventNoPermissions, eventPerms[eventID])
}

func TestEventPermissions_teamFromDirectory(t *testing.T) {
	t.Parallel()
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	querier.addRow(eventID, "team:Council", modeWrite, validityAlways)
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware", TeamIDs: []int64{3}, TeamNames: []string{"Council"}},
		2: {ID: 2, Handle: "Software", TeamIDs: []int64{4}, TeamNames: []string{"Tech Team"}},
	}}
	dbq := newDBQ(querier)
	users := newUserStore(src)

	eventPerms, _, err := EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Hardware", "1"))
	require.NoError(t, err)
	require.Equal(t, writerPerm, eventPerms[eventID])

	eventPerms, _, err = EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Software", "2"))
	require.NoError(t, err)
	require.Equal(t, EventNoPermissions, eventPerms[eventID])
}

func TestEventPermissions_onDutyFromDirectory(t *testing.T) {
	t.Parallel()
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	querier.addRow(eventID, "onduty:Operator", modeWrite, validityAlways)
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		// On duty as Operator.
		1: {ID: 1, Handle: "Hardware", PositionNames: []string{"Operator"}, OnDutyPositionID: new(int64(5)), OnDutyPositionName: new("Operator")},
		// Holds the position, but is on duty as something else.
		2: {ID: 2, Handle: "Software", PositionNames: []string{"Operator", "Dirt"}, OnDutyPositionID: new(int64(6)), OnDutyPositionName: new("Dirt")},
		// Holds the position, but isn't on duty at all.
		3: {ID: 3, Handle: "Firmware", PositionNames: []string{"Operator"}},
	}}
	dbq := newDBQ(querier)
	users := newUserStore(src)

	eventPerms, _, err := EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Hardware", "1"))
	require.NoError(t, err)
	require.Equal(t, writerPerm, eventPerms[eventID])

	eventPerms, _, err = EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Software", "2"))
	require.NoError(t, err)
	require.Equal(t, EventNoPermissions, eventPerms[eventID])

	eventPerms, _, err = EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Firmware", "3"))
	require.NoError(t, err)
	require.Equal(t, EventNoPermissions, eventPerms[eventID])
}

func TestEventPermissions_onsiteFromDirectory(t *testing.T) {
	t.Parallel()
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	querier.addRow(eventID, "*", modeReport, validityOnsite)
	querier.addRow(eventID, "*", modeRead, validityAlways)
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware", Onsite: true},
		2: {ID: 2, Handle: "Software", Onsite: false},
	}}
	dbq := newDBQ(querier)
	users := newUserStore(src)

	eventPerms, _, err := EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Hardware", "1"))
	require.NoError(t, err)
	require.Equal(t, readerPerm|reporterPerm, eventPerms[eventID])

	eventPerms, _, err = EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Software", "2"))
	require.NoError(t, err)
	require.Equal(t, readerPerm, eventPerms[eventID])
}

// The token's subject, not its handle, is what selects the directory record.
func TestEventPermissions_directoryLookupUsesSubject(t *testing.T) {
	t.Parallel()
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	querier.addRow(eventID, "position:Dirt", modeRead, validityAlways)
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware", PositionNames: []string{"Dirt"}},
		2: {ID: 2, Handle: "Software"},
	}}

	eventPerms, _, err := EventPermissions(
		t.Context(), &eventID, newDBQ(querier), newUserStore(src), testAdmins, claimsFor("Hardware", "2"),
	)
	require.NoError(t, err)
	require.Equal(t, EventNoPermissions, eventPerms[eventID])
}

func TestEventPermissions_userMissingFromDirectory(t *testing.T) {
	t.Parallel()
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	querier.addRow(eventID, "person:Hardware", modeReport, validityAlways)
	querier.addRow(eventID, "*", modeRead, validityAlways)
	querier.addRow(eventID, "*", modeWriteVisits, validityOnsite)
	querier.addRow(eventID, "position:Dirt", modeWrite, validityAlways)
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware", Onsite: true, PositionNames: []string{"Dirt"}},
	}}
	dbq := newDBQ(querier)
	users := newUserStore(src)

	// A subject with no directory record has no positions, teams, or duty, and
	// isn't onsite. Handle-based and always-valid wildcard rules still apply.
	eventPerms, globalPerms, err := EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Hardware", "999"))
	require.NoError(t, err)
	require.Equal(t, reporterPerm|readerPerm, eventPerms[eventID])
	require.Equal(t, authenticatedUserPerms, globalPerms)

	// Likewise for a subject that isn't a number at all.
	eventPerms, _, err = EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claimsFor("Hardware", "not-an-id"))
	require.NoError(t, err)
	require.Equal(t, reporterPerm|readerPerm, eventPerms[eventID])
}

// Tokens issued before positions, teams, onsite, and on-duty moved out of the
// JWT still carry those claims. They must have no effect.
func TestEventPermissions_ignoresLegacyTokenClaims(t *testing.T) {
	t.Parallel()
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	querier.addRow(eventID, "position:Dirt", modeWrite, validityAlways)
	querier.addRow(eventID, "team:Council", modeWrite, validityAlways)
	querier.addRow(eventID, "onduty:Dirt", modeWrite, validityAlways)
	querier.addRow(eventID, "*", modeRead, validityOnsite)
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware", Onsite: false},
	}}

	claims := claimsFor("Hardware", "1")

	eventPerms, _, err := EventPermissions(
		t.Context(), &eventID, newDBQ(querier), newUserStore(src), testAdmins, claims,
	)
	require.NoError(t, err)
	require.Equal(t, EventNoPermissions, eventPerms[eventID])
}

// With the directory consulted on every call, a change there takes effect as
// soon as the user store's cache is refreshed, without a new token.
func TestEventPermissions_directoryChangesApplyToExistingToken(t *testing.T) {
	t.Parallel()
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	querier.addRow(eventID, "position:Dirt", modeWrite, validityAlways)
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware", PositionNames: []string{"Dirt"}},
	}}
	dbq := newDBQ(querier)
	users := newUserStore(src)
	claims := claimsFor("Hardware", "1")

	eventPerms, _, err := EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claims)
	require.NoError(t, err)
	require.Equal(t, writerPerm, eventPerms[eventID])

	// The position is revoked in the directory.
	src.users = map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware"},
	}

	// Until the cache is refreshed, the old position is still in effect.
	eventPerms, _, err = EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claims)
	require.NoError(t, err)
	require.Equal(t, writerPerm, eventPerms[eventID])

	users.Flush()
	eventPerms, _, err = EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claims)
	require.NoError(t, err)
	require.Equal(t, EventNoPermissions, eventPerms[eventID])

	// And it's granted back again.
	src.users = map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware", PositionNames: []string{"Dirt"}},
	}
	users.Flush()
	eventPerms, _, err = EventPermissions(t.Context(), &eventID, dbq, users, testAdmins, claims)
	require.NoError(t, err)
	require.Equal(t, writerPerm, eventPerms[eventID])
}

func TestEventPermissions_accessQueryError(t *testing.T) {
	t.Parallel()
	eventID := int32(123)
	querier := &fakeAccessQuerier{err: errors.New("db is on fire")}
	src := &fakeDirectorySource{users: map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware"},
	}}

	eventPerms, globalPerms, err := EventPermissions(
		t.Context(), &eventID, newDBQ(querier), newUserStore(src), testAdmins, claimsFor("Hardware", "1"),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "[EventAccess]")
	require.Contains(t, err.Error(), "db is on fire")
	require.Nil(t, eventPerms)
	require.Equal(t, GlobalNoPermissions, globalPerms)
}

func TestEventPermissions_directoryError(t *testing.T) {
	t.Parallel()
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	querier.addRow(eventID, "*", modeRead, validityAlways)
	src := &fakeDirectorySource{err: errors.New("clubhouse is down")}

	eventPerms, globalPerms, err := EventPermissions(
		t.Context(), &eventID, newDBQ(querier), newUserStore(src), testAdmins, claimsFor("AdminCat", "1"),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "[PositionsForRanger]")
	require.Contains(t, err.Error(), "clubhouse is down")
	require.Nil(t, eventPerms)
	require.Equal(t, GlobalNoPermissions, globalPerms)

	// Directory failures matter even without an event, since the lookup
	// happens regardless.
	eventPerms, globalPerms, err = EventPermissions(
		t.Context(), nil, newDBQ(querier), newUserStore(src), testAdmins, claimsFor("AdminCat", "1"),
	)
	require.Error(t, err)
	require.Nil(t, eventPerms)
	require.Equal(t, GlobalNoPermissions, globalPerms)
}

// Each directory lookup re-reads the user store, so any of them can fail on its
// own. A zero TTL makes every lookup hit the source, letting the source start
// failing partway through.
func TestEventPermissions_laterDirectoryLookupErrors(t *testing.T) {
	t.Parallel()
	eventID := int32(123)
	querier := &fakeAccessQuerier{}
	users := map[int64]*directory.User{
		1: {ID: 1, Handle: "Hardware"},
	}
	claims := claimsFor("Hardware", "1")

	teamsFail := &fakeDirectorySource{users: users, err: errors.New("teams failed"), failAfter: 1}
	eventPerms, globalPerms, err := EventPermissions(
		t.Context(), &eventID, newDBQ(querier), directory.NewUserStore(teamsFail, 0), testAdmins, claims,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "[TeamsForRanger]")
	require.Nil(t, eventPerms)
	require.Equal(t, GlobalNoPermissions, globalPerms)

	onDutyFails := &fakeDirectorySource{users: users, err: errors.New("on duty failed"), failAfter: 2}
	eventPerms, globalPerms, err = EventPermissions(
		t.Context(), &eventID, newDBQ(querier), directory.NewUserStore(onDutyFails, 0), testAdmins, claims,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "[OnDutyForRanger]")
	require.Nil(t, eventPerms)
	require.Equal(t, GlobalNoPermissions, globalPerms)

	onSiteFails := &fakeDirectorySource{users: users, err: errors.New("on site failed"), failAfter: 3}
	eventPerms, globalPerms, err = EventPermissions(
		t.Context(), &eventID, newDBQ(querier), directory.NewUserStore(onSiteFails, 0), testAdmins, claims,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "[OnSiteForRanger]")
	require.Nil(t, eventPerms)
	require.Equal(t, GlobalNoPermissions, globalPerms)
}
