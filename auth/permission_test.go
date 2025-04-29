package auth

import (
	"github.com/srabraham/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/require"
	"testing"
)

var (
	testAdmins = []string{"AdminCat", "AdminDog"}

	readerPerm             = EventReadEventName | EventReadIncidents | EventReadOwnFieldReports | EventReadAllFieldReports
	writerPerm             = EventReadEventName | EventReadIncidents | EventWriteIncidents | EventReadAllFieldReports | EventReadOwnFieldReports | EventWriteAllFieldReports | EventWriteOwnFieldReports
	reporterPerm           = EventReadEventName | EventReadOwnFieldReports | EventWriteOwnFieldReports
	authenticatedUserPerms = GlobalListEvents | GlobalReadIncidentTypes | GlobalReadPersonnel | GlobalReadStreets
	adminGlobalPerms       = GlobalAdministrateEvents | GlobalAdministrateStreets | GlobalAdministrateIncidentTypes
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
	)
	require.Equal(t, EventNoPermissions, permissions[999])
	require.Equal(t, EventNoPermissions, permissions[123])
	require.Equal(t, authenticatedUserPerms|adminGlobalPerms, globalPermissions)
}

func TestManyEventPermissions_positionRules(t *testing.T) {
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
	)
	require.Equal(t, EventNoPermissions, permissions[999])
	require.Equal(t, readerPerm|reporterPerm, permissions[123])
	require.Equal(t, authenticatedUserPerms, globalPermissions)
}

func TestManyEventPermissions_teamRules(t *testing.T) {
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
	)
	require.Equal(t, EventNoPermissions, permissions[999])
	require.Equal(t, readerPerm|reporterPerm, permissions[123])
	require.Equal(t, authenticatedUserPerms, globalPermissions)
}

func TestManyEventPermissions_wildcardValidity(t *testing.T) {
	accessByEvent := make(map[int32][]imsdb.EventAccess)
	addPerm(accessByEvent, 123, "*", "report", "onsite")

	permissions, globalPermissions := ManyEventPermissions(
		accessByEvent,
		testAdmins,
		"Onsite Ranger",
		true,
		[]string{"Runner", "Swimmer"},
		[]string{"Running Squad", "Swimming Squad"},
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
	)
	require.Equal(t, EventNoPermissions, permissions[123])
	require.Equal(t, authenticatedUserPerms, globalPermissions)
}
