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
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"net/http"
	"sync"
	"time"
)

type GetEventAccesses struct {
	imsDB     *store.DB
	imsAdmins []string
}

func (action GetEventAccesses) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errH := action.getEventAccesses(req)
	if errH != nil {
		errH.Src("[getEventAccesses]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, resp)
}
func (action GetEventAccesses) getEventAccesses(req *http.Request) (imsjson.EventsAccess, *herr.HTTPError) {
	var empty imsjson.EventsAccess
	_, globalPermissions, errH := mustGetGlobalPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return empty, errH.Src("[mustGetGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateEvents == 0 {
		return empty, herr.S403("The requestor does not have GlobalAdministrateEvents permission", nil)
	}

	resp, errH := getEventsAccess(req.Context(), action.imsDB)
	if errH != nil {
		return empty, errH.Src("[getEventsAccess]")
	}
	return resp, nil
}

func getEventsAccess(ctx context.Context, imsDB *store.DB) (imsjson.EventsAccess, *herr.HTTPError) {
	allEventRows, err := imsdb.New(imsDB).Events(ctx)
	if err != nil {
		return nil, herr.S500("Failed to fetch Events", err).Src("[Events]")
	}
	var storedEvents []imsdb.Event
	for _, aer := range allEventRows {
		storedEvents = append(storedEvents, aer.Event)
	}

	accessRows, err := imsdb.New(imsDB).EventAccessAll(ctx)
	if err != nil {
		return nil, herr.S500("Failed to fetch EventAccess", err).Src("[EventAccessAll]")
	}
	accessRowByEventID := make(map[int32][]imsdb.EventAccess)
	for _, ar := range accessRows {
		accessRowByEventID[ar.EventAccess.Event] = append(accessRowByEventID[ar.EventAccess.Event], ar.EventAccess)
	}

	result := make(imsjson.EventsAccess)
	for _, e := range storedEvents {
		ea := imsjson.EventAccess{
			Readers:   []imsjson.AccessRule{},
			Writers:   []imsjson.AccessRule{},
			Reporters: []imsjson.AccessRule{},
		}
		for _, accessRow := range accessRowByEventID[e.ID] {
			access := accessRow
			rule := imsjson.AccessRule{Expression: access.Expression, Validity: string(access.Validity)}
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
	imsDB     *store.DB
	imsAdmins []string
}

var eventAccessWriteMu sync.Mutex

func (action PostEventAccess) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errH := action.postEventAccess(req)
	if errH != nil {
		errH.Src("[postEventAccess]").WriteResponse(w)
		return
	}
	http.Error(w, "Successfully set event access", http.StatusNoContent)
}

func (action PostEventAccess) postEventAccess(req *http.Request) *herr.HTTPError {
	_, globalPermissions, errH := mustGetGlobalPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return errH.Src("[mustGetGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateEvents == 0 {
		return herr.S403("The requestor does not have GlobalAdministrateEvents permission", nil)
	}
	ctx := req.Context()
	eventsAccess, errH := mustReadBodyAs[imsjson.EventsAccess](req)
	if errH != nil {
		return errH.Src("[mustReadBodyAs]")
	}
	for eventName, access := range eventsAccess {
		event, errH := mustGetEvent(req, eventName, action.imsDB)
		if errH != nil {
			return errH.Src("[mustReadBodyAs]")
		}
		if errH = action.maybeSetAccess(ctx, event, access.Readers, imsdb.EventAccessModeRead); errH != nil {
			return errH.Src("[maybeSetAccess] EventAccessModeRead")
		}
		if errH = action.maybeSetAccess(ctx, event, access.Writers, imsdb.EventAccessModeWrite); errH != nil {
			return errH.Src("[maybeSetAccess] EventAccessModeWrite")
		}
		if errH = action.maybeSetAccess(ctx, event, access.Reporters, imsdb.EventAccessModeReport); errH != nil {
			return errH.Src("[maybeSetAccess] EventAccessModeReport")
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

	txn, err := action.imsDB.BeginTx(ctx, nil)
	if err != nil {
		return herr.S500("Failed to begin transaction", err).Src("[BeginTx]")
	}
	defer rollback(txn)
	err = imsdb.New(txn).ClearEventAccessForMode(ctx, imsdb.ClearEventAccessForModeParams{
		Event: event.ID,
		Mode:  mode,
	})
	if err != nil {
		return herr.S500("Failed to begin transaction", err).Src("[ClearEventAccessForMode]")
	}
	for _, rule := range rules {
		err = imsdb.New(txn).ClearEventAccessForExpression(ctx, imsdb.ClearEventAccessForExpressionParams{
			Event:      event.ID,
			Expression: rule.Expression,
		})
		if err != nil {
			return herr.S500("Failed to clear event access", err).Src("[ClearEventAccessForExpression]")
		}
		_, err = imsdb.New(txn).AddEventAccess(ctx, imsdb.AddEventAccessParams{
			Event:      event.ID,
			Expression: rule.Expression,
			Mode:       mode,
			Validity:   imsdb.EventAccessValidity(rule.Validity),
		})
		if err != nil {
			return herr.S500("Failed to add event access", err).Src("[AddEventAccess]")
		}
	}
	if err = txn.Commit(); err != nil {
		return herr.S500("Failed to commit transaction", err).Src("[Commit]")
	}
	return nil
}
