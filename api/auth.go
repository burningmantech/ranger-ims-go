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
	"github.com/burningmantech/ranger-ims-go/auth"
	"github.com/burningmantech/ranger-ims-go/auth/password"
	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
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

	accessTokenExpiration := time.Now().Add(action.accessTokenDuration)
	jwt, err := auth.JWTer{SecretKey: action.jwtSecret}.
		CreateAccessToken(matchedPerson.Handle, matchedPerson.DirectoryID, foundPositionNames, foundTeamNames, matchedPerson.Onsite, accessTokenExpiration)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to create access token", err)
	}

	suggestedRefreshTime := accessTokenExpiration.Add(auth.SuggestedEarlyAccessTokenRefresh).UnixMilli()
	resp := PostAuthResponse{Token: jwt, ExpiresUnixMs: suggestedRefreshTime}

	// The refresh token should be valid much longer than the access token.
	refreshTokenExpiration := time.Now().Add(action.refreshTokenDuration)
	refreshToken, err := auth.JWTer{SecretKey: action.jwtSecret}.
		CreateRefreshToken(matchedPerson.Handle, matchedPerson.DirectoryID, refreshTokenExpiration)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to create refresh token", err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.RefreshTokenCookieName,
		Value:    refreshToken,
		Path:     "/",
		MaxAge:   int(action.refreshTokenDuration.Milliseconds() / 1000),
		HttpOnly: true,
		Secure:   true,
		// We only ever read this cookie on POSTs to the refresh endpoint,
		// so strict is fine.
		SameSite: http.SameSiteStrictMode,
	})

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
	refreshCookie, err := req.Cookie(auth.RefreshTokenCookieName)
	if err != nil {
		handleErr(w, req, http.StatusUnauthorized, "Bad or no refresh token cookie found", err)
		return
	}
	jwt, err := auth.JWTer{SecretKey: action.jwtSecret}.AuthenticateRefreshToken(refreshCookie.Value)
	if err != nil {
		handleErr(w, req, http.StatusUnauthorized, "Failed to authenticate refresh token", err)
		return
	}
	if jwt.RangerHandle() == "" {
		handleErr(w, req, http.StatusUnauthorized, "No Ranger handle associated with refresh token", err)
		return
	}
	slog.Info("Refreshing access token", "ranger", jwt.RangerHandle())
	rangers, err := action.userStore.GetRangers(req.Context())
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch personnel", err)
		return
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
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Clubhouse positions/teams data", err)
		return
	}
	accessTokenExpiration := time.Now().Add(action.accessTokenDuration)
	accessToken, err := auth.JWTer{SecretKey: action.jwtSecret}.
		CreateAccessToken(jwt.RangerHandle(), matchedPerson.DirectoryID, foundPositionNames, foundTeamNames, matchedPerson.Onsite, accessTokenExpiration)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to create access token", err)
	}
	resp := RefreshAccessTokenResponse{Token: accessToken, ExpiresUnixMs: accessTokenExpiration.Add(auth.SuggestedEarlyAccessTokenRefresh).UnixMilli()}
	mustWriteJSON(w, resp)
}
