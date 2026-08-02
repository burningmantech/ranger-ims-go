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

package format_test

import (
	"testing"

	"github.com/burningmantech/ranger-ims-go/lib/format"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeAddress_radialThenConcentric(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "7:00 & E", format.NormalizeAddress("7+e"))
	assert.Equal(t, "7:00 & E", format.NormalizeAddress("7:00&e"))
	assert.Equal(t, "7:00 & E", format.NormalizeAddress("7 and e"))
	assert.Equal(t, "7:00 & E", format.NormalizeAddress("7 & E"))
	assert.Equal(t, "7:00 & E", format.NormalizeAddress("7, e"))
	assert.Equal(t, "7:00 & E", format.NormalizeAddress("7/e"))
	assert.Equal(t, "7:00 & E", format.NormalizeAddress("7 e"))
	assert.Equal(t, "7:00 & E", format.NormalizeAddress("  7:00  &  e.  "))
	assert.Equal(t, "7:00 & E", format.NormalizeAddress("07:00 & E"))
	assert.Equal(t, "7:15 & K", format.NormalizeAddress("7:15+k"))
	assert.Equal(t, "7:05 & A", format.NormalizeAddress("7:5 & a"))
	assert.Equal(t, "12:00 & L", format.NormalizeAddress("12&l"))
	assert.Equal(t, "0:15 & B", format.NormalizeAddress("0:15 b"))
}

func TestNormalizeAddress_colonlessRadial(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "8:13 & G", format.NormalizeAddress("813&g"))
	assert.Equal(t, "8:13 & G", format.NormalizeAddress("813 g"))
	assert.Equal(t, "10:15 & G", format.NormalizeAddress("1015 + g"))
	assert.Equal(t, "12:30 & Esplanade", format.NormalizeAddress("1230 esp"))
	assert.Equal(t, "0:15 & B", format.NormalizeAddress("015 and b"))
	assert.Equal(t, "G & 8:13", format.NormalizeAddress("g,813"))
	assert.Equal(t, "8:13 500'", format.NormalizeAddress("813 500'"))

	// The hour and minute still have to be real ones.
	assert.Equal(t, "999 & G", format.NormalizeAddress("999 & G"))
	assert.Equal(t, "1330 & G", format.NormalizeAddress("1330 & G"))
}

func TestNormalizeAddress_concentricThenRadial(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "E & 7:00", format.NormalizeAddress("e+7"))
	assert.Equal(t, "E & 7:00", format.NormalizeAddress("E & 7:00"))
	assert.Equal(t, "E & 7:30", format.NormalizeAddress("e and 7:30"))
	assert.Equal(t, "C & 3:00", format.NormalizeAddress("c,3"))
}

func TestNormalizeAddress_esplanade(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "4:00 & Esplanade", format.NormalizeAddress("4&esp"))
	assert.Equal(t, "4:00 & Esplanade", format.NormalizeAddress("4:00 & Esplanade"))
	assert.Equal(t, "4:00 & Esplanade", format.NormalizeAddress("4 espl"))
	assert.Equal(t, "4:00 & Esplanade", format.NormalizeAddress("4 + ESP."))
	assert.Equal(t, "Esplanade & 4:00", format.NormalizeAddress("esp&4"))
}

func TestNormalizeAddress_artAddresses(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "12:00 2500'", format.NormalizeAddress("12:00 2500'"))
	assert.Equal(t, "12:00 2500'", format.NormalizeAddress("12 2500'"))
	assert.Equal(t, "12:00 2500'", format.NormalizeAddress("12, 2500ft"))
	assert.Equal(t, "12:00 2500'", format.NormalizeAddress("12 & 2500 feet"))
	assert.Equal(t, "12:00 2500'", format.NormalizeAddress("12:00 2500"))
	assert.Equal(t, "0:15 500'", format.NormalizeAddress("0:15 500'"))
	assert.Equal(t, "0:15 500'", format.NormalizeAddress("0:15 0500"))
	assert.Equal(t, "6:00 80'", format.NormalizeAddress("6 80ft"))
}

func TestNormalizeAddress_leavesUnrecognizedTextAlone(t *testing.T) {
	t.Parallel()

	assert.Empty(t, format.NormalizeAddress(""))
	assert.Equal(t, "  ", format.NormalizeAddress("  "))
	assert.Equal(t, "behind the porta potties", format.NormalizeAddress("behind the porta potties"))
	assert.Equal(t, "Center Camp", format.NormalizeAddress("Center Camp"))
	assert.Equal(t, "7:00 Plaza", format.NormalizeAddress("7:00 Plaza"))
	assert.Equal(t, "7 & Bacon", format.NormalizeAddress("7 & Bacon"))
	// M is past the last concentric street the normalizer knows.
	assert.Equal(t, "7 & M", format.NormalizeAddress("7 & M"))
	// Hours and minutes out of range.
	assert.Equal(t, "13:00 & E", format.NormalizeAddress("13:00 & E"))
	assert.Equal(t, "7:60 & E", format.NormalizeAddress("7:60 & E"))
	// Too short to be a distance without a unit, and no unit given.
	assert.Equal(t, "12 30", format.NormalizeAddress("12 30"))
	// Not an address at all.
	assert.Equal(t, "7", format.NormalizeAddress("7"))
	assert.Equal(t, "E", format.NormalizeAddress("E"))
	assert.Equal(t, "7:00 & E & 7:00", format.NormalizeAddress("7:00 & E & 7:00"))
}
