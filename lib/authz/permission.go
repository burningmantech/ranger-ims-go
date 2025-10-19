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
	"fmt"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"slices"
	"strings"
	"time"
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
	// Event-specific permissions.

	EventReadIncidents EventPermissionMask = 1 << iota
	EventWriteIncidents
	EventReadAllFieldReports
	EventReadOwnFieldReports
	EventWriteAllFieldReports
	EventWriteOwnFieldReports
	EventReadEventName
	EventReadDestinations
)

const (
	// Permissions that aren't event-specific.

	GlobalListEvents GlobalPermissionMask = 1 << iota
	GlobalReadIncidentTypes
	GlobalReadStreets
	GlobalReadPersonnel
	GlobalAdministrateEvents
	GlobalAdministrateStreets
	GlobalAdministrateIncidentTypes
	GlobalAdministrateDestinations
	GlobalAdministrateDebugging
)

var RolesToGlobalPerms = map[Role]GlobalPermissionMask{
	AnyAuthenticatedUser: GlobalListEvents | GlobalReadIncidentTypes | GlobalReadPersonnel | GlobalReadStreets,
	Administrator:        GlobalAdministrateEvents | GlobalAdministrateStreets | GlobalAdministrateIncidentTypes | GlobalAdministrateDestinations | GlobalAdministrateDebugging,
}

var RolesToEventPerms = map[Role]EventPermissionMask{
	EventReporter: EventReadEventName | EventReadOwnFieldReports | EventWriteOwnFieldReports | EventReadDestinations,
	EventReader:   EventReadEventName | EventReadIncidents | EventReadOwnFieldReports | EventReadAllFieldReports | EventReadDestinations,
	EventWriter:   EventReadEventName | EventReadIncidents | EventWriteIncidents | EventReadAllFieldReports | EventReadOwnFieldReports | EventWriteAllFieldReports | EventWriteOwnFieldReports | EventReadDestinations,
}

func EventPermissions(
	ctx context.Context,
	eventID *int32, // nil for no event
	imsDBQ *store.DBQ,
	userStore *directory.UserStore,
	imsAdmins []string,
	claims IMSClaims,
) (eventPermissions map[int32]EventPermissionMask, globalPermissions GlobalPermissionMask, err error) {
	accessByEvent := make(map[int32][]imsdb.EventAccess)
	if eventID != nil {
		accessRows, err := imsDBQ.EventAccess(ctx, imsDBQ, *eventID)
		if err != nil {
			return nil, GlobalNoPermissions, fmt.Errorf("[EventAccess]: %w", err)
		}
		for _, ea := range accessRows {
			accessByEvent[*eventID] = append(accessByEvent[*eventID], ea.EventAccess)
		}
	}
	allPositions, allTeams, err := userStore.GetPositionsAndTeams(ctx)
	if err != nil {
		return nil, GlobalNoPermissions, fmt.Errorf("[GetPositionsAndTeams]: %w", err)
	}

	userPosIDs := claims.RangerPositions()
	userPosNames := make([]string, 0, len(userPosIDs))
	for _, userPosID := range userPosIDs {
		userPosNames = append(userPosNames, allPositions[userPosID])
	}
	userTeamIDs := claims.RangerTeams()
	userTeamNames := make([]string, 0, len(userTeamIDs))
	for _, userTeamID := range userTeamIDs {
		userTeamNames = append(userTeamNames, allTeams[userTeamID])
	}
	onDutyPosition := ""
	onDutyPositionID := claims.RangerOnDutyPosition()
	if onDutyPositionID != nil {
		onDutyPosition = allPositions[*onDutyPositionID]
	}

	eventPermissions, globalPermissions = ManyEventPermissions(
		accessByEvent,
		imsAdmins,
		claims.RangerHandle(),
		claims.RangerOnSite(),
		userPosNames,
		userTeamNames,
		onDutyPosition,
	)
	return eventPermissions, globalPermissions, nil
}

func ManyEventPermissions(
	accessByEvent map[int32][]imsdb.EventAccess, // eventID as key
	imsAdmins []string,
	handle string,
	onsite bool,
	positions []string,
	teams []string,
	onDutyPosition string,
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
			if PersonMatches(ea, handle, positions, teams, onsite, onDutyPosition) {
				eventPermissions[eventID] |= RolesToEventPerms[modeToRole[ea.Mode]]
			}
		}
	}
	return eventPermissions, globalPermissions
}

func PersonMatches(
	ea imsdb.EventAccess,
	handle string,
	positions []string,
	teams []string,
	onsite bool,
	onDutyPosition string,
) bool {
	if ea.Expires.Valid && conv.FloatToTime(ea.Expires.Float64).Before(time.Now()) {
		return false
	}
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
	if strings.HasPrefix(ea.Expression, "onduty:") &&
		onDutyPosition == strings.TrimPrefix(ea.Expression, "onduty:") {
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
	return matchExpr && matchValidity
}
