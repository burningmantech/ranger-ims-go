package auth

import (
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

// CreateRefreshToken creates a refresh token, which the client can use to request new access tokens,
// based on any updated claims from the UserStore. It's an implementation detail that this uses an
// access token-style JWT. Ideally a refresh token is supposed to be persisted, so that it can be
// invalidated from the server side. As a stopgap before we have such a per-user persistence component,
// we instead rely on the security of JWT signing.
func (j JWTer) CreateRefreshToken(rangerName string, clubhouseID int64, expiration time.Time) (string, error) {
	return j.createJWT(
		NewIMSClaims().
			WithIssuedAt(time.Now()).
			WithExpiration(expiration).
			WithIssuer("ranger-ims-go").
			WithRangerHandle(rangerName).
			WithSubject(strconv.FormatInt(clubhouseID, 10)),
	)
}

// AuthenticateRefreshToken is like AuthenticateJWT, in that it validates that the
// supplied token is valid (was signed by the same secret key and hasn't expired).
// It's an implementation detail that refresh tokens are also JWTs. Clients of IMS
// should treat them as simply opaque strings.
func (j JWTer) AuthenticateRefreshToken(refreshToken string) (*IMSClaims, error) {
	return j.authenticateJWT(refreshToken)
}
