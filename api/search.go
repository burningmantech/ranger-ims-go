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
	"database/sql"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
)

// GetSearch serves cross-event search: it matches a text query against
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

	textLike := sql.NullString{String: "%" + escapeLikePattern(query) + "%", Valid: true}

	if searchIncidents && len(incidentEventIDs) > 0 {
		rows, err := action.imsDBQ.SearchIncidents(ctx, action.imsDBQ, imsdb.SearchIncidentsParams{
			TextLike: textLike,
			EventIds: incidentEventIDs,
			Limit:    limit,
		})
		if err != nil {
			return resp, herr.InternalServerError("Failed to search Incidents", err).From("[SearchIncidents]")
		}
		for _, row := range rows {
			resp.Hits = append(resp.Hits, imsjson.SearchResult{
				Kind:    imsjson.SearchResultKindIncident,
				Event:   row.EventName,
				EventID: row.Event,
				Number:  row.Number,
				Created: conv.FloatToTime(row.Created),
				Summary: row.Summary.String,
				Snippet: searchSnippet(textColumn(row.MatchedEntryText), query),
			})
		}
		resp.Truncated = resp.Truncated || len(rows) == int(limit)
	}

	if searchFieldReports && len(fieldReportEventIDs) > 0 {
		rows, err := action.imsDBQ.SearchFieldReports(ctx, action.imsDBQ, imsdb.SearchFieldReportsParams{
			TextLike: textLike,
			EventIds: fieldReportEventIDs,
			Limit:    limit,
		})
		if err != nil {
			return resp, herr.InternalServerError("Failed to search Field Reports", err).From("[SearchFieldReports]")
		}
		for _, row := range rows {
			resp.Hits = append(resp.Hits, imsjson.SearchResult{
				Kind:     imsjson.SearchResultKindFieldReport,
				Event:    row.EventName,
				EventID:  row.Event,
				Number:   row.Number,
				Created:  conv.FloatToTime(row.Created),
				Summary:  row.Summary.String,
				Snippet:  searchSnippet(textColumn(row.MatchedEntryText), query),
				Incident: conv.SqlToInt32(row.IncidentNumber),
			})
		}
		resp.Truncated = resp.Truncated || len(rows) == int(limit)
	}

	if searchVisits && len(visitEventIDs) > 0 {
		rows, err := action.imsDBQ.SearchVisits(ctx, action.imsDBQ, imsdb.SearchVisitsParams{
			TextLike: textLike,
			EventIds: visitEventIDs,
			Limit:    limit,
		})
		if err != nil {
			return resp, herr.InternalServerError("Failed to search Visits", err).From("[SearchVisits]")
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
				Snippet:  searchSnippet(textColumn(row.MatchedEntryText), query),
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

// searchSnippet returns an excerpt of text around the first occurrence of
// query, for display in search results. Matching here is simpler than the
// database's collation-based matching, so the query may not be found, in
// which case the excerpt comes from the start of the text.
func searchSnippet(text, query string) string {
	if text == "" {
		return ""
	}
	matchAt := max(indexFold(text, query), 0)

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
