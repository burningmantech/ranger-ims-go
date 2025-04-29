package api

import (
	"github.com/srabraham/ranger-ims-go/auth"
	imsjson "github.com/srabraham/ranger-ims-go/json"
	"github.com/srabraham/ranger-ims-go/store"
	"github.com/srabraham/ranger-ims-go/store/imsdb"
	"net/http"
	"slices"
)

type GetIncidentTypes struct {
	imsDB     *store.DB
	imsAdmins []string
}

func (action GetIncidentTypes) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	response := make(imsjson.IncidentTypes, 0)
	_, globalPermissions, ok := mustGetGlobalPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if globalPermissions&auth.GlobalReadIncidentTypes == 0 {
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

	w.Header().Set("Cache-Control", "max-age=1200, private")
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
	if globalPermissions&auth.GlobalAdministrateIncidentTypes == 0 {
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
