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
	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"net/http"
	"time"
)

type GetStreets struct {
	imsDBQ            *store.DBQ
	userStore         *directory.UserStore
	imsAdmins         []string
	cacheControlShort time.Duration
}

func (action GetStreets) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getStreets(req)
	if errHTTP != nil {
		errHTTP.From("[getStreets]").WriteResponse(w)
		return
	}
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%v, private", action.cacheControlShort.Milliseconds()/1000))
	mustWriteJSON(w, req, resp)
}

func (action GetStreets) getStreets(req *http.Request) (imsjson.EventsStreets, *herr.HTTPError) {
	ctx := req.Context()
	// eventName --> street ID --> street name
	resp := make(imsjson.EventsStreets)
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return nil, errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalReadStreets == 0 {
		return nil, herr.Forbidden("The requestor does not have GlobalReadStreets permission", nil)
	}

	err := req.ParseForm()
	if err != nil {
		return nil, herr.BadRequest("Failed to parse form", err)
	}
	eventName := req.Form.Get("event_id")
	var events []imsdb.Event
	if eventName != "" {
		event, errHTTP := eventFromFormValue(req, action.imsDBQ)
		if errHTTP != nil {
			return nil, errHTTP.From("[eventFromFormValue]")
		}
		events = append(events, imsdb.Event{ID: event.ID, Name: event.Name})
	} else {
		eventRows, err := action.imsDBQ.Events(ctx, action.imsDBQ)
		if err != nil {
			return nil, herr.InternalServerError("Failed to fetch Events", err).From("[Events]")
		}
		for _, er := range eventRows {
			events = append(events, er.Event)
		}
	}

	for _, event := range events {
		streets, err := action.imsDBQ.ConcentricStreets(ctx, action.imsDBQ, event.ID)
		if err != nil {
			return nil, herr.InternalServerError("Failed to fetch Streets", err).From("[ConcentricStreets]")
		}
		resp[event.Name] = make(imsjson.EventStreets)
		for _, street := range streets {
			resp[event.Name][street.ConcentricStreet.ID] = street.ConcentricStreet.Name
		}
	}
	return resp, nil
}

type EditStreets struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

func (action EditStreets) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.editStreets(req)
	if errHTTP != nil {
		errHTTP.From("[editStreets]").WriteResponse(w)
		return
	}
	http.Error(w, "Success", http.StatusNoContent)
}

func (action EditStreets) editStreets(req *http.Request) *herr.HTTPError {
	ctx := req.Context()
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateStreets == 0 {
		return herr.Forbidden("The requestor does not have GlobalAdministrateStreets permission", nil)
	}
	eventsStreets, errHTTP := readBodyAs[imsjson.EventsStreets](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}
	for eventName, newEventStreets := range eventsStreets {
		event, errHTTP := getEvent(req, eventName, action.imsDBQ)
		if errHTTP != nil {
			return errHTTP.From("[getEvent]")
		}
		currentStreets, err := action.imsDBQ.ConcentricStreets(req.Context(), action.imsDBQ, event.ID)
		if err != nil {
			return herr.InternalServerError("Failed to fetch Streets", err).From("[ConcentricStreets]")
		}
		currentStreetIDs := make(map[string]bool)
		for _, street := range currentStreets {
			currentStreetIDs[street.ConcentricStreet.ID] = true
		}
		for streetID, streetName := range newEventStreets {
			if !currentStreetIDs[streetID] {
				err = action.imsDBQ.CreateConcentricStreet(ctx, action.imsDBQ,
					imsdb.CreateConcentricStreetParams{
						Event: event.ID,
						ID:    streetID,
						Name:  streetName,
					},
				)
				if err != nil {
					return herr.InternalServerError("Failed to create Street", err).From("[CreateConcentricStreet]")
				}
			}
		}
	}
	return nil
}
