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
	"context"
	"encoding/json"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/bmapi"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"net/http"
	"strings"
	"time"
)

type GetPlaces struct {
	imsDBQ            *store.DBQ
	userStore         *directory.UserStore
	imsAdmins         []string
	cacheControlShort time.Duration
}

func (action GetPlaces) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.run(req)
	if errHTTP != nil {
		errHTTP.From("[run]").WriteResponse(w)
		return
	}
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%v, private", action.cacheControlShort.Milliseconds()/1000))
	mustWriteJSON(w, req, resp)
}

func (action GetPlaces) run(req *http.Request) (imsjson.Places, *herr.HTTPError) {
	ctx := req.Context()
	resp := make(imsjson.Places)
	event, _, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return nil, errHTTP.From("[getEventPermissions]")
	}
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return nil, errHTTP.From("[getGlobalPermissions]")
	}
	if eventPermissions&authz.EventReadPlaces == 0 && globalPermissions&authz.GlobalAdministratePlaces == 0 {
		return nil, herr.Forbidden("The requestor does not have EventReadPlaces permission", nil)
	}
	err := req.ParseForm()
	if err != nil {
		return nil, herr.BadRequest("Failed to parse form", err)
	}
	excludeExternalData := strings.EqualFold(req.Form.Get("exclude_external_data"), "true")

	places, err := action.imsDBQ.Places(ctx, action.imsDBQ,
		imsdb.PlacesParams{
			Event:               event.ID,
			ExcludeExternalData: excludeExternalData,
		},
	)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch Places", err).From("[Places]")
	}

	isAdmin := globalPermissions&authz.GlobalAdministratePlaces != 0
	now := time.Now()
	for _, rowDest := range places {
		dType := string(rowDest.Type)
		apiDest := imsjson.Place{
			Name:           rowDest.Name,
			LocationString: rowDest.LocationString,
		}
		if !excludeExternalData {
			ed := make(map[string]any)
			err := json.Unmarshal(rowDest.ExternalData.([]byte), &ed)
			if err != nil {
				return nil, herr.InternalServerError("Failed to unmarshal place", err).From("[Unmarshal]")
			}
			apiDest.ExternalData = ed
		}
		if !isAdmin && placeLocationsEmbargoed(event, rowDest.Type, now) {
			redactPlaceLocation(&apiDest)
		}
		resp[dType] = append(resp[dType], apiDest)
	}

	return resp, nil
}

type UpdatePlaces struct {
	imsDBQ            *store.DBQ
	userStore         *directory.UserStore
	imsAdmins         []string
	cacheControlShort time.Duration
}

func (action UpdatePlaces) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.run(req)
	if errHTTP != nil {
		errHTTP.From("[run]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action UpdatePlaces) run(req *http.Request) *herr.HTTPError {
	ctx := req.Context()
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministratePlaces == 0 {
		return herr.Forbidden("The requestor does not have GlobalAdministratePlaces permission", nil)
	}
	event, errHTTP := getEvent(req, req.PathValue("eventName"), action.imsDBQ)
	if errHTTP != nil {
		return errHTTP.From("[getEvent]")
	}
	destByType, errHTTP := readBodyAs[imsjson.Places](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}

	// for each type supplied, delete everything we have currently for that type
	// before adding in everything from the request.
	for dType, dests := range destByType {
		errHTTP = replacePlaces(ctx, action.imsDBQ, event.ID, imsdb.PlaceType(dType), dests)
		if errHTTP != nil {
			return errHTTP.From("[replacePlaces]")
		}
	}

	return nil
}

// replacePlaces swaps out everything the event has of one place type for the
// places provided.
func replacePlaces(
	ctx context.Context, imsDBQ *store.DBQ, eventID int32, placeType imsdb.PlaceType, places []imsjson.Place,
) *herr.HTTPError {
	err := imsDBQ.RemovePlaces(ctx, imsDBQ,
		imsdb.RemovePlacesParams{
			Event: eventID,
			Type:  placeType,
		},
	)
	if err != nil {
		return herr.InternalServerError("Failed to remove places", err).From("[RemovePlaces]")
	}

	for i, d := range places {
		marshal, err := json.Marshal(d.ExternalData)
		if err != nil {
			return herr.InternalServerError("Failed to marshal place", err).From("[Marshal]")
		}
		err = imsDBQ.CreatePlace(ctx, imsDBQ,
			imsdb.CreatePlaceParams{
				Event:          eventID,
				Number:         int32(i),
				Type:           placeType,
				Name:           d.Name,
				LocationString: d.LocationString,
				ExternalData:   marshal,
			},
		)
		if err != nil {
			return herr.InternalServerError("Failed to create place", err).From("[UpdatePlace]")
		}
	}
	return nil
}

// ImportPlaces replaces one place type's places for an event with what the
// Burning Man API has for a given year. The year is a request parameter rather
// than something derived from the event, since an IMS event isn't necessarily
// named for the year whose data it wants.
type ImportPlaces struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
	bmAPI     conf.BurningManAPI
}

type ImportPlacesResponse struct {
	// Count is how many places were stored.
	Count int `json:"count"`
}

func (action ImportPlaces) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.run(req)
	if errHTTP != nil {
		errHTTP.From("[run]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}

func (action ImportPlaces) run(req *http.Request) (ImportPlacesResponse, *herr.HTTPError) {
	ctx := req.Context()
	var resp ImportPlacesResponse
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return resp, errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministratePlaces == 0 {
		return resp, herr.Forbidden("The requestor does not have GlobalAdministratePlaces permission", nil)
	}
	if !action.bmAPI.Enabled() {
		return resp, herr.New(http.StatusServiceUnavailable,
			"This server has no Burning Man API key configured", nil)
	}
	event, errHTTP := getEvent(req, req.PathValue("eventName"), action.imsDBQ)
	if errHTTP != nil {
		return resp, errHTTP.From("[getEvent]")
	}
	err := req.ParseForm()
	if err != nil {
		return resp, herr.BadRequest("Failed to parse form", err)
	}
	kind, err := bmapi.ParseKind(req.Form.Get("place_type"))
	if err != nil {
		return resp, herr.BadRequest(err.Error(), err)
	}
	year, err := conv.ParseInt32(req.Form.Get("year"))
	if err != nil {
		return resp, herr.BadRequest("The year must be a number", err)
	}

	records, err := bmapi.NewClient(action.bmAPI.URL, action.bmAPI.APIKey).Fetch(ctx, kind, year)
	if err != nil {
		return resp, herr.New(http.StatusBadGateway,
			fmt.Sprintf("Failed to fetch %v data for %v from the Burning Man API: %v", kind, year, err),
			err,
		).From("[Fetch]")
	}
	// A year the API has no data for comes back as an empty array, which would
	// otherwise silently wipe out whatever the event already had.
	if len(records) == 0 {
		return resp, herr.New(http.StatusBadGateway,
			fmt.Sprintf("The Burning Man API has no %v data for %v. Nothing was changed.", kind, year),
			nil,
		)
	}

	places := make([]imsjson.Place, 0, len(records))
	for _, r := range records {
		places = append(places, imsjson.Place{
			Name:           r.Name,
			LocationString: r.LocationString,
			ExternalData:   r.Raw,
		})
	}
	errHTTP = replacePlaces(ctx, action.imsDBQ, event.ID, imsdb.PlaceType(kind), places)
	if errHTTP != nil {
		return resp, errHTTP.From("[replacePlaces]")
	}

	resp.Count = len(places)
	return resp, nil
}
