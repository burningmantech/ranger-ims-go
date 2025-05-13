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
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"io"
	"log/slog"
	"net/http"
)

func mustReadBodyAs[T any](req *http.Request) (T, *herr.HTTPError) {
	empty := *new(T)
	defer shut(req.Body)
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return empty, herr.BadRequest("Failed to read request body", err).From("[io.ReadAll]")
	}
	var t T
	if err = json.Unmarshal(bodyBytes, &t); err != nil {
		return empty, herr.BadRequest("Failed to unmarshal request body", err).From("[Unmarshal]")
	}
	return t, nil
}

func mustEventFromFormValue(req *http.Request, imsDB *store.DB) (imsdb.Event, *herr.HTTPError) {
	empty := imsdb.Event{}
	if err := req.ParseForm(); err != nil {
		return empty, herr.BadRequest("Failed to parse form", err).From("ParseForm")
	}
	eventName := req.FormValue("event_id")
	if eventName == "" {
		return empty, herr.BadRequest("No event_id was found in the URL", nil)
	}
	eventRow, err := imsdb.New(imsDB).QueryEventID(req.Context(), eventName)
	if err != nil {
		return empty, herr.New(http.StatusInternalServerError, "Failed to get empty ID", fmt.Errorf("[QueryEventID]: %w", err))
	}
	return eventRow.Event, nil
}

func mustGetEvent(req *http.Request, eventName string, imsDB *store.DB) (imsdb.Event, *herr.HTTPError) {
	var empty imsdb.Event
	if eventName == "" {
		return empty, herr.BadRequest("No eventName was provided", nil)
	}
	eventRow, err := imsdb.New(imsDB).QueryEventID(req.Context(), eventName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return empty, herr.NotFound("Event not found", err)
		}
		return empty, herr.InternalServerError("Failed to fetch Event", err).From("[QueryEventID]")
	}
	return eventRow.Event, nil
}

func mustWriteJSON(w http.ResponseWriter, resp any) (success bool) {
	marshalled, err := json.Marshal(resp)
	if err != nil {
		slog.Error("Failed to marshal JSON", "error", err)
		http.Error(w, "Failed to marshal JSON", http.StatusInternalServerError)
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(marshalled)
	if err != nil {
		slog.Error("Failed to write JSON", "error", err)
		http.Error(w, "Failed to write JSON", http.StatusInternalServerError)
		return false
	}
	return true
}

func mustGetJwtCtx(req *http.Request) (JWTContext, *herr.HTTPError) {
	jwtCtx, found := req.Context().Value(JWTContextKey).(JWTContext)
	if !found {
		return JWTContext{}, herr.InternalServerError(
			"This endpoint has been misconfigured. Please report this to the tech team",
			errors.New("the OptionalAuthN adapter must be called before RequireAuthN"),
		)
	}
	return jwtCtx, nil
}

func mustGetEventPermissions(req *http.Request, imsDB *store.DB, imsAdmins []string) (
	imsdb.Event, JWTContext, authz.EventPermissionMask, *herr.HTTPError,
) {
	event, errHTTP := mustGetEvent(req, req.PathValue("eventName"), imsDB)
	if errHTTP != nil {
		return imsdb.Event{}, JWTContext{}, 0, errHTTP.From("[mustGetEvent]")
	}
	jwtCtx, errHTTP := mustGetJwtCtx(req)
	if errHTTP != nil {
		return imsdb.Event{}, JWTContext{}, 0, errHTTP.From("[mustGetJwtCtx]")
	}
	eventPermissions, _, err := authz.EventPermissions(req.Context(), &event.ID, imsDB, imsAdmins, *jwtCtx.Claims)
	if err != nil {
		return imsdb.Event{}, JWTContext{}, 0, herr.InternalServerError("Failed to compute permissions", err).From("[EventPermissions]")
	}
	return event, jwtCtx, eventPermissions[event.ID], nil
}

func mustGetGlobalPermissions(req *http.Request, imsDB *store.DB, imsAdmins []string) (
	JWTContext, authz.GlobalPermissionMask, *herr.HTTPError,
) {
	empty := JWTContext{}
	jwtCtx, errHTTP := mustGetJwtCtx(req)
	if errHTTP != nil {
		return empty, 0, errHTTP.From("[mustGetJwtCtx]")
	}
	_, globalPermissions, err := authz.EventPermissions(req.Context(), nil, imsDB, imsAdmins, *jwtCtx.Claims)
	if err != nil {
		return empty, 0, herr.InternalServerError("Failed to compute permissions", err).From("[EventPermissions]")
	}
	return jwtCtx, globalPermissions, nil
}

func rollback(txn *sql.Tx) {
	err := txn.Rollback()
	if err != nil && !errors.Is(err, sql.ErrTxDone) {
		slog.Error("Failed to rollback transaction", "error", err)
	}
}

func shut(c io.Closer) {
	if err := c.Close(); err != nil {
		slog.Error("Failed to close Closer", "error", err)
	}
}
