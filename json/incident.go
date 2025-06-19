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

	Name         *string `json:"name"`
	Concentric   *string `json:"concentric"`
	RadialHour   *string `json:"radial_hour"`
	RadialMinute *string `json:"radial_minute"`
	Description  *string `json:"description"`
}

const (
	IncidentPriorityHigh   = 5
	IncidentPriorityNormal = 3
	IncidentPriorityLow    = 1
)

type Incident struct {
	Event             string        `json:"event"`
	EventID           int32         `json:"event_id"`
	Number            int32         `json:"number"`
	Created           time.Time     `json:"created,omitzero"`
	LastModified      time.Time     `json:"last_modified,omitzero"`
	State             string        `json:"state"`
	Started           time.Time     `json:"started,omitzero"`
	Priority          int8          `json:"priority"`
	Summary           *string       `json:"summary"`
	Location          Location      `json:"location"`
	IncidentTypeNames *[]string     `json:"incident_types"`
	IncidentTypeIDs   *[]int32      `json:"incident_type_ids"`
	FieldReports      *[]int32      `json:"field_reports"`
	RangerHandles     *[]string     `json:"ranger_handles"`
	ReportEntries     []ReportEntry `json:"report_entries"`
}
