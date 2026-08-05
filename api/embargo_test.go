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
	"time"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/assert"
)

func TestEmbargoed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

	// An unset release time is no embargo at all.
	assert.False(t, embargoed(conv.TimeToNullFloat(time.Time{}), now))

	// A release time in the future embargoes the data it guards.
	assert.True(t, embargoed(conv.TimeToNullFloat(now.Add(time.Second)), now))

	// A release time in the past does not.
	assert.False(t, embargoed(conv.TimeToNullFloat(now.Add(-time.Second)), now))

	// The release time itself counts as released.
	assert.False(t, embargoed(conv.TimeToNullFloat(now), now))
}

func TestPlaceLocationsEmbargoed(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	future := conv.TimeToNullFloat(now.Add(time.Hour))
	past := conv.TimeToNullFloat(now.Add(-time.Hour))

	// Camps and art are embargoed by their own release times, independently.
	campEmbargoed := imsdb.Event{CampLocationsRelease: future, ArtLocationsRelease: past}
	assert.True(t, placeLocationsEmbargoed(campEmbargoed, imsdb.PlaceTypeCamp, now))
	assert.False(t, placeLocationsEmbargoed(campEmbargoed, imsdb.PlaceTypeArt, now))

	artEmbargoed := imsdb.Event{CampLocationsRelease: past, ArtLocationsRelease: future}
	assert.False(t, placeLocationsEmbargoed(artEmbargoed, imsdb.PlaceTypeCamp, now))
	assert.True(t, placeLocationsEmbargoed(artEmbargoed, imsdb.PlaceTypeArt, now))

	// Mutant vehicles and "other" places are never embargoed, even on an event
	// that embargoes everything it can.
	bothEmbargoed := imsdb.Event{CampLocationsRelease: future, ArtLocationsRelease: future}
	assert.False(t, placeLocationsEmbargoed(bothEmbargoed, imsdb.PlaceTypeMv, now))
	assert.False(t, placeLocationsEmbargoed(bothEmbargoed, imsdb.PlaceTypeOther, now))

	// An event with no release times embargoes nothing.
	noEmbargo := imsdb.Event{}
	assert.False(t, placeLocationsEmbargoed(noEmbargo, imsdb.PlaceTypeCamp, now))
	assert.False(t, placeLocationsEmbargoed(noEmbargo, imsdb.PlaceTypeArt, now))
}

func TestRedactPlaceLocation(t *testing.T) {
	t.Parallel()

	// The location string and the structured location both go away, including
	// the external data's own copies of them, while the rest survives.
	camp := imsjson.Place{
		Name:           "Camp Fun Times",
		LocationString: "4:15 & E",
		ExternalData: map[string]any{
			"name":            "Camp Fun Times",
			"description":     "We have fun",
			"location_string": "4:15 & E",
			"location": map[string]any{
				"frontage":       "E",
				"intersection":   "4:15",
				"exact_location": "Mid-block",
			},
		},
	}
	redactPlaceLocation(&camp)
	assert.Equal(t, "Camp Fun Times", camp.Name)
	assert.Empty(t, camp.LocationString)
	assert.Equal(t,
		map[string]any{"name": "Camp Fun Times", "description": "We have fun"},
		camp.ExternalData,
	)

	// A place fetched with exclude_external_data has no external data to redact.
	art := imsjson.Place{Name: "Art Thing", LocationString: "9:00 500'"}
	redactPlaceLocation(&art)
	assert.Equal(t, "Art Thing", art.Name)
	assert.Empty(t, art.LocationString)
	assert.Nil(t, art.ExternalData)
}
