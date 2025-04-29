package web

import (
	"fmt"
	"github.com/a-h/templ"
	"github.com/srabraham/ranger-ims-go/conf"
	"github.com/srabraham/ranger-ims-go/web/template"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

func AddToMux(mux *http.ServeMux, cfg *conf.IMSConfig) *http.ServeMux {
	if mux == nil {
		mux = http.NewServeMux()
	}

	mux.Handle("GET /ims/static/",
		Adapt(
			http.StripPrefix("/ims/", http.FileServerFS(StaticFS)).ServeHTTP,
			Static(1*time.Hour),
		),
	)
	mux.Handle("GET /ims/app",
		AdaptTempl(template.Root(cfg.Core.Deployment)),
	)
	mux.Handle("GET /ims/app/admin",
		AdaptTempl(template.AdminRoot(cfg.Core.Deployment)),
	)
	mux.Handle("GET /ims/app/admin/events",
		AdaptTempl(template.AdminEvents(cfg.Core.Deployment)),
	)
	mux.Handle("GET /ims/app/admin/streets",
		AdaptTempl(template.AdminStreets(cfg.Core.Deployment)),
	)
	mux.Handle("GET /ims/app/admin/types",
		AdaptTempl(template.AdminTypes(cfg.Core.Deployment)),
	)
	mux.Handle("GET /ims/app/events/{eventName}/field_reports",
		AdaptTempl(template.FieldReports(cfg.Core.Deployment)),
	)
	mux.Handle("GET /ims/app/events/{eventName}/field_reports/{fieldReportNumber}",
		AdaptTempl(template.FieldReport(cfg.Core.Deployment)),
	)
	mux.Handle("GET /ims/app/events/{eventName}/incidents",
		AdaptTempl(template.Incidents(cfg.Core.Deployment)),
	)
	mux.Handle("GET /ims/app/events/{eventName}/incidents/{incidentNumber}",
		AdaptTempl(template.Incident(cfg.Core.Deployment)),
	)
	mux.Handle("GET /ims/auth/login",
		AdaptTempl(template.Login(cfg.Core.Deployment)),
	)
	mux.Handle("GET /ims/auth/logout",
		Adapt(
			func(w http.ResponseWriter, req *http.Request) {
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

func Static(dur time.Duration) Adapter {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			durSec := int64(dur.Seconds())
			w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%v, private", durSec))
			next.ServeHTTP(w, r.WithContext(r.Context()))
		})
	}
}

type Adapter func(http.Handler) http.Handler

func Adapt(h http.HandlerFunc, adapters ...Adapter) http.Handler {
	handler := http.Handler(h)
	for i := range adapters {
		adapter := adapters[len(adapters)-1-i] // range in reverse
		handler = adapter(h)
	}
	return handler
}

func AdaptTempl(comp templ.Component, adapters ...Adapter) http.Handler {
	adapters = append(adapters, Static(1*time.Hour))
	return Adapt(
		func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Cache-Control", "max-age=1200, private")
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
