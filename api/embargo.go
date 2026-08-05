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
	"database/sql"
	"time"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
)

// Burning Man doesn't publish camp and art placement until shortly before the
// event, so an Event can carry release times that embargo that data (and the
// event's map link) until then. Only IMS admins see embargoed data.

// embargoed says whether the given release time is still in the future, i.e.
// whether the data it guards must be withheld. An unset release time means the
// data was never embargoed.
func embargoed(release sql.NullFloat64, now time.Time) bool {
	if !release.Valid {
		return false
	}
	return now.Before(conv.FloatToTime(release.Float64))
}

// redactPlaceLocation removes the location details from an embargoed Place: the
// location string, plus the external data's own copies of it. That external
// data is the Burning Man API's object for the place, which repeats the
// location string and holds the same placement in structured form (a camp's
// frontage and intersection, or an art piece's GPS coordinates). The rest of
// the place, e.g. its name and description, still goes out.
func redactPlaceLocation(place *imsjson.Place) {
	place.LocationString = ""
	if ed, ok := place.ExternalData.(map[string]any); ok {
		delete(ed, "location")
		delete(ed, "location_string")
	}
}

// placeLocationsEmbargoed says whether the given event withholds locations for
// the given type of Place from non-admins right now. Only camps and art are
// ever embargoed; mutant vehicles carry no location, and "other" places are
// IMS's own data rather than something Burning Man is holding back.
func placeLocationsEmbargoed(event imsdb.Event, placeType imsdb.PlaceType, now time.Time) bool {
	switch placeType {
	case imsdb.PlaceTypeCamp:
		return embargoed(event.CampLocationsRelease, now)
	case imsdb.PlaceTypeArt:
		return embargoed(event.ArtLocationsRelease, now)
	case imsdb.PlaceTypeMv, imsdb.PlaceTypeOther:
		return false
	}
	return false
}
