package api

import (
	"github.com/burningmantech/ranger-ims-go/auth"
	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/store"
	"net/http"
)

type GetPersonnel struct {
	imsDB     *store.DB
	userStore *directory.UserStore
	imsAdmins []string
}

type GetPersonnelResponse []imsjson.Person

func (action GetPersonnel) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	response := make(GetPersonnelResponse, 0)
	_, globalPermissions, ok := mustGetGlobalPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if globalPermissions&auth.GlobalReadPersonnel == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have GlobalReadPersonnel permission", nil)
		return
	}

	rangers, err := action.userStore.GetRangers(req.Context())
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to get personnel", nil)
		return
	}

	for _, ranger := range rangers {
		response = append(response, imsjson.Person{
			Handle: ranger.Handle,
			// Don't send email addresses in the API.
			// This is also done as a backstop in imsjson.Person itself, with `json:"-"`
			Email: "",
			// Don't send passwords in the API
			// This is also done as a backstop in imsjson.Person itself, with `json:"-"`
			Password:    "",
			Status:      ranger.Status,
			Onsite:      ranger.Onsite,
			DirectoryID: ranger.DirectoryID,
		})
	}

	w.Header().Set("Cache-Control", "max-age=1200, private")
	mustWriteJSON(w, response)
}
