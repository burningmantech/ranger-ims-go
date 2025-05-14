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
	imsDBQ            *store.DBQ
	imsAdmins         []string
	cacheControlShort time.Duration
}

func (action GetIncidentTypes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getIncidentTypes(req)
	if errHTTP != nil {
		errHTTP.From("[getIncidentTypes]").WriteResponse(w)
		return
	}
	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%v, private", action.cacheControlShort.Milliseconds()/1000))
	mustWriteJSON(w, req, resp)
}
func (action GetIncidentTypes) getIncidentTypes(req *http.Request) (imsjson.IncidentTypes, *herr.HTTPError) {
	response := make(imsjson.IncidentTypes, 0)
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.imsAdmins)
	if errHTTP != nil {
		return response, errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalReadIncidentTypes == 0 {
		return response, herr.Forbidden("The requestor does not have GlobalReadIncidentTypes permission", nil)
	}

	if err := req.ParseForm(); err != nil {
		return response, herr.BadRequest("Unable to parse HTTP form", err).From("[ParseForm]")
	}
	includeHidden := req.Form.Get("hidden") == "true"
	typeRows, err := action.imsDBQ.IncidentTypes(req.Context(), action.imsDBQ)
	if err != nil {
		return response, herr.InternalServerError("Failed to fetch Incident Types", err).From("[IncidentTypes]")
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
	imsDBQ    *store.DBQ
	imsAdmins []string
}

func (action EditIncidentTypes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if errHTTP := action.editIncidentTypes(req); errHTTP != nil {
		errHTTP.From("[editIncidentTypes]").WriteResponse(w)
		return
	}
	http.Error(w, "Success", http.StatusNoContent)
}
func (action EditIncidentTypes) editIncidentTypes(req *http.Request) *herr.HTTPError {
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateIncidentTypes == 0 {
		return herr.Forbidden("The requestor does not have GlobalAdministrateIncidentTypes permission", nil)
	}
	ctx := req.Context()
	typesReq, errHTTP := readBodyAs[imsjson.EditIncidentTypesRequest](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}
	for _, it := range typesReq.Add {
		err := action.imsDBQ.CreateIncidentTypeOrIgnore(ctx, action.imsDBQ,
			imsdb.CreateIncidentTypeOrIgnoreParams{
				Name:   it,
				Hidden: false,
			},
		)
		if err != nil {
			return herr.InternalServerError("Failed to create Incident Type", err).From("[CreateIncidentTypeOrIgnore]")
		}
	}
	for _, it := range typesReq.Hide {
		err := action.imsDBQ.HideShowIncidentType(ctx, action.imsDBQ,
			imsdb.HideShowIncidentTypeParams{
				Name:   it,
				Hidden: true,
			},
		)
		if err != nil {
			return herr.InternalServerError("Failed to hide incident type", nil).From("[HideShowIncidentType]")
		}
	}
	for _, it := range typesReq.Show {
		err := action.imsDBQ.HideShowIncidentType(ctx, action.imsDBQ,
			imsdb.HideShowIncidentTypeParams{
				Name:   it,
				Hidden: false,
			},
		)
		if err != nil {
			return herr.InternalServerError("Failed to unhide incident type", err).From("[HideShowIncidentType]")
		}
	}
	return nil
}
