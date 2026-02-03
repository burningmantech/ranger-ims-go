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

	Name         *string `json:"name,omitempty"`
	Address      *string `json:"address,omitempty"`
	Concentric   *string `json:"concentric,omitempty"`
	RadialHour   *string `json:"radial_hour,omitempty"`
	RadialMinute *string `json:"radial_minute,omitempty"`
	Description  *string `json:"description,omitempty"`
}

const (
	IncidentPriorityHigh   = 5
	IncidentPriorityNormal = 3
	IncidentPriorityLow    = 1
)

type Incident struct {
	Event           string            `json:"event"`
	EventID         int32             `json:"event_id"`
	Number          int32             `json:"number"`
	Created         time.Time         `json:"created,omitzero"`
	LastModified    time.Time         `json:"last_modified,omitzero"`
	State           string            `json:"state"`
	Started         time.Time         `json:"started,omitzero"`
	Closed          time.Time         `json:"closed,omitzero"`
	Priority        int8              `json:"priority"`
	Summary         *string           `json:"summary"`
	Location        Location          `json:"location"`
	IncidentTypeIDs *[]int32          `json:"incident_type_ids"`
	FieldReports    *[]int32          `json:"field_reports"`
	Stays           *[]int32          `json:"stays"`
	Rangers         *[]IncidentRanger `json:"rangers"`
	LinkedIncidents *[]LinkedIncident `json:"linked_incidents,omitzero"`
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
