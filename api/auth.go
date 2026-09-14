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
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/authn"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
)

type authError string

func (e authError) Error() string {
	return string(e)
}

const (
	ErrLongPassword = authError("rejected very long password")
)

type PostAuth struct {
	imsDBQ        *store.DBQ
	userStore     *directory.UserStore
	jwtSecret     string
	tokenLifetime time.Duration
}

type PostAuthRequest struct {
	Identification string `json:"identification"`
	// #nosec G117 // Exported secret field
	Password string `json:"password"`

	// TokenInBody asks for the access token in the response body, for a client
	// that will send it back in an Authorization header. Otherwise the token only
	// goes out in an HttpOnly cookie, where the web client's JavaScript (and any
	// script injected into it) can't read it.
	TokenInBody bool `json:"token_in_body,omitzero"`
}
type PostAuthResponse struct {
	Token         string `json:"token,omitzero"`
	ExpiresUnixMs int64  `json:"expires_unix_ms"`
}

func (action PostAuth) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, cookie, errHTTP := action.postAuth(req)
	if errHTTP != nil {
		errHTTP.From("[postAuth]").WriteResponse(w)
		return
	}
	if cookie != nil {
		http.SetCookie(w, cookie)
	}
	http.SetCookie(w, authz.ExpiredLegacyRefreshTokenCookie(req))
	mustWriteJSON(w, req, resp)
}
func (action PostAuth) postAuth(req *http.Request) (PostAuthResponse, *http.Cookie, *herr.HTTPError) {
	// This endpoint is unauthenticated, as the point of this is to take a
	// username and password to create a new JWT.
	var empty PostAuthResponse

	vals, errHTTP := readBodyAs[PostAuthRequest](req)
	if errHTTP != nil {
		return empty, nil, errHTTP.From("[readBodyAs]")
	}

	rangers, err := action.userStore.GetAllUsers(req.Context())
	if err != nil {
		return empty, nil, herr.InternalServerError("Failed to fetch personnel", err).From("[GetRangers]")
	}
	var matchedPerson *directory.User
	for _, person := range rangers {
		callsignMatch := person.Handle != "" && strings.EqualFold(person.Handle, vals.Identification)
		if callsignMatch {
			matchedPerson = person
			break
		}
		emailMatch := person.Email != "" && strings.EqualFold(person.Email, vals.Identification)
		if emailMatch {
			matchedPerson = person
			break
		}
	}

	// See https://instatunnel.my/blog/the-1mb-password-crashing-backends-via-hashing-exhaustion
	if len(vals.Password) > 256 {
		return empty, nil, herr.BadRequest(
			"Outrageously long passwords are disallowed",
			ErrLongPassword,
		)
	}

	if matchedPerson == nil {
		// Run Verify against some dummy hashed password.
		// We want to avoid timing attacks, where the client could know
		// the username is invalid because the login attempt is fast, so
		// we force a password verification even if no one matched.
		_, _ = authn.Verify(vals.Password, "$argon2id$v=19$m=8192,t=4,p=1$Ke9wio+D+PfBYlVzJ3CTAA$/kNb/yXgSLyFpfmwIfwKwcNnBRRrUqJp8YXPtDKfNTE")
		return empty, nil, herr.Unauthorized(
			"Failed login attempt (bad credentials)",
			fmt.Errorf("login attempt for nonexistent user. Identification: %v", vals.Identification),
		)
	}

	correct, err := authn.Verify(vals.Password, matchedPerson.Password)
	if err != nil {
		return empty, nil, herr.InternalServerError(
			"Stored credentials are stale. A simple login to Clubhouse should fix this. You do not need to change your password.",
			fmt.Errorf("%w. Identification: %v", err, vals.Identification)).From("[Verify]")
	}
	if !correct {
		return empty, nil, herr.Unauthorized(
			"Failed login attempt (bad credentials)",
			fmt.Errorf("bad password for valid user. Identification: %v", vals.Identification),
		)
	}

	slog.Info("Successful login for Ranger", "identification", matchedPerson.Handle)

	expiration := time.Now().Add(action.tokenLifetime)
	jwt, err := authz.JWTer{SecretKey: action.jwtSecret}.
		CreateAccessToken(matchedPerson.Handle, matchedPerson.ID, expiration)
	if err != nil {
		return empty, nil, herr.InternalServerError("Failed to create access token", err).From("[CreateAccessToken]")
	}

	resp := PostAuthResponse{ExpiresUnixMs: expiration.UnixMilli()}
	if vals.TokenInBody {
		resp.Token = jwt
		return resp, nil, nil
	}
	return resp, authz.AccessTokenCookie(req, jwt, action.tokenLifetime), nil
}

type GetAuth struct {
	imsDBQ               *store.DBQ
	userStore            *directory.UserStore
	jwtSecret            string
	admins               []string
	attachmentsEnabled   bool
	eventDeletionEnabled bool
	bmAPIEnabled         bool
}

type GetAuthResponse struct {
	Authenticated bool                      `json:"authenticated"`
	User          string                    `json:"user,omitzero"`
	Admin         bool                      `json:"admin"`
	EventAccess   map[string]AccessForEvent `json:"event_access"`

	// EventDeletionAllowed tells the admin events page whether this server
	// permits deleting events (the EventDeletionEnabled server config).
	EventDeletionAllowed bool `json:"event_deletion_allowed"`

	// PlacesImportAllowed tells the admin places page whether this server has a
	// Burning Man API key, and so can pull an event's places from that API.
	PlacesImportAllowed bool `json:"places_import_allowed"`
}

type AccessForEvent struct {
	EventID           int32 `json:"event_id"`
	ReadIncidents     bool  `json:"readIncidents"`
	WriteIncidents    bool  `json:"writeIncidents"`
	WriteFieldReports bool  `json:"writeFieldReports"`
	ReadVisits        bool  `json:"readVisits"`
	WriteVisits       bool  `json:"writeVisits"`
	AttachFiles       bool  `json:"attachFiles"`
}

func (action GetAuth) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getAuth(req)
	if errHTTP != nil {
		errHTTP.From("[getAuth]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}
func (action GetAuth) getAuth(req *http.Request) (GetAuthResponse, *herr.HTTPError) {
	var resp GetAuthResponse

	// This endpoint is unauthenticated, and reports whether the requestor is.
	jwtCtx, found := req.Context().Value(JWTContextKey).(JWTContext)
	if !found || jwtCtx.Error != nil || jwtCtx.Claims == nil {
		resp = GetAuthResponse{
			Authenticated: false,
		}
		return resp, nil //lint:ignore nilerr since the jwtCtx.Error is irrelevant
	}
	claims := jwtCtx.Claims
	handle := claims.RangerHandle()
	var roles []authz.Role
	if slices.Contains(action.admins, handle) {
		roles = append(roles, authz.Administrator)
	}
	resp = GetAuthResponse{
		Authenticated:        true,
		User:                 handle,
		Admin:                slices.Contains(roles, authz.Administrator),
		EventDeletionAllowed: action.eventDeletionEnabled,
		PlacesImportAllowed:  action.bmAPIEnabled,
	}
	// event_id is an optional query param for this endpoint
	eventName := req.FormValue("event_id")
	if eventName != "" {
		event, errHTTP := getEvent(req, eventName, action.imsDBQ)
		if errHTTP != nil {
			if errHTTP.Code != http.StatusNotFound {
				return resp, errHTTP.From("[getEvent]")
			} else {
				// We don't want to return a 404 if the event doesn't exist.
				// Just make it look like the event might exist, but that the
				// user has no access.
				resp.EventAccess = map[string]AccessForEvent{
					eventName: {
						ReadIncidents:     false,
						WriteIncidents:    false,
						WriteFieldReports: false,
						ReadVisits:        false,
						WriteVisits:       false,
						AttachFiles:       false,
					},
				}
				return resp, nil
			}
		}

		eventPermissions, _, err := authz.EventPermissions(req.Context(), &event.ID, action.imsDBQ, action.userStore, action.admins, *claims)
		if err != nil {
			return resp, herr.InternalServerError("Failed to fetch event permissions", err).From("[EventPermissions]")
		}

		resp.EventAccess = map[string]AccessForEvent{
			eventName: {
				EventID:           event.ID,
				ReadIncidents:     eventPermissions[event.ID]&authz.EventReadIncidents != 0,
				WriteIncidents:    eventPermissions[event.ID]&authz.EventWriteIncidents != 0,
				WriteFieldReports: eventPermissions[event.ID]&(authz.EventWriteOwnFieldReports|authz.EventWriteAllFieldReports) != 0,
				ReadVisits:        eventPermissions[event.ID]&authz.EventReadVisits != 0,
				WriteVisits:       eventPermissions[event.ID]&authz.EventWriteVisits != 0,
				AttachFiles:       action.attachmentsEnabled,
			},
		}
	}
	return resp, nil
}
