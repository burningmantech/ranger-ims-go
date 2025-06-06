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
	"github.com/burningmantech/ranger-ims-go/lib/attachment"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/store"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

func AddToMux(
	mux *http.ServeMux,
	es *EventSourcerer,
	cfg *conf.IMSConfig,
	db *store.DBQ,
	userStore *directory.UserStore,
	s3Client *attachment.S3Client,
) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}

	jwter := authz.JWTer{SecretKey: cfg.Core.JWTSecret}
	attachmentsEnabled := cfg.AttachmentsStore.Type != conf.AttachmentsStoreNone

	mux.Handle("GET /ims/api/access",
		Adapt(
			GetEventAccesses{db, userStore, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("POST /ims/api/access",
		Adapt(
			PostEventAccess{db, userStore, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
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
			RecoverFromPanic(),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
			// This endpoint does not require authentication, nor
			// does it even consider the request's Authorization header,
			// because the point of this is to make a new JWT.
		),
	)

	mux.Handle("GET /ims/api/auth",
		Adapt(
			GetAuth{
				db,
				userStore,
				cfg.Core.JWTSecret,
				cfg.Core.Admins,
				attachmentsEnabled,
			},
			RecoverFromPanic(),
			// This endpoint does not require authentication or authorization, by design
			OptionalAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
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
			RecoverFromPanic(),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
			// This endpoint does not require authentication, nor
			// does it even consider the request's Authorization header,
			// because the point of this is to make a new access token.
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/incidents",
		Adapt(
			GetIncidents{db, userStore, cfg.Core.Admins, attachmentsEnabled},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/incidents",
		Adapt(
			NewIncident{db, userStore, es, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/incidents/{incidentNumber}",
		Adapt(
			GetIncident{db, userStore, cfg.Core.Admins, attachmentsEnabled},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/incidents/{incidentNumber}",
		Adapt(
			EditIncident{db, userStore, es, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/incidents/{incidentNumber}/attachments/{attachmentNumber}",
		Adapt(
			GetIncidentAttachment{db, userStore, cfg.AttachmentsStore, s3Client, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/incidents/{incidentNumber}/attachments",
		Adapt(
			AttachToIncident{db, userStore, es, cfg.AttachmentsStore, s3Client, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/incidents/{incidentNumber}/report_entries/{reportEntryId}",
		Adapt(
			EditIncidentReportEntry{db, userStore, es, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/field_reports",
		Adapt(
			GetFieldReports{db, userStore, cfg.Core.Admins, attachmentsEnabled},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/field_reports",
		Adapt(
			NewFieldReport{db, userStore, es, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/field_reports/{fieldReportNumber}",
		Adapt(
			GetFieldReport{db, userStore, cfg.Core.Admins, attachmentsEnabled},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/field_reports/{fieldReportNumber}",
		Adapt(
			EditFieldReport{db, userStore, es, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("GET /ims/api/events/{eventName}/field_reports/{fieldReportNumber}/attachments/{attachmentNumber}",
		Adapt(
			GetFieldReportAttachment{db, userStore, cfg.AttachmentsStore, s3Client, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/field_reports/{fieldReportNumber}/attachments",
		Adapt(
			AttachToFieldReport{db, userStore, es, cfg.AttachmentsStore, s3Client, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("POST /ims/api/events/{eventName}/field_reports/{fieldReportNumber}/report_entries/{reportEntryId}",
		Adapt(
			EditFieldReportReportEntry{db, userStore, es, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("GET /ims/api/events",
		Adapt(
			GetEvents{db, userStore, cfg.Core.Admins, cfg.Core.CacheControlShort},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("POST /ims/api/events",
		Adapt(
			EditEvents{db, userStore, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("GET /ims/api/streets",
		Adapt(
			GetStreets{db, userStore, cfg.Core.Admins, cfg.Core.CacheControlShort},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("POST /ims/api/streets",
		Adapt(
			EditStreets{db, userStore, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("GET /ims/api/incident_types",
		Adapt(
			GetIncidentTypes{db, userStore, cfg.Core.Admins, cfg.Core.CacheControlShort},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("POST /ims/api/incident_types",
		Adapt(
			EditIncidentTypes{db, userStore, cfg.Core.Admins},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("GET /ims/api/personnel",
		Adapt(
			GetPersonnel{db, userStore, cfg.Core.Admins, cfg.Core.CacheControlShort},
			RecoverFromPanic(),
			RequireAuthN(jwter),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	mux.Handle("GET /ims/api/eventsource",
		Adapt(
			es.Server.Handler(EventSourceChannel),
			RecoverFromPanic(),
			LogRequest(),
			LimitRequestBytes(cfg.Core.MaxRequestBytes),
		),
	)

	return AddBasicHandlers(mux)
}

func AddBasicHandlers(mux *http.ServeMux) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}

	mux.HandleFunc("GET /{$}",
		func(w http.ResponseWriter, req *http.Request) {
			http.Error(w, "IMS", http.StatusOK)
		},
	)

	mux.HandleFunc("GET /ims/api/ping",
		func(w http.ResponseWriter, req *http.Request) {
			http.Error(w, "ack", http.StatusOK)
		},
	)

	mux.HandleFunc("GET /ims/api/debug/buildinfo",
		func(w http.ResponseWriter, req *http.Request) {
			bi := buildInfo()
			http.Error(w, bi.String(), http.StatusOK)
		},
	)

	return mux
}

var buildInfo = sync.OnceValue[debug.BuildInfo](func() debug.BuildInfo {
	bi, ok := debug.ReadBuildInfo()
	if ok {
		return *bi
	}
	// The conditions for this to happen aren't really possible, but returning an
	// empty struct instead is a good alternative. These values are just used for
	// informational purposes in the server anyway.
	slog.Info("Build info was unavailable, so an empty placeholder will be used instead")
	return debug.BuildInfo{}
})

type Adapter func(http.Handler) http.Handler

// responseWriter is a wrapper around http.ResponseWriter that lets us
// capture details about the response.
type responseWriter struct {
	http.ResponseWriter
	http.Flusher
	code int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.code = code
	rw.ResponseWriter.WriteHeader(code)
}

func LimitRequestBytes(maxRequestBytes int64) Adapter {
	return func(next http.Handler) http.Handler {
		return http.MaxBytesHandler(next, maxRequestBytes)
	}
}

func LogRequest() Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			writ := &responseWriter{w, w.(http.Flusher), http.StatusOK}

			next.ServeHTTP(writ, r)

			username := "(unauthenticated)"
			jwtCtx, _ := r.Context().Value(JWTContextKey).(JWTContext)
			if jwtCtx.Claims != nil {
				username = jwtCtx.Claims.RangerHandle()
			}

			durationMS := float64(time.Since(start).Microseconds()) / 1000.0

			// TODO(https://github.com/burningmantech/ranger-ims-go/issues/35)
			// Finalize the set of columns to collect, then make this a DB insert rather than
			// a logging statement.
			slog.Debug("Tentative access log table entry",
				"start-time", start,
				"method", r.Method,
				"path", r.URL.Path,
				"user", username,
				"http-status", writ.code,
				"duration-micros", time.Since(start).Microseconds(),
				// TODO: decide whether to bother including this. Wow is it verbose.
				// "headers", r.Header.Get("User-Agent"),
				"remote-addr", r.RemoteAddr,
				// TODO: maybe include? Maybe not
				// "x-forwarded-for", r.Header.Get("X-Forwarded-For"),
				"build", buildInfo().Main.Version,
			)

			slog.Debug(fmt.Sprintf("Served request for: %v %v ", r.Method, r.URL.Path),
				"duration", fmt.Sprintf("%.3fms", durationMS),
				"method", r.Method,
				"user", username,
				"code", writ.code,
			)
		})
	}
}

func RecoverFromPanic() Adapter {
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
				handleErr(w, r, http.StatusUnauthorized, "Invalid Authorization token", err)
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
