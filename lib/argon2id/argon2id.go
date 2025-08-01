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

// Package argon2id provides a convenience wrapper around Go's golang.org/x/crypto/argon2
// implementation, making it simpler to securely hash and verify passwords
// using Argon2.
//
// It enforces use of the Argon2id algorithm variant and cryptographically-secure
// random salts.
//
// This package was copied from https://github.com/alexedwards/argon2id
package argon2id

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"

	"golang.org/x/crypto/argon2"
)

var (
	// ErrInvalidHash in returned by ComparePasswordAndHash if the provided
	// hash isn't in the expected format.
	ErrInvalidHash = errors.New("argon2id: hash is not in the correct format")

	// ErrIncompatibleVariant is returned by ComparePasswordAndHash if the
	// provided hash was created using a unsupported variant of Argon2.
	// Currently only argon2id is supported by this package.
	ErrIncompatibleVariant = errors.New("argon2id: incompatible variant of argon2")

	// ErrIncompatibleVersion is returned by ComparePasswordAndHash if the
	// provided hash was created using a different version of Argon2.
	ErrIncompatibleVersion = errors.New("argon2id: incompatible version of argon2")
)

// DevelopmentParams provides some sane default parameters for hashing passwords.
//
// Follows recommendations given by the Argon2 RFC:
// "The Argon2id variant with t=1 and maximum available memory is RECOMMENDED as a
// default setting for all environments. This setting is secure against side-channel
// attacks and maximizes adversarial costs on dedicated bruteforce hardware.""
//
// The default parameters should generally be used for development/testing purposes
// only. Custom parameters should be set for production applications depending on
// available memory/CPU resources and business requirements.
//
// See RFC 9106 for recommended parameter choices:
// https://www.rfc-editor.org/rfc/rfc9106.html#name-parameter-choice
var DevelopmentParams = &Params{
	MemoryKiB:   64 * 1024, // 64 MiB
	Iterations:  1,
	Parallelism: uint8(runtime.NumCPU()),
	SaltLength:  16,
	KeyLength:   32,
}

// FirstRecommendedParams should be used if an "option that is not tailored to your application
// or hardware is acceptable".
// This is RFC 9106's first recommended option.
// See https://www.rfc-editor.org/rfc/rfc9106.html#name-parameter-choice
var FirstRecommendedParams = &Params{
	MemoryKiB:   2 * 1024 * 1024, // 2 GiB
	Iterations:  1,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

// SecondRecommendedParams should be used if "much less memory is available".
// This is RFC 9106's second recommended option.
// See https://www.rfc-editor.org/rfc/rfc9106.html#name-parameter-choice
var SecondRecommendedParams = &Params{
	MemoryKiB:   64 * 1024, // 64 MiB
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

// PHPDefaultParams are the default parameters used by PHP via libargon2 and
// PASSWORD_ARGON2ID, as of 2025-07-25.
var PHPDefaultParams = &Params{
	MemoryKiB:   64 * 1024, // 64 MiB
	Iterations:  4,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

// ClubhouseParams are the parameters used by Clubhouse, last updated 2025-07-25.
var ClubhouseParams = &Params{
	MemoryKiB:   8 * 1024, // 8 MiB
	Iterations:  4,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

// Params describes the input parameters used by the Argon2id algorithm. The
// MemoryKiB and Iterations parameters control the computational cost of hashing
// the password. The higher these figures are, the greater the cost of generating
// the hash and the longer the runtime. It also follows that the greater the cost
// will be for any attacker trying to guess the password. If the code is running
// on a machine with multiple cores, then you can decrease the runtime without
// reducing the cost by increasing the Parallelism parameter. This controls the
// number of threads that the work is spread across. Important note: Changing the
// value of the Parallelism parameter changes the hash output.
//
// For guidance and an outline process for choosing appropriate parameters see
// https://tools.ietf.org/html/draft-irtf-cfrg-argon2-04#section-4
type Params struct {
	// The amount of memory used by the algorithm (in kibibytes).
	MemoryKiB uint32

	// The number of iterations over the memory.
	Iterations uint32

	// The number of threads (or lanes) used by the algorithm.
	// Recommended value is between 1 and runtime.NumCPU().
	Parallelism uint8

	// Length of the random salt. 16 bytes is recommended for password hashing.
	SaltLength uint32

	// Length of the generated key. 16 bytes or more is recommended.
	KeyLength uint32
}

// CreateHash returns an Argon2id hash of a plain-text password using the
// provided algorithm parameters. The returned hash follows the format used by
// the Argon2 reference C implementation and contains the base64-encoded Argon2id
// derived key prefixed by the salt and parameters. It looks like this:
//
//	$argon2id$v=19$m=65536,t=3,p=2$c29tZXNhbHQ$RdescudvJCsgt3ub+b+dWRWJTmaaJObG
func CreateHash(password string, params *Params) (hash string) {
	salt := generateRandomBytes(params.SaltLength)

	key := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKiB, params.Parallelism, params.KeyLength)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Key := base64.RawStdEncoding.EncodeToString(key)

	hash = fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, params.MemoryKiB, params.Iterations,
		params.Parallelism, b64Salt, b64Key,
	)
	return hash
}

// ComparePasswordAndHash performs a constant-time comparison between a
// plain-text password and Argon2id hash, using the parameters and salt
// contained in the hash. It returns true if they match, otherwise it returns
// false.
func ComparePasswordAndHash(password, hash string) (match bool, err error) {
	match, _, err = CheckHash(password, hash)
	return match, err
}

// CheckHash is like ComparePasswordAndHash, except it also returns the params that the hash was
// created with. This can be useful if you want to update your hash params over time (which you
// should).
func CheckHash(password, hash string) (match bool, params *Params, err error) {
	params, salt, key, err := DecodeHash(hash)
	if err != nil {
		return false, nil, err
	}

	otherKey := argon2.IDKey(
		[]byte(password), salt, params.Iterations,
		params.MemoryKiB, params.Parallelism, params.KeyLength,
	)

	keyLen := int32(len(key))
	otherKeyLen := int32(len(otherKey))

	if subtle.ConstantTimeEq(keyLen, otherKeyLen) == 0 {
		return false, params, nil
	}
	if subtle.ConstantTimeCompare(key, otherKey) == 1 {
		return true, params, nil
	}
	return false, params, nil
}

func generateRandomBytes(n uint32) []byte {
	b := make([]byte, n)
	// this is guaranteed to never return an error
	_, _ = rand.Read(b)
	return b
}

// DecodeHash expects a hash created from this package, and parses it to return the params used to
// create it, as well as the salt and key (password hash).
func DecodeHash(hash string) (params *Params, salt, key []byte, err error) {
	r := strings.NewReader(hash)

	_, err = fmt.Fscanf(r, "$argon2id$")
	if err != nil {
		return nil, nil, nil, ErrIncompatibleVariant
	}

	var version int
	_, err = fmt.Fscanf(r, "v=%d$", &version)
	if err != nil {
		return nil, nil, nil, err
	}
	if version != argon2.Version {
		return nil, nil, nil, ErrIncompatibleVersion
	}

	params = &Params{}
	_, err = fmt.Fscanf(r, "m=%d,t=%d,p=%d$", &params.MemoryKiB, &params.Iterations, &params.Parallelism)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("[fmt.Fscanf] params: %w", err)
	}

	rest, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("[io.ReadAll] rest: %w", err)
	}
	if bytes.ContainsAny(rest, "\r\n") { // base64 decoder ignores these
		return nil, nil, nil, ErrInvalidHash
	}

	var i int
	if i = bytes.IndexByte(rest, '$'); i == -1 {
		return nil, nil, nil, ErrInvalidHash
	}
	b64Enc := base64.RawStdEncoding.Strict()

	salt = make([]byte, b64Enc.DecodedLen(i))
	_, err = b64Enc.Decode(salt, rest[:i])
	if err != nil {
		return nil, nil, nil, fmt.Errorf("[Decode] salt: %w", err)
	}
	params.SaltLength = uint32(len(salt))

	key = make([]byte, b64Enc.DecodedLen(len(rest)-i-1))
	_, err = b64Enc.Decode(key, rest[i+1:])
	if err != nil {
		return nil, nil, nil, err
	}
	params.KeyLength = uint32(len(key))

	return params, salt, key, nil
}
