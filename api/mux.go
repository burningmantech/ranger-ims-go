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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"time"

	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/attachment"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/actionlog"
	"github.com/burningmantech/ranger-ims-go/store/errorlog"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
)

func AddToMux(
	mux *http.ServeMux,
	es *EventSourcerer,
	cfg *conf.IMSConfig,
	db *store.DBQ,
	userStore *directory.UserStore,
	s3Client *attachment.S3Client,
	actionLogger *actionlog.Logger,
	errorLogger *errorlog.Logger,
) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}

	jwter := authz.JWTer{SecretKey: cfg.Core.JWTSecret}
	attachmentsEnabled := cfg.AttachmentsStore.Type != conf.AttachmentsStoreNone

	// authed registers a route wrapped in the standard middleware stack for an
	// authenticated endpoint: error logging, panic recovery, JWT
	// authentication, action logging, and a request-size limit. logAction
	// controls whether the request is written to the action log. Using this for
	// every authenticated route makes it impossible to silently forget
	// RequireAuthN.
	authed := func(pattern string, handler http.Handler, logAction bool) {
		mux.Handle(pattern, Adapt(
			handler,
			RecordErrors(errorLogger),
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(logAction, actionLogger, userStore),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		))
	}

	// unauthed registers a route that deliberately skips JWT authentication.
	// Zero or more auth adapters (e.g. OptionalAuthN) may still be supplied;
	// pass none for endpoints that ignore the Authorization header entirely.
	unauthed := func(pattern string, handler http.Handler, logAction bool, authN ...Adapter) {
		adapters := append([]Adapter{RecordErrors(errorLogger), RecoverFromPanic()}, authN...)
		adapters = append(adapters,
			LogRequest(logAction, actionLogger, userStore),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		)
		mux.Handle(pattern, Adapt(handler, adapters...))
	}

	authed("GET /ims/api/access", GetEventAccesses{db, userStore, cfg.Core.Admins}, true)
	authed("GET /ims/api/access_targets", GetAccessTargets{db, userStore, cfg.Core.Admins}, true)
	authed("POST /ims/api/access", PostEventAccess{db, userStore, cfg.Core.Admins}, true)
	authed("GET /ims/api/actionlogs", GetActionLogs{db, userStore, cfg.Core.Admins}, true)
	authed("GET /ims/api/errorlogs", GetErrorLogs{db, userStore, cfg.Core.Admins}, true)

	// This endpoint does not require authentication, nor does it even consider
	// the request's Authorization header, because the point of this is to make
	// a new JWT.
	unauthed("POST /ims/api/auth",
		PostAuth{
			db,
			userStore,
			cfg.Core.JWTSecret,
			cfg.Core.AccessTokenLifetime,
			cfg.Core.RefreshTokenLifetime,
		}, true)

	// This endpoint does not require authentication or authorization, by design.
	unauthed("GET /ims/api/auth",
		GetAuth{
			db,
			userStore,
			cfg.Core.JWTSecret,
			cfg.Core.Admins,
			attachmentsEnabled,
			cfg.Core.EventDeletionEnabled,
			cfg.BurningManAPI.Enabled(),
		}, true, OptionalAuthN(jwter))

	// This endpoint does not require authentication, nor does it even consider
	// the request's Authorization header, because the point of this is to make
	// a new access token.
	unauthed("POST /ims/api/auth/refresh",
		RefreshAccessToken{
			db,
			userStore,
			cfg.Core.JWTSecret,
			cfg.Core.AccessTokenLifetime,
		}, false)

	authed("GET /ims/api/events/{eventName}/incidents", GetIncidents{db, userStore, cfg.Core.Admins, attachmentsEnabled}, false)
	authed("POST /ims/api/events/{eventName}/incidents", NewIncident{db, userStore, es, cfg.Core.Admins}, true)
	authed("GET /ims/api/events/{eventName}/incidents/{incidentNumber}", GetIncident{db, userStore, cfg.Core.Admins, attachmentsEnabled}, false)
	authed("POST /ims/api/events/{eventName}/incidents/{incidentNumber}", EditIncident{db, userStore, es, cfg.Core.Admins}, true)
	authed("GET /ims/api/events/{eventName}/incidents/{incidentNumber}/attachments/{attachmentNumber}", GetIncidentAttachment{db, userStore, cfg.AttachmentsStore, s3Client, cfg.Core.Admins}, true)
	authed("POST /ims/api/events/{eventName}/incidents/{incidentNumber}/attachments", AttachToIncident{db, userStore, es, cfg.AttachmentsStore, s3Client, cfg.Core.Admins}, true)
	authed("POST /ims/api/events/{eventName}/incidents/{incidentNumber}/rangers/{rangerName}", AttachRangerToIncident{db, userStore, es, cfg.Core.Admins}, true)
	authed("DELETE /ims/api/events/{eventName}/incidents/{incidentNumber}/rangers/{rangerName}", DetachRangerFromIncident{db, userStore, es, cfg.Core.Admins}, true)
	authed("POST /ims/api/events/{eventName}/incidents/{incidentNumber}/report_entries/{reportEntryId}", EditIncidentReportEntry{db, userStore, es, cfg.Core.Admins}, true)
	authed("POST /ims/api/events/{eventName}/incidents/{incidentNumber}/incident_types/{incidentTypeId}", AttachTypeToIncident{db, userStore, es, cfg.Core.Admins}, true)
	authed("DELETE /ims/api/events/{eventName}/incidents/{incidentNumber}/incident_types/{incidentTypeId}", DetachTypeFromIncident{db, userStore, es, cfg.Core.Admins}, true)
	authed("POST /ims/api/events/{eventName}/incidents/{incidentNumber}/linked_incidents/{linkedEventName}/{linkedIncidentNumber}", LinkIncident{db, userStore, es, cfg.Core.Admins}, true)
	authed("DELETE /ims/api/events/{eventName}/incidents/{incidentNumber}/linked_incidents/{linkedEventName}/{linkedIncidentNumber}", UnlinkIncident{db, userStore, es, cfg.Core.Admins}, true)

	authed("GET /ims/api/events/{eventName}/field_reports", GetFieldReports{db, userStore, cfg.Core.Admins, attachmentsEnabled}, false)
	authed("POST /ims/api/events/{eventName}/field_reports", NewFieldReport{db, userStore, es, cfg.Core.Admins}, true)
	authed("GET /ims/api/events/{eventName}/field_reports/{fieldReportNumber}", GetFieldReport{db, userStore, cfg.Core.Admins, attachmentsEnabled}, false)
	authed("POST /ims/api/events/{eventName}/field_reports/{fieldReportNumber}", EditFieldReport{db, userStore, es, cfg.Core.Admins}, true)
	authed("GET /ims/api/events/{eventName}/field_reports/{fieldReportNumber}/attachments/{attachmentNumber}", GetFieldReportAttachment{db, userStore, cfg.AttachmentsStore, s3Client, cfg.Core.Admins}, true)
	authed("POST /ims/api/events/{eventName}/field_reports/{fieldReportNumber}/attachments", AttachToFieldReport{db, userStore, es, cfg.AttachmentsStore, s3Client, cfg.Core.Admins}, true)
	authed("POST /ims/api/events/{eventName}/field_reports/{fieldReportNumber}/report_entries/{reportEntryId}", EditFieldReportReportEntry{db, userStore, es, cfg.Core.Admins}, true)

	authed("GET /ims/api/events/{eventName}/visits", GetVisits{db, userStore, cfg.Core.Admins, attachmentsEnabled}, false)
	authed("GET /ims/api/events/{eventName}/visits/{visitNumber}", GetVisit{db, userStore, cfg.Core.Admins, attachmentsEnabled}, false)
	authed("POST /ims/api/events/{eventName}/visits", NewVisit{db, userStore, es, cfg.Core.Admins}, false)
	authed("POST /ims/api/events/{eventName}/visits/{visitNumber}", EditVisit{db, userStore, es, cfg.Core.Admins}, false)
	authed("POST /ims/api/events/{eventName}/visits/{visitNumber}/rangers/{rangerName}", AttachRangerToVisit{db, userStore, es, cfg.Core.Admins}, true)
	authed("DELETE /ims/api/events/{eventName}/visits/{visitNumber}/rangers/{rangerName}", DetachRangerFromVisit{db, userStore, es, cfg.Core.Admins}, true)
	authed("GET /ims/api/events/{eventName}/visits/{visitNumber}/attachments/{attachmentNumber}", GetVisitAttachment{db, userStore, cfg.AttachmentsStore, s3Client, cfg.Core.Admins}, true)
	authed("POST /ims/api/events/{eventName}/visits/{visitNumber}/attachments", AttachToVisit{db, userStore, es, cfg.AttachmentsStore, s3Client, cfg.Core.Admins}, true)
	authed("POST /ims/api/events/{eventName}/visits/{visitNumber}/report_entries/{reportEntryId}", EditVisitReportEntry{db, userStore, es, cfg.Core.Admins}, true)

	authed("GET /ims/api/events/{eventName}/places", GetPlaces{db, userStore, cfg.Core.Admins, cfg.Core.CacheControlShort}, true)
	authed("POST /ims/api/events/{eventName}/places", UpdatePlaces{db, userStore, cfg.Core.Admins, cfg.Core.CacheControlShort}, true)
	authed("POST /ims/api/events/{eventName}/places/import", ImportPlaces{db, userStore, cfg.Core.Admins, cfg.BurningManAPI}, true)

	authed("GET /ims/api/events", GetEvents{db, userStore, cfg.Core.Admins, cfg.Core.CacheControlShort}, false)
	authed("POST /ims/api/events", EditEvent{db, userStore, cfg.Core.Admins}, true)
	authed("DELETE /ims/api/events/{eventName}", DeleteEvent{db, userStore, cfg.Core.Admins, cfg.Core.EventDeletionEnabled}, true)

	authed("GET /ims/api/search", GetSearch{db, userStore, cfg.Core.Admins}, false)

	authed("GET /ims/api/incident_types", GetIncidentTypes{db, userStore, cfg.Core.Admins, cfg.Core.CacheControlShort}, false)
	authed("POST /ims/api/incident_types", EditIncidentTypes{db, userStore, cfg.Core.Admins}, true)

	authed("GET /ims/api/personnel", GetPersonnel{db, userStore, cfg.Core.Admins, cfg.Core.CacheControlShort}, false)

	// Admin management of the IMS-native user directory. These endpoints
	// reject all requests unless the deployment uses IMS_DIRECTORY=ims.
	directoryIsIMS := cfg.Directory.Directory == conf.DirectoryTypeIMS
	authed("GET /ims/api/directory", GetDirectory{db, userStore, cfg.Core.Admins, directoryIsIMS}, true)
	authed("POST /ims/api/directory/persons", EditDirectoryPerson{db, userStore, cfg.Core.Admins, directoryIsIMS}, true)
	authed("POST /ims/api/directory/persons/{personId}/password", SetDirectoryPersonPassword{db, userStore, cfg.Core.Admins, directoryIsIMS}, true)
	authed("DELETE /ims/api/directory/persons/{personId}", DeleteDirectoryPerson{db, userStore, cfg.Core.Admins, directoryIsIMS}, true)
	authed("POST /ims/api/directory/teams", EditDirectoryTeam{db, userStore, cfg.Core.Admins, directoryIsIMS}, true)
	authed("DELETE /ims/api/directory/teams/{teamId}", DeleteDirectoryTeam{db, userStore, cfg.Core.Admins, directoryIsIMS}, true)
	authed("POST /ims/api/directory/positions", EditDirectoryPosition{db, userStore, cfg.Core.Admins, directoryIsIMS}, true)
	authed("DELETE /ims/api/directory/positions/{positionId}", DeleteDirectoryPosition{db, userStore, cfg.Core.Admins, directoryIsIMS}, true)

	// The SSE stream only carries notification metadata (event and record
	// numbers), not record contents; clients fetch the actual data through the
	// authenticated endpoints above.
	unauthed("GET /ims/api/eventsource", es.Server.Handler(EventSourceChannel), false)

	authed("GET /ims/api/debug/buildinfo", GetBuildInfo{db, userStore, cfg.Core.Admins}, true)
	authed("GET /ims/api/debug/runtimemetrics", GetRuntimeMetrics{db, userStore, cfg.Core.Admins}, true)
	authed("POST /ims/api/debug/gc", PerformGC{db, userStore, cfg.Core.Admins}, true)

	// Uncomment these to add pprof into the program. Note that we'd probably want
	// these endpoints to be restricted to admins only, were this going to run in
	// production.
	// https://pkg.go.dev/runtime/pprof
	// https://github.com/google/pprof/blob/main/doc/README.md
	// mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	// mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	// mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	// mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	// mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)

	return AddBasicHandlers(mux)
}

func AddBasicHandlers(mux *http.ServeMux) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}

	mux.HandleFunc("GET /{$}",
		func(w http.ResponseWriter, req *http.Request) {
			herr.WriteOKResponse(w, "IMS")
		},
	)

	mux.HandleFunc("GET /ims/api/ping",
		func(w http.ResponseWriter, req *http.Request) {
			herr.WriteOKResponse(w, "ack")
		},
	)

	return mux
}

type Adapter func(http.Handler) http.Handler

// responseWriter is a wrapper around http.ResponseWriter that lets us
// capture details about the response.
type responseWriter struct {
	http.ResponseWriter
	http.Flusher

	code int

	// The first error written through this writer, plus the stack of the panic
	// (if any) that RecoverFromPanic turned into that error. These feed the
	// error log.
	errHTTP    *herr.HTTPError
	panicStack []byte

	// Identity of the requestor, deposited by LogRequest before the request is
	// served, so that it's available even if the handler panics.
	userID       sql.NullInt64
	userName     sql.NullString
	positionID   sql.NullInt64
	positionName sql.NullString
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.code = code
	rw.ResponseWriter.WriteHeader(code)
}

// RecordHTTPError implements herr.ErrorRecorder. A handler writes at most one
// error response, so later errors are the noise of an already-failed request.
func (rw *responseWriter) RecordHTTPError(e *herr.HTTPError) {
	if rw.errHTTP == nil {
		rw.errHTTP = e
	}
}

func (rw *responseWriter) recordPanic(stack []byte) {
	rw.panicStack = stack
}

func (rw *responseWriter) setUser(userID, positionID sql.NullInt64, userName, positionName sql.NullString) {
	rw.userID = userID
	rw.userName = userName
	rw.positionID = positionID
	rw.positionName = positionName
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	flusher, _ := w.(http.Flusher)
	return &responseWriter{ResponseWriter: w, Flusher: flusher, code: http.StatusOK}
}

func LimitRequestBytes(maxRequestBytes int64) Adapter {
	return func(next http.Handler) http.Handler {
		return http.MaxBytesHandler(next, maxRequestBytes)
	}
}

func clientAddress(r *http.Request) string {
	if connectingIP := r.Header.Get("CF-Connecting-IP"); connectingIP != "" {
		return connectingIP
	}
	if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
		return forwardedFor
	}
	addrPort, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return addrPort.Addr().String()
}

// requestReferrer trims the referrer down to the IMS-relative part, which is
// the only bit worth recording.
func requestReferrer(r *http.Request) *string {
	referrerHeader := r.Header.Get("Referer")
	referrerUsefulIndex := strings.Index(referrerHeader, "/ims")
	if referrerUsefulIndex != -1 {
		referrerHeader = referrerHeader[referrerUsefulIndex:]
	}
	return conv.EmptyToNil(referrerHeader)
}

func LogRequest(enable bool, actionLogger *actionlog.Logger, userStore *directory.UserStore) Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			// RecordErrors, when it's in play, already made the wrapper.
			writ, ok := w.(*responseWriter)
			if !ok {
				writ = newResponseWriter(w)
			}

			// The identity is read before the request is served, and stashed on
			// the wrapper, so that the error log can still name the requestor
			// when the handler panics past everything below here.
			var username sql.NullString
			var userID sql.NullInt64
			var positionID sql.NullInt64
			var positionName sql.NullString
			jwtCtx, _ := r.Context().Value(JWTContextKey).(JWTContext)
			if jwtCtx.Claims != nil {
				username = conv.StringToSql(new(jwtCtx.Claims.RangerHandle()), 128)
				userID = sql.NullInt64{Int64: jwtCtx.Claims.DirectoryID(), Valid: true}
				if posID := jwtCtx.Claims.RangerOnDutyPosition(); posID != nil {
					positionID = sql.NullInt64{Int64: *posID, Valid: true}
					positions, _, _ := userStore.GetPositionsAndTeams(r.Context())
					if positions != nil {
						posName := positions[*posID]
						positionName = conv.StringToSql(conv.EmptyToNil(posName), 128)
					}
				}
			}
			writ.setUser(userID, positionID, username, positionName)

			next.ServeHTTP(writ, r)

			if enable {
				referrer := requestReferrer(r)
				remoteAddr := clientAddress(r)
				actionLogger.Log(
					r.Context(),
					imsdb.AddActionLogParams{
						CreatedAt:      conv.TimeToFloat(time.Now()),
						ActionType:     "api",
						Method:         conv.StringToSql(&r.Method, 128),
						Path:           conv.StringToSql(&r.URL.Path, 128),
						Referrer:       conv.StringToSql(referrer, 128),
						UserID:         userID,
						UserName:       username,
						PositionID:     positionID,
						PositionName:   positionName,
						ClientAddress:  conv.StringToSql(&remoteAddr, 128),
						HttpStatus:     sql.NullInt16{Int16: int16(writ.code), Valid: true},
						DurationMicros: sql.NullInt64{Int64: time.Since(start).Microseconds(), Valid: true},
					})
			}

			// #nosec G706 // log injection
			slog.Debug(fmt.Sprintf("Served request for: %v %v ", r.Method, r.URL.Path),
				"duration", fmt.Sprintf("%.3fms", float64(time.Since(start).Microseconds())/1000.0),
				"method", r.Method,
				"user", username.String,
				"code", writ.code,
			)
		})
	}
}

// ErrorLogger writes rows to the error log. *errorlog.Logger implements it;
// the indirection is what lets RecordErrors be tested without a database.
type ErrorLogger interface {
	Log(ctx context.Context, record imsdb.AddErrorLogParams)
}

// RecordErrors writes an error log row for any request that ends in a 5xx
// response or a recovered panic. It has to be the outermost adapter: it owns
// the responseWriter that herr.WriteResponse records onto, and sitting outside
// RecoverFromPanic is what lets it see panics at all.
//
// Unlike the action log, this ignores the per-route logAction flag. Errors from
// the quiet read endpoints are exactly the ones worth seeing.
func RecordErrors(errorLogger ErrorLogger) Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			writ := newResponseWriter(w)

			next.ServeHTTP(writ, r)

			errHTTP := writ.errHTTP
			if errHTTP == nil || errHTTP.Code < http.StatusInternalServerError {
				return
			}
			// A client that hung up mid-request isn't a server fault, and
			// nobody ever saw the response we failed to write. Both conditions
			// are required, so that a genuine bug in our own use of contexts
			// can't hide behind this. Note that context.DeadlineExceeded is
			// deliberately not covered: a blown deadline is the server failing
			// to keep a promise, which is exactly what the error log is for.
			if r.Context().Err() != nil && errors.Is(errHTTP, context.Canceled) {
				return
			}
			var internalError string
			if errHTTP.InternalErr != nil {
				internalError = errHTTP.InternalErr.Error()
			}
			remoteAddr := clientAddress(r)
			errorLogger.Log(
				r.Context(),
				imsdb.AddErrorLogParams{
					CreatedAt:       conv.TimeToFloat(time.Now()),
					HttpStatus:      int16(errHTTP.Code),
					ResponseMessage: conv.StringToSql(conv.EmptyToNil(errHTTP.ResponseMessage), 1024),
					InternalError:   conv.StringToSql(conv.EmptyToNil(internalError), 1024),
					StackTrace:      conv.StringToSql(conv.EmptyToNil(string(writ.panicStack)), 1024),
					Method:          conv.StringToSql(&r.Method, 128),
					Path:            conv.StringToSql(&r.URL.Path, 255),
					Referrer:        conv.StringToSql(requestReferrer(r), 255),
					UserID:          writ.userID,
					UserName:        writ.userName,
					PositionID:      writ.positionID,
					PositionName:    writ.positionName,
					ClientAddress:   conv.StringToSql(&remoteAddr, 128),
					DurationMicros:  sql.NullInt64{Int64: time.Since(start).Microseconds(), Valid: true},
				})
		})
	}
}

func RecoverFromPanic() Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					stack := debug.Stack()
					slog.Error("Recovered from panic", "err", err, "stack", string(stack))
					if writ, ok := w.(*responseWriter); ok {
						writ.recordPanic(stack)
					}
					herr.InternalServerError(
						"The server malfunctioned", fmt.Errorf("panic: %v", err),
					).WriteResponse(w)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type ContextKey string

const JWTContextKey ContextKey = "JWTContext"

type JWTContext struct {
	Claims *authz.IMSClaims
	Error  error
}

func OptionalAuthN(j authz.JWTer) Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			claims, err := j.AuthenticateJWT(strings.TrimPrefix(header, "Bearer "))
			ctx := context.WithValue(r.Context(), JWTContextKey, JWTContext{
				Claims: claims,
				Error:  err,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAuthN(j authz.JWTer) Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			claims, err := j.AuthenticateJWT(strings.TrimPrefix(header, "Bearer "))
			if err != nil || claims == nil {
				herr.Unauthorized("Invalid Authorization token", err).WriteResponse(w)
				return
			}
			jwtCtx := context.WithValue(r.Context(), JWTContextKey, JWTContext{
				Claims: claims,
				Error:  err,
			})
			next.ServeHTTP(w, r.WithContext(jwtCtx))
		})
	}
}

func Adapt(handler http.Handler, adapters ...Adapter) http.Handler {
	for i := range adapters {
		adapter := adapters[len(adapters)-1-i] // range in reverse
		handler = adapter(handler)
	}
	return handler
}
