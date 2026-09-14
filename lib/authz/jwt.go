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
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

var ErrNoJWTString = errors.New("no JWT string provided")

type JWTer struct {
	SecretKey string
}

func (j JWTer) createJWT(claims IMSClaims) (string, error) {
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).
		SignedString([]byte(j.SecretKey))
	if err != nil {
		return "", fmt.Errorf("[SignedString]: %w", err)
	}
	return token, nil
}

func (j JWTer) authenticateJWT(jwtStr, wantTokenType string) (*IMSClaims, error) {
	if jwtStr == "" {
		return nil, ErrNoJWTString
	}
	claims := IMSClaims{}
	tok, err := jwt.ParseWithClaims(jwtStr, &claims, func(token *jwt.Token) (any, error) {
		return []byte(j.SecretKey), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}))
	if err != nil {
		return nil, fmt.Errorf("[jwt.Parse]: %w", err)
	}
	if tok == nil {
		return nil, errors.New("token is nil")
	}
	if !tok.Valid {
		return nil, errors.New("token is invalid")
	}
	if claims.RangerHandle() == "" {
		return nil, errors.New("ranger handle is required")
	}
	// This is what stops an old refresh token, signed by the same key, from
	// being used as an access token.
	if claims.TokenType != wantTokenType {
		return nil, fmt.Errorf("token type %q is not valid here", claims.TokenType)
	}
	return &claims, nil
}
