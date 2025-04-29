package api

import (
	"context"
	"errors"
	"fmt"
	"github.com/srabraham/ranger-ims-go/auth"
	imsjson "github.com/srabraham/ranger-ims-go/json"
	"github.com/srabraham/ranger-ims-go/store"
	"github.com/srabraham/ranger-ims-go/store/imsdb"
	"net/http"
	"sync"
)

type GetEventAccesses struct {
	imsDB     *store.DB
	imsAdmins []string
}

func (action GetEventAccesses) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp := imsjson.EventsAccess{}
	_, globalPermissions, ok := mustGetGlobalPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if globalPermissions&auth.GlobalAdministrateEvents == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have GlobalAdministrateEvents permission", nil)
		return
	}

	resp, err := getEventsAccess(req.Context(), action.imsDB)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to get events access", err)
		return
	}
	mustWriteJSON(w, resp)
}

func getEventsAccess(ctx context.Context, imsDB *store.DB) (imsjson.EventsAccess, error) {
	allEventRows, err := imsdb.New(imsDB).Events(ctx)
	if err != nil {
		return nil, fmt.Errorf("[Events]: %w", err)
	}
	var storedEvents []imsdb.Event
	for _, aer := range allEventRows {
		storedEvents = append(storedEvents, aer.Event)
	}

	accessRows, err := imsdb.New(imsDB).EventAccessAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("[EventAccessAll]: %w", err)
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
	_, globalPermissions, ok := mustGetGlobalPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if globalPermissions&auth.GlobalAdministrateEvents == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have GlobalAdministrate permission", nil)
		return
	}
	ctx := req.Context()
	eventsAccess, ok := mustReadBodyAs[imsjson.EventsAccess](w, req)
	if !ok {
		return
	}
	var errs []error
	for eventName, access := range eventsAccess {
		event, success := mustGetEvent(w, req, eventName, action.imsDB)
		if !success {
			return
		}
		errs = append(errs, action.maybeSetAccess(ctx, event, access.Readers, imsdb.EventAccessModeRead))
		errs = append(errs, action.maybeSetAccess(ctx, event, access.Writers, imsdb.EventAccessModeWrite))
		errs = append(errs, action.maybeSetAccess(ctx, event, access.Reporters, imsdb.EventAccessModeReport))
	}
	if err := errors.Join(errs...); err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to set event access", err)
		return
	}
	http.Error(w, "Successfully set event access", http.StatusNoContent)
}

func (action PostEventAccess) maybeSetAccess(ctx context.Context, event imsdb.Event, rules []imsjson.AccessRule, mode imsdb.EventAccessMode) error {
	if rules == nil {
		return nil
	}

	eventAccessWriteMu.Lock()
	defer eventAccessWriteMu.Unlock()

	txn, err := action.imsDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("[BeginTx]: %w", err)
	}
	defer txn.Rollback()
	err = imsdb.New(txn).ClearEventAccessForMode(ctx, imsdb.ClearEventAccessForModeParams{
		Event: event.ID,
		Mode:  mode,
	})
	if err != nil {
		return fmt.Errorf("[ClearEventAccessForMode]: %w", err)
	}
	for _, rule := range rules {
		err = imsdb.New(txn).ClearEventAccessForExpression(ctx, imsdb.ClearEventAccessForExpressionParams{
			Event:      event.ID,
			Expression: rule.Expression,
		})
		if err != nil {
			return fmt.Errorf("[ClearEventAccessForExpression]: %w", err)
		}
		_, err = imsdb.New(txn).AddEventAccess(ctx, imsdb.AddEventAccessParams{
			Event:      event.ID,
			Expression: rule.Expression,
			Mode:       mode,
			Validity:   imsdb.EventAccessValidity(rule.Validity),
		})
		if err != nil {
			return fmt.Errorf("[AddEventAccess]: %w", err)
		}
	}
	if err = txn.Commit(); err != nil {
		return fmt.Errorf("[Commit]: %w", err)
	}
	return nil
}
