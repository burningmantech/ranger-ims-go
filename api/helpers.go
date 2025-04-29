package api

import (
	"database/sql"
	"encoding/json"
	"github.com/srabraham/ranger-ims-go/auth"
	"github.com/srabraham/ranger-ims-go/store"
	"github.com/srabraham/ranger-ims-go/store/imsdb"
	"io"
	"log/slog"
	"net/http"
	"strconv"
)

func mustParseForm(w http.ResponseWriter, req *http.Request) (success bool) {
	if err := req.ParseForm(); err != nil {
		slog.Error("Failed to parse form", "error", err, "path", req.URL.Path)
		http.Error(w, "Failed to parse HTTP form", http.StatusBadRequest)
		return false
	}
	return true
}

func mustReadBodyAs[T any](w http.ResponseWriter, req *http.Request) (t T, success bool) {
	defer req.Body.Close()
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		slog.Error("Failed to read request body", "error", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return t, false
	}
	if err = json.Unmarshal(bodyBytes, &t); err != nil {
		slog.Error("Failed to unmarshal request body", "error", err)
		http.Error(w, "Failed to unmarshal request body", http.StatusBadRequest)
		return t, false
	}
	return t, true
}

func mustEventFromFormValue(w http.ResponseWriter, req *http.Request, imsDB *store.DB) (event imsdb.Event, success bool) {
	if ok := mustParseForm(w, req); !ok {
		return imsdb.Event{}, false
	}
	eventName := req.FormValue("event_id")
	if eventName == "" {
		slog.Error("No event_id was found in the URL path", "path", req.URL.Path)
		http.Error(w, "No event_id was found in the URL", http.StatusBadRequest)
		return imsdb.Event{}, false
	}
	eventRow, err := imsdb.New(imsDB).QueryEventID(req.Context(), eventName)
	if err != nil {
		slog.Error("Failed to get event ID", "error", err)
		http.Error(w, "Failed to get event ID", http.StatusInternalServerError)
		return imsdb.Event{}, false
	}
	return eventRow.Event, true
}

func mustGetEvent(w http.ResponseWriter, req *http.Request, eventName string, imsDB *store.DB) (event imsdb.Event, success bool) {
	if eventName == "" {
		slog.Error("No eventName was provided")
		http.Error(w, "No eventName was provided", http.StatusInternalServerError)
		return imsdb.Event{}, false
	}

	eventRow, err := imsdb.New(imsDB).QueryEventID(req.Context(), eventName)
	if err != nil {
		slog.Error("Failed to fetch event", "error", err)
		http.Error(w, "Event not found", http.StatusNotFound)
		return imsdb.Event{}, false
	}
	return eventRow.Event, true
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

func mustGetJwtCtx(w http.ResponseWriter, req *http.Request) (JWTContext, bool) {
	jwtCtx, found := req.Context().Value(JWTContextKey).(JWTContext)
	if !found {
		slog.Error("the OptionalAuthN adapter must be called before RequireAuthN")
		http.Error(w, "This endpoint has been misconfigured. Please report this to the tech team",
			http.StatusInternalServerError)
		return JWTContext{}, false
	}
	return jwtCtx, true
}

func mustGetEventPermissions(w http.ResponseWriter, req *http.Request, imsDB *store.DB, imsAdmins []string) (imsdb.Event, JWTContext, auth.EventPermissionMask, bool) {
	event, ok := mustGetEvent(w, req, req.PathValue("eventName"), imsDB)
	if !ok {
		return imsdb.Event{}, JWTContext{}, 0, false
	}
	jwtCtx, ok := mustGetJwtCtx(w, req)
	if !ok {
		return imsdb.Event{}, JWTContext{}, 0, false
	}
	eventPermissions, _, err := auth.EventPermissions(req.Context(), &event.ID, imsDB, imsAdmins, *jwtCtx.Claims)
	if err != nil {
		slog.Error("Failed to compute permissions", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return imsdb.Event{}, JWTContext{}, 0, false
	}
	return event, jwtCtx, eventPermissions[event.ID], true
}

func mustGetGlobalPermissions(w http.ResponseWriter, req *http.Request, imsDB *store.DB, imsAdmins []string) (JWTContext, auth.GlobalPermissionMask, bool) {
	jwtCtx, ok := mustGetJwtCtx(w, req)
	if !ok {
		return JWTContext{}, 0, false
	}
	_, globalPermissions, err := auth.EventPermissions(req.Context(), nil, imsDB, imsAdmins, *jwtCtx.Claims)
	if err != nil {
		slog.Error("Failed to compute permissions", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return JWTContext{}, 0, false
	}
	return jwtCtx, globalPermissions, true
}

func handleErr(w http.ResponseWriter, req *http.Request, statusCode int, errorForUser string, internalError error) {
	slog.Error(errorForUser, "error", internalError, "statusCode", statusCode, "path", req.URL.Path)
	http.Error(w, errorForUser, statusCode)
}

func formatInt16(i sql.NullInt16) *string {
	if i.Valid {
		result := strconv.FormatInt(int64(i.Int16), 10)
		return &result
	}
	return nil
}

func parseInt16(s *string) sql.NullInt16 {
	if s == nil {
		return sql.NullInt16{}
	}
	parsed, err := strconv.ParseInt(*s, 10, 16)
	if err != nil {
		return sql.NullInt16{}
	}
	return sql.NullInt16{
		Int16: int16(parsed),
		Valid: true,
	}
}

func isInt32(s string) bool {
	_, err := strconv.ParseInt(s, 10, 32)
	return err == nil
}

func toInt32(s string) int32 {
	i, _ := strconv.ParseInt(s, 10, 32)
	return int32(i)
}

func stringOrNil(v sql.NullString) *string {
	if v.Valid {
		return &v.String
	}
	return nil
}

func ptr[T any](t T) *T {
	return &t
}
