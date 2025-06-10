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
	imsDBQ               *store.DBQ
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
	resp, cookie, errHTTP := action.postAuth(req)
	if errHTTP != nil {
		errHTTP.From("[postAuth]").WriteResponse(w)
		return
	}
	http.SetCookie(w, cookie)
	mustWriteJSON(w, req, resp)
}
func (action PostAuth) postAuth(req *http.Request) (PostAuthResponse, *http.Cookie, *herr.HTTPError) {
	// This endpoint is unauthenticated (doesn't require an Authorization header)
	// as the point of this is to take a username and password to create a new JWT.
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
		callsignMatch := person.Person.Callsign != "" && strings.EqualFold(person.Person.Callsign, vals.Identification)
		if callsignMatch {
			matchedPerson = person
			break
		}
		emailMatch := person.Person.Email.String != "" && strings.EqualFold(person.Person.Email.String, vals.Identification)
		if emailMatch {
			matchedPerson = person
			break
		}
	}

	if matchedPerson == nil {
		return empty, nil, herr.Unauthorized(
			"Failed login attempt (bad credentials)",
			fmt.Errorf("login attempt for nonexistent user. Identification: %v", vals.Identification),
		)
	}

	correct, err := authn.Verify(vals.Password, matchedPerson.Person.Password.String)
	if err != nil {
		return empty, nil, herr.InternalServerError("Invalid stored password. Get in touch with the tech team.", err).From("[Verify]")
	}
	if !correct {
		return empty, nil, herr.Unauthorized(
			"Failed login attempt (bad credentials)",
			fmt.Errorf("bad password for valid user. Identification: %v", vals.Identification),
		)
	}

	slog.Info("Successful login for Ranger", "identification", matchedPerson.Person.Callsign)

	accessTokenExpiration := time.Now().Add(action.accessTokenDuration)
	jwt, err := authz.JWTer{SecretKey: action.jwtSecret}.
		CreateAccessToken(matchedPerson.Person.Callsign, matchedPerson.Person.ID, matchedPerson.PositionIDs, matchedPerson.TeamIDs, matchedPerson.Person.OnSite, accessTokenExpiration)
	if err != nil {
		return empty, nil, herr.InternalServerError("Failed to create access token", err).From("[CreateAccessToken]")
	}

	suggestedRefreshTime := accessTokenExpiration.Add(authz.SuggestedEarlyAccessTokenRefresh).UnixMilli()
	resp := PostAuthResponse{Token: jwt, ExpiresUnixMs: suggestedRefreshTime}

	// The refresh token should be valid much longer than the access token.
	refreshTokenExpiration := time.Now().Add(action.refreshTokenDuration)
	refreshToken, err := authz.JWTer{SecretKey: action.jwtSecret}.
		CreateRefreshToken(matchedPerson.Person.Callsign, matchedPerson.Person.ID, refreshTokenExpiration)
	if err != nil {
		return empty, nil, herr.InternalServerError("Failed to create refresh token", err).From("[CreateRefreshToken]")
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
	imsDBQ             *store.DBQ
	userStore          *directory.UserStore
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
	resp, errHTTP := action.getAuth(req)
	if errHTTP != nil {
		errHTTP.From("[getAuth]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}
func (action GetAuth) getAuth(req *http.Request) (GetAuthResponse, *herr.HTTPError) {
	resp := GetAuthResponse{}

	// This endpoint is unauthenticated (doesn't require an Authorization header).
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
		Authenticated: true,
		User:          handle,
		Admin:         slices.Contains(roles, authz.Administrator),
	}
	if err := req.ParseForm(); err != nil {
		return resp, herr.BadRequest("Failed to parse HTTP form", err).From("[ParseForm]")
	}
	// event_id is an optional query param for this endpoint
	eventName := req.Form.Get("event_id")
	if eventName != "" {
		event, errHTTP := getEvent(req, eventName, action.imsDBQ)
		if errHTTP != nil {
			return resp, errHTTP.From("[getEvent]")
		}

		eventPermissions, _, err := authz.EventPermissions(req.Context(), &event.ID, action.imsDBQ, action.userStore, action.admins, *claims)
		if err != nil {
			return resp, herr.InternalServerError("Failed to fetch event permissions", err).From("[EventPermissions]")
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
	imsDBQ              *store.DBQ
	userStore           *directory.UserStore
	jwtSecret           string
	accessTokenDuration time.Duration
}

type RefreshAccessTokenResponse struct {
	Token         string `json:"token"`
	ExpiresUnixMs int64  `json:"expires_unix_ms"`
}

func (action RefreshAccessToken) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.refreshAccessToken(req)
	if errHTTP != nil {
		errHTTP.From("[refreshAccessToken]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}
func (action RefreshAccessToken) refreshAccessToken(req *http.Request) (RefreshAccessTokenResponse, *herr.HTTPError) {
	var empty RefreshAccessTokenResponse
	refreshCookie, err := req.Cookie(authz.RefreshTokenCookieName)
	if err != nil {
		return empty, herr.Unauthorized("Bad or no refresh token cookie found", err).From("[Cookie]")
	}
	jwt, err := authz.JWTer{SecretKey: action.jwtSecret}.AuthenticateRefreshToken(refreshCookie.Value)
	if err != nil {
		return empty, herr.Unauthorized("Failed to authenticate refresh token", err).From("[AuthenticateRefreshToken]")
	}

	slog.Info("Refreshing access token", "ranger", jwt.RangerHandle())
	rangers, err := action.userStore.GetAllUsers(req.Context())
	if err != nil {
		return empty, herr.InternalServerError("Failed to fetch personnel", err).From("[GetRangers]")
	}
	var matchedPerson *directory.User
	for _, ranger := range rangers {
		if ranger.Person.Callsign == jwt.RangerHandle() && ranger.Person.ID == jwt.DirectoryID() {
			matchedPerson = ranger
			break
		}
	}
	if matchedPerson == nil {
		return empty, herr.Unauthorized("User not found", nil)
	}
	accessTokenExpiration := time.Now().Add(action.accessTokenDuration)
	accessToken, err := authz.JWTer{SecretKey: action.jwtSecret}.
		CreateAccessToken(
			jwt.RangerHandle(), matchedPerson.Person.ID, matchedPerson.PositionIDs, matchedPerson.TeamIDs, matchedPerson.Person.OnSite, accessTokenExpiration,
		)
	if err != nil {
		return empty, herr.InternalServerError("Failed to create access token", err).From("[CreateAccessToken]")
	}
	resp := RefreshAccessTokenResponse{
		Token:         accessToken,
		ExpiresUnixMs: accessTokenExpiration.Add(authz.SuggestedEarlyAccessTokenRefresh).UnixMilli(),
	}
	return resp, nil
}
