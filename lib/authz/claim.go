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
	"github.com/burningmantech/ranger-ims-go/lib/conv"
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

// DirectoryID returns the Clubhouse ID for a Ranger.
// It returns -1 if the ID cannot be determined.
func (c IMSClaims) DirectoryID() int64 {
	sub, err := c.GetSubject()
	if err != nil {
		return -1
	}
	subN, err := conv.ParseInt64(sub)
	if err != nil {
		return -1
	}
	return subN
}
