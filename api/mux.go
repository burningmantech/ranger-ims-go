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
	"github.com/burningmantech/ranger-ims-go/auth"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/store"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

func AddToMux(ctx context.Context, mux *http.ServeMux, cfg *conf.IMSConfig, db *store.DB, userStore *directory.UserStore) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}

	jwter := auth.JWTer{SecretKey: cfg.Core.JWTSecret}
	es := NewEventSourcerer(ctx)

	mux.Handle("GET /ims/api/access",
		Adapt(
			GetEventAccesses{imsDB: db, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/access",
		Adapt(
			PostEventAccess{imsDB: db, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/auth",
		Adapt(
			PostAuth{
				imsDB:                db,
				userStore:            userStore,
				jwtSecret:            cfg.Core.JWTSecret,
				accessTokenDuration:  cfg.Core.AccessTokenLifetime,
				refreshTokenDuration: cfg.Core.RefreshTokenLifetime,
			},
			RecoverOnPanic(),
			LogBeforeAfter(),
			// This endpoint does not require authentication, nor
			// does it even consider the request's Authorization header,
			// because the point of this is to make a new JWT.
		),
	)

	mux.Handle("GET /ims/api/auth",
		Adapt(
			GetAuth{
				imsDB:     db,
				jwtSecret: cfg.Core.JWTSecret,
				admins:    cfg.Core.Admins,
			},
			RecoverOnPanic(),
			// This endpoint does not require authentication or authorization, by design
			OptionalAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/auth/refresh",
		Adapt(
			RefreshAccessToken{
				imsDB:               db,
				userStore:           userStore,
				jwtSecret:           cfg.Core.JWTSecret,
				accessTokenDuration: cfg.Core.AccessTokenLifetime,
			},
			RecoverOnPanic(),
			LogBeforeAfter(),
			// This endpoint does not require authentication, nor
			// does it even consider the request's Authorization header,
			// because the point of this is to make a new access token.
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/incidents",
		Adapt(
			GetIncidents{imsDB: db, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/incidents",
		Adapt(
			NewIncident{imsDB: db, es: es, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/incidents/{incidentNumber}",
		Adapt(
			GetIncident{imsDB: db, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/incidents/{incidentNumber}",
		Adapt(
			EditIncident{imsDB: db, es: es, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/incidents/{incidentNumber}/report_entries/{reportEntryId}",
		Adapt(
			EditIncidentReportEntry{imsDB: db, eventSource: es, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/field_reports",
		Adapt(
			GetFieldReports{imsDB: db, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/field_reports",
		Adapt(
			NewFieldReport{imsDB: db, eventSource: es, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/field_reports/{fieldReportNumber}",
		Adapt(
			GetFieldReport{imsDB: db, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/field_reports/{fieldReportNumber}",
		Adapt(
			EditFieldReport{imsDB: db, eventSource: es, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/field_reports/{fieldReportNumber}/report_entries/{reportEntryId}",
		Adapt(
			EditFieldReportReportEntry{imsDB: db, eventSource: es, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/events",
		Adapt(
			GetEvents{imsDB: db, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events",
		Adapt(
			EditEvents{imsDB: db, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/streets",
		Adapt(
			GetStreets{imsDB: db, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/streets",
		Adapt(
			EditStreets{imsDB: db, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/incident_types",
		Adapt(
			GetIncidentTypes{imsDB: db, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/incident_types",
		Adapt(
			EditIncidentTypes{imsDB: db, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/personnel",
		Adapt(
			GetPersonnel{imsDB: db, userStore: userStore, imsAdmins: cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/eventsource",
		Adapt(
			es.Server.Handler(EventSourceChannel),
			RecoverOnPanic(),
			LogBeforeAfter(),
		),
	)

	mux.HandleFunc("GET /ims/api/ping",
		func(w http.ResponseWriter, req *http.Request) {
			http.Error(w, "ack", http.StatusOK)
		},
	)

	mux.HandleFunc("GET /ims/api/debug/buildinfo",
		func(w http.ResponseWriter, req *http.Request) {
			bi, ok := debug.ReadBuildInfo()
			if !ok {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
			http.Error(w, bi.String(), http.StatusOK)
		},
	)

	return mux
}

type Adapter func(http.Handler) http.Handler

func LogBeforeAfter() Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)

			username := "(unauthenticated)"
			jwtCtx, _ := r.Context().Value(JWTContextKey).(JWTContext)
			if jwtCtx.Claims != nil {
				username = jwtCtx.Claims.RangerHandle()
			}
			slog.Debug("Done serving request",
				"duration", time.Since(start).Round(100*time.Microsecond),
				"method", r.Method,
				"path", r.URL.Path,
				"user", username,
			)
		})
	}
}

func RecoverOnPanic() Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					slog.Error("Recovered from panic", "err", err)
					debug.PrintStack()
					http.Error(w, "The server malfunctioned", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type ContextKey string

const JWTContextKey ContextKey = "JWTContext"

type JWTContext struct {
	Claims *auth.IMSClaims
	Error  error
}

func OptionalAuthN(j auth.JWTer) Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			claims, err := j.AuthenticateJWT(header)
			ctx := context.WithValue(r.Context(), JWTContextKey, JWTContext{
				Claims: claims,
				Error:  err,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAuthN(j auth.JWTer) Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			claims, err := j.AuthenticateJWT(header)
			if err != nil || claims == nil {
				slog.Error("Failed to authenticate JWT", "error", err)
				http.Error(w, "Invalid Authorization token", http.StatusUnauthorized)
				return
			}
			if claims.RangerHandle() == "" {
				slog.Error("No Ranger handle in JWT")
				http.Error(w, "Invalid Authorization token", http.StatusUnauthorized)
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
