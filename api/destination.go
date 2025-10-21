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

type GetDestinations struct {
	imsDBQ            *store.DBQ
	userStore         *directory.UserStore
	imsAdmins         []string
	cacheControlShort time.Duration
}

func (action GetDestinations) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.run(req)
	if errHTTP != nil {
		errHTTP.From("[run]").WriteResponse(w)
		return
	}
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%v, private", action.cacheControlShort.Milliseconds()/1000))
	mustWriteJSON(w, req, resp)
}

func (action GetDestinations) run(req *http.Request) (imsjson.Destinations, *herr.HTTPError) {
	ctx := req.Context()
	resp := make(imsjson.Destinations)
	event, _, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return nil, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventReadDestinations == 0 {
		return nil, herr.Forbidden("The requestor does not have EventReadDestinations permission", nil)
	}
	err := req.ParseForm()
	if err != nil {
		return nil, herr.BadRequest("Failed to parse form", err)
	}
	includeExternalData := !strings.EqualFold(req.Form.Get("exclude_external_data"), "true")

	// TODO: it'd be faster to not load the external_data column if the client doesn't want it
	destinations, err := action.imsDBQ.Destinations(ctx, action.imsDBQ, event.ID)
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch Destinations", err).From("[Destinations]")
	}

	for _, row := range destinations {
		rowDest := row.Destination
		ed := make(map[string]any)
		err := json.Unmarshal(rowDest.ExternalData, &ed)
		if err != nil {
			return nil, herr.InternalServerError("Failed to unmarshal destination", err).From("[Unmarshal]")
		}
		dType := string(rowDest.Type)
		apiDest := imsjson.Destination{
			Name:           rowDest.Name,
			LocationString: rowDest.LocationString,
		}
		if includeExternalData {
			apiDest.ExternalData = ed
		}
		resp[dType] = append(resp[dType], apiDest)
	}

	return resp, nil
}

type UpdateDestinations struct {
	imsDBQ            *store.DBQ
	userStore         *directory.UserStore
	imsAdmins         []string
	cacheControlShort time.Duration
}

func (action UpdateDestinations) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.run(req)
	if errHTTP != nil {
		errHTTP.From("[run]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action UpdateDestinations) run(req *http.Request) *herr.HTTPError {
	ctx := req.Context()
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateDestinations == 0 {
		return herr.Forbidden("The requestor does not have GlobalAdministrateDestinations permission", nil)
	}
	event, errHTTP := getEvent(req, req.PathValue("eventName"), action.imsDBQ)
	if errHTTP != nil {
		return errHTTP.From("[getEvent]")
	}
	destByType, errHTTP := readBodyAs[imsjson.Destinations](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}

	// for each type supplied, delete everything we have currently for that type
	// before adding in everything from the request.
	for dType, dests := range destByType {
		err := action.imsDBQ.RemoveDestinations(ctx, action.imsDBQ,
			imsdb.RemoveDestinationsParams{
				Event: event.ID,
				Type:  imsdb.DestinationType(dType),
			},
		)
		if err != nil {
			return herr.InternalServerError("Failed to remove destinations", err).From("[RemoveDestinations]")
		}

		for i, d := range dests {
			marshal, err := json.Marshal(d.ExternalData)
			if err != nil {
				return herr.InternalServerError("Failed to marshal destination", err).From("[Marshal]")
			}
			err = action.imsDBQ.CreateDestination(ctx, action.imsDBQ,
				imsdb.CreateDestinationParams{
					Event:          event.ID,
					Number:         int32(i),
					Type:           imsdb.DestinationType(dType),
					Name:           d.Name,
					LocationString: d.LocationString,
					ExternalData:   marshal,
				},
			)
			if err != nil {
				return herr.InternalServerError("Failed to create destination", err).From("[UpdateDestination]")
			}
		}
	}

	return nil
}
