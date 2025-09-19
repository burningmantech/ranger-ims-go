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
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"io"
	"log/slog"
	"net/http"
)

func readBodyAs[T any](req *http.Request) (T, *herr.HTTPError) {
	empty := *new(T)
	defer shut(req.Body)
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return empty, herr.BadRequest("Failed to read request body", err).From("[io.ReadAll]")
	}
	var t T
	err = json.Unmarshal(bodyBytes, &t)
	if err != nil {
		return empty, herr.BadRequest("Failed to unmarshal request body", err).From("[Unmarshal]")
	}
	return t, nil
}

func eventFromFormValue(req *http.Request, imsDBQ *store.DBQ) (imsdb.Event, *herr.HTTPError) {
	empty := imsdb.Event{}
	err := req.ParseForm()
	if err != nil {
		return empty, herr.BadRequest("Failed to parse form", err).From("ParseForm")
	}
	eventName := req.FormValue("event_id")
	if eventName == "" {
		return empty, herr.BadRequest("No event_id was found in the URL", nil)
	}
	eventRow, err := imsDBQ.QueryEventID(req.Context(), imsDBQ, eventName)
	if err != nil {
		return empty, herr.New(http.StatusInternalServerError, "Failed to get event ID", fmt.Errorf("[QueryEventID]: %w", err))
	}
	return eventRow.Event, nil
}

func getEvent(req *http.Request, eventName string, imsDBQ *store.DBQ) (imsdb.Event, *herr.HTTPError) {
	var empty imsdb.Event
	if eventName == "" {
		return empty, herr.BadRequest("No eventName was provided", nil)
	}
	eventRow, err := imsDBQ.QueryEventID(req.Context(), imsDBQ, eventName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return empty, herr.NotFound("Event not found", err)
		}
		return empty, herr.InternalServerError("Failed to fetch Event", err).From("[QueryEventID]")
	}
	return eventRow.Event, nil
}

func mustWriteJSON(w http.ResponseWriter, req *http.Request, resp any) (success bool) {
	marshalled, err := json.Marshal(resp)
	if err != nil {
		herr.InternalServerError("Failed to marshal JSON", err).From("[Marshal]").WriteResponse(w)
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(marshalled)
	if err != nil {
		herr.InternalServerError("Failed to write JSON", err).From("[Write]").WriteResponse(w)
		return false
	}
	return true
}

func getJwtCtx(req *http.Request) (JWTContext, *herr.HTTPError) {
	jwtCtx, found := req.Context().Value(JWTContextKey).(JWTContext)
	if !found {
		return JWTContext{}, herr.InternalServerError("This endpoint has been misconfigured", nil)
	}
	return jwtCtx, nil
}

func getEventPermissions(req *http.Request, imsDBQ *store.DBQ, userStore *directory.UserStore, imsAdmins []string) (
	imsdb.Event, JWTContext, authz.EventPermissionMask, *herr.HTTPError,
) {
	event, errHTTP := getEvent(req, req.PathValue("eventName"), imsDBQ)
	if errHTTP != nil {
		return imsdb.Event{}, JWTContext{}, 0, errHTTP.From("[getEvent]")
	}
	jwtCtx, errHTTP := getJwtCtx(req)
	if errHTTP != nil {
		return imsdb.Event{}, JWTContext{}, 0, errHTTP.From("[getJwtCtx]")
	}
	eventPermissions, _, err := authz.EventPermissions(req.Context(), &event.ID, imsDBQ, userStore, imsAdmins, *jwtCtx.Claims)
	if err != nil {
		return imsdb.Event{}, JWTContext{}, 0, herr.InternalServerError("Failed to compute permissions", err).From("[EventPermissions]")
	}
	return event, jwtCtx, eventPermissions[event.ID], nil
}

func getGlobalPermissions(req *http.Request, imsDBQ *store.DBQ, userStore *directory.UserStore, imsAdmins []string) (
	JWTContext, authz.GlobalPermissionMask, *herr.HTTPError,
) {
	empty := JWTContext{}
	jwtCtx, errHTTP := getJwtCtx(req)
	if errHTTP != nil {
		return empty, 0, errHTTP.From("[getJwtCtx]")
	}
	_, globalPermissions, err := authz.EventPermissions(req.Context(), nil, imsDBQ, userStore, imsAdmins, *jwtCtx.Claims)
	if err != nil {
		return empty, 0, herr.InternalServerError("Failed to compute permissions", err).From("[EventPermissions]")
	}
	return jwtCtx, globalPermissions, nil
}

func permissionsByEvent(ctx context.Context, jwtCtx JWTContext, imsDBQ *store.DBQ, userStore *directory.UserStore, imsAdmins []string) (
	map[int32]authz.EventPermissionMask, *herr.HTTPError,
) {
	accessRows, err := imsDBQ.EventAccessAll(ctx, imsDBQ)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch event access", err).From("[EventAccessAll]")
	}
	accessRowByEventID := make(map[int32][]imsdb.EventAccess)
	for _, ar := range accessRows {
		accessRowByEventID[ar.EventAccess.Event] = append(accessRowByEventID[ar.EventAccess.Event], ar.EventAccess)
	}

	allPositions, allTeams, err := userStore.GetPositionsAndTeams(ctx)
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
		imsAdmins,
		jwtCtx.Claims.RangerHandle(),
		jwtCtx.Claims.RangerOnSite(),
		userPosNames,
		userTeamNames,
		onDutyPosition,
	)
	return permissionsByEvent, nil
}

func rollback(txn *sql.Tx) {
	err := txn.Rollback()
	if err != nil && !errors.Is(err, sql.ErrTxDone) {
		slog.Error("Failed to rollback transaction", "error", err)
	}
}

func shut(c io.Closer) {
	err := c.Close()
	if err != nil {
		slog.Error("Failed to close Closer", "error", err)
	}
}
