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
)

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
