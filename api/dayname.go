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
	"context"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// maxDayNameLength matches the EVENT_DAY_NAME.NAME column's width.
const maxDayNameLength = 128

type GetEventDayNames struct {
	imsDBQ            *store.DBQ
	userStore         *directory.UserStore
	imsAdmins         []string
	cacheControlShort time.Duration
}

func (action GetEventDayNames) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.run(req)
	if errHTTP != nil {
		errHTTP.From("[run]").WriteResponse(w)
		return
	}
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%v, private", action.cacheControlShort.Milliseconds()/1000))
	mustWriteJSON(w, req, resp)
}

func (action GetEventDayNames) run(req *http.Request) ([]imsjson.EventDayName, *herr.HTTPError) {
	ctx := req.Context()
	event, errHTTP := getEvent(req, req.PathValue("eventName"), action.imsDBQ)
	if errHTTP != nil {
		return nil, errHTTP.From("[getEvent]")
	}
	jwtCtx, errHTTP := getJwtCtx(req)
	if errHTTP != nil {
		return nil, errHTTP.From("[getJwtCtx]")
	}
	eventPermissions, globalPermissions, err := authz.EventPermissions(ctx, &event.ID, action.imsDBQ, action.userStore, action.imsAdmins, *jwtCtx.Claims)
	if err != nil {
		return nil, herr.InternalServerError("Failed to compute permissions", err).From("[EventPermissions]")
	}
	// Anyone with any role on the event can see its report entries, so they
	// get its day names too. Admins get them for the admin page, even without
	// a role on the event.
	if eventPermissions[event.ID]&authz.EventReadEventName == 0 &&
		globalPermissions&authz.GlobalAdministrateEvents == 0 {
		return nil, herr.Forbidden("The requestor does not have EventReadEventName permission", nil)
	}

	rows, err := action.imsDBQ.EventDayNames(ctx, action.imsDBQ, event.ID)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch day names", err).From("[EventDayNames]")
	}
	resp := make([]imsjson.EventDayName, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, imsjson.EventDayName{Date: row.Date, Name: row.Name})
	}
	return resp, nil
}

// UpdateEventDayNames replaces all of an event's day names with those in the
// request body.
type UpdateEventDayNames struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

func (action UpdateEventDayNames) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.run(req)
	if errHTTP != nil {
		errHTTP.From("[run]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action UpdateEventDayNames) run(req *http.Request) *herr.HTTPError {
	ctx := req.Context()
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateEvents == 0 {
		return herr.Forbidden("The requestor does not have GlobalAdministrateEvents permission", nil)
	}
	event, errHTTP := getEvent(req, req.PathValue("eventName"), action.imsDBQ)
	if errHTTP != nil {
		return errHTTP.From("[getEvent]")
	}
	if event.IsGroup {
		return herr.BadRequest("An event group can't have day names", nil)
	}
	dayNames, errHTTP := readBodyAs[[]imsjson.EventDayName](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}
	dayNames, errHTTP = validateDayNames(dayNames)
	if errHTTP != nil {
		return errHTTP.From("[validateDayNames]")
	}
	errHTTP = retryOnDeadlockErr(func() *herr.HTTPError {
		return action.replaceDayNames(ctx, event.ID, dayNames)
	})
	if errHTTP != nil {
		return errHTTP.From("[replaceDayNames]")
	}
	return nil
}

// validateDayNames checks each day name, returning them with the names trimmed.
func validateDayNames(dayNames []imsjson.EventDayName) ([]imsjson.EventDayName, *herr.HTTPError) {
	seen := make(map[string]bool, len(dayNames))
	result := make([]imsjson.EventDayName, 0, len(dayNames))
	for _, dn := range dayNames {
		parsed, err := time.Parse(time.DateOnly, dn.Date)
		// Parse accepts e.g. "2026-9-5"; insist on the canonical form, since
		// clients match on the string.
		if err != nil || parsed.Format(time.DateOnly) != dn.Date {
			return nil, herr.BadRequest(fmt.Sprintf("Invalid date %q; use YYYY-MM-DD", dn.Date), err)
		}
		if seen[dn.Date] {
			return nil, herr.BadRequest(fmt.Sprintf("Date %v appears more than once", dn.Date), nil)
		}
		seen[dn.Date] = true
		name := strings.TrimSpace(dn.Name)
		if name == "" {
			return nil, herr.BadRequest(fmt.Sprintf("The name for %v is empty", dn.Date), nil)
		}
		// A newline would split the name across lines of the admin page's CSV.
		if strings.ContainsFunc(name, unicode.IsControl) {
			return nil, herr.BadRequest(fmt.Sprintf("The name for %v contains a line break or other control character", dn.Date), nil)
		}
		if utf8.RuneCountInString(name) > maxDayNameLength {
			return nil, herr.BadRequest(fmt.Sprintf("The name for %v is longer than %v characters", dn.Date, maxDayNameLength), nil)
		}
		result = append(result, imsjson.EventDayName{Date: dn.Date, Name: name})
	}
	return result, nil
}

func (action UpdateEventDayNames) replaceDayNames(ctx context.Context, eventID int32, dayNames []imsjson.EventDayName) *herr.HTTPError {
	txn, err := action.imsDBQ.BeginTx(ctx, nil)
	if err != nil {
		return herr.InternalServerError("Failed to begin transaction", err).From("[BeginTx]")
	}
	defer rollback(txn)
	err = action.imsDBQ.DeleteEventDayNames(ctx, txn, eventID)
	if err != nil {
		return herr.InternalServerError("Failed to remove day names", err).From("[DeleteEventDayNames]")
	}
	for _, dn := range dayNames {
		err = action.imsDBQ.CreateEventDayName(ctx, txn, imsdb.CreateEventDayNameParams{
			Event: eventID,
			Date:  dn.Date,
			Name:  dn.Name,
		})
		if err != nil {
			return herr.InternalServerError("Failed to save day name", err).From("[CreateEventDayName]")
		}
	}
	err = txn.Commit()
	if err != nil {
		return herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}
	return nil
}
