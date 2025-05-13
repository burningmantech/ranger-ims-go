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
	"slices"
	"time"
)

type GetIncidentTypes struct {
	imsDB             *store.DB
	imsAdmins         []string
	cacheControlShort time.Duration
}

func (action GetIncidentTypes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errH := action.getIncidentTypes(req)
	if errH != nil {
		errH.Src("[getIncidentTypes]").WriteResponse(w)
		return
	}
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%v, private", action.cacheControlShort.Milliseconds()/1000))
	mustWriteJSON(w, resp)
}
func (action GetIncidentTypes) getIncidentTypes(req *http.Request) (imsjson.IncidentTypes, *herr.HTTPError) {
	response := make(imsjson.IncidentTypes, 0)
	_, globalPermissions, errH := mustGetGlobalPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return response, errH.Src("[mustGetGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalReadIncidentTypes == 0 {
		return response, herr.S403("The requestor does not have GlobalReadIncidentTypes permission", nil)
	}

	if err := req.ParseForm(); err != nil {
		return response, herr.S400("Unable to parse HTTP form", err).Src("[ParseForm]")
	}
	includeHidden := req.Form.Get("hidden") == "true"
	typeRows, err := imsdb.New(action.imsDB).IncidentTypes(req.Context())
	if err != nil {
		return response, herr.S500("Failed to fetch Incident Types", err).Src("[IncidentTypes]")
	}

	for _, typeRow := range typeRows {
		t := typeRow.IncidentType
		if includeHidden || !t.Hidden {
			response = append(response, t.Name)
		}
	}
	slices.Sort(response)

	return response, nil
}

type EditIncidentTypes struct {
	imsDB     *store.DB
	imsAdmins []string
}

func (action EditIncidentTypes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if hErr := action.editIncidentTypes(req); hErr != nil {
		hErr.Src("[editIncidentTypes]").WriteResponse(w)
		return
	}
	http.Error(w, "Success", http.StatusNoContent)
}
func (action EditIncidentTypes) editIncidentTypes(req *http.Request) *herr.HTTPError {
	_, globalPermissions, errH := mustGetGlobalPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return errH.Src("[mustGetGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateIncidentTypes == 0 {
		return herr.S403("The requestor does not have GlobalAdministrateIncidentTypes permission", nil)
	}
	ctx := req.Context()
	typesReq, errH := mustReadBodyAs[imsjson.EditIncidentTypesRequest](req)
	if errH != nil {
		return errH.Src("[mustReadBodyAs]")
	}
	for _, it := range typesReq.Add {
		err := imsdb.New(action.imsDB).CreateIncidentTypeOrIgnore(ctx, imsdb.CreateIncidentTypeOrIgnoreParams{
			Name:   it,
			Hidden: false,
		})
		if err != nil {
			return herr.S500("Failed to create Incident Type", err).Src("[CreateIncidentTypeOrIgnore]")
		}
	}
	for _, it := range typesReq.Hide {
		err := imsdb.New(action.imsDB).HideShowIncidentType(ctx, imsdb.HideShowIncidentTypeParams{
			Name:   it,
			Hidden: true,
		})
		if err != nil {
			return herr.S500("Failed to hide incident type", nil).Src("[HideShowIncidentType]")
		}
	}
	for _, it := range typesReq.Show {
		err := imsdb.New(action.imsDB).HideShowIncidentType(ctx, imsdb.HideShowIncidentTypeParams{
			Name:   it,
			Hidden: false,
		})
		if err != nil {
			return herr.S500("Failed to unhide incident type", err).Src("[HideShowIncidentType]")
		}
	}
	return nil
}
