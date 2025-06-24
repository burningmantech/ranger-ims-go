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
	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"net/http"
	"slices"
	"sync"
	"time"
)

type GetEventAccesses struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

func (action GetEventAccesses) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getEventAccesses(req)
	if errHTTP != nil {
		errHTTP.From("[getEventAccesses]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}
func (action GetEventAccesses) getEventAccesses(req *http.Request) (imsjson.EventsAccess, *herr.HTTPError) {
	var empty imsjson.EventsAccess
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return empty, errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateEvents == 0 {
		return empty, herr.Forbidden("The requestor does not have GlobalAdministrateEvents permission", nil)
	}

	resp, errHTTP := action.getEventsAccess(req.Context())
	if errHTTP != nil {
		return empty, errHTTP.From("[getEventsAccess]")
	}
	return resp, nil
}

func (action GetEventAccesses) getEventsAccess(ctx context.Context) (imsjson.EventsAccess, *herr.HTTPError) {
	allEventRows, err := action.imsDBQ.Events(ctx, action.imsDBQ)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch Events", err).From("[Events]")
	}
	var storedEvents []imsdb.Event
	for _, aer := range allEventRows {
		storedEvents = append(storedEvents, aer.Event)
	}

	accessRows, err := action.imsDBQ.EventAccessAll(ctx, action.imsDBQ)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch EventAccess", err).From("[EventAccessAll]")
	}
	accessRowByEventID := make(map[int32][]imsdb.EventAccess)
	for _, ar := range accessRows {
		accessRowByEventID[ar.EventAccess.Event] = append(accessRowByEventID[ar.EventAccess.Event], ar.EventAccess)
	}

	result := make(imsjson.EventsAccess)

	users, err := action.userStore.GetAllUsers(ctx)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch Users", err).From("[Users]")
	}

	for _, e := range storedEvents {
		ea := imsjson.EventAccess{
			Readers:   []imsjson.AccessRule{},
			Writers:   []imsjson.AccessRule{},
			Reporters: []imsjson.AccessRule{},
		}
		for _, accessRow := range accessRowByEventID[e.ID] {
			access := accessRow
			rule := imsjson.AccessRule{Expression: access.Expression, Validity: string(access.Validity)}

			if access.Expression == "*" && access.Validity == imsdb.EventAccessValidityAlways {
				rule.DebugInfo.MatchesAllUsers = true
			} else {
				for _, person := range users {
					onDutyPosition := ""
					if person.OnDutyPositionName != nil {
						onDutyPosition = *person.OnDutyPositionName
					}
					if authz.PersonMatches(access, person.Handle, person.PositionNames, person.TeamNames, person.Onsite, onDutyPosition) {
						rule.DebugInfo.MatchesUsers = append(rule.DebugInfo.MatchesUsers, person.Handle)
					}
				}
				if len(rule.DebugInfo.MatchesUsers) == 0 {
					rule.DebugInfo.MatchesNoOne = true
				}
			}
			slices.Sort(rule.DebugInfo.MatchesUsers)

			switch access.Mode {
			case imsdb.EventAccessModeRead:
				ea.Readers = append(ea.Readers, rule)
			case imsdb.EventAccessModeWrite:
				ea.Writers = append(ea.Writers, rule)
			case imsdb.EventAccessModeReport:
				ea.Reporters = append(ea.Reporters, rule)
			}
		}
		result[e.Name] = ea
	}
	return result, nil
}

type PostEventAccess struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

var eventAccessWriteMu sync.Mutex

func (action PostEventAccess) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.postEventAccess(req)
	if errHTTP != nil {
		errHTTP.From("[postEventAccess]").WriteResponse(w)
		return
	}
	http.Error(w, "Successfully set event access", http.StatusNoContent)
}

func (action PostEventAccess) postEventAccess(req *http.Request) *herr.HTTPError {
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateEvents == 0 {
		return herr.Forbidden("The requestor does not have GlobalAdministrateEvents permission", nil)
	}
	ctx := req.Context()
	eventsAccess, errHTTP := readBodyAs[imsjson.EventsAccess](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}
	for eventName, access := range eventsAccess {
		event, errHTTP := getEvent(req, eventName, action.imsDBQ)
		if errHTTP != nil {
			return errHTTP.From("[readBodyAs]")
		}
		if errHTTP = action.maybeSetAccess(ctx, event, access.Readers, imsdb.EventAccessModeRead); errHTTP != nil {
			return errHTTP.From("[maybeSetAccess] EventAccessModeRead")
		}
		if errHTTP = action.maybeSetAccess(ctx, event, access.Writers, imsdb.EventAccessModeWrite); errHTTP != nil {
			return errHTTP.From("[maybeSetAccess] EventAccessModeWrite")
		}
		if errHTTP = action.maybeSetAccess(ctx, event, access.Reporters, imsdb.EventAccessModeReport); errHTTP != nil {
			return errHTTP.From("[maybeSetAccess] EventAccessModeReport")
		}
	}
	return nil
}

func (action PostEventAccess) maybeSetAccess(
	ctx context.Context, event imsdb.Event, rules []imsjson.AccessRule, mode imsdb.EventAccessMode,
) *herr.HTTPError {
	if rules == nil {
		return nil
	}

	// Lock out any other callers from concurrently invoking this method.
	// This function is very prone to transaction deadlock, because it does
	// multiple transactional deletes and inserts. Add a timeout in here too,
	// just to be safe that we don't end up holding the lock forever.
	eventAccessWriteMu.Lock()
	defer eventAccessWriteMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	txn, err := action.imsDBQ.BeginTx(ctx, nil)
	if err != nil {
		return herr.InternalServerError("Failed to begin transaction", err).From("[BeginTx]")
	}
	defer rollback(txn)
	err = action.imsDBQ.ClearEventAccessForMode(ctx, txn,
		imsdb.ClearEventAccessForModeParams{
			Event: event.ID,
			Mode:  mode,
		},
	)
	if err != nil {
		return herr.InternalServerError("Failed to begin transaction", err).From("[ClearEventAccessForMode]")
	}
	for _, rule := range rules {
		err = action.imsDBQ.ClearEventAccessForExpression(ctx, txn,
			imsdb.ClearEventAccessForExpressionParams{
				Event:      event.ID,
				Expression: rule.Expression,
			},
		)
		if err != nil {
			return herr.InternalServerError("Failed to clear event access", err).From("[ClearEventAccessForExpression]")
		}
		_, err = action.imsDBQ.AddEventAccess(ctx, txn,
			imsdb.AddEventAccessParams{
				Event:      event.ID,
				Expression: rule.Expression,
				Mode:       mode,
				Validity:   imsdb.EventAccessValidity(rule.Validity),
			},
		)
		if err != nil {
			return herr.InternalServerError("Failed to add event access", err).From("[AddEventAccess]")
		}
	}
	if err = txn.Commit(); err != nil {
		return herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}
	return nil
}
