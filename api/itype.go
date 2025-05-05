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
	response := make(imsjson.IncidentTypes, 0)
	_, globalPermissions, ok := mustGetGlobalPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if globalPermissions&authz.GlobalReadIncidentTypes == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have GlobalReadIncidentTypes permission", nil)
		return
	}

	if success := mustParseForm(w, req); !success {
		return
	}
	includeHidden := req.Form.Get("hidden") == "true"
	typeRows, err := imsdb.New(action.imsDB).IncidentTypes(req.Context())
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Incident Types", nil)
		return
	}

	for _, typeRow := range typeRows {
		t := typeRow.IncidentType
		if includeHidden || !t.Hidden {
			response = append(response, t.Name)
		}
	}
	slices.Sort(response)

	w.Header().Set("Cache-Control", fmt.Sprintf("max-age=%v, private", action.cacheControlShort.Milliseconds()/1000))
	mustWriteJSON(w, response)
}

type EditIncidentTypes struct {
	imsDB     *store.DB
	imsAdmins []string
}

func (action EditIncidentTypes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	_, globalPermissions, ok := mustGetGlobalPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if globalPermissions&authz.GlobalAdministrateIncidentTypes == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have GlobalAdministrateIncidentTypes permission", nil)
		return
	}
	ctx := req.Context()
	typesReq, ok := mustReadBodyAs[imsjson.EditIncidentTypesRequest](w, req)
	if !ok {
		return
	}
	for _, it := range typesReq.Add {
		err := imsdb.New(action.imsDB).CreateIncidentTypeOrIgnore(ctx, imsdb.CreateIncidentTypeOrIgnoreParams{
			Name:   it,
			Hidden: false,
		})
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to create incident type", nil)
			return
		}
	}
	for _, it := range typesReq.Hide {
		err := imsdb.New(action.imsDB).HideShowIncidentType(ctx, imsdb.HideShowIncidentTypeParams{
			Name:   it,
			Hidden: true,
		})
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to hide incident type", nil)
			return
		}
	}
	for _, it := range typesReq.Show {
		err := imsdb.New(action.imsDB).HideShowIncidentType(ctx, imsdb.HideShowIncidentTypeParams{
			Name:   it,
			Hidden: false,
		})
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to unhide incident type", nil)
			return
		}
	}
	http.Error(w, "Success", http.StatusNoContent)
}
