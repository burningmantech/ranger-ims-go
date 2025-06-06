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

package authz_test

import (
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/stretchr/testify/require"
	"math/big"
	"testing"
	"time"
)

func TestCreateAndGetValidJWT(t *testing.T) {
	t.Parallel()

	jwter := authz.JWTer{SecretKey: "some-secret"}
	j, err := jwter.CreateAccessToken(
		"Hardware",
		12345,
		[]int64{10, 20, 40, 150},
		[]int64{15, 25, 45, 155},
		true,
		time.Now().Add(1*time.Hour),
	)
	require.NoError(t, err)
	claims, err := jwter.AuthenticateJWT(j)
	require.NoError(t, err)
	sub, err := claims.GetSubject()
	require.NoError(t, err)
	require.Equal(t, "Hardware", claims.RangerHandle())
	require.Equal(t, "12345", sub)
	require.Equal(t, []int64{10, 20, 40, 150}, claims.RangerPositions())
	require.Equal(t, []int64{15, 25, 45, 155}, claims.RangerTeams())
	require.True(t, claims.RangerOnSite())
}

func TestCreateAndGetInvalidJWTs(t *testing.T) {
	t.Parallel()
	jwter := authz.JWTer{SecretKey: "some-secret"}
	{
		expiredJWT, err := jwter.CreateAccessToken(
			"Hardware",
			1,
			nil,
			nil,
			true,
			time.Now().Add(-1*time.Hour),
		)
		require.NoError(t, err)
		_, err = jwter.AuthenticateJWT(expiredJWT)
		require.Error(t, err)
		require.Contains(t, err.Error(), "expired")
	}
	{
		signedWithDifferentKeyJWT, err := authz.JWTer{SecretKey: "some-other-secret"}.CreateAccessToken(
			"Hardware",
			1,
			nil,
			nil,
			true,
			time.Now().Add(1*time.Hour),
		)
		require.NoError(t, err)
		_, err = jwter.AuthenticateJWT(signedWithDifferentKeyJWT)
		require.Error(t, err)
		require.Contains(t, err.Error(), "signature is invalid")
	}
	{
		hasNoRangerHandleJWT, err := jwter.CreateAccessToken(
			// empty RangerName
			"",
			12345,
			nil,
			nil,
			true,
			time.Now().Add(1*time.Hour),
		)
		require.NoError(t, err)
		_, err = jwter.AuthenticateJWT(hasNoRangerHandleJWT)
		require.Error(t, err)
		require.Contains(t, err.Error(), "ranger handle is required")
	}
}

func BenchmarkBitSet(b *testing.B) {
	var ints []int64
	// there are about 170 positions in Clubhouse prod, so 200 is a useful benchmark number
	for i := range 200 {
		ints = append(ints, int64(i))
	}
	for b.Loop() {
		tally := big.NewInt(0)
		for _, p := range ints {
			tally.SetBit(tally, int(p), 1)
		}
	}
}
