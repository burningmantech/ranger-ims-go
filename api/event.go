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

package api

import (
	"cmp"
	"context"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"time"
)

type GetEvents struct {
	imsDBQ            *store.DBQ
	userStore         *directory.UserStore
	imsAdmins         []string
	cacheControlShort time.Duration
}

func (action GetEvents) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getEvents(req)
	if errHTTP != nil {
		errHTTP.From("[getEvents]").WriteResponse(w)
		return
	}
	w.Header().Set("Cache-Control", fmt.Sprintf(
		"max-age=%v, private", action.cacheControlShort.Milliseconds()/1000))
	mustWriteJSON(w, req, resp)
}
func (action GetEvents) getEvents(req *http.Request) (imsjson.Events, *herr.HTTPError) {
	var empty imsjson.Events
	jwt, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return empty, errHTTP.From("[getGlobalPermissions]")
	}
	// This is the first level of authorization. Per-event filtering is done farther down.
	if globalPermissions&authz.GlobalListEvents == 0 {
		return empty, herr.Forbidden("The requestor does not have GlobalListEvents permission", nil)
	}

	allEvents, err := action.imsDBQ.Events(req.Context(), action.imsDBQ)
	if err != nil {
		return nil, herr.InternalServerError("Failed to get events", err).From("[Events]")
	}
	permissionsByEvent, errHTTP := action.permissionsByEvent(req.Context(), jwt)
	if errHTTP != nil {
		return empty, errHTTP.From("[permissionsByEvent]")
	}

	var authorizedEvents []imsdb.EventsRow
	for _, eve := range allEvents {
		if permissionsByEvent[eve.Event.ID]&authz.EventReadEventName != 0 {
			authorizedEvents = append(authorizedEvents, eve)
		}
	}
	resp := make(imsjson.Events, 0, len(authorizedEvents))
	for _, eve := range authorizedEvents {
		resp = append(resp, imsjson.Event{
			ID:   eve.Event.ID,
			Name: eve.Event.Name,
		})
	}

	slices.SortFunc(resp, func(a, b imsjson.Event) int {
		return cmp.Compare(a.ID, b.ID)
	})

	return resp, nil
}

func (action GetEvents) permissionsByEvent(ctx context.Context, jwtCtx JWTContext) (
	map[int32]authz.EventPermissionMask, *herr.HTTPError,
) {
	accessRows, err := action.imsDBQ.EventAccessAll(ctx, action.imsDBQ)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch event access", err).From("[EventAccessAll]")
	}
	accessRowByEventID := make(map[int32][]imsdb.EventAccess)
	for _, ar := range accessRows {
		accessRowByEventID[ar.EventAccess.Event] = append(accessRowByEventID[ar.EventAccess.Event], ar.EventAccess)
	}

	allPositions, allTeams, err := action.userStore.GetPositionsAndTeams(ctx)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch positions and teams", err).From("[GetPositionsAndTeams]")
	}
	userPosIDs := jwtCtx.Claims.RangerPositions()
	userPosNames := make([]string, 0, len(userPosIDs))
	for _, userPosID := range userPosIDs {
		userPosNames = append(userPosNames, allPositions[userPosID])
	}
	userTeamIDs := jwtCtx.Claims.RangerTeams()
	userTeamNames := make([]string, 0, len(userTeamIDs))
	for _, userTeamID := range userTeamIDs {
		userTeamNames = append(userTeamNames, allTeams[userTeamID])
	}
	onDutyPosition := ""
	onDutyPositionID := jwtCtx.Claims.RangerOnDutyPosition()
	if onDutyPositionID != nil {
		onDutyPosition = allPositions[*onDutyPositionID]
	}

	permissionsByEvent, _ := authz.ManyEventPermissions(
		accessRowByEventID,
		action.imsAdmins,
		jwtCtx.Claims.RangerHandle(),
		jwtCtx.Claims.RangerOnSite(),
		userPosNames,
		userTeamNames,
		onDutyPosition,
	)
	return permissionsByEvent, nil
}

type EditEvents struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

// Require basic cleanliness for EventName, since it's used in IMS URLs
// and in filesystem directory paths.
var allowedEventNames = regexp.MustCompile(`^[\w-]+$`)

func (action EditEvents) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.editEvents(req)
	if errHTTP != nil {
		errHTTP.From("[editEvents]").WriteResponse(w)
		return
	}
	http.Error(w, "Success", http.StatusNoContent)
}
func (action EditEvents) editEvents(req *http.Request) *herr.HTTPError {
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateEvents == 0 {
		return herr.Forbidden("The requestor does not have GlobalAdministrateEvents permission", nil)
	}
	err := req.ParseForm()
	if err != nil {
		return herr.BadRequest("Failed to parse HTTP form", err)
	}
	editRequest, errHTTP := readBodyAs[imsjson.EditEventsRequest](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}

	for _, eventName := range editRequest.Add {
		if !allowedEventNames.MatchString(eventName) {
			return herr.BadRequest("Event names must match the pattern "+allowedEventNames.String(), fmt.Errorf("invalid event name: '%s'", eventName))
		}
		id, err := action.imsDBQ.CreateEvent(req.Context(), action.imsDBQ, eventName)
		if err != nil {
			return herr.InternalServerError("Failed to create event", err).From("[CreateEvent]")
		}
		slog.Info("Created event", "eventName", eventName, "id", id)
	}
	return nil
}
