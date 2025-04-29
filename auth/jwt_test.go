package auth

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestCreateAndGetValidJWT(t *testing.T) {
	jwter := JWTer{"some-secret"}
	j := jwter.CreateJWT(
		"Hardware",
		12345,
		[]string{"Fluffer", "Operator"},
		[]string{"Fluff Squad"},
		true,
		1*time.Hour,
	)
	claims, err := jwter.AuthenticateJWT(j)
	require.NoError(t, err)
	sub, err := claims.GetSubject()
	require.NoError(t, err)
	require.Equal(t, "Hardware", claims.RangerHandle())
	require.Equal(t, "12345", sub)
	require.Equal(t, []string{"Fluffer", "Operator"}, claims.RangerPositions())
	require.Equal(t, []string{"Fluff Squad"}, claims.RangerTeams())
	require.Equal(t, true, claims.RangerOnSite())
}

func TestCreateAndGetInvalidJWTs(t *testing.T) {
	jwter := JWTer{"some-secret"}
	expiredJWT := jwter.CreateJWT(
		"Hardware",
		1,
		nil,
		nil,
		true,
		-1*time.Hour,
	)
	differentKeyJWT := JWTer{"some-other-secret"}.CreateJWT(
		"Hardware",
		1,
		nil,
		nil,
		true,
		1*time.Hour,
	)
	_, err := jwter.AuthenticateJWT(expiredJWT)
	require.Error(t, err)
	require.Contains(t, err.Error(), "expired")
	_, err = jwter.AuthenticateJWT(differentKeyJWT)
	require.Error(t, err)
	require.Contains(t, err.Error(), "signature is invalid")
}
