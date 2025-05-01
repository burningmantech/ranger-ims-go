package auth

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
	positions []string,
	teams []string,
	onsite bool,
	expiration time.Time,
) (string, error) {
	return j.createJWT(
		NewIMSClaims().
			WithIssuedAt(time.Now()).
			WithExpiration(expiration).
			WithIssuer("ranger-ims-go").
			WithRangerHandle(rangerName).
			WithRangerOnSite(onsite).
			WithRangerPositions(positions...).
			WithRangerTeams(teams...).
			WithSubject(strconv.FormatInt(clubhouseID, 10)),
	)
}

// AuthenticateJWT gives JWT claims for a valid, authenticated JWT string, or
// returns an error otherwise. A JWT may be invalid because it was signed by a
// different key, because it has expired, etc.
func (j JWTer) AuthenticateJWT(jwtStr string) (*IMSClaims, error) {
	return j.authenticateJWT(jwtStr)
}
