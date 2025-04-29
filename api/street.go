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
	"github.com/burningmantech/ranger-ims-go/auth"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"net/http"
)

type GetStreets struct {
	imsDB     *store.DB
	imsAdmins []string
}

func (action GetStreets) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// eventName --> street ID --> street name
	resp := make(imsjson.EventsStreets)
	_, globalPermissions, ok := mustGetGlobalPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if globalPermissions&auth.GlobalReadStreets == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have GlobalReadStreets permission", nil)
		return
	}

	if ok := mustParseForm(w, req); !ok {
		return
	}
	eventName := req.Form.Get("event_id")
	var events []imsdb.Event
	if eventName != "" {
		event, ok := mustEventFromFormValue(w, req, action.imsDB)
		if !ok {
			return
		}
		events = append(events, imsdb.Event{ID: event.ID, Name: event.Name})
	} else {
		eventRows, err := imsdb.New(action.imsDB).Events(req.Context())
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Events", err)
			return
		}
		for _, er := range eventRows {
			events = append(events, er.Event)
		}
	}

	for _, event := range events {
		streets, err := imsdb.New(action.imsDB).ConcentricStreets(req.Context(), event.ID)
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Streets", err)
			return
		}
		resp[event.Name] = make(imsjson.EventStreets)
		for _, street := range streets {
			resp[event.Name][street.ConcentricStreet.ID] = street.ConcentricStreet.Name
		}
	}
	w.Header().Set("Cache-Control", "max-age=1200, private")
	mustWriteJSON(w, resp)
}

type EditStreets struct {
	imsDB     *store.DB
	imsAdmins []string
}

func (action EditStreets) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	_, globalPermissions, ok := mustGetGlobalPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if globalPermissions&auth.GlobalAdministrateStreets == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have GlobalAdministrateStreets permission", nil)
		return
	}
	eventsStreets, ok := mustReadBodyAs[imsjson.EventsStreets](w, req)
	if !ok {
		return
	}
	for eventName, newEventStreets := range eventsStreets {
		event, ok := mustGetEvent(w, req, eventName, action.imsDB)
		if !ok {
			return
		}
		currentStreets, err := imsdb.New(action.imsDB).ConcentricStreets(req.Context(), event.ID)
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Streets", err)
			return
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
					handleErr(w, req, http.StatusInternalServerError, "Failed to create Street", err)
					return
				}
			}
		}
	}
	http.Error(w, "Success", http.StatusNoContent)
}
