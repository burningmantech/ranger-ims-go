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

type ActionLogs []ActionLog

type ActionLog struct {
	ID            int64     `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	ActionType    string    `json:"action_type"`
	Method        string    `json:"method,omitzero"`
	Path          string    `json:"path,omitzero"`
	Referrer      string    `json:"referrer,omitzero"`
	UserID        int64     `json:"user_id,omitzero"`
	UserName      string    `json:"user_name"`
	PositionID    int64     `json:"position_id,omitzero"`
	PositionName  string    `json:"position_name"`
	ClientAddress string    `json:"client_address,omitzero"`
	HttpStatus    int16     `json:"http_status,omitzero"`
	Duration      string    `json:"duration,omitzero"`
}
