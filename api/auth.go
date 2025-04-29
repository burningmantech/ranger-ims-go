package api

import (
	"fmt"
	"github.com/srabraham/ranger-ims-go/auth"
	"github.com/srabraham/ranger-ims-go/auth/password"
	"github.com/srabraham/ranger-ims-go/directory"
	imsjson "github.com/srabraham/ranger-ims-go/json"
	"github.com/srabraham/ranger-ims-go/store"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
)

type PostAuth struct {
	imsDB       *store.DB
	userStore   *directory.UserStore
	jwtSecret   string
	jwtDuration time.Duration
}

type PostAuthRequest struct {
	Identification string `json:"identification"`
	Password       string `json:"password"`
}
type PostAuthResponse struct {
	Token string `json:"token"`
}

func (action PostAuth) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// This endpoint is unauthenticated (doesn't require an Authorization header)
	// as the point of this is to take a username and password to create a new JWT.

	vals, ok := mustReadBodyAs[PostAuthRequest](w, req)
	if !ok {
		return
	}

	rangers, err := action.userStore.GetRangers(req.Context())
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch personnel", err)
		return
	}
	var matchedPerson *imsjson.Person
	for _, person := range rangers {
		callsignMatch := person.Handle != "" && person.Handle == vals.Identification
		if callsignMatch {
			matchedPerson = &person
			break
		}
		emailMatch := person.Email != "" && strings.ToLower(person.Email) == strings.ToLower(vals.Identification)
		if emailMatch {
			matchedPerson = &person
			break
		}
	}

	if matchedPerson == nil {
		handleErr(w, req, http.StatusUnauthorized, "Failed login attempt (bad credentials)",
			fmt.Errorf("login attempt for nonexistent user. Identification: %v", vals.Identification))
		return
	}

	correct, err := password.Verify(vals.Password, matchedPerson.Password)
	if !correct {
		handleErr(w, req, http.StatusUnauthorized, "Failed login attempt (bad credentials)",
			fmt.Errorf("bad password for valid user. Identification: %v", vals.Identification))
		return
	}
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to verify password", err)
		return
	}
	slog.Info("Successful login for Ranger", "identification", matchedPerson.Handle)

	foundPositionNames, foundTeamNames, err := action.userStore.GetUserPositionsTeams(req.Context(), matchedPerson.DirectoryID)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Clubhouse positions/teams data", err)
		return
	}

	jwt := auth.JWTer{SecretKey: action.jwtSecret}.
		CreateJWT(matchedPerson.Handle, matchedPerson.DirectoryID, foundPositionNames, foundTeamNames, matchedPerson.Onsite, action.jwtDuration)
	resp := PostAuthResponse{Token: jwt}

	mustWriteJSON(w, resp)
}

type GetAuth struct {
	imsDB     *store.DB
	jwtSecret string
	admins    []string
}

type GetAuthResponse struct {
	Authenticated bool                      `json:"authenticated"`
	User          string                    `json:"user,omitzero"`
	Admin         bool                      `json:"admin"`
	EventAccess   map[string]AccessForEvent `json:"event_access"`
}

type AccessForEvent struct {
	ReadIncidents     bool `json:"readIncidents"`
	WriteIncidents    bool `json:"writeIncidents"`
	WriteFieldReports bool `json:"writeFieldReports"`
	AttachFiles       bool `json:"attachFiles"`
}

func (action GetAuth) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// This endpoint is unauthenticated (doesn't require an Authorization header).
	resp := GetAuthResponse{}

	jwtCtx, found := req.Context().Value(JWTContextKey).(JWTContext)
	if !found || jwtCtx.Error != nil || jwtCtx.Claims == nil {
		resp.Authenticated = false
		mustWriteJSON(w, resp)
		return
	}
	claims := jwtCtx.Claims
	handle := claims.RangerHandle()
	var roles []auth.Role
	if slices.Contains(action.admins, handle) {
		roles = append(roles, auth.Administrator)
	}
	resp.Authenticated = true
	resp.User = handle
	resp.Admin = slices.Contains(roles, auth.Administrator)

	if ok := mustParseForm(w, req); !ok {
		return
	}
	eventName := req.Form.Get("event_id")
	if eventName != "" {
		event, ok := mustGetEvent(w, req, eventName, action.imsDB)
		if !ok {
			return
		}

		eventPermissions, _, err := auth.EventPermissions(req.Context(), &event.ID, action.imsDB, action.admins, *claims)
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to fetch event permissions", err)
			return
		}

		resp.EventAccess = map[string]AccessForEvent{
			eventName: {
				ReadIncidents:     eventPermissions[event.ID]&auth.EventReadIncidents != 0,
				WriteIncidents:    eventPermissions[event.ID]&auth.EventWriteIncidents != 0,
				WriteFieldReports: eventPermissions[event.ID]&(auth.EventWriteOwnFieldReports|auth.EventWriteAllFieldReports) != 0,
				AttachFiles:       false,
			},
		}
	}

	mustWriteJSON(w, resp)
}
