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
	"testing"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildIncidentUpdateNormalizesLocationAddress(t *testing.T) {
	t.Parallel()

	address := "4&esp"
	update, logs := buildIncidentUpdate(imsdb.Incident{}, imsjson.Incident{
		Location: imsjson.Location{Address: &address},
	}, true)
	assert.Equal(t, "4:00 & Esplanade", update.LocationAddress.String)
	assert.Contains(t, logs, "Changed location address: 4:00 & Esplanade")

	freeform := "behind the porta potties"
	update, _ = buildIncidentUpdate(imsdb.Incident{}, imsjson.Incident{
		Location: imsjson.Location{Address: &freeform},
	}, true)
	assert.Equal(t, "behind the porta potties", update.LocationAddress.String)

	// An absent address leaves the stored one alone.
	stored := imsdb.Incident{}
	stored.LocationAddress.String = "9:00 & A"
	stored.LocationAddress.Valid = true
	update, logs = buildIncidentUpdate(stored, imsjson.Incident{}, true)
	assert.Equal(t, "9:00 & A", update.LocationAddress.String)
	assert.Empty(t, logs)
}

func TestBuildVisitUpdateNormalizesGuestCampAddress(t *testing.T) {
	t.Parallel()

	address := "e+7"
	update, logs, errHTTP := buildVisitUpdate(imsdb.Visit{}, imsjson.Visit{
		GuestCampAddress: &address,
	}, true)
	require.Nil(t, errHTTP)
	assert.Equal(t, "E & 7:00", update.GuestCampAddress.String)
	assert.Contains(t, logs, "Changed GuestCampAddress: E & 7:00")
}

func TestBuildUpdatesLeaveAddressesAloneWhenTheEventDoesNotNormalize(t *testing.T) {
	t.Parallel()

	address := "4&esp"
	incidentUpdate, logs := buildIncidentUpdate(imsdb.Incident{}, imsjson.Incident{
		Location: imsjson.Location{Address: &address},
	}, false)
	assert.Equal(t, "4&esp", incidentUpdate.LocationAddress.String)
	assert.Contains(t, logs, "Changed location address: 4&esp")

	campAddress := "e+7"
	visitUpdate, logs, errHTTP := buildVisitUpdate(imsdb.Visit{}, imsjson.Visit{
		GuestCampAddress: &campAddress,
	}, false)
	require.Nil(t, errHTTP)
	assert.Equal(t, "e+7", visitUpdate.GuestCampAddress.String)
	assert.Contains(t, logs, "Changed GuestCampAddress: e+7")
}
