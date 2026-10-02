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
	"cmp"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// The event-wide list endpoints (Incidents, Field Reports, Visits) return
// slimmed-down records without report entries. They take two optional,
// mutually exclusive filters:
//
//   - number: return only the record with that number, e.g. so a list page
//     can refresh one row after an update without fetching the full record.
//   - q: return only the records matching a search, for the list pages'
//     search boxes. This is the server-side stand-in for the client-side
//     DataTables search those pages used to do over every record's full text.
//
// listFilter is what the request asked for.
type listFilter struct {
	// number is the record number to return, if hasNumber.
	number    int32
	hasNumber bool
	// match is nil when there's no search.
	match listMatcher
	// query is the raw search text, for the action log.
	query string
}

func parseListFilter(req *http.Request) (listFilter, *herr.HTTPError) {
	var filter listFilter
	err := req.ParseForm()
	if err != nil {
		return filter, herr.BadRequest("Failed to parse form", err).From("[ParseForm]")
	}
	numberParam := req.Form.Get("number")
	query := strings.TrimSpace(req.Form.Get("q"))
	if numberParam != "" && query != "" {
		return filter, herr.BadRequest("The 'number' and 'q' parameters can't be used together", nil)
	}
	if numberParam != "" {
		number, err := conv.ParseInt32(numberParam)
		if err != nil {
			return filter, herr.BadRequest("The 'number' parameter must be an integer", err).From("[ParseInt32]")
		}
		filter.number = number
		filter.hasNumber = true
	}
	if query != "" {
		match, errHTTP := parseListQuery(query)
		if errHTTP != nil {
			return filter, errHTTP.From("[parseListQuery]")
		}
		filter.match = match
		filter.query = query
	}
	return filter, nil
}

// listMatcher reports whether a record's searchable text matches a search.
type listMatcher func(text string) bool

// smartSearchToken is a word or quoted phrase in a search, optionally negated
// with a leading "!". This is the same syntax as DataTables' smart search.
var smartSearchToken = regexp.MustCompile(`!?["\x{201C}][^"\x{201D}]+["\x{201D}]|[^ ]+`)

// parseListQuery parses a list page's search box. A query wrapped in slashes,
// like /fire|smoke/, is a case-insensitive regular expression. Anything else
// is a smart search, as DataTables did it when the list pages searched on the
// client: every word or "quoted phrase" must appear somewhere in the record,
// in any order, and none of the !negated ones may. Matching ignores case and
// diacritics.
func parseListQuery(query string) (listMatcher, *herr.HTTPError) {
	if len(query) >= 2 && strings.HasPrefix(query, "/") && strings.HasSuffix(query, "/") {
		re, err := regexp.Compile("(?i)" + query[1:len(query)-1])
		if err != nil {
			return nil, herr.BadRequest("Invalid regular expression", err).From("[regexp.Compile]")
		}
		return re.MatchString, nil
	}

	var required, excluded []string
	for _, token := range smartSearchToken.FindAllString(foldForSearch(query), -1) {
		negated := strings.HasPrefix(token, "!")
		token = strings.TrimPrefix(token, "!")
		token = strings.Trim(token, "\"“”")
		if token == "" {
			continue
		}
		if negated {
			// As with DataTables, a lone "!x" is ignored, since it's usually
			// just the start of a negation still being typed.
			if len([]rune(token)) > 1 {
				excluded = append(excluded, token)
			}
			continue
		}
		required = append(required, token)
	}
	return func(text string) bool {
		text = foldForSearch(text)
		for _, term := range required {
			if !strings.Contains(text, term) {
				return false
			}
		}
		for _, term := range excluded {
			if strings.Contains(text, term) {
				return false
			}
		}
		return true
	}, nil
}

// foldForSearch lowercases s and strips its diacritics, so "Café" matches "cafe".
func foldForSearch(s string) string {
	stripped, _, err := transform.String(
		transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), s,
	)
	if err != nil {
		stripped = s
	}
	return strings.ToLower(stripped)
}

// searchDoc accumulates a record's searchable text.
type searchDoc struct {
	parts []string
}

// String joins the parts with double spaces, as DataTables joins columns, so
// that a phrase can't match across two fields.
func (d *searchDoc) String() string {
	return strings.Join(d.parts, "  ")
}

func (d *searchDoc) add(texts ...string) {
	for _, t := range texts {
		if t != "" {
			d.parts = append(d.parts, t)
		}
	}
}

// addEntries adds the text of the non-system entries.
func (d *searchDoc) addEntries(entries []imsdb.ReportEntry) {
	for _, e := range entries {
		if !e.Generated {
			d.add(e.Text)
		}
	}
}

// sortEntries orders report entries oldest first. The queries that fetch
// entries in bulk don't order them.
func sortEntries(entries []imsdb.ReportEntry) {
	slices.SortFunc(entries, func(a, b imsdb.ReportEntry) int {
		return cmp.Or(cmp.Compare(a.Created, b.Created), cmp.Compare(a.ID, b.ID))
	})
}

// listSummary is the summary a list shows for a record: its own summary, or
// failing that an excerpt of the first line of the first non-system entry.
// entries must be sorted.
func listSummary(summary string, entries []imsdb.ReportEntry) string {
	if summary != "" {
		return summary
	}
	for _, e := range entries {
		if e.Generated {
			continue
		}
		for line := range strings.Lines(e.Text) {
			if line = strings.TrimRight(line, "\r\n"); line != "" {
				return searchSnippet(line, 0)
			}
		}
	}
	return ""
}

// lastModified is the later of a record's creation time and its newest entry.
func lastModified(created float64, entries []imsdb.ReportEntry) time.Time {
	latest := created
	for _, e := range entries {
		latest = max(latest, e.Created)
	}
	return conv.FloatToTime(latest)
}
