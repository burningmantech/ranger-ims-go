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
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/go-sql-driver/mysql"
)

// maxNumberAllocAttempts bounds the retries when creating an Incident, Field
// Report, or Visit whose freshly allocated number was claimed by a concurrent
// creator in the same event first.
const maxNumberAllocAttempts = 3

// maxDeadlockAttempts bounds the retries when InnoDB picks a transaction as its
// deadlock victim and rolls it back.
//
// Three wasn't enough: with several Rangers writing one Incident's roster at
// once, a retry can lose again to the next writer in the queue. Six, with the
// backoff below, held up over repeated runs of the concurrency tests.
const maxDeadlockAttempts = 6

// deadlockRetryBackoff is the base delay before retrying a deadlock victim,
// doubled per attempt and jittered. Retrying immediately puts every victim back
// in contention at the same instant, which is how they collided in the first
// place. The worst case adds up to a few tens of milliseconds before the
// attempts run out, which is cheap next to returning a 500.
const deadlockRetryBackoff = 2 * time.Millisecond

var (
	errNoContentType      = errors.New("request has no Content-Type header")
	errNonJSONContentType = errors.New("request Content-Type is not application/json")
)

// isDuplicateKeyError reports whether err is a MySQL/MariaDB duplicate-key
// error (ER_DUP_ENTRY), i.e. an INSERT lost a race for a unique key.
func isDuplicateKeyError(err error) bool {
	const mySQLErDupEntry = 1062
	mysqlErr, ok := errors.AsType[*mysql.MySQLError](err)
	return ok && mysqlErr.Number == mySQLErDupEntry
}

// isDeadlockError reports whether err is a MySQL/MariaDB deadlock error
// (ER_LOCK_DEADLOCK), meaning InnoDB chose this transaction as the victim and
// rolled it back. Nothing it did was applied, so the whole transaction can be
// run again; MariaDB's own error text says as much.
func isDeadlockError(err error) bool {
	const mySQLErLockDeadlock = 1213
	mysqlErr, ok := errors.AsType[*mysql.MySQLError](err)
	return ok && mysqlErr.Number == mySQLErLockDeadlock
}

// retryOnDeadlock runs body, running it again if InnoDB rolled its transaction
// back as a deadlock victim.
//
// Ordering the locks a transaction takes reduces deadlocks but can't remove
// them: foreign keys make writers take shared locks on parent rows they never
// name, and InnoDB also locks index gaps, so two transactions can still end up
// each holding what the other needs. The database resolves that by killing one
// of them, and the survivor's work is unaffected. Retrying the victim is what
// keeps that from reaching a Ranger as a 500.
//
// body must do all of its work inside one transaction and must not publish
// anything outside the database (an SSE notification, say) until that
// transaction has committed, or a retry would emit it twice.
func retryOnDeadlock[T any](body func() (T, *herr.HTTPError)) (T, *herr.HTTPError) {
	backoff := deadlockRetryBackoff
	for attempt := 1; ; attempt++ {
		result, errHTTP := body()
		if errHTTP == nil || !isDeadlockError(errHTTP.InternalErr) {
			return result, errHTTP
		}
		if attempt == maxDeadlockAttempts {
			return result, errHTTP.From(fmt.Sprintf("[retryOnDeadlock] gave up after %v attempts", attempt))
		}
		time.Sleep(rand.Jitter(backoff))
		backoff *= 2
	}
}

// retryOnDeadlockErr is retryOnDeadlock for a transaction that returns only an
// error. The same rule applies: nothing may escape the database until the
// commit.
func retryOnDeadlockErr(body func() *herr.HTTPError) *herr.HTTPError {
	_, errHTTP := retryOnDeadlock(func() (struct{}, *herr.HTTPError) {
		return struct{}{}, body()
	})
	return errHTTP
}

// applyStringChange overwrites dst if the client provided a new value, and
// records a "Changed <label>: <value>" line in logs.
func applyStringChange(dst *sql.NullString, newVal *string, label string, logs *[]string) {
	if newVal == nil {
		return
	}
	*dst = conv.StringToSql(newVal, 0)
	*logs = append(*logs, fmt.Sprintf("Changed %v: %v", label, dst.String))
}

func readBodyAs[T any](req *http.Request) (T, *herr.HTTPError) {
	empty := *new(T)
	defer shut(req.Body)
	errHTTP := requireJSONContentType(req)
	if errHTTP != nil {
		return empty, errHTTP.From("[requireJSONContentType]")
	}
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

// requireJSONContentType rejects a request whose body isn't declared as JSON.
//
// This is what keeps a malicious page from forging a cross-site request to IMS
// on a signed-in Ranger's behalf. Without JavaScript (which the same-origin
// policy already blocks, since IMS sends no CORS headers), another origin can
// only send us a body via an HTML form, and a form can only be text/plain,
// application/x-www-form-urlencoded, or multipart/form-data. Requiring
// application/json means a cross-site body-carrying request must first pass a
// CORS preflight, which IMS never approves. Notably, this closes login CSRF on
// the unauthenticated POST /ims/api/auth, where an attacker could otherwise
// silently swap a Ranger's session for one of the attacker's own.
func requireJSONContentType(req *http.Request) *herr.HTTPError {
	header := req.Header.Get("Content-Type")
	if header == "" {
		return herr.UnsupportedMediaType(
			"Request Content-Type must be application/json",
			errNoContentType,
		)
	}
	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil {
		return herr.UnsupportedMediaType(
			"Request Content-Type must be application/json",
			fmt.Errorf("%w: %w", errNonJSONContentType, err),
		).From("[ParseMediaType]")
	}
	if !strings.EqualFold(mediaType, "application/json") {
		return herr.UnsupportedMediaType(
			"Request Content-Type must be application/json",
			fmt.Errorf("%w: got %v", errNonJSONContentType, mediaType),
		)
	}
	return nil
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
	// #nosec G705 // XSS via taint analysis
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
	// This query doesn't know about parent groups. We'll start by accumulating EventAccesses directly referencing
	// events, then worry about parent groups below.
	accessRows, err := imsDBQ.EventAccessAll(ctx, imsDBQ)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch event access", err).From("[EventAccessAll]")
	}
	accessRowByEventID := make(map[int32][]imsdb.EventAccess)
	for _, ar := range accessRows {
		accessRowByEventID[ar.EventAccess.Event] = append(accessRowByEventID[ar.EventAccess.Event], ar.EventAccess)
	}

	// Now add in parent group EventAccesses.
	events, err := imsDBQ.Events(ctx, imsDBQ)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch Events", err).From("[Events]")
	}
	for _, e := range events {
		child := e.Event
		// No parent, nothing to do
		if !child.ParentGroup.Valid {
			continue
		}
		// Has a parent. Add in all the EventAccesses from the parent.
		for _, ar := range accessRowByEventID[child.ParentGroup.Int32] {
			accessRowByEventID[child.ID] = append(accessRowByEventID[child.ID], ar)
		}
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
