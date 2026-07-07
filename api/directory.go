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
	"crypto/rand"
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strconv"

	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/argon2id"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
)

const (
	maxDirectoryHandleLen   = 128
	maxDirectoryEmailLen    = 256
	maxDirectoryTitleLen    = 128
	maxDirectoryPasswordLen = 256
)

// requireDirectoryAdmin does the checks common to all the directory admin
// endpoints: the deployment must use the IMS-native directory, and the
// requestor must have GlobalAdministrateDirectory permission.
func requireDirectoryAdmin(
	req *http.Request,
	imsDBQ *store.DBQ,
	userStore *directory.UserStore,
	imsAdmins []string,
	directoryIsIMS bool,
) *herr.HTTPError {
	if !directoryIsIMS {
		return herr.Forbidden(
			"This deployment's user directory is not managed by IMS "+
				"(IMS_DIRECTORY is not 'ims'), so it cannot be administered here",
			nil,
		)
	}
	_, globalPermissions, errHTTP := getGlobalPermissions(req, imsDBQ, userStore, imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateDirectory == 0 {
		return herr.Forbidden("The requestor does not have GlobalAdministrateDirectory permission", nil)
	}
	return nil
}

type GetDirectory struct {
	imsDBQ         *store.DBQ
	userStore      *directory.UserStore
	imsAdmins      []string
	directoryIsIMS bool
}

func (action GetDirectory) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getDirectory(req)
	if errHTTP != nil {
		errHTTP.From("[getDirectory]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}
func (action GetDirectory) getDirectory(req *http.Request) (imsjson.Directory, *herr.HTTPError) {
	empty := imsjson.Directory{}
	errHTTP := requireDirectoryAdmin(req, action.imsDBQ, action.userStore, action.imsAdmins, action.directoryIsIMS)
	if errHTTP != nil {
		return empty, errHTTP.From("[requireDirectoryAdmin]")
	}
	ctx := req.Context()

	var errs []error
	persons, err := action.imsDBQ.DirectoryAllPersons(ctx, action.imsDBQ)
	errs = append(errs, err)
	teams, err := action.imsDBQ.DirectoryAllTeams(ctx, action.imsDBQ)
	errs = append(errs, err)
	positions, err := action.imsDBQ.DirectoryAllPositions(ctx, action.imsDBQ)
	errs = append(errs, err)
	personTeams, err := action.imsDBQ.DirectoryPersonTeams(ctx, action.imsDBQ)
	errs = append(errs, err)
	personPositions, err := action.imsDBQ.DirectoryPersonPositions(ctx, action.imsDBQ)
	errs = append(errs, err)
	err = errors.Join(errs...)
	if err != nil {
		return empty, herr.InternalServerError("Failed to fetch directory", err).From("[DirectoryAll*]")
	}

	teamIDsByPerson := make(map[int64][]int64)
	for _, pt := range personTeams {
		teamIDsByPerson[pt.PersonID] = append(teamIDsByPerson[pt.PersonID], pt.TeamID)
	}
	positionIDsByPerson := make(map[int64][]int64)
	for _, pp := range personPositions {
		positionIDsByPerson[pp.PersonID] = append(positionIDsByPerson[pp.PersonID], pp.PositionID)
	}

	resp := imsjson.Directory{
		Persons:   make([]imsjson.DirectoryPerson, 0, len(persons)),
		Teams:     make([]imsjson.DirectoryGroup, 0, len(teams)),
		Positions: make([]imsjson.DirectoryGroup, 0, len(positions)),
	}
	for _, p := range persons {
		teamIDs := teamIDsByPerson[p.ID]
		if teamIDs == nil {
			teamIDs = []int64{}
		}
		positionIDs := positionIDsByPerson[p.ID]
		if positionIDs == nil {
			positionIDs = []int64{}
		}
		slices.Sort(teamIDs)
		slices.Sort(positionIDs)
		resp.Persons = append(resp.Persons, imsjson.DirectoryPerson{
			ID:          p.ID,
			Handle:      new(p.Handle),
			Email:       conv.SqlToString(p.Email),
			Active:      new(p.Active),
			Onsite:      new(p.Onsite),
			TeamIDs:     &teamIDs,
			PositionIDs: &positionIDs,
		})
	}
	for _, t := range teams {
		resp.Teams = append(resp.Teams, imsjson.DirectoryGroup{
			ID:     t.ID,
			Title:  new(t.Title),
			Active: new(t.Active),
		})
	}
	for _, p := range positions {
		resp.Positions = append(resp.Positions, imsjson.DirectoryGroup{
			ID:     p.ID,
			Title:  new(p.Title),
			Active: new(p.Active),
		})
	}
	slices.SortFunc(resp.Persons, func(a, b imsjson.DirectoryPerson) int {
		return int(a.ID - b.ID)
	})
	slices.SortFunc(resp.Teams, func(a, b imsjson.DirectoryGroup) int {
		return int(a.ID - b.ID)
	})
	slices.SortFunc(resp.Positions, func(a, b imsjson.DirectoryGroup) int {
		return int(a.ID - b.ID)
	})
	return resp, nil
}

type EditDirectoryPerson struct {
	imsDBQ         *store.DBQ
	userStore      *directory.UserStore
	imsAdmins      []string
	directoryIsIMS bool
}

func (action EditDirectoryPerson) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	newID, errHTTP := action.editDirectoryPerson(req)
	if errHTTP != nil {
		errHTTP.From("[editDirectoryPerson]").WriteResponse(w)
		return
	}
	action.userStore.Flush()
	if newID != nil {
		w.Header().Set("IMS-Directory-Person-ID", strconv.FormatInt(*newID, 10))
	}
	herr.WriteNoContentResponse(w, "Success")
}
func (action EditDirectoryPerson) editDirectoryPerson(req *http.Request) (newPersonID *int64, errHTTP *herr.HTTPError) {
	errHTTP = requireDirectoryAdmin(req, action.imsDBQ, action.userStore, action.imsAdmins, action.directoryIsIMS)
	if errHTTP != nil {
		return nil, errHTTP.From("[requireDirectoryAdmin]")
	}
	ctx := req.Context()
	personReq, errHTTP := readBodyAs[imsjson.DirectoryPerson](req)
	if errHTTP != nil {
		return nil, errHTTP.From("[readBodyAs]")
	}
	if personReq.Handle != nil {
		if *personReq.Handle == "" {
			return nil, herr.BadRequest("Handle must not be empty", nil)
		}
		if len(*personReq.Handle) > maxDirectoryHandleLen {
			return nil, herr.BadRequest("Handle is too long", nil)
		}
	}
	if personReq.Email != nil && len(*personReq.Email) > maxDirectoryEmailLen {
		return nil, herr.BadRequest("Email is too long", nil)
	}

	var personID int64
	if personReq.ID == 0 {
		if personReq.Handle == nil {
			return nil, herr.BadRequest("Handle is required for a new person", nil)
		}
		// New persons start with an unguessable placeholder password, so
		// they can't log in until an admin sets a real password for them.
		placeholder := argon2id.CreateHash(rand.Text(), argon2id.SecondRecommendedParams)
		var email sql.NullString
		if personReq.Email != nil {
			email = conv.StringToSql(conv.EmptyToNil(*personReq.Email), maxDirectoryEmailLen)
		}
		id, err := action.imsDBQ.DirectoryCreatePerson(ctx, action.imsDBQ, imsdb.DirectoryCreatePersonParams{
			Handle:   *personReq.Handle,
			Email:    email,
			Password: placeholder,
			Active:   personReq.Active == nil || *personReq.Active,
			Onsite:   personReq.Onsite != nil && *personReq.Onsite,
		})
		if err != nil {
			return nil, herr.BadRequest("Failed to create person. Handles and emails must be unique.", err).
				From("[DirectoryCreatePerson]")
		}
		personID = id
	} else {
		personID = personReq.ID
		existing, err := action.imsDBQ.DirectoryPersonByID(ctx, action.imsDBQ, personID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, herr.NotFound("Person not found", err)
			}
			return nil, herr.InternalServerError("Failed to fetch person", err).From("[DirectoryPersonByID]")
		}
		handle := existing.Handle
		if personReq.Handle != nil {
			handle = *personReq.Handle
		}
		email := existing.Email
		if personReq.Email != nil {
			email = conv.StringToSql(conv.EmptyToNil(*personReq.Email), maxDirectoryEmailLen)
		}
		active := existing.Active
		if personReq.Active != nil {
			active = *personReq.Active
		}
		onsite := existing.Onsite
		if personReq.Onsite != nil {
			onsite = *personReq.Onsite
		}
		err = action.imsDBQ.DirectoryUpdatePerson(ctx, action.imsDBQ, imsdb.DirectoryUpdatePersonParams{
			Handle: handle,
			Email:  email,
			Active: active,
			Onsite: onsite,
			ID:     personID,
		})
		if err != nil {
			return nil, herr.BadRequest("Failed to update person. Handles and emails must be unique.", err).
				From("[DirectoryUpdatePerson]")
		}
	}

	errHTTP = action.setMemberships(req, personID, personReq.TeamIDs, personReq.PositionIDs)
	if errHTTP != nil {
		return nil, errHTTP.From("[setMemberships]")
	}
	if personReq.ID == 0 {
		return &personID, nil
	}
	return nil, nil
}

func (action EditDirectoryPerson) setMemberships(
	req *http.Request, personID int64, teamIDs, positionIDs *[]int64,
) *herr.HTTPError {
	ctx := req.Context()
	if teamIDs != nil {
		err := action.imsDBQ.DirectoryClearPersonTeams(ctx, action.imsDBQ, personID)
		if err != nil {
			return herr.InternalServerError("Failed to update team memberships", err).From("[DirectoryClearPersonTeams]")
		}
		for _, teamID := range *teamIDs {
			err = action.imsDBQ.DirectoryAddPersonTeam(ctx, action.imsDBQ, imsdb.DirectoryAddPersonTeamParams{
				PersonID: personID,
				TeamID:   teamID,
			})
			if err != nil {
				return herr.BadRequest("Failed to add team membership. Does the team exist?", err).
					From("[DirectoryAddPersonTeam]")
			}
		}
	}
	if positionIDs != nil {
		err := action.imsDBQ.DirectoryClearPersonPositions(ctx, action.imsDBQ, personID)
		if err != nil {
			return herr.InternalServerError("Failed to update position memberships", err).From("[DirectoryClearPersonPositions]")
		}
		for _, positionID := range *positionIDs {
			err = action.imsDBQ.DirectoryAddPersonPosition(ctx, action.imsDBQ, imsdb.DirectoryAddPersonPositionParams{
				PersonID:   personID,
				PositionID: positionID,
			})
			if err != nil {
				return herr.BadRequest("Failed to add position membership. Does the position exist?", err).
					From("[DirectoryAddPersonPosition]")
			}
		}
	}
	return nil
}

type SetDirectoryPersonPassword struct {
	imsDBQ         *store.DBQ
	userStore      *directory.UserStore
	imsAdmins      []string
	directoryIsIMS bool
}

func (action SetDirectoryPersonPassword) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.setDirectoryPersonPassword(req)
	if errHTTP != nil {
		errHTTP.From("[setDirectoryPersonPassword]").WriteResponse(w)
		return
	}
	action.userStore.Flush()
	herr.WriteNoContentResponse(w, "Success")
}
func (action SetDirectoryPersonPassword) setDirectoryPersonPassword(req *http.Request) *herr.HTTPError {
	errHTTP := requireDirectoryAdmin(req, action.imsDBQ, action.userStore, action.imsAdmins, action.directoryIsIMS)
	if errHTTP != nil {
		return errHTTP.From("[requireDirectoryAdmin]")
	}
	ctx := req.Context()
	personID, err := conv.ParseInt64(req.PathValue("personId"))
	if err != nil {
		return herr.BadRequest("Invalid person ID", err).From("[ParseInt64]")
	}
	passwordReq, errHTTP := readBodyAs[imsjson.DirectoryPersonPassword](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}
	if passwordReq.Password == "" {
		return herr.BadRequest("Password must not be empty", nil)
	}
	if len(passwordReq.Password) > maxDirectoryPasswordLen {
		return herr.BadRequest("Password is too long", nil)
	}
	// Ensure the person exists, so we can 404 rather than silently updating
	// zero rows.
	_, err = action.imsDBQ.DirectoryPersonByID(ctx, action.imsDBQ, personID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return herr.NotFound("Person not found", err)
		}
		return herr.InternalServerError("Failed to fetch person", err).From("[DirectoryPersonByID]")
	}
	hashed := argon2id.CreateHash(passwordReq.Password, argon2id.SecondRecommendedParams)
	err = action.imsDBQ.DirectorySetPersonPassword(ctx, action.imsDBQ, imsdb.DirectorySetPersonPasswordParams{
		Password: hashed,
		ID:       personID,
	})
	if err != nil {
		return herr.InternalServerError("Failed to set password", err).From("[DirectorySetPersonPassword]")
	}
	return nil
}

type DeleteDirectoryPerson struct {
	imsDBQ         *store.DBQ
	userStore      *directory.UserStore
	imsAdmins      []string
	directoryIsIMS bool
}

func (action DeleteDirectoryPerson) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.deleteDirectoryPerson(req)
	if errHTTP != nil {
		errHTTP.From("[deleteDirectoryPerson]").WriteResponse(w)
		return
	}
	action.userStore.Flush()
	herr.WriteNoContentResponse(w, "Success")
}
func (action DeleteDirectoryPerson) deleteDirectoryPerson(req *http.Request) *herr.HTTPError {
	errHTTP := requireDirectoryAdmin(req, action.imsDBQ, action.userStore, action.imsAdmins, action.directoryIsIMS)
	if errHTTP != nil {
		return errHTTP.From("[requireDirectoryAdmin]")
	}
	personID, err := conv.ParseInt64(req.PathValue("personId"))
	if err != nil {
		return herr.BadRequest("Invalid person ID", err).From("[ParseInt64]")
	}
	err = action.imsDBQ.DirectoryDeletePerson(req.Context(), action.imsDBQ, personID)
	if err != nil {
		return herr.InternalServerError("Failed to delete person", err).From("[DirectoryDeletePerson]")
	}
	return nil
}

type EditDirectoryTeam struct {
	imsDBQ         *store.DBQ
	userStore      *directory.UserStore
	imsAdmins      []string
	directoryIsIMS bool
}

func (action EditDirectoryTeam) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	newID, errHTTP := editDirectoryGroup(
		req, action.imsDBQ, action.userStore, action.imsAdmins, action.directoryIsIMS, false)
	if errHTTP != nil {
		errHTTP.From("[editDirectoryGroup]").WriteResponse(w)
		return
	}
	action.userStore.Flush()
	if newID != nil {
		w.Header().Set("IMS-Directory-Team-ID", strconv.FormatInt(*newID, 10))
	}
	herr.WriteNoContentResponse(w, "Success")
}

type EditDirectoryPosition struct {
	imsDBQ         *store.DBQ
	userStore      *directory.UserStore
	imsAdmins      []string
	directoryIsIMS bool
}

func (action EditDirectoryPosition) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	newID, errHTTP := editDirectoryGroup(
		req, action.imsDBQ, action.userStore, action.imsAdmins, action.directoryIsIMS, true)
	if errHTTP != nil {
		errHTTP.From("[editDirectoryGroup]").WriteResponse(w)
		return
	}
	action.userStore.Flush()
	if newID != nil {
		w.Header().Set("IMS-Directory-Position-ID", strconv.FormatInt(*newID, 10))
	}
	herr.WriteNoContentResponse(w, "Success")
}

// editDirectoryGroup creates or updates a team or position (they have
// identical shapes).
func editDirectoryGroup(
	req *http.Request,
	imsDBQ *store.DBQ,
	userStore *directory.UserStore,
	imsAdmins []string,
	directoryIsIMS bool,
	isPosition bool,
) (newGroupID *int64, errHTTP *herr.HTTPError) {
	errHTTP = requireDirectoryAdmin(req, imsDBQ, userStore, imsAdmins, directoryIsIMS)
	if errHTTP != nil {
		return nil, errHTTP.From("[requireDirectoryAdmin]")
	}
	ctx := req.Context()
	groupReq, errHTTP := readBodyAs[imsjson.DirectoryGroup](req)
	if errHTTP != nil {
		return nil, errHTTP.From("[readBodyAs]")
	}
	if groupReq.Title != nil {
		if *groupReq.Title == "" {
			return nil, herr.BadRequest("Title must not be empty", nil)
		}
		if len(*groupReq.Title) > maxDirectoryTitleLen {
			return nil, herr.BadRequest("Title is too long", nil)
		}
	}

	if groupReq.ID == 0 {
		if groupReq.Title == nil {
			return nil, herr.BadRequest("Title is required for a new team or position", nil)
		}
		active := groupReq.Active == nil || *groupReq.Active
		var id int64
		var err error
		if isPosition {
			id, err = imsDBQ.DirectoryCreatePosition(ctx, imsDBQ, imsdb.DirectoryCreatePositionParams{
				Title:  *groupReq.Title,
				Active: active,
			})
		} else {
			id, err = imsDBQ.DirectoryCreateTeam(ctx, imsDBQ, imsdb.DirectoryCreateTeamParams{
				Title:  *groupReq.Title,
				Active: active,
			})
		}
		if err != nil {
			return nil, herr.BadRequest("Failed to create. Titles must be unique.", err).From("[DirectoryCreate]")
		}
		return &id, nil
	}

	existingTitle, existingActive, errHTTP := findDirectoryGroup(req, imsDBQ, isPosition, groupReq.ID)
	if errHTTP != nil {
		return nil, errHTTP.From("[findDirectoryGroup]")
	}
	title := existingTitle
	if groupReq.Title != nil {
		title = *groupReq.Title
	}
	active := existingActive
	if groupReq.Active != nil {
		active = *groupReq.Active
	}
	var err error
	if isPosition {
		err = imsDBQ.DirectoryUpdatePosition(ctx, imsDBQ, imsdb.DirectoryUpdatePositionParams{
			Title:  title,
			Active: active,
			ID:     groupReq.ID,
		})
	} else {
		err = imsDBQ.DirectoryUpdateTeam(ctx, imsDBQ, imsdb.DirectoryUpdateTeamParams{
			Title:  title,
			Active: active,
			ID:     groupReq.ID,
		})
	}
	if err != nil {
		return nil, herr.BadRequest("Failed to update. Titles must be unique.", err).From("[DirectoryUpdate]")
	}
	return nil, nil
}

func findDirectoryGroup(
	req *http.Request, imsDBQ *store.DBQ, isPosition bool, id int64,
) (title string, active bool, errHTTP *herr.HTTPError) {
	ctx := req.Context()
	if isPosition {
		rows, err := imsDBQ.DirectoryAllPositions(ctx, imsDBQ)
		if err != nil {
			return "", false, herr.InternalServerError("Failed to fetch positions", err).From("[DirectoryAllPositions]")
		}
		for _, row := range rows {
			if row.ID == id {
				return row.Title, row.Active, nil
			}
		}
		return "", false, herr.NotFound("Position not found", nil)
	}
	rows, err := imsDBQ.DirectoryAllTeams(ctx, imsDBQ)
	if err != nil {
		return "", false, herr.InternalServerError("Failed to fetch teams", err).From("[DirectoryAllTeams]")
	}
	for _, row := range rows {
		if row.ID == id {
			return row.Title, row.Active, nil
		}
	}
	return "", false, herr.NotFound("Team not found", nil)
}

type DeleteDirectoryTeam struct {
	imsDBQ         *store.DBQ
	userStore      *directory.UserStore
	imsAdmins      []string
	directoryIsIMS bool
}

func (action DeleteDirectoryTeam) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := requireDirectoryAdmin(req, action.imsDBQ, action.userStore, action.imsAdmins, action.directoryIsIMS)
	if errHTTP != nil {
		errHTTP.From("[requireDirectoryAdmin]").WriteResponse(w)
		return
	}
	teamID, err := conv.ParseInt64(req.PathValue("teamId"))
	if err != nil {
		herr.BadRequest("Invalid team ID", err).WriteResponse(w)
		return
	}
	err = action.imsDBQ.DirectoryDeleteTeam(req.Context(), action.imsDBQ, teamID)
	if err != nil {
		herr.InternalServerError("Failed to delete team", err).From("[DirectoryDeleteTeam]").WriteResponse(w)
		return
	}
	action.userStore.Flush()
	herr.WriteNoContentResponse(w, "Success")
}

type DeleteDirectoryPosition struct {
	imsDBQ         *store.DBQ
	userStore      *directory.UserStore
	imsAdmins      []string
	directoryIsIMS bool
}

func (action DeleteDirectoryPosition) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := requireDirectoryAdmin(req, action.imsDBQ, action.userStore, action.imsAdmins, action.directoryIsIMS)
	if errHTTP != nil {
		errHTTP.From("[requireDirectoryAdmin]").WriteResponse(w)
		return
	}
	positionID, err := conv.ParseInt64(req.PathValue("positionId"))
	if err != nil {
		herr.BadRequest("Invalid position ID", err).WriteResponse(w)
		return
	}
	err = action.imsDBQ.DirectoryDeletePosition(req.Context(), action.imsDBQ, positionID)
	if err != nil {
		herr.InternalServerError("Failed to delete position", err).From("[DirectoryDeletePosition]").WriteResponse(w)
		return
	}
	action.userStore.Flush()
	herr.WriteNoContentResponse(w, "Success")
}
