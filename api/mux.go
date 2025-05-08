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
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/store"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

func AddToMux(mux *http.ServeMux, es *EventSourcerer, cfg *conf.IMSConfig, db *store.DB, userStore *directory.UserStore) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}

	jwter := authz.JWTer{SecretKey: cfg.Core.JWTSecret}
	attachmentsEnabled := cfg.AttachmentsStore.Type != conf.AttachmentsStoreNone

	mux.Handle("GET /ims/api/access",
		Adapt(
			GetEventAccesses{db, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/access",
		Adapt(
			PostEventAccess{db, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/auth",
		Adapt(
			PostAuth{
				db,
				userStore,
				cfg.Core.JWTSecret,
				cfg.Core.AccessTokenLifetime,
				cfg.Core.RefreshTokenLifetime,
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
				db,
				cfg.Core.JWTSecret,
				cfg.Core.Admins,
				attachmentsEnabled,
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
				db,
				userStore,
				cfg.Core.JWTSecret,
				cfg.Core.AccessTokenLifetime,
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
			GetIncidents{db, cfg.Core.Admins, attachmentsEnabled},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/incidents",
		Adapt(
			NewIncident{db, es, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/incidents/{incidentNumber}",
		Adapt(
			GetIncident{db, cfg.Core.Admins, attachmentsEnabled},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/incidents/{incidentNumber}",
		Adapt(
			EditIncident{db, es, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/incidents/{incidentNumber}/attachments/{attachmentNumber}",
		Adapt(
			GetIncidentAttachment{db, cfg.AttachmentsStore, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/incidents/{incidentNumber}/attachments",
		Adapt(
			AttachToIncident{db, es, cfg.AttachmentsStore, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/incidents/{incidentNumber}/report_entries/{reportEntryId}",
		Adapt(
			EditIncidentReportEntry{db, es, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/field_reports",
		Adapt(
			GetFieldReports{db, cfg.Core.Admins, attachmentsEnabled},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/field_reports",
		Adapt(
			NewFieldReport{db, es, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/field_reports/{fieldReportNumber}",
		Adapt(
			GetFieldReport{db, cfg.Core.Admins, attachmentsEnabled},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/field_reports/{fieldReportNumber}",
		Adapt(
			EditFieldReport{db, es, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/field_reports/{fieldReportNumber}/attachments/{attachmentNumber}",
		Adapt(
			GetFieldReportAttachment{db, cfg.AttachmentsStore, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/field_reports/{fieldReportNumber}/attachments",
		Adapt(
			AttachToFieldReport{db, es, cfg.AttachmentsStore, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/field_reports/{fieldReportNumber}/report_entries/{reportEntryId}",
		Adapt(
			EditFieldReportReportEntry{db, es, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/events",
		Adapt(
			GetEvents{db, cfg.Core.Admins, cfg.Core.CacheControlShort},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/events",
		Adapt(
			EditEvents{db, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/streets",
		Adapt(
			GetStreets{db, cfg.Core.Admins, cfg.Core.CacheControlShort},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/streets",
		Adapt(
			EditStreets{db, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/incident_types",
		Adapt(
			GetIncidentTypes{db, cfg.Core.Admins, cfg.Core.CacheControlShort},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("POST /ims/api/incident_types",
		Adapt(
			EditIncidentTypes{db, cfg.Core.Admins},
			RecoverOnPanic(),
			RequireAuthN(jwter),
			LogBeforeAfter(),
		),
	)

	mux.Handle("GET /ims/api/personnel",
		Adapt(
			GetPersonnel{db, userStore, cfg.Core.Admins, cfg.Core.CacheControlShort},
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
			// TODO: log to ERROR if it was an error
			slog.Debug("APILog",
				"path", r.URL.Path,
				"duration", fmt.Sprint(time.Since(start).Microseconds(), "µs"),
				"method", r.Method,
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
