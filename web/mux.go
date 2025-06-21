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

package web

import (
	"fmt"
	"github.com/a-h/templ"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/web/template"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

func AddToMux(mux *http.ServeMux, cfg *conf.IMSConfig) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}

	deployment := string(cfg.Core.Deployment)
	mux.Handle("GET /ims/static/ext/",
		Adapt(
			http.StripPrefix("/ims/", http.FileServerFS(StaticFS)).ServeHTTP,
			CacheControl(cfg.Core.CacheControlLong),
		),
	)
	mux.Handle("GET /ims/static/logos/",
		Adapt(
			http.StripPrefix("/ims/", http.FileServerFS(StaticFS)).ServeHTTP,
			CacheControl(cfg.Core.CacheControlLong),
		),
	)
	mux.Handle("GET /ims/static/",
		Adapt(
			http.StripPrefix("/ims/", http.FileServerFS(StaticFS)).ServeHTTP,
			// Cache IMS's internal JS and CSS for a shorter duration than external JS/CSS
			// and logos, since we want updates to these files to get sent out to users
			// somewhat soon after deployment to production. If we don't do some custom
			// overriding here, then Cloudflare sets a 4-hour Cache-Control header.
			CacheControl(cfg.Core.CacheControlShort),
			CdnCacheControlOff(),
		),
	)
	mux.Handle("GET /ims/app",
		AdaptTempl(template.Root(deployment), cfg.Core.CacheControlLong),
	)
	mux.Handle("GET /ims/app/admin",
		AdaptTempl(template.AdminRoot(deployment), cfg.Core.CacheControlLong),
	)
	mux.Handle("GET /ims/app/admin/events",
		AdaptTempl(template.AdminEvents(deployment), cfg.Core.CacheControlLong),
	)
	mux.Handle("GET /ims/app/admin/streets",
		AdaptTempl(template.AdminStreets(deployment), cfg.Core.CacheControlLong),
	)
	mux.Handle("GET /ims/app/admin/types",
		AdaptTempl(template.AdminTypes(deployment), cfg.Core.CacheControlLong),
	)
	mux.Handle("GET /ims/app/admin/debug",
		AdaptTempl(template.AdminDebug(deployment), cfg.Core.CacheControlLong),
	)
	mux.HandleFunc("GET /ims/app/events/{eventName}/field_reports",
		func(w http.ResponseWriter, r *http.Request) {
			AdaptTempl(
				template.FieldReports(deployment, r.PathValue("eventName")),
				cfg.Core.CacheControlLong,
			).ServeHTTP(w, r)
		},
	)
	mux.HandleFunc("GET /ims/app/events/{eventName}/field_reports/{fieldReportNumber}",
		func(w http.ResponseWriter, r *http.Request) {
			AdaptTempl(
				template.FieldReport(deployment, r.PathValue("eventName")),
				cfg.Core.CacheControlLong,
			).ServeHTTP(w, r)
		},
	)
	mux.HandleFunc("GET /ims/app/events/{eventName}/incidents",
		func(w http.ResponseWriter, r *http.Request) {
			AdaptTempl(
				template.Incidents(deployment, r.PathValue("eventName")),
				cfg.Core.CacheControlLong,
			).ServeHTTP(w, r)
		},
	)
	mux.HandleFunc("GET /ims/app/events/{eventName}/incidents/{incidentNumber}",
		func(w http.ResponseWriter, r *http.Request) {
			AdaptTempl(
				template.Incident(deployment, r.PathValue("eventName")),
				cfg.Core.CacheControlLong,
			).ServeHTTP(w, r)
		},
	)
	mux.HandleFunc("GET /ims/app/events/{eventName}",
		func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/ims/app/events/"+r.PathValue("eventName")+"/incidents", http.StatusFound)
		},
	)
	mux.Handle("GET /ims/auth/login",
		AdaptTempl(template.Login(deployment), cfg.Core.CacheControlLong),
	)
	mux.Handle("GET /ims/auth/logout",
		Adapt(
			func(w http.ResponseWriter, req *http.Request) {
				slog.Info("Redirecting from logout")
				http.SetCookie(w, &http.Cookie{
					Name:     authz.RefreshTokenCookieName,
					MaxAge:   -1,
					Path:     "/",
					HttpOnly: true,
					Secure:   true,
					SameSite: http.SameSiteStrictMode,
				})
				http.Redirect(w, req, "/ims/app?logout", http.StatusSeeOther)
			},
		),
	)

	// Catch-all handler. Requests to the above handlers with a trailing slash will get
	// a 404 response, so we redirect here instead.
	mux.HandleFunc("GET /ims/app/{anything...}", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, strings.TrimSuffix(r.URL.Path, "/"), http.StatusMovedPermanently)
			return
		}
		http.NotFound(w, r)
	})

	return mux
}

func CacheControl(maxAge time.Duration) Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			durSec := maxAge.Milliseconds() / 1000
			w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%v", durSec))
			next.ServeHTTP(w, r)
		})
	}
}

func CdnCacheControlOff() Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// https://developers.cloudflare.com/cache/concepts/cdn-cache-control/
			w.Header().Set("CDN-Cache-Control", "max-age=0, no-store")
			next.ServeHTTP(w, r)
		})
	}
}

type Adapter func(http.Handler) http.Handler

func Adapt(h http.HandlerFunc, adapters ...Adapter) http.Handler {
	handler := http.Handler(h)
	for i := range adapters {
		adapter := adapters[len(adapters)-1-i] // range in reverse
		handler = adapter(handler)
	}
	return handler
}

func AdaptTempl(comp templ.Component, cacheControlLong time.Duration, adapters ...Adapter) http.Handler {
	adapters = append(adapters, CacheControl(cacheControlLong))
	return Adapt(
		func(w http.ResponseWriter, req *http.Request) {
			err := comp.Render(req.Context(), w)
			if err != nil {
				slog.Error("Failed to render template", "error", err)
				http.Error(w, "Failed to parse template", http.StatusInternalServerError)
				return
			}
		},
		adapters...,
	)
}
