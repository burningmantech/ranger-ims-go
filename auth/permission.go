package auth

import (
	"context"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"slices"
	"strings"
)

type Role string

var (
	modeToRole = map[imsdb.EventAccessMode]Role{
		imsdb.EventAccessModeRead:   EventReader,
		imsdb.EventAccessModeWrite:  EventWriter,
		imsdb.EventAccessModeReport: EventReporter,
	}
)

const (
	AnyAuthenticatedUser Role = "AnyAuthenticatedUser"
	EventReporter        Role = "EventReporter"
	EventReader          Role = "EventReader"
	EventWriter          Role = "EventWriter"
	Administrator        Role = "Administrator"
)

type GlobalPermissionMask uint16
type EventPermissionMask uint16

const (
	EventNoPermissions  EventPermissionMask  = 0
	GlobalNoPermissions GlobalPermissionMask = 0
)

const (
	// Event-specific permissions

	EventReadIncidents EventPermissionMask = 1 << iota
	EventWriteIncidents
	EventReadAllFieldReports
	EventReadOwnFieldReports
	EventWriteAllFieldReports
	EventWriteOwnFieldReports
	EventReadEventName
)

const (
	// Permissions that aren't event-specific

	GlobalListEvents GlobalPermissionMask = 1 << iota
	GlobalReadIncidentTypes
	GlobalReadStreets
	GlobalReadPersonnel
	GlobalAdministrateEvents
	GlobalAdministrateStreets
	GlobalAdministrateIncidentTypes
)

var RolesToGlobalPerms = map[Role]GlobalPermissionMask{
	AnyAuthenticatedUser: GlobalListEvents | GlobalReadIncidentTypes | GlobalReadPersonnel | GlobalReadStreets,
	Administrator:        GlobalAdministrateEvents | GlobalAdministrateStreets | GlobalAdministrateIncidentTypes,
}

var RolesToEventPerms = map[Role]EventPermissionMask{
	EventReporter: EventReadEventName | EventReadOwnFieldReports | EventWriteOwnFieldReports,
	EventReader:   EventReadEventName | EventReadIncidents | EventReadOwnFieldReports | EventReadAllFieldReports,
	EventWriter:   EventReadEventName | EventReadIncidents | EventWriteIncidents | EventReadAllFieldReports | EventReadOwnFieldReports | EventWriteAllFieldReports | EventWriteOwnFieldReports,
}

func EventPermissions(
	ctx context.Context,
	eventID *int32, // nil for no event
	imsDB *store.DB,
	imsAdmins []string,
	claims IMSClaims,
) (eventPermissions map[int32]EventPermissionMask, globalPermissions GlobalPermissionMask, err error) {
	accessByEvent := make(map[int32][]imsdb.EventAccess)
	if eventID != nil {
		accessRows, err := imsdb.New(imsDB).EventAccess(ctx, *eventID)
		if err != nil {
			return nil, GlobalNoPermissions, fmt.Errorf("EventAccess: %w", err)
		}
		for _, ea := range accessRows {
			accessByEvent[*eventID] = append(accessByEvent[*eventID], ea.EventAccess)
		}
	}
	eventPermissions, globalPermissions = ManyEventPermissions(accessByEvent, imsAdmins, claims.RangerHandle(), claims.RangerOnSite(), claims.RangerPositions(), claims.RangerTeams())
	return eventPermissions, globalPermissions, nil
}

func ManyEventPermissions(
	accessByEvent map[int32][]imsdb.EventAccess, // eventID as key
	imsAdmins []string,
	handle string,
	onsite bool,
	positions []string,
	teams []string,
) (eventPermissions map[int32]EventPermissionMask, globalPermissions GlobalPermissionMask) {

	eventPermissions = make(map[int32]EventPermissionMask)
	globalPermissions = GlobalNoPermissions

	if handle != "" {
		globalPermissions |= RolesToGlobalPerms[AnyAuthenticatedUser]
	}

	if slices.Contains(imsAdmins, handle) {
		globalPermissions |= RolesToGlobalPerms[Administrator]
	}

	for eventID, accesses := range accessByEvent {
		eventPermissions[eventID] = EventNoPermissions
		for _, ea := range accesses {
			matchExpr := false
			if ea.Expression == "*" {
				matchExpr = true
			}
			if strings.HasPrefix(ea.Expression, "person:") &&
				strings.TrimPrefix(ea.Expression, "person:") == handle {
				matchExpr = true
			}
			if strings.HasPrefix(ea.Expression, "position:") &&
				slices.Contains(positions, strings.TrimPrefix(ea.Expression, "position:")) {
				matchExpr = true
			}
			if strings.HasPrefix(ea.Expression, "team:") &&
				slices.Contains(teams, strings.TrimPrefix(ea.Expression, "team:")) {
				matchExpr = true
			}
			matchValidity := false
			if ea.Validity == imsdb.EventAccessValidityAlways {
				matchValidity = true
			}
			if ea.Validity == imsdb.EventAccessValidityOnsite && onsite {
				matchValidity = true
			}
			if matchExpr && matchValidity {
				eventPermissions[eventID] |= RolesToEventPerms[modeToRole[ea.Mode]]
			}
		}
	}
	return eventPermissions, globalPermissions
}
