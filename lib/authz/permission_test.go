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

package authz_test

import (
	. "github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/require"
	"testing"
)

var testAdmins = []string{"AdminCat", "AdminDog"}

const (
	readerPerm             = EventReadEventName | EventReadIncidents | EventReadOwnFieldReports | EventReadAllFieldReports | EventReadDestinations
	writerPerm             = EventReadEventName | EventReadIncidents | EventWriteIncidents | EventReadAllFieldReports | EventReadOwnFieldReports | EventWriteAllFieldReports | EventWriteOwnFieldReports | EventReadDestinations
	reporterPerm           = EventReadEventName | EventReadOwnFieldReports | EventWriteOwnFieldReports | EventReadDestinations
	authenticatedUserPerms = GlobalListEvents | GlobalReadIncidentTypes | GlobalReadPersonnel | GlobalReadStreets
	adminGlobalPerms       = GlobalAdministrateEvents | GlobalAdministrateStreets | GlobalAdministrateIncidentTypes | GlobalAdministrateDebugging | GlobalAdministrateStreets | GlobalAdministrateDestinations
)

func addPerm(m map[int32][]imsdb.EventAccess, eventID int32, expr, mode, validity string) {
	m[eventID] = append(m[eventID], imsdb.EventAccess{
		Event:      eventID,
		Expression: expr,
		Mode:       imsdb.EventAccessMode(mode),
		Validity:   imsdb.EventAccessValidity(validity),
	})
}

func TestManyEventPermissions_personRules(t *testing.T) {
	t.Parallel()
	accessByEvent := make(map[int32][]imsdb.EventAccess)
	addPerm(accessByEvent, 999, "person:SomeoneElse", "read", "always")
	addPerm(accessByEvent, 123, "person:EventReaderGuy", "read", "always")
	addPerm(accessByEvent, 123, "person:EventWriterGal", "write", "always")
	addPerm(accessByEvent, 123, "person:EventReporterPerson", "report", "always")
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
	addPerm(accessByEvent, 123, "person:Running Ranger", "report", "always")
	addPerm(accessByEvent, 123, "position:Runner", "read", "always")
	addPerm(accessByEvent, 999, "position:Non-Runner", "read", "always")

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
	addPerm(accessByEvent, 123, "position:Runner", "report", "always")
	addPerm(accessByEvent, 123, "team:Running Squad", "read", "always")
	addPerm(accessByEvent, 999, "team:Non-Runner", "read", "always")

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
	addPerm(accessByEvent, 123, "person:Running Ranger", "report", "always")
	addPerm(accessByEvent, 123, "onduty:Runner", "read", "always")
	addPerm(accessByEvent, 999, "position:Runner", "read", "always")

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
	addPerm(accessByEvent, 123, "*", "report", "onsite")

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
