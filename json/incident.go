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

import (
	"time"
)

type Incidents []Incident

type Location struct {
	// Various fields here are nilable, because client can set them empty, and the server must be able
	// to distinguish empty from unset.

	Name        *string `json:"name,omitempty"`
	Address     *string `json:"address,omitempty"`
	Description *string `json:"description,omitempty"`
}

const (
	IncidentPriorityHigh   = 5
	IncidentPriorityNormal = 3
	IncidentPriorityLow    = 1
)

type Incident struct {
	Event        string    `json:"event"`
	EventID      int32     `json:"event_id"`
	Number       int32     `json:"number"`
	Created      time.Time `json:"created,omitzero"`
	LastModified time.Time `json:"last_modified,omitzero"`
	// Version is the optimistic-concurrency counter guarding an edit's
	// read-merge-write of the fields below. It moves only when those fields
	// do: not for report entries, and not for the response-only sets, which
	// live in their own tables and so can't be clobbered by an edit. It's
	// reported here, but clients don't send it back. Stored versions start at
	// 1, so omitzero only elides it from client-sent edit bodies, never from
	// server responses.
	Version  int32     `json:"version,omitzero"`
	State    string    `json:"state"`
	Started  time.Time `json:"started,omitzero"`
	Closed   time.Time `json:"closed,omitzero"`
	Priority int8      `json:"priority"`
	Summary  *string   `json:"summary"`
	Location Location  `json:"location"`
	// These five are response-only. Each is mutated one member at a time
	// through its own endpoint, so that concurrent writers don't undo each
	// other; sending any of the first four on an edit is a 400.
	IncidentTypeIDs *[]int32          `json:"incident_type_ids"`
	FieldReports    *[]int32          `json:"field_reports"`
	Visits          *[]int32          `json:"visits"`
	LinkedIncidents *[]LinkedIncident `json:"linked_incidents,omitzero"`
	Rangers         *[]IncidentRanger `json:"rangers"`
	ReportEntries   []ReportEntry     `json:"report_entries"`
}

type IncidentRanger struct {
	Handle string  `json:"handle,omitempty"`
	Role   *string `json:"role,omitempty"`
}

type LinkedIncident struct {
	EventName string `json:"event_name"`
	EventID   int32  `json:"event_id"`
	Number    int32  `json:"number"`
	Summary   string `json:"summary,omitempty"`
}
