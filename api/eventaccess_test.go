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

package api

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestKnownTarget(t *testing.T) {
	t.Parallel()
	handles := map[string]bool{"Irate": true}
	positions := map[string]bool{"Dirt": true}
	onDutyPositions := map[string]bool{"Dirt": true, "Sanctuary": true}
	teams := map[string]bool{"Green Dot": true}

	// The wildcard is always a known target
	assert.True(t, knownTarget("*", handles, positions, onDutyPositions, teams))

	assert.True(t, knownTarget("person:Irate", handles, positions, onDutyPositions, teams))
	assert.False(t, knownTarget("person:Nobody", handles, positions, onDutyPositions, teams))

	assert.True(t, knownTarget("position:Dirt", handles, positions, onDutyPositions, teams))
	assert.False(t, knownTarget("position:Nothing", handles, positions, onDutyPositions, teams))

	// An "onduty" expression targets a position as well
	assert.True(t, knownTarget("onduty:Dirt", handles, positions, onDutyPositions, teams))
	assert.False(t, knownTarget("onduty:Nothing", handles, positions, onDutyPositions, teams))

	// A position available to all Rangers is a target only for "onduty"
	assert.True(t, knownTarget("onduty:Sanctuary", handles, positions, onDutyPositions, teams))
	assert.False(t, knownTarget("position:Sanctuary", handles, positions, onDutyPositions, teams))

	assert.True(t, knownTarget("team:Green Dot", handles, positions, onDutyPositions, teams))
	assert.False(t, knownTarget("team:No Team", handles, positions, onDutyPositions, teams))

	// A target must not match across categories
	assert.False(t, knownTarget("person:Dirt", handles, positions, onDutyPositions, teams))

	// A prefix is required for anything other than the wildcard
	assert.False(t, knownTarget("Irate", handles, positions, onDutyPositions, teams))
	assert.False(t, knownTarget("", handles, positions, onDutyPositions, teams))
}
