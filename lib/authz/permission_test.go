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
	"testing"

	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/require"
)

var testAdmins = []string{"AdminCat", "AdminDog"}

const (
	readerPerm             = EventReadEventName | EventReadIncidents | EventReadOwnFieldReports | EventReadAllFieldReports | EventReadStays | EventReadDestinations
	writerPerm             = EventReadEventName | EventReadIncidents | EventWriteIncidents | EventReadAllFieldReports | EventReadOwnFieldReports | EventWriteAllFieldReports | EventWriteOwnFieldReports | EventReadStays | EventWriteStays | EventReadDestinations
	reporterPerm           = EventReadEventName | EventReadOwnFieldReports | EventWriteOwnFieldReports | EventReadDestinations
	stayWriterPerm         = EventReadEventName | EventReadStays | EventWriteStays | EventReadDestinations
	authenticatedUserPerms = GlobalListEvents | GlobalReadIncidentTypes | GlobalReadPersonnel | GlobalReadStreets
	adminGlobalPerms       = GlobalAdministrateEvents | GlobalAdministrateStreets | GlobalAdministrateIncidentTypes | GlobalAdministrateDebugging | GlobalAdministrateStreets | GlobalAdministrateDestinations
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

func TestManyEventPermissions_personRules(t *testing.T) {
	t.Parallel()
	accessByEvent := make(map[int32][]imsdb.EventAccess)
	addPerm(accessByEvent, 999, "person:SomeoneElse", modeRead, validityAlways)
	addPerm(accessByEvent, 123, "person:EventReaderGuy", modeRead, validityAlways)
	addPerm(accessByEvent, 123, "person:EventWriterGal", modeWrite, validityAlways)
	addPerm(accessByEvent, 123, "person:EventReporterPerson", modeReport, validityAlways)
	addPerm(accessByEvent, 123, "person:EventStayWriterPerson", modeWriteStays, validityAlways)

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
		"EventStayWriterPerson",
		true,
		[]string{},
		[]string{},
		"",
	)
	require.Equal(t, EventNoPermissions, permissions[999])
	require.Equal(t, stayWriterPerm, permissions[123])
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
