package api

import (
	"cmp"
	"context"
	"fmt"
	"github.com/srabraham/ranger-ims-go/auth"
	imsjson "github.com/srabraham/ranger-ims-go/json"
	"github.com/srabraham/ranger-ims-go/store"
	"github.com/srabraham/ranger-ims-go/store/imsdb"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
)

type GetEvents struct {
	imsDB     *store.DB
	imsAdmins []string
}

func (action GetEvents) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var resp imsjson.Events
	jwt, globalPermissions, ok := mustGetGlobalPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	// This is the first level of authorization. Per-event filtering is done farther down.
	if globalPermissions&auth.GlobalListEvents == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have GlobalListEvents permission", nil)
		return
	}

	allEvents, err := imsdb.New(action.imsDB).Events(req.Context())
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to get events", err)
		return
	}
	permissionsByEvent, err := action.permissionsByEvent(req.Context(), jwt)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to get permissions", err)
		return
	}

	var authorizedEvents []imsdb.EventsRow
	for _, eve := range allEvents {
		if permissionsByEvent[eve.Event.ID]&auth.EventReadEventName != 0 {
			authorizedEvents = append(authorizedEvents, eve)
		}
	}
	resp = make(imsjson.Events, 0, len(authorizedEvents))
	for _, eve := range authorizedEvents {
		resp = append(resp, imsjson.Event{
			ID:   eve.Event.ID,
			Name: eve.Event.Name,
		})
	}

	slices.SortFunc(resp, func(a, b imsjson.Event) int {
		return cmp.Compare(a.ID, b.ID)
	})

	w.Header().Set("Cache-Control", "max-age=1200, private")
	mustWriteJSON(w, resp)
}

func (action GetEvents) permissionsByEvent(ctx context.Context, jwtCtx JWTContext) (map[int32]auth.EventPermissionMask, error) {
	accessRows, err := imsdb.New(action.imsDB).EventAccessAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("[EventAccessAll]: %w", err)
	}
	accessRowByEventID := make(map[int32][]imsdb.EventAccess)
	for _, ar := range accessRows {
		accessRowByEventID[ar.EventAccess.Event] = append(accessRowByEventID[ar.EventAccess.Event], ar.EventAccess)
	}

	permissionsByEvent, _ := auth.ManyEventPermissions(
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
	_, globalPermissions, ok := mustGetGlobalPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if globalPermissions&auth.GlobalAdministrateEvents == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have GlobalAdministrateEvents permission", nil)
		return
	}
	if ok = mustParseForm(w, req); !ok {
		return
	}
	editRequest, ok := mustReadBodyAs[imsjson.EditEventsRequest](w, req)
	if !ok {
		return
	}
	for _, eventName := range editRequest.Add {
		if !allowedEventNames.MatchString(eventName) {
			handleErr(w, req, http.StatusBadRequest, "Event names must match the pattern "+allowedEventNames.String(), fmt.Errorf("invalid event name: '%s'", eventName))
			return
		}
		id, err := imsdb.New(action.imsDB).CreateEvent(req.Context(), eventName)
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to create event", err)
			return
		}
		slog.Info("Created event", "eventName", eventName, "id", id)
	}
	http.Error(w, "Success", http.StatusNoContent)
}
