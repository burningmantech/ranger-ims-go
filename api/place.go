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
	"encoding/json"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
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
		err := action.imsDBQ.RemovePlaces(ctx, action.imsDBQ,
			imsdb.RemovePlacesParams{
				Event: event.ID,
				Type:  imsdb.PlaceType(dType),
			},
		)
		if err != nil {
			return herr.InternalServerError("Failed to remove places", err).From("[RemovePlaces]")
		}

		for i, d := range dests {
			marshal, err := json.Marshal(d.ExternalData)
			if err != nil {
				return herr.InternalServerError("Failed to marshal place", err).From("[Marshal]")
			}
			err = action.imsDBQ.CreatePlace(ctx, action.imsDBQ,
				imsdb.CreatePlaceParams{
					Event:          event.ID,
					Number:         int32(i),
					Type:           imsdb.PlaceType(dType),
					Name:           d.Name,
					LocationString: d.LocationString,
					ExternalData:   marshal,
				},
			)
			if err != nil {
				return herr.InternalServerError("Failed to create place", err).From("[UpdatePlace]")
			}
		}
	}

	return nil
}
