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
	"fmt"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"net/http"
	"time"
)

type GetStreets struct {
	imsDB             *store.DB
	imsAdmins         []string
	cacheControlShort time.Duration
}

func (action GetStreets) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, hErr := action.getStreets(req)
	if hErr != nil {
		hErr.Src("[getStreets]").WriteResponse(w)
		return
	}
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%v, private", action.cacheControlShort.Milliseconds()/1000))
	mustWriteJSON(w, resp)
}

func (action GetStreets) getStreets(req *http.Request) (imsjson.EventsStreets, *herr.HTTPError) {
	ctx := req.Context()
	// eventName --> street ID --> street name
	resp := make(imsjson.EventsStreets)
	_, globalPermissions, hErr := mustGetGlobalPermissions(req, action.imsDB, action.imsAdmins)
	if hErr != nil {
		return nil, hErr.Src("[mustGetGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalReadStreets == 0 {
		return nil, herr.S403("The requestor does not have GlobalReadStreets permission", nil)
	}

	if err := req.ParseForm(); err != nil {
		return nil, herr.S400("Failed to parse form", err)
	}
	eventName := req.Form.Get("event_id")
	var events []imsdb.Event
	if eventName != "" {
		event, hErr := mustEventFromFormValue2(req, action.imsDB)
		if hErr != nil {
			return nil, hErr.Src("[mustEventFromFormValue2]")
		}
		events = append(events, imsdb.Event{ID: event.ID, Name: event.Name})
	} else {
		eventRows, err := imsdb.New(action.imsDB).Events(ctx)
		if err != nil {
			return nil, herr.S500("Failed to fetch Events", err).Src("[Events]")
		}
		for _, er := range eventRows {
			events = append(events, er.Event)
		}
	}

	for _, event := range events {
		streets, err := imsdb.New(action.imsDB).ConcentricStreets(ctx, event.ID)
		if err != nil {
			return nil, herr.S500("Failed to fetch Streets", err).Src("[ConcentricStreets]")
		}
		resp[event.Name] = make(imsjson.EventStreets)
		for _, street := range streets {
			resp[event.Name][street.ConcentricStreet.ID] = street.ConcentricStreet.Name
		}
	}
	return resp, nil
}

type EditStreets struct {
	imsDB     *store.DB
	imsAdmins []string
}

func (action EditStreets) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if hErr := action.editStreets(req); hErr != nil {
		hErr.Src("[editStreets]").WriteResponse(w)
		return
	}
	http.Error(w, "Success", http.StatusNoContent)
}

func (action EditStreets) editStreets(req *http.Request) *herr.HTTPError {
	ctx := req.Context()
	_, globalPermissions, hErr := mustGetGlobalPermissions(req, action.imsDB, action.imsAdmins)
	if hErr != nil {
		return hErr.Src("[mustGetGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateStreets == 0 {
		return herr.S403("The requestor does not have GlobalAdministrateStreets permission", nil)
	}
	eventsStreets, hErr := mustReadBodyAs[imsjson.EventsStreets](req)
	if hErr != nil {
		return hErr.Src("[mustReadBodyAs]")
	}
	for eventName, newEventStreets := range eventsStreets {
		event, hErr := mustGetEvent(req, eventName, action.imsDB)
		if hErr != nil {
			return hErr.Src("[mustGetEvent]")
		}
		currentStreets, err := imsdb.New(action.imsDB).ConcentricStreets(req.Context(), event.ID)
		if err != nil {
			return herr.S500("Failed to fetch Streets", err).Src("[ConcentricStreets]")
		}
		currentStreetIDs := make(map[string]bool)
		for _, street := range currentStreets {
			currentStreetIDs[street.ConcentricStreet.ID] = true
		}
		for streetID, streetName := range newEventStreets {
			if !currentStreetIDs[streetID] {
				err = imsdb.New(action.imsDB).CreateConcentricStreet(ctx, imsdb.CreateConcentricStreetParams{
					Event: event.ID,
					ID:    streetID,
					Name:  streetName,
				})
				if err != nil {
					return herr.S500("Failed to create Street", err).Src("[CreateConcentricStreet]")
				}
			}
		}
	}
	return nil
}
