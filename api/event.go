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
	imsDB             *store.DB
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
	mustWriteJSON(w, resp)
}
func (action GetEvents) getEvents(req *http.Request) (imsjson.Events, *herr.HTTPError) {
	var empty imsjson.Events
	jwt, globalPermissions, errHTTP := mustGetGlobalPermissions(req, action.imsDB, action.imsAdmins)
	if errHTTP != nil {
		return empty, errHTTP.From("[mustGetGlobalPermissions]")
	}
	// This is the first level of authorization. Per-event filtering is done farther down.
	if globalPermissions&authz.GlobalListEvents == 0 {
		return empty, herr.Forbidden("The requestor does not have GlobalListEvents permission", nil)
	}

	allEvents, err := imsdb.New(action.imsDB).Events(req.Context())
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
	accessRows, err := imsdb.New(action.imsDB).EventAccessAll(ctx)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch event access", err).From("[EventAccessAll]")
	}
	accessRowByEventID := make(map[int32][]imsdb.EventAccess)
	for _, ar := range accessRows {
		accessRowByEventID[ar.EventAccess.Event] = append(accessRowByEventID[ar.EventAccess.Event], ar.EventAccess)
	}

	permissionsByEvent, _ := authz.ManyEventPermissions(
		accessRowByEventID,
		action.imsAdmins,
		jwtCtx.Claims.RangerHandle(),
		jwtCtx.Claims.RangerOnSite(),
		jwtCtx.Claims.RangerPositions(),
		jwtCtx.Claims.RangerTeams(),
	)
	return permissionsByEvent, nil
}

type EditEvents struct {
	imsDB     *store.DB
	imsAdmins []string
}

// Require basic cleanliness for EventName, since it's used in IMS URLs
// and in filesystem directory paths.
var allowedEventNames = regexp.MustCompile(`^[\w-]+$`)

func (action EditEvents) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if errHTTP := action.editEvents(req); errHTTP != nil {
		errHTTP.From("[editEvents]").WriteResponse(w)
		return
	}
	http.Error(w, "Success", http.StatusNoContent)
}
func (action EditEvents) editEvents(req *http.Request) *herr.HTTPError {
	_, globalPermissions, errHTTP := mustGetGlobalPermissions(req, action.imsDB, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[mustGetGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateEvents == 0 {
		return herr.Forbidden("The requestor does not have GlobalAdministrateEvents permission", nil)
	}
	if err := req.ParseForm(); err != nil {
		return herr.BadRequest("Failed to parse HTTP form", err)
	}
	editRequest, errHTTP := mustReadBodyAs[imsjson.EditEventsRequest](req)
	if errHTTP != nil {
		return errHTTP.From("[mustReadBodyAs]")
	}

	for _, eventName := range editRequest.Add {
		if !allowedEventNames.MatchString(eventName) {
			return herr.BadRequest("Event names must match the pattern "+allowedEventNames.String(), fmt.Errorf("invalid event name: '%s'", eventName))
		}
		id, err := imsdb.New(action.imsDB).CreateEvent(req.Context(), eventName)
		if err != nil {
			return herr.InternalServerError("Failed to create event", err).From("[CreateEvent]")
		}
		slog.Info("Created event", "eventName", eventName, "id", id)
	}
	return nil
}
