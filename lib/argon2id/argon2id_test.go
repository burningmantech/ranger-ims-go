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

package argon2id

import (
	"errors"
	"github.com/stretchr/testify/require"
	"regexp"
	"strings"
	"testing"
)

func TestCreateHash(t *testing.T) {
	t.Parallel()

	hashRX := regexp.MustCompile(`^\$argon2id\$v=19\$m=65536,t=1,p=[0-9]{1,4}\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`)

	hash1 := CreateHash("pa$$word", DevelopmentParams)
	if !hashRX.MatchString(hash1) {
		t.Errorf("hash %q not in correct format", hash1)
	}

	hash2 := CreateHash("pa$$word", DevelopmentParams)

	if strings.Compare(hash1, hash2) == 0 {
		t.Error("hashes must be unique")
	}
}

func TestComparePasswordAndHash(t *testing.T) {
	t.Parallel()

	hash := CreateHash("pa$$word", DevelopmentParams)

	match, err := ComparePasswordAndHash("pa$$word", hash)
	if err != nil {
		t.Fatal(err)
	}

	if !match {
		t.Error("expected password and hash to match")
	}

	match, err = ComparePasswordAndHash("otherPa$$word", hash)
	if err != nil {
		t.Fatal(err)
	}

	if match {
		t.Error("expected password and hash to not match")
	}
}

func TestDecodeHash(t *testing.T) {
	t.Parallel()

	hash := CreateHash("pa$$word", DevelopmentParams)

	params, _, _, err := DecodeHash(hash)
	if err != nil {
		t.Fatal(err)
	}
	if *params != *DevelopmentParams {
		t.Fatalf("expected %#v got %#v", *DevelopmentParams, *params)
	}
}

func TestCheckHash(t *testing.T) {
	t.Parallel()

	hash := CreateHash("pa$$word", DevelopmentParams)

	ok, params, err := CheckHash("pa$$word", hash)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected password to match")
	}
	if *params != *DevelopmentParams {
		t.Fatalf("expected %#v got %#v", *DevelopmentParams, *params)
	}
}

func TestStrictDecoding(t *testing.T) {
	t.Parallel()

	// "bug" valid hash: $argon2id$v=19$m=65536,t=1,p=2$UDk0zEuIzbt0x3bwkf8Bgw$ihSfHWUJpTgDvNWiojrgcN4E0pJdUVmqCEdRZesx9tE
	ok, _, err := CheckHash("bug", "$argon2id$v=19$m=65536,t=1,p=2$UDk0zEuIzbt0x3bwkf8Bgw$ihSfHWUJpTgDvNWiojrgcN4E0pJdUVmqCEdRZesx9tE")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected password to match")
	}

	// changed one last character of the hash
	ok, _, err = CheckHash("bug", "$argon2id$v=19$m=65536,t=1,p=2$UDk0zEuIzbt0x3bwkf8Bgw$ihSfHWUJpTgDvNWiojrgcN4E0pJdUVmqCEdRZesx9tF")
	if err == nil {
		t.Fatal("Hash validation should fail")
	}

	if ok {
		t.Fatal("Hash validation should fail")
	}
}

func TestVariant(t *testing.T) {
	t.Parallel()

	// Hash contains wrong variant
	_, _, err := CheckHash("pa$$word", "$argon2i$v=19$m=65536,t=1,p=2$mFe3kxhovyEByvwnUtr0ow$nU9AqnoPfzMOQhCHa9BDrQ+4bSfj69jgtvGu/2McCxU")
	if !errors.Is(err, ErrIncompatibleVariant) {
		t.Fatalf("expected error %s", ErrIncompatibleVariant)
	}
}

func TestPHPExample(t *testing.T) {
	t.Parallel()

	// This example comes from https://www.erianna.com/introducing-support-for-argon2id-in-php73/
	match, err := ComparePasswordAndHash(
		"test",
		"$argon2id$v=19$m=1024,t=2,p=2$WS90MHJhd3AwSC5xTDJpZg$8tn2DaIJR2/UX4Cjcy2t3EZaLDL/qh+NbLQAOvTmdAg",
	)
	require.NoError(t, err)
	require.True(t, match)
}
