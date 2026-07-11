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
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/go-sql-driver/mysql"
)

// GetSearch serves cross-event search: it matches a text query — a literal
// substring by default, or a regular expression with regex=true — against
// Incidents, Field Reports, and Visits in every Event the requestor is
// permitted to read, returning a single merged result list.
type GetSearch struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

const (
	searchMinQueryRunes  = 2
	searchDefaultLimit   = 100
	searchMaxLimit       = 1000
	searchQueryTimeout   = 10 * time.Second
	searchSnippetPrefix  = 40
	searchSnippetMaxLen  = 200
	searchSnippetMarker  = "…"
	searchAllResultKinds = imsjson.SearchResultKindIncident + "," +
		imsjson.SearchResultKindFieldReport + "," +
		imsjson.SearchResultKindVisit
)

func (action GetSearch) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getSearch(req)
	if errHTTP != nil {
		errHTTP.From("[getSearch]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}

func (action GetSearch) getSearch(req *http.Request) (imsjson.SearchResults, *herr.HTTPError) {
	resp := imsjson.SearchResults{Hits: []imsjson.SearchResult{}}
	jwtCtx, errHTTP := getJwtCtx(req)
	if errHTTP != nil {
		return resp, errHTTP.From("[getJwtCtx]")
	}

	err := req.ParseForm()
	if err != nil {
		return resp, herr.BadRequest("Failed to parse form", err).From("[ParseForm]")
	}
	query := strings.TrimSpace(req.Form.Get("q"))
	if utf8.RuneCountInString(query) < searchMinQueryRunes {
		return resp, herr.BadRequest("The 'q' parameter must be at least 2 characters long", nil)
	}

	regex := false
	if regexParam := req.Form.Get("regex"); regexParam != "" {
		regex, err = strconv.ParseBool(regexParam)
		if err != nil {
			return resp, herr.BadRequest("The 'regex' parameter must be a boolean", nil)
		}
	}

	kinds := req.Form.Get("kinds")
	if kinds == "" {
		kinds = searchAllResultKinds
	}
	var searchIncidents, searchFieldReports, searchVisits bool
	for kind := range strings.SplitSeq(kinds, ",") {
		switch strings.TrimSpace(kind) {
		case imsjson.SearchResultKindIncident:
			searchIncidents = true
		case imsjson.SearchResultKindFieldReport:
			searchFieldReports = true
		case imsjson.SearchResultKindVisit:
			searchVisits = true
		default:
			return resp, herr.BadRequest("The 'kinds' parameter must be a comma-separated subset of "+searchAllResultKinds, nil)
		}
	}

	limit := int32(searchDefaultLimit)
	if limitParam := req.Form.Get("limit"); limitParam != "" {
		parsed, err := strconv.ParseInt(limitParam, 10, 32)
		if err != nil || parsed < 1 || parsed > searchMaxLimit {
			return resp, herr.BadRequest("The 'limit' parameter must be an integer between 1 and 1000", nil)
		}
		limit = int32(parsed)
	}

	ctx := req.Context()
	permsByEvent, errHTTP := permissionsByEvent(ctx, jwtCtx, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return resp, errHTTP.From("[permissionsByEvent]")
	}

	events, err := action.imsDBQ.Events(ctx, action.imsDBQ)
	if err != nil {
		return resp, herr.InternalServerError("Failed to fetch Events", err).From("[Events]")
	}
	var incidentEventIDs, fieldReportEventIDs, visitEventIDs []int32
	for _, e := range events {
		if e.Event.IsGroup {
			continue
		}
		perms := permsByEvent[e.Event.ID]
		if perms&authz.EventReadIncidents != 0 {
			incidentEventIDs = append(incidentEventIDs, e.Event.ID)
		}
		// Users whose access is limited to their own Field Reports
		// (EventReadOwnFieldReports without EventReadAllFieldReports) don't get
		// Field Report results for that Event, since these queries have no
		// per-report author filtering.
		if perms&authz.EventReadAllFieldReports != 0 {
			fieldReportEventIDs = append(fieldReportEventIDs, e.Event.ID)
		}
		if perms&authz.EventReadVisits != 0 {
			visitEventIDs = append(visitEventIDs, e.Event.ID)
		}
	}

	// The search queries scan every readable event's records, and regexp
	// matching in particular can cost tens of milliseconds per row on
	// pathological patterns, so bound the total database work.
	ctx, cancel := context.WithTimeout(ctx, searchQueryTimeout)
	defer cancel()

	// Exactly one of textLike and textRegexp is non-null; the Search* queries
	// use whichever is set and let the other drop out.
	var textLike, textRegexp sql.NullString
	var queryRegexp *regexp.Regexp
	if regex {
		// Compiling validates the pattern before it reaches the database. Go's
		// RE2 syntax is essentially a subset of the PCRE syntax that MariaDB's
		// REGEXP_INSTR uses, so a pattern accepted here is valid there too.
		// The (?i) mirrors the database's case-insensitive collation.
		queryRegexp, err = regexp.Compile("(?i)" + query)
		if err != nil {
			return resp, herr.BadRequest("Invalid regular expression", err)
		}
		textRegexp = sql.NullString{String: query, Valid: true}
	} else {
		textLike = sql.NullString{String: "%" + escapeLikePattern(query) + "%", Valid: true}
	}

	// snippet extracts a result excerpt from a MATCHED_ENTRY_TEXT column,
	// locating the match with whichever mode the search used.
	snippet := func(matchedEntryText any) string {
		text := textColumn(matchedEntryText)
		matchAt := -1
		if queryRegexp != nil {
			if loc := queryRegexp.FindStringIndex(text); loc != nil {
				matchAt = loc[0]
			}
		} else {
			matchAt = indexFold(text, query)
		}
		return searchSnippet(text, matchAt)
	}

	if searchIncidents && len(incidentEventIDs) > 0 {
		rows, err := action.imsDBQ.SearchIncidents(ctx, action.imsDBQ, imsdb.SearchIncidentsParams{
			TextLike:   textLike,
			TextRegexp: textRegexp,
			EventIds:   incidentEventIDs,
			Limit:      limit,
		})
		if err != nil {
			return resp, searchQueryError("Incidents", err).From("[SearchIncidents]")
		}
		for _, row := range rows {
			resp.Hits = append(resp.Hits, imsjson.SearchResult{
				Kind:    imsjson.SearchResultKindIncident,
				Event:   row.EventName,
				EventID: row.Event,
				Number:  row.Number,
				Created: conv.FloatToTime(row.Created),
				Summary: row.Summary.String,
				Snippet: snippet(row.MatchedEntryText),
			})
		}
		resp.Truncated = resp.Truncated || len(rows) == int(limit)
	}

	if searchFieldReports && len(fieldReportEventIDs) > 0 {
		rows, err := action.imsDBQ.SearchFieldReports(ctx, action.imsDBQ, imsdb.SearchFieldReportsParams{
			TextLike:   textLike,
			TextRegexp: textRegexp,
			EventIds:   fieldReportEventIDs,
			Limit:      limit,
		})
		if err != nil {
			return resp, searchQueryError("Field Reports", err).From("[SearchFieldReports]")
		}
		for _, row := range rows {
			resp.Hits = append(resp.Hits, imsjson.SearchResult{
				Kind:     imsjson.SearchResultKindFieldReport,
				Event:    row.EventName,
				EventID:  row.Event,
				Number:   row.Number,
				Created:  conv.FloatToTime(row.Created),
				Summary:  row.Summary.String,
				Snippet:  snippet(row.MatchedEntryText),
				Incident: conv.SqlToInt32(row.IncidentNumber),
			})
		}
		resp.Truncated = resp.Truncated || len(rows) == int(limit)
	}

	if searchVisits && len(visitEventIDs) > 0 {
		rows, err := action.imsDBQ.SearchVisits(ctx, action.imsDBQ, imsdb.SearchVisitsParams{
			TextLike:   textLike,
			TextRegexp: textRegexp,
			EventIds:   visitEventIDs,
			Limit:      limit,
		})
		if err != nil {
			return resp, searchQueryError("Visits", err).From("[SearchVisits]")
		}
		for _, row := range rows {
			summary := row.GuestPreferredName.String
			if summary == "" {
				summary = row.GuestLegalName.String
			}
			resp.Hits = append(resp.Hits, imsjson.SearchResult{
				Kind:     imsjson.SearchResultKindVisit,
				Event:    row.EventName,
				EventID:  row.Event,
				Number:   row.Number,
				Created:  conv.FloatToTime(row.Created),
				Summary:  summary,
				Snippet:  snippet(row.MatchedEntryText),
				Incident: conv.SqlToInt32(row.IncidentNumber),
			})
		}
		resp.Truncated = resp.Truncated || len(rows) == int(limit)
	}

	slices.SortFunc(resp.Hits, func(a, b imsjson.SearchResult) int {
		return b.Created.Compare(a.Created)
	})

	return resp, nil
}

// escapeLikePattern escapes the characters that have special meaning inside
// a SQL LIKE pattern, so that user input only ever matches literally.
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// textColumn converts a text column that sqlc typed as interface{} into a
// string. The MySQL driver returns such columns as []byte.
func textColumn(v any) string {
	switch t := v.(type) {
	case []byte:
		return string(t)
	case string:
		return t
	}
	return ""
}

// searchQueryError converts a failure of one of the Search* queries into an
// HTTP error. A pattern that Go's regexp compiler accepted but the database's
// PCRE engine rejected (MariaDB error 1139) is the client's fault, not ours,
// and hitting the search deadline deserves better advice than a plain 500.
func searchQueryError(what string, err error) *herr.HTTPError {
	const mySQLErRegexpError = 1139
	mysqlErr, ok := errors.AsType[*mysql.MySQLError](err)
	if ok && mysqlErr.Number == mySQLErRegexpError {
		return herr.BadRequest("Invalid regular expression", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return herr.New(http.StatusServiceUnavailable, "Search took too long — try a more specific query", err)
	}
	return herr.InternalServerError("Failed to search "+what, err)
}

// searchSnippet returns an excerpt of text around a match at byte offset
// matchAt, for display in search results. The caller locates the match with
// simpler semantics than the database's collation-based matching, so the
// match may not have been found (matchAt < 0), in which case the excerpt
// comes from the start of the text.
func searchSnippet(text string, matchAt int) string {
	if text == "" {
		return ""
	}
	matchAt = max(matchAt, 0)

	start := max(matchAt-searchSnippetPrefix, 0)
	end := min(start+searchSnippetMaxLen, len(text))
	// Don't split multi-byte characters at either edge of the excerpt.
	for start > 0 && !utf8.RuneStart(text[start]) {
		start--
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}

	snippet := text[start:end]
	if start > 0 {
		snippet = searchSnippetMarker + snippet
	}
	if end < len(text) {
		snippet += searchSnippetMarker
	}
	return snippet
}

// indexFold returns the byte offset in s of the first case-insensitive match
// of substr, or -1 if there is none. Indexing into ToLower(s) instead would
// give offsets that are invalid in s, because lowercasing a rune can change
// its encoded length (e.g. "Ⱥ" is 2 bytes and "ⱥ" is 3).
func indexFold(s, substr string) int {
	for i := range s {
		if hasFoldPrefix(s[i:], substr) {
			return i
		}
	}
	return -1
}

func hasFoldPrefix(s, prefix string) bool {
	for _, pr := range prefix {
		r, size := utf8.DecodeRuneInString(s)
		if size == 0 {
			return false
		}
		if unicode.ToLower(r) != unicode.ToLower(pr) {
			return false
		}
		s = s[size:]
	}
	return true
}
