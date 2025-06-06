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
	"math/big"
	"time"
)

const compactIntBase = 62

type IMSClaims struct {
	jwt.RegisteredClaims
	Handle    string `json:"han"`
	Onsite    bool   `json:"ons"`
	Positions string `json:"pos"`
	Teams     string `json:"tea"`
}

func unmarshalBigInt(s string) *big.Int {
	if s == "" {
		return big.NewInt(0)
	}
	var z big.Int
	_, ok := z.SetString(s, compactIntBase)
	if !ok {
		return big.NewInt(0)
	}
	return &z
}

func bitSetToInts(bigint *big.Int) []int64 {
	if bigint.Cmp(big.NewInt(0)) == -1 {
		panic("got bigint less than zero")
	}
	var ints []int64
	for i := range bigint.BitLen() {
		if bigint.Bit(i) != 0 {
			ints = append(ints, int64(i))
		}
	}
	return ints
}

func intsToBitSet(ints []int64) *big.Int {
	bitset := big.NewInt(0)
	for _, t := range ints {
		bitset.SetBit(bitset, int(t), 1)
	}
	return bitset
}

func marshalBigInt(b *big.Int) string {
	return b.Text(compactIntBase)
}

func (c IMSClaims) WithExpiration(t time.Time) IMSClaims {
	c.ExpiresAt = jwt.NewNumericDate(t)
	return c
}

func (c IMSClaims) WithIssuedAt(t time.Time) IMSClaims {
	c.IssuedAt = jwt.NewNumericDate(t)
	return c
}

func (c IMSClaims) WithIssuer(s string) IMSClaims {
	c.Issuer = s
	return c
}

func (c IMSClaims) WithSubject(s string) IMSClaims {
	c.Subject = s
	return c
}

func (c IMSClaims) WithRangerHandle(s string) IMSClaims {
	c.Handle = s
	return c
}

func (c IMSClaims) WithRangerOnSite(onsite bool) IMSClaims {
	c.Onsite = onsite
	return c
}

func (c IMSClaims) WithRangerPositions(pos ...int64) IMSClaims {
	c.Positions = marshalBigInt(intsToBitSet(pos))
	return c
}

func (c IMSClaims) WithRangerTeams(teams ...int64) IMSClaims {
	c.Teams = marshalBigInt(intsToBitSet(teams))
	return c
}

func (c IMSClaims) RangerHandle() string {
	return c.Handle
}

func (c IMSClaims) RangerOnSite() bool {
	return c.Onsite
}

func (c IMSClaims) RangerPositions() []int64 {
	return bitSetToInts(unmarshalBigInt(c.Positions))
}

func (c IMSClaims) RangerTeams() []int64 {
	return bitSetToInts(unmarshalBigInt(c.Teams))
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
