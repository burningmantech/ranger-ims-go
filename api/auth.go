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
	"github.com/burningmantech/ranger-ims-go/lib/authn"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
)

type PostAuth struct {
	imsDB                *store.DB
	userStore            *directory.UserStore
	jwtSecret            string
	accessTokenDuration  time.Duration
	refreshTokenDuration time.Duration
}

type PostAuthRequest struct {
	Identification string `json:"identification"`
	Password       string `json:"password"`
}
type PostAuthResponse struct {
	Token         string `json:"token"`
	ExpiresUnixMs int64  `json:"expires_unix_ms"`
}

func (action PostAuth) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, cookie, errH := action.postAuth(req)
	if errH != nil {
		errH.Src("[postAuth]").WriteResponse(w)
		return
	}
	http.SetCookie(w, cookie)
	mustWriteJSON(w, resp)
}
func (action PostAuth) postAuth(req *http.Request) (PostAuthResponse, *http.Cookie, *herr.HTTPError) {
	// This endpoint is unauthenticated (doesn't require an Authorization header)
	// as the point of this is to take a username and password to create a new JWT.
	var empty PostAuthResponse

	vals, errH := mustReadBodyAs[PostAuthRequest](req)
	if errH != nil {
		return empty, nil, errH.Src("[mustReadBodyAs]")
	}

	rangers, err := action.userStore.GetRangers(req.Context())
	if err != nil {
		return empty, nil, herr.S500("Failed to fetch personnel", err).Src("[GetRangers]")
	}
	var matchedPerson *imsjson.Person
	for _, person := range rangers {
		callsignMatch := person.Handle != "" && strings.EqualFold(person.Handle, vals.Identification)
		if callsignMatch {
			matchedPerson = &person
			break
		}
		emailMatch := person.Email != "" && strings.EqualFold(person.Email, vals.Identification)
		if emailMatch {
			matchedPerson = &person
			break
		}
	}

	if matchedPerson == nil {
		return empty, nil, herr.S401(
			"Failed login attempt (bad credentials)",
			fmt.Errorf("login attempt for nonexistent user. Identification: %v", vals.Identification),
		)
	}

	correct, err := authn.Verify(vals.Password, matchedPerson.Password)
	if err != nil {
		return empty, nil, herr.S500("Invalid stored password. Get in touch with the tech team.", err).Src("[Verify]")
	}
	if !correct {
		return empty, nil, herr.S401(
			"Failed login attempt (bad credentials)",
			fmt.Errorf("bad password for valid user. Identification: %v", vals.Identification),
		)
	}

	slog.Info("Successful login for Ranger", "identification", matchedPerson.Handle)

	foundPositionNames, foundTeamNames, err := action.userStore.GetUserPositionsTeams(req.Context(), matchedPerson.DirectoryID)
	if err != nil {
		return empty, nil, herr.S500("Failed to fetch Clubhouse positions/teams data", err).Src("[GetUserPositionsTeams]")
	}

	accessTokenExpiration := time.Now().Add(action.accessTokenDuration)
	jwt, err := authz.JWTer{SecretKey: action.jwtSecret}.
		CreateAccessToken(matchedPerson.Handle, matchedPerson.DirectoryID, foundPositionNames, foundTeamNames, matchedPerson.Onsite, accessTokenExpiration)
	if err != nil {
		return empty, nil, herr.S500("Failed to create access token", err).Src("[CreateAccessToken]")
	}

	suggestedRefreshTime := accessTokenExpiration.Add(authz.SuggestedEarlyAccessTokenRefresh).UnixMilli()
	resp := PostAuthResponse{Token: jwt, ExpiresUnixMs: suggestedRefreshTime}

	// The refresh token should be valid much longer than the access token.
	refreshTokenExpiration := time.Now().Add(action.refreshTokenDuration)
	refreshToken, err := authz.JWTer{SecretKey: action.jwtSecret}.
		CreateRefreshToken(matchedPerson.Handle, matchedPerson.DirectoryID, refreshTokenExpiration)
	if err != nil {
		return empty, nil, herr.S500("Failed to create refresh token", err).Src("[CreateRefreshToken]")
	}

	refreshCookie := &http.Cookie{
		Name:     authz.RefreshTokenCookieName,
		Value:    refreshToken,
		Path:     "/",
		MaxAge:   int(action.refreshTokenDuration.Milliseconds() / 1000),
		HttpOnly: true,
		Secure:   true,
		// We only ever read this cookie on POSTs to the refresh endpoint,
		// so strict is fine.
		SameSite: http.SameSiteStrictMode,
	}

	return resp, refreshCookie, nil
}

type GetAuth struct {
	imsDB              *store.DB
	jwtSecret          string
	admins             []string
	attachmentsEnabled bool
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
	resp, errH := action.getAuth(req)
	if errH != nil {
		errH.Src("[getAuth]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, resp)
}
func (action GetAuth) getAuth(req *http.Request) (GetAuthResponse, *herr.HTTPError) {
	resp := GetAuthResponse{}

	// This endpoint is unauthenticated (doesn't require an Authorization header).
	jwtCtx, found := req.Context().Value(JWTContextKey).(JWTContext)
	if !found || jwtCtx.Error != nil || jwtCtx.Claims == nil {
		resp.Authenticated = false
		return resp, nil //lint:ignore nilerr since the jwtCtx.Error is irrelevant
	}
	claims := jwtCtx.Claims
	handle := claims.RangerHandle()
	var roles []authz.Role
	if slices.Contains(action.admins, handle) {
		roles = append(roles, authz.Administrator)
	}
	resp.Authenticated = true
	resp.User = handle
	resp.Admin = slices.Contains(roles, authz.Administrator)
	if err := req.ParseForm(); err != nil {
		return resp, herr.S400("Failed to parse HTTP form", err).Src("[ParseForm]")
	}
	eventName := req.Form.Get("event_id")
	if eventName != "" {
		event, errH := mustGetEvent(req, eventName, action.imsDB)
		if errH != nil {
			return resp, errH.Src("[mustGetEvent]")
		}

		eventPermissions, _, err := authz.EventPermissions(req.Context(), &event.ID, action.imsDB, action.admins, *claims)
		if err != nil {
			return resp, herr.S500("Failed to fetch event permissions", err).Src("[EventPermissions]")
		}

		resp.EventAccess = map[string]AccessForEvent{
			eventName: {
				ReadIncidents:     eventPermissions[event.ID]&authz.EventReadIncidents != 0,
				WriteIncidents:    eventPermissions[event.ID]&authz.EventWriteIncidents != 0,
				WriteFieldReports: eventPermissions[event.ID]&(authz.EventWriteOwnFieldReports|authz.EventWriteAllFieldReports) != 0,
				AttachFiles:       action.attachmentsEnabled,
			},
		}
	}
	return resp, nil
}

type RefreshAccessToken struct {
	imsDB               *store.DB
	userStore           *directory.UserStore
	jwtSecret           string
	accessTokenDuration time.Duration
}

type RefreshAccessTokenResponse struct {
	Token         string `json:"token"`
	ExpiresUnixMs int64  `json:"expires_unix_ms"`
}

func (action RefreshAccessToken) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errH := action.refreshAccessToken(req)
	if errH != nil {
		errH.Src("[refreshAccessToken]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, resp)
}
func (action RefreshAccessToken) refreshAccessToken(req *http.Request) (RefreshAccessTokenResponse, *herr.HTTPError) {
	var empty RefreshAccessTokenResponse
	refreshCookie, err := req.Cookie(authz.RefreshTokenCookieName)
	if err != nil {
		return empty, herr.S401("Bad or no refresh token cookie found", err).Src("[Cookie]")
	}
	jwt, err := authz.JWTer{SecretKey: action.jwtSecret}.AuthenticateRefreshToken(refreshCookie.Value)
	if err != nil {
		return empty, herr.S401("Failed to authenticate refresh token", err).Src("[AuthenticateRefreshToken]")
	}
	if jwt.RangerHandle() == "" {
		return empty, herr.S500("No Ranger handle associated with refresh token", nil)
	}
	slog.Info("Refreshing access token", "ranger", jwt.RangerHandle())
	rangers, err := action.userStore.GetRangers(req.Context())
	if err != nil {
		return empty, herr.S500("Failed to fetch personnel", err).Src("[GetRangers]")
	}
	var matchedPerson imsjson.Person
	for _, ranger := range rangers {
		if ranger.Handle == jwt.RangerHandle() && ranger.DirectoryID == jwt.DirectoryID() {
			matchedPerson = ranger
			break
		}
	}
	foundPositionNames, foundTeamNames, err := action.userStore.GetUserPositionsTeams(req.Context(), matchedPerson.DirectoryID)
	if err != nil {
		return empty, herr.S500("Failed to fetch Clubhouse positions/teams data", err).Src("[GetUserPositionsTeams]")
	}
	accessTokenExpiration := time.Now().Add(action.accessTokenDuration)
	accessToken, err := authz.JWTer{SecretKey: action.jwtSecret}.
		CreateAccessToken(jwt.RangerHandle(), matchedPerson.DirectoryID, foundPositionNames, foundTeamNames, matchedPerson.Onsite, accessTokenExpiration)
	if err != nil {
		return empty, herr.S500("Failed to create access token", err).Src("[CreateAccessToken]")
	}
	resp := RefreshAccessTokenResponse{Token: accessToken, ExpiresUnixMs: accessTokenExpiration.Add(authz.SuggestedEarlyAccessTokenRefresh).UnixMilli()}
	return resp, nil
}
