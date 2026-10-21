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

// Command rendergolden renders every IMS page with the Go server's templ
// templates, for the Rust port's tests to compare its Askama pages against.
//
//	go run ./rust/tools/rendergolden -out DIR -deployment dev
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/a-h/templ"
	"github.com/burningmantech/ranger-ims-go/web/template"
)

func main() {
	out := flag.String("out", "", "directory to write the pages into")
	d := flag.String("deployment", "dev", "deployment type to render for")
	n := flag.String("version-name", "test-version", "version name")
	r := flag.String("version-ref", "0123456789abcdef", "version ref")
	e := flag.String("event", "2025", "event name for the event pages")
	flag.Parse()
	if *out == "" {
		log.Fatal("-out is required")
	}
	pages := map[string]templ.Component{
		"login.html":            template.Login(*d, *n, *r),
		"root.html":             template.Root(*d, *n, *r),
		"adminroot.html":        template.AdminRoot(*d, *n, *r),
		"adminactionlogs.html":  template.AdminActionLogs(*d, *n, *r),
		"adminerrorlogs.html":   template.AdminErrorLogs(*d, *n, *r),
		"adminevents.html":      template.AdminEvents(*d, *n, *r),
		"admintypes.html":       template.AdminTypes(*d, *n, *r),
		"admindebug.html":       template.AdminDebug(*d, *n, *r),
		"admindirectory.html":   template.AdminDirectory(*d, *n, *r),
		"adminplaces.html":      template.AdminPlaces(*d, *n, *r),
		"settings.html":         template.Settings(*d, *n, *r),
		"help.html":             template.Help(*d, *n, *r),
		"search.html":           template.Search(*d, *n, *r),
		"places.html":           template.Places(*d, *n, *r, *e),
		"field_reports.html":    template.FieldReports(*d, *n, *r, *e),
		"field_report.html":     template.FieldReport(*d, *n, *r, *e),
		"incidents.html":        template.Incidents(*d, *n, *r, *e),
		"incident.html":         template.Incident(*d, *n, *r, *e),
		"sanctuary_visits.html": template.SanctuaryVisits(*d, *n, *r, *e),
		"sanctuary_visit.html":  template.SanctuaryVisit(*d, *n, *r, *e),
	}
	err := os.MkdirAll(*out, 0o750)
	if err != nil {
		log.Fatal(err)
	}
	for name, comp := range pages {
		// #nosec G304 // The output directory is the caller's to choose.
		f, err := os.Create(filepath.Join(*out, name))
		if err != nil {
			log.Fatal(err)
		}
		err = comp.Render(context.Background(), f)
		if err != nil {
			log.Fatal(err)
		}
		err = f.Close()
		if err != nil {
			log.Fatal(err)
		}
	}
}
