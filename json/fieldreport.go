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

type FieldReports []FieldReport
type FieldReport struct {
	Event   string    `json:"event"`
	Number  int32     `json:"number"`
	Created time.Time `json:"created,omitzero"`
	// Version is the optimistic-concurrency counter; see Incident.Version.
	Version       int32         `json:"version,omitzero"`
	Summary       *string       `json:"summary"`
	Incident      *int32        `json:"incident,omitzero"`
	ReportEntries []ReportEntry `json:"report_entries"`
}

type FieldReportListItems []FieldReportListItem

// FieldReportListItem is a Field Report as the event-wide Field Reports list
// returns it. See IncidentListItem.
type FieldReportListItem struct {
	Event        string    `json:"event"`
	Number       int32     `json:"number"`
	Created      time.Time `json:"created,omitzero"`
	LastModified time.Time `json:"last_modified,omitzero"`
	// Summary is as for IncidentListItem.Summary.
	Summary  string `json:"summary"`
	Incident *int32 `json:"incident,omitzero"`
	// Author is the author of the Field Report's first report entry.
	Author string `json:"author"`
}
