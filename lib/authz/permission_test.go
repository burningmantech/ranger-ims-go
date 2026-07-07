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
	"database/sql"
	"testing"
	"time"

	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/require"
)

var testAdmins = []string{"AdminCat", "AdminDog"}

const (
	readerPerm             = EventReadEventName | EventReadIncidents | EventReadOwnFieldReports | EventReadAllFieldReports | EventReadVisits | EventReadPlaces
	writerPerm             = EventReadEventName | EventReadIncidents | EventWriteIncidents | EventReadAllFieldReports | EventReadOwnFieldReports | EventWriteAllFieldReports | EventWriteOwnFieldReports | EventReadVisits | EventWriteVisits | EventReadPlaces
	reporterPerm           = EventReadEventName | EventReadOwnFieldReports | EventWriteOwnFieldReports | EventReadPlaces
	visitWriterPerm        = EventReadEventName | EventReadVisits | EventWriteVisits | EventReadPlaces
	authenticatedUserPerms = GlobalListEvents | GlobalReadIncidentTypes | GlobalReadPersonnel
	adminGlobalPerms       = GlobalAdministrateEvents | GlobalAdministrateIncidentTypes | GlobalAdministrateDebugging | GlobalAdministratePlaces | GlobalAdministrateDirectory
)

func addPerm(m map[int32][]imsdb.EventAccess, eventID int32, expr string, mode imsdb.EventAccessMode, validity imsdb.EventAccessValidity) {
	m[eventID] = append(m[eventID],
		imsdb.EventAccess{
			Event:      eventID,
			Expression: expr,
			Mode:       mode,
			Validity:   validity,
		},
	)
}

func addPermWithTimes(m map[int32][]imsdb.EventAccess, eventID int32, expr string, mode imsdb.EventAccessMode, validity imsdb.EventAccessValidity, notAfter, notBefore sql.NullFloat64) {
	m[eventID] = append(m[eventID],
		imsdb.EventAccess{
			Event:      eventID,
			Expression: expr,
			Mode:       mode,
			Validity:   validity,
			NotAfter:   notAfter,
			NotBefore:  notBefore,
		},
	)
}

// TestPersonMatches_notAfterNotBeforeGuards tests the NotAfter and NotBefore enforcement
// in PersonMatches, which is the single point where time-based access is evaluated.
func TestPersonMatches_notAfterNotBeforeGuards(t *testing.T) {
	t.Parallel()

	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	const eventID int32 = 123

	// Helper: run ManyEventPermissions for a single wildcard/always rule with given times.
	check := func(notAfter, notBefore sql.NullFloat64) EventPermissionMask {
		accessByEvent := make(map[int32][]imsdb.EventAccess)
		addPermWithTimes(accessByEvent, eventID, "*", modeRead, validityAlways, notAfter, notBefore)
		perms, _ := ManyEventPermissions(accessByEvent, testAdmins, "AnyRanger", false, nil, nil, "")
		return perms[eventID]
	}

	noTime := sql.NullFloat64{Valid: false}

	// No time bounds: rule grants access.
	require.Equal(t, readerPerm, check(noTime, noTime), "no bounds should grant access")

	// NotAfter in the past: rule is expired, no access.
	require.Equal(t, EventNoPermissions, check(conv.TimeToNullFloat(past), noTime), "expired rule should deny access")

	// NotAfter in the future: rule is active, access granted.
	require.Equal(t, readerPerm, check(conv.TimeToNullFloat(future), noTime), "unexpired rule should grant access")

	// NotBefore in the future: rule not yet active, no access.
	require.Equal(t, EventNoPermissions, check(noTime, conv.TimeToNullFloat(future)), "pending rule should deny access")

	// NotBefore in the past: rule is active, access granted.
	require.Equal(t, readerPerm, check(noTime, conv.TimeToNullFloat(past)), "started rule should grant access")

	// NotBefore in past, NotAfter in future: fully active, access granted.
	require.Equal(t, readerPerm, check(conv.TimeToNullFloat(future), conv.TimeToNullFloat(past)), "active rule should grant access")

	// NotBefore in future, NotAfter in future: not yet active even though not expired.
	require.Equal(t, EventNoPermissions, check(conv.TimeToNullFloat(future), conv.TimeToNullFloat(future)), "pending (not_before>now, not_after>now) should deny access")

	// NotBefore in past, NotAfter in past: expired wins.
	require.Equal(t, EventNoPermissions, check(conv.TimeToNullFloat(past), conv.TimeToNullFloat(past)), "expired rule with past not_before should deny access")
}

func TestManyEventPermissions_personRules(t *testing.T) {
	t.Parallel()
	accessByEvent := make(map[int32][]imsdb.EventAccess)
	addPerm(accessByEvent, 999, "person:SomeoneElse", modeRead, validityAlways)
	addPerm(accessByEvent, 123, "person:EventReaderGuy", modeRead, validityAlways)
	addPerm(accessByEvent, 123, "person:EventWriterGal", modeWrite, validityAlways)
	addPerm(accessByEvent, 123, "person:EventReporterPerson", modeReport, validityAlways)
	addPerm(accessByEvent, 123, "person:EventVisitWriterPerson", modeWriteVisits, validityAlways)

	permissions, globalPermissions := ManyEventPermissions(
		accessByEvent,
		testAdmins,
		"EventReaderGuy",
		true,
		[]string{},
		[]string{},
		"",
	)
	require.Equal(t, EventNoPermissions, permissions[999])
	require.Equal(t, readerPerm, permissions[123])
	require.Equal(t, authenticatedUserPerms, globalPermissions)

	permissions, globalPermissions = ManyEventPermissions(
		accessByEvent,
		testAdmins,
		"EventWriterGal",
		true,
		[]string{},
		[]string{},
		"",
	)
	require.Equal(t, EventNoPermissions, permissions[999])
	require.Equal(t, writerPerm, permissions[123])
	require.Equal(t, authenticatedUserPerms, globalPermissions)

	permissions, globalPermissions = ManyEventPermissions(
		accessByEvent,
		testAdmins,
		"EventReporterPerson",
		true,
		[]string{},
		[]string{},
		"",
	)
	require.Equal(t, EventNoPermissions, permissions[999])
	require.Equal(t, reporterPerm, permissions[123])
	require.Equal(t, authenticatedUserPerms, globalPermissions)

	permissions, globalPermissions = ManyEventPermissions(
		accessByEvent,
		testAdmins,
		"EventVisitWriterPerson",
		true,
		[]string{},
		[]string{},
		"",
	)
	require.Equal(t, EventNoPermissions, permissions[999])
	require.Equal(t, visitWriterPerm, permissions[123])
	require.Equal(t, authenticatedUserPerms, globalPermissions)

	permissions, globalPermissions = ManyEventPermissions(
		accessByEvent,
		testAdmins,
		"AdminCat",
		true,
		[]string{},
		[]string{},
		"",
	)
	require.Equal(t, EventNoPermissions, permissions[999])
	require.Equal(t, EventNoPermissions, permissions[123])
	require.Equal(t, authenticatedUserPerms|adminGlobalPerms, globalPermissions)
}

func TestManyEventPermissions_positionRules(t *testing.T) {
	t.Parallel()
	accessByEvent := make(map[int32][]imsdb.EventAccess)
	addPerm(accessByEvent, 123, "person:Running Ranger", modeReport, validityAlways)
	addPerm(accessByEvent, 123, "position:Runner", modeRead, validityAlways)
	addPerm(accessByEvent, 999, "position:Non-Runner", modeRead, validityAlways)

	// this user matches both a person and a position rule on event 123
	permissions, globalPermissions := ManyEventPermissions(
		accessByEvent,
		testAdmins,
		"Running Ranger",
		true,
		[]string{"Runner", "Swimmer"},
		[]string{},
		"",
	)
	require.Equal(t, EventNoPermissions, permissions[999])
	require.Equal(t, readerPerm|reporterPerm, permissions[123])
	require.Equal(t, authenticatedUserPerms, globalPermissions)
}

func TestManyEventPermissions_teamRules(t *testing.T) {
	t.Parallel()
	accessByEvent := make(map[int32][]imsdb.EventAccess)
	addPerm(accessByEvent, 123, "position:Runner", modeReport, validityAlways)
	addPerm(accessByEvent, 123, "team:Running Squad", modeRead, validityAlways)
	addPerm(accessByEvent, 999, "team:Non-Runner", modeRead, validityAlways)

	// this user matches both a team and position rule on event 123
	permissions, globalPermissions := ManyEventPermissions(
		accessByEvent,
		testAdmins,
		"Running Ranger",
		true,
		[]string{"Runner", "Swimmer"},
		[]string{"Running Squad", "Swimming Squad"},
		"",
	)
	require.Equal(t, EventNoPermissions, permissions[999])
	require.Equal(t, readerPerm|reporterPerm, permissions[123])
	require.Equal(t, authenticatedUserPerms, globalPermissions)
}

func TestManyEventPermissions_onDutyRules(t *testing.T) {
	t.Parallel()
	accessByEvent := make(map[int32][]imsdb.EventAccess)
	addPerm(accessByEvent, 123, "person:Running Ranger", modeReport, validityAlways)
	addPerm(accessByEvent, 123, "onduty:Runner", modeRead, validityAlways)
	addPerm(accessByEvent, 999, "position:Runner", modeRead, validityAlways)

	// this user matches both a person and an onduty rule on event 123
	permissions, globalPermissions := ManyEventPermissions(
		accessByEvent,
		testAdmins,
		"Running Ranger",
		true,
		[]string{},
		[]string{},
		"Runner",
	)
	require.Equal(t, EventNoPermissions, permissions[999])
	require.Equal(t, readerPerm|reporterPerm, permissions[123])
	require.Equal(t, authenticatedUserPerms, globalPermissions)
}

func TestManyEventPermissions_wildcardValidity(t *testing.T) {
	t.Parallel()
	accessByEvent := make(map[int32][]imsdb.EventAccess)
	addPerm(accessByEvent, 123, "*", modeReport, validityOnsite)

	permissions, globalPermissions := ManyEventPermissions(
		accessByEvent,
		testAdmins,
		"Onsite Ranger",
		true,
		[]string{"Runner", "Swimmer"},
		[]string{"Running Squad", "Swimming Squad"},
		"",
	)
	require.Equal(t, reporterPerm, permissions[123])
	require.Equal(t, authenticatedUserPerms, globalPermissions)

	permissions, globalPermissions = ManyEventPermissions(
		accessByEvent,
		testAdmins,
		"Offsite Ranger",
		false,
		[]string{"Runner", "Swimmer"},
		[]string{"Running Squad", "Swimming Squad"},
		"",
	)
	require.Equal(t, EventNoPermissions, permissions[123])
	require.Equal(t, authenticatedUserPerms, globalPermissions)
}
