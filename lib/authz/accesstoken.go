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

package authz

import (
	"strconv"
	"time"
)

// SuggestedEarlyAccessTokenRefresh is how long before an access token actually expires that web
// clients should consider refreshing the token. This prevents annoying client-side errors,
// when the client thinks its access token is still valid, makes a request, but by the time the server
// is actually getting around to processing the request, the access token is already expired.
const SuggestedEarlyAccessTokenRefresh time.Duration = -10 * time.Second

func (j JWTer) CreateAccessToken(
	rangerName string,
	clubhouseID int64,
	positionIDs []int64,
	teamIDs []int64,
	onsite bool,
	onDutyPositionID *int64,
	expiration time.Time,
) (string, error) {
	return j.createJWT(
		IMSClaims{}.
			WithIssuedAt(time.Now()).
			WithExpiration(expiration).
			WithIssuer("ims").
			WithRangerHandle(rangerName).
			WithRangerOnSite(onsite).
			WithRangerOnDutyPosition(onDutyPositionID).
			WithRangerPositions(positionIDs...).
			WithRangerTeams(teamIDs...).
			WithSubject(strconv.FormatInt(clubhouseID, 10)),
	)
}

// AuthenticateJWT gives JWT claims for a valid, authenticated JWT string, or
// returns an error otherwise. A JWT may be invalid because it was signed by a
// different key, because it has expired, etc.
func (j JWTer) AuthenticateJWT(jwtStr string) (*IMSClaims, error) {
	return j.authenticateJWT(jwtStr)
}
