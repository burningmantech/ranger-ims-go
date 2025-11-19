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
	"database/sql"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
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
	err := req.ParseForm()
	if err != nil {
		return nil, herr.BadRequest("Failed to parse form", err)
	}
	excludeGroups := !strings.EqualFold(req.Form.Get("include_groups"), "true")

	allEvents, err := action.imsDBQ.Events(req.Context(), action.imsDBQ)
	if err != nil {
		return nil, herr.InternalServerError("Failed to get events", err).From("[Events]")
	}
	permsByEvent, errHTTP := permissionsByEvent(req.Context(), jwt, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return empty, errHTTP.From("[permissionsByEvent]")
	}

	var authorizedEvents []imsdb.EventsRow
	for _, eve := range allEvents {
		if eve.Event.IsGroup && excludeGroups {
			continue
		}
		if permsByEvent[eve.Event.ID]&authz.EventReadEventName != 0 || globalPermissions&authz.GlobalAdministrateEvents != 0 {
			authorizedEvents = append(authorizedEvents, eve)
		}
	}
	resp := make(imsjson.Events, 0, len(authorizedEvents))
	for _, eve := range authorizedEvents {
		resp = append(resp, imsjson.Event{
			ID:          eve.Event.ID,
			Name:        &eve.Event.Name,
			IsGroup:     &eve.Event.IsGroup,
			ParentGroup: conv.SqlToInt32(eve.Event.ParentGroup),
		})
	}

	slices.SortFunc(resp, func(a, b imsjson.Event) int {
		return cmp.Compare(a.ID, b.ID)
	})

	return resp, nil
}

type EditEvent struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

// Require basic cleanliness for EventName, since it's used in IMS URLs
// and in filesystem directory paths.
var allowedEventNames = regexp.MustCompile(`^[\w-]+$`)

func (action EditEvent) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	newID, errHTTP := action.editEvents(req)
	if errHTTP != nil {
		errHTTP.From("[editEvents]").WriteResponse(w)
		return
	}
	if newID != nil {
		w.Header().Set("IMS-Event-ID", strconv.Itoa(int(*newID)))
	}
	herr.WriteNoContentResponse(w, "Success")
}
func (action EditEvent) editEvents(req *http.Request) (newEventID *int32, errHTTP *herr.HTTPError) {
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return nil, errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateEvents == 0 {
		return nil, herr.Forbidden("The requestor does not have GlobalAdministrateEvents permission", nil)
	}
	err := req.ParseForm()
	if err != nil {
		return nil, herr.BadRequest("Failed to parse HTTP form", err)
	}
	editRequest, errHTTP := readBodyAs[imsjson.Event](req)
	if errHTTP != nil {
		return nil, errHTTP.From("[readBodyAs]")
	}

	if editRequest.ID == 0 {
		// We're making a new Event.
		if editRequest.Name == nil || !allowedEventNames.MatchString(*editRequest.Name) {
			return nil, herr.BadRequest("Event names must match the pattern "+allowedEventNames.String(), fmt.Errorf("invalid event name: '%v'", editRequest.Name))
		}
		createParams := imsdb.CreateEventParams{
			Name: *editRequest.Name,
		}
		if editRequest.IsGroup != nil {
			createParams.IsGroup = *editRequest.IsGroup
		}
		if editRequest.ParentGroup != nil {
			createParams.ParentGroup = sql.NullInt32{Int32: *editRequest.ParentGroup, Valid: true}
		}
		id, err := action.imsDBQ.CreateEvent(req.Context(), action.imsDBQ, createParams)
		if err != nil {
			return nil, herr.InternalServerError("Failed to create event", err).From("[CreateEvent]")
		}
		slog.Info("Created event", "eventName", *editRequest.Name, "id", id)
		newID := conv.MustInt32(id)
		return &newID, nil
	}

	existingEventRow, err := action.imsDBQ.Event(req.Context(), action.imsDBQ, editRequest.ID)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch event", err).From("[Event]")
	}

	updateParams := imsdb.UpdateEventParams{
		ID:          editRequest.ID,
		Name:        existingEventRow.Event.Name,
		IsGroup:     existingEventRow.Event.IsGroup,
		ParentGroup: existingEventRow.Event.ParentGroup,
	}

	if editRequest.Name != nil {
		if !allowedEventNames.MatchString(*editRequest.Name) {
			return nil, herr.BadRequest("Event names must match the pattern "+allowedEventNames.String(), fmt.Errorf("invalid event name: '%v'", editRequest.Name))
		}
		updateParams.Name = *editRequest.Name
	}
	if editRequest.IsGroup != nil {
		updateParams.IsGroup = *editRequest.IsGroup
	}
	if editRequest.ParentGroup != nil {
		if *editRequest.ParentGroup == editRequest.ID {
			return nil, herr.BadRequest("Event parent group cannot be the same as the event itself", nil)
		}
		if *editRequest.ParentGroup > 0 {
			targetParentGroup, err := action.imsDBQ.Event(req.Context(), action.imsDBQ, *editRequest.ParentGroup)
			if err != nil {
				return nil, herr.InternalServerError("Failed to fetch parent group", err).From("[Event]")
			}
			if !targetParentGroup.Event.IsGroup {
				return nil, herr.BadRequest("Event parent must be an event group", nil)
			}
			updateParams.ParentGroup = sql.NullInt32{Int32: *editRequest.ParentGroup, Valid: true}
		} else {
			updateParams.ParentGroup = sql.NullInt32{}
		}
	}
	if updateParams.IsGroup && updateParams.ParentGroup.Valid {
		return nil, herr.BadRequest("An event group cannot have a parent event group", nil)
	}

	err = action.imsDBQ.UpdateEvent(req.Context(), action.imsDBQ, updateParams)
	if err != nil {
		return nil, herr.InternalServerError("Failed to update event", err).From("[UpdateEvent]")
	}

	return nil, nil
}
