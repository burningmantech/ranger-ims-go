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

package json

import "time"

const (
	SearchResultKindIncident    = "incident"
	SearchResultKindFieldReport = "field_report"
	SearchResultKindVisit       = "visit"
)

type SearchResults struct {
	Hits []SearchResult `json:"hits"`
	// Truncated indicates that at least one kind of record had more matches
	// than the requested limit, so some matches were omitted.
	Truncated bool `json:"truncated"`
}

// SearchResult is one hit from cross-event search: an Incident, a Field
// Report, or a Visit, as indicated by Kind.
type SearchResult struct {
	Kind    string    `json:"kind"`
	Event   string    `json:"event"`
	EventID int32     `json:"event_id"`
	Number  int32     `json:"number"`
	Created time.Time `json:"created,omitzero"`
	Summary string    `json:"summary,omitempty"`
	// Snippet is an excerpt of a report entry that matched the search query,
	// when the hit has such an entry.
	Snippet string `json:"snippet,omitempty"`
	// Incident is the attached Incident number, for Field Report and Visit hits.
	Incident *int32 `json:"incident,omitempty"`
}
