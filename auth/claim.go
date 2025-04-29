package auth

import (
	"github.com/golang-jwt/jwt/v5"
	"strings"
	"time"
)

const (
	handleKey    = "handle"
	onsiteKey    = "onsite"
	positionsKey = "positions"
	teamsKey     = "teams"
)

type IMSClaims struct {
	jwt.MapClaims
}

func NewIMSClaims() IMSClaims {
	return IMSClaims{MapClaims: make(jwt.MapClaims)}
}

func (c IMSClaims) WithExpiration(t time.Time) IMSClaims {
	c.MapClaims["exp"] = t.Unix()
	return c
}

func (c IMSClaims) WithIssuedAt(t time.Time) IMSClaims {
	c.MapClaims["iat"] = t.Unix()
	return c
}

func (c IMSClaims) WithIssuer(s string) IMSClaims {
	c.MapClaims["iss"] = s
	return c
}

func (c IMSClaims) WithSubject(s string) IMSClaims {
	c.MapClaims["sub"] = s
	return c
}

func (c IMSClaims) WithRangerHandle(s string) IMSClaims {
	c.MapClaims[handleKey] = s
	return c
}

func (c IMSClaims) WithRangerOnSite(onsite bool) IMSClaims {
	c.MapClaims[onsiteKey] = onsite
	return c
}

func (c IMSClaims) WithRangerPositions(pos ...string) IMSClaims {
	c.MapClaims[positionsKey] = strings.Join(pos, ",")
	return c
}

func (c IMSClaims) WithRangerTeams(teams ...string) IMSClaims {
	c.MapClaims[teamsKey] = strings.Join(teams, ",")
	return c
}

func (c IMSClaims) RangerHandle() string {
	rh, _ := c.MapClaims[handleKey].(string)
	return rh
}

func (c IMSClaims) RangerOnSite() bool {
	onsite, _ := c.MapClaims[onsiteKey].(bool)
	return onsite
}

func (c IMSClaims) RangerPositions() []string {
	positions, _ := c.MapClaims[positionsKey].(string)
	return strings.Split(positions, ",")
}

func (c IMSClaims) RangerTeams() []string {
	teams, _ := c.MapClaims[teamsKey].(string)
	return strings.Split(teams, ",")
}
