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

type Stays []Stay

type Stay struct {
	Event          string    `json:"event"`
	EventID        int32     `json:"event_id"`
	Number         int32     `json:"number"`
	Created        time.Time `json:"created,omitzero"`
	LastModified   time.Time `json:"last_modified,omitzero"`
	IncidentNumber *int32    `json:"incident_number,omitzero"`

	GuestPreferredName   *string `json:"guest_preferred_name,omitzero"`
	GuestLegalName       *string `json:"guest_legal_name,omitzero"`
	GuestDescription     *string `json:"guest_description,omitzero"`
	GuestCampName        *string `json:"guest_camp_name,omitzero"`
	GuestCampAddress     *string `json:"guest_camp_address,omitzero"`
	GuestCampDescription *string `json:"guest_camp_description,omitzero"`

	ArrivalTime       *time.Time `json:"arrival_time,omitempty"`
	ArrivalMethod     *string    `json:"arrival_method,omitzero"`
	ArrivalState      *string    `json:"arrival_state,omitzero"`
	ArrivalReason     *string    `json:"arrival_reason,omitzero"`
	ArrivalBelongings *string    `json:"arrival_belongings,omitzero"`

	DepartureTime   *time.Time `json:"departure_time,omitempty"`
	DepartureMethod *string    `json:"departure_method,omitzero"`
	DepartureState  *string    `json:"departure_state,omitzero"`

	ResourceRest    *string `json:"resource_rest,omitzero"`
	ResourceClothes *string `json:"resource_clothes,omitzero"`
	ResourcePogs    *string `json:"resource_pogs,omitzero"`
	ResourceFoodBev *string `json:"resource_food_bev,omitzero"`
	ResourceOther   *string `json:"resource_other,omitzero"`

	Rangers       *[]StayRanger `json:"rangers"`
	ReportEntries []ReportEntry `json:"report_entries"`
}

type StayRanger struct {
	Handle string  `json:"handle,omitempty"`
	Role   *string `json:"role,omitempty"`
}
