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
	"net/http"
	"time"

	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
)

// GetOnDutyRangers lists the Rangers who were signed in to a position at the
// time given by the "at" parameter (RFC 3339, default now), from the
// directory's timesheet, so the Incident page can add them with their position
// as their role. It's empty for a directory with no timesheet.
type GetOnDutyRangers struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

func (action GetOnDutyRangers) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.run(req)
	if errHTTP != nil {
		errHTTP.From("[run]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}

func (action GetOnDutyRangers) run(req *http.Request) ([]imsjson.OnDutyRanger, *herr.HTTPError) {
	ctx := req.Context()
	event, errHTTP := getEvent(req, req.PathValue("eventName"), action.imsDBQ)
	if errHTTP != nil {
		return nil, errHTTP.From("[getEvent]")
	}
	jwtCtx, errHTTP := getJwtCtx(req)
	if errHTTP != nil {
		return nil, errHTTP.From("[getJwtCtx]")
	}
	eventPermissions, _, err := authz.EventPermissions(ctx, &event.ID, action.imsDBQ, action.userStore, action.imsAdmins, *jwtCtx.Claims)
	if err != nil {
		return nil, herr.InternalServerError("Failed to compute permissions", err).From("[EventPermissions]")
	}
	if eventPermissions[event.ID]&authz.EventReadIncidents == 0 {
		return nil, herr.Forbidden("The requestor does not have EventReadIncidents permission", nil)
	}

	at := time.Now()
	if atParam := req.FormValue("at"); atParam != "" {
		at, err = time.Parse(time.RFC3339, atParam)
		if err != nil {
			return nil, herr.BadRequest("Invalid 'at' time", err)
		}
	}

	onDuty, err := action.userStore.OnDuty(ctx, at)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch on-duty Rangers", err).From("[OnDuty]")
	}
	resp := make([]imsjson.OnDutyRanger, 0, len(onDuty))
	for _, r := range onDuty {
		resp = append(resp, imsjson.OnDutyRanger{Handle: r.Handle, Position: r.Position})
	}
	return resp, nil
}
