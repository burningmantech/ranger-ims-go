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

// Directory is the admin view of the IMS-native user directory.
type Directory struct {
	Persons   []DirectoryPerson `json:"persons"`
	Teams     []DirectoryGroup  `json:"teams"`
	Positions []DirectoryGroup  `json:"positions"`
}

// DirectoryPerson is a user in the IMS-native directory. Password hashes are
// never included. Pointer fields are optional in write requests, meaning
// "leave unchanged" when updating an existing person.
type DirectoryPerson struct {
	ID          int64    `json:"id"`
	Handle      *string  `json:"handle"`
	Email       *string  `json:"email,omitempty"`
	Active      *bool    `json:"active"`
	Onsite      *bool    `json:"onsite"`
	TeamIDs     *[]int64 `json:"team_ids"`
	PositionIDs *[]int64 `json:"position_ids"`
}

// DirectoryGroup is a team or position in the IMS-native directory.
type DirectoryGroup struct {
	ID     int64   `json:"id"`
	Title  *string `json:"title"`
	Active *bool   `json:"active"`
}

// DirectoryPersonPassword is the request body for setting a person's password.
type DirectoryPersonPassword struct {
	// #nosec G117 // Exported secret field
	Password string `json:"password"`
}
