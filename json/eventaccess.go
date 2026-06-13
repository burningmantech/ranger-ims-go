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

type EventsAccess map[string]EventAccess

type AccessRule struct {
	Expression string    `json:"expression"`
	Validity   string    `json:"validity"`
	NotAfter   time.Time `json:"not_after,omitzero"`

	// Expired is a read-only field, saying if the AccessRule's NotAfter time is in the past.
	Expired bool `json:"expired,omitzero"`

	NotBefore time.Time `json:"not_before,omitzero"`

	// Pending is a read-only field, saying if the AccessRule's NotBefore time is in the future.
	Pending bool `json:"pending,omitzero"`

	DebugInfo struct {
		MatchesUsers    []string `json:"matches_users,omitempty"`
		MatchesAllUsers bool     `json:"matches_all_users,omitempty"`
		MatchesNoOne    bool     `json:"matches_no_one,omitempty"`
		KnownTarget     bool     `json:"known_target"`
	} `json:"debug_info"`
}

type EventAccess struct {
	Readers      []AccessRule `json:"readers"`
	Writers      []AccessRule `json:"writers"`
	Reporters    []AccessRule `json:"reporters"`
	VisitWriters []AccessRule `json:"visit_writers"`
}

// AccessTargets lists the names that are valid targets for AccessRule
// expressions, e.g. a Persons entry of "Tool" means "person:Tool" is valid.
type AccessTargets struct {
	Persons   []string `json:"persons"`
	Positions []string `json:"positions"`
	Teams     []string `json:"teams"`
}
