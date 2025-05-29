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

type ReportEntry struct {
	ID            int32     `json:"id"`
	Created       time.Time `json:"created,omitzero"`
	Author        string    `json:"author"`
	SystemEntry   bool      `json:"system_entry"`
	Text          string    `json:"text"`
	Stricken      *bool     `json:"stricken"`
	HasAttachment bool      `json:"has_attachment"`
}
