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

package auth

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"strconv"
	"time"
)

// RefreshTokenCookieName is the cookie name for the refresh token value.
// Ideally we'd use the "__Host-" prefix, but that would make local development
// with Chrome more difficult :(.
//
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/Cookies#cookie_prefixes
// https://issues.chromium.org/issues/40202941
const RefreshTokenCookieName = "refresh_token"

// SuggestedEarlyAccessTokenRefresh is how long before an access token actually expires that the
// client should consider refreshing the token. This prevents annoying client-side errors,
// when the client thinks its access token is still  valid, but the relevant API request then gets
// rejected for being slightly too late.
const SuggestedEarlyAccessTokenRefresh time.Duration = -10 * time.Second

type JWTer struct {
	SecretKey string
}

// CreateRefreshToken creates a refresh token, which the client can use to request new access tokens,
// based on any updated claims from the UserStore. It's an implementation detail that this uses an
// access token-style JWT. Ideally a refresh token is supposed to be persisted, so that it can be
// invalidated from the server side. As a stopgap before we have such a per-user persistence component,
// we instead rely on the security of JWT signing.
func (j JWTer) CreateRefreshToken(rangerName string, clubhouseID int64, expiration time.Time) (string, error) {
	token, err := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		NewIMSClaims().
			WithIssuedAt(time.Now()).
			WithExpiration(expiration).
			WithIssuer("ranger-ims-go").
			WithRangerHandle(rangerName).
			WithSubject(strconv.FormatInt(clubhouseID, 10)),
	).SignedString([]byte(j.SecretKey))
	if err != nil {
		return "", fmt.Errorf("[SignedString]: %w", err)
	}
	return token, nil
}

func (j JWTer) CreateJWT(
	rangerName string,
	clubhouseID int64,
	positions []string,
	teams []string,
	onsite bool,
	expiration time.Time,
) (string, error) {
	token, err := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		NewIMSClaims().
			WithIssuedAt(time.Now()).
			WithExpiration(expiration).
			WithIssuer("ranger-ims-go").
			WithRangerHandle(rangerName).
			WithRangerOnSite(onsite).
			WithRangerPositions(positions...).
			WithRangerTeams(teams...).
			WithSubject(strconv.FormatInt(clubhouseID, 10)),
	).SignedString([]byte(j.SecretKey))
	if err != nil {
		return "", fmt.Errorf("[SignedString]: %w", err)
	}
	return token, nil
}

// AuthenticateJWT gives JWT claims for a valid, authenticated JWT string, or
// returns an error otherwise. A JWT may be invalid because it was signed by a
// different key, because it has expired, etc.
func (j JWTer) AuthenticateJWT(jwtStr string) (*IMSClaims, error) {
	return j.authenticateJWT(jwtStr)
}

// AuthenticateRefreshToken is like AuthenticateJWT, in that it validates that the
// supplied token is valid (was signed by the same secret key and hasn't expired).
// It's an implementation detail that refresh tokens are also JWTs. Clients of IMS
// should treat them as simply opaque strings.
func (j JWTer) AuthenticateRefreshToken(refreshToken string) (*IMSClaims, error) {
	return j.authenticateJWT(refreshToken)
}

func (j JWTer) authenticateJWT(jwtStr string) (*IMSClaims, error) {
	if jwtStr == "" {
		return nil, fmt.Errorf("no JWT string provided")
	}
	claims := IMSClaims{}
	tok, err := jwt.ParseWithClaims(jwtStr, &claims, func(token *jwt.Token) (any, error) {
		return []byte(j.SecretKey), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))
	if err != nil {
		return nil, fmt.Errorf("[jwt.Parse]: %w", err)
	}
	if tok == nil {
		return nil, fmt.Errorf("token is nil")
	}
	if !tok.Valid {
		return nil, fmt.Errorf("token is invalid")
	}
	if claims.RangerHandle() == "" {
		return nil, fmt.Errorf("ranger handle is required")
	}
	return &claims, nil
}
