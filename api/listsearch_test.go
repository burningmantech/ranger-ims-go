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

package api

import (
	"testing"

	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseListQuery_SmartSearch(t *testing.T) {
	t.Parallel()
	match, errHTTP := parseListQuery("fire  CAMP")
	require.Nil(t, errHTTP)
	assert.True(t, match("camp is on fire"))
	assert.True(t, match("Fire at the Camp"))
	assert.False(t, match("fire only"))
}

func TestParseListQuery_Phrase(t *testing.T) {
	t.Parallel()
	match, errHTTP := parseListQuery(`"on fire" camp`)
	require.Nil(t, errHTTP)
	assert.True(t, match("camp is on fire"))
	assert.False(t, match("fire on camp"))

	match, errHTTP = parseListQuery("“on fire”")
	require.Nil(t, errHTTP)
	assert.True(t, match("camp is on fire"))
	assert.False(t, match("fire on camp"))
}

func TestParseListQuery_Negation(t *testing.T) {
	t.Parallel()
	match, errHTTP := parseListQuery("camp !fire")
	require.Nil(t, errHTTP)
	assert.True(t, match("camp is calm"))
	assert.False(t, match("camp is on fire"))

	match, errHTTP = parseListQuery(`!"on fire"`)
	require.Nil(t, errHTTP)
	assert.True(t, match("fire on camp"))
	assert.False(t, match("camp is on fire"))

	// A one-character negation is ignored, since it's likely mid-typing.
	match, errHTTP = parseListQuery("camp !f")
	require.Nil(t, errHTTP)
	assert.True(t, match("camp is on fire"))
}

func TestParseListQuery_Diacritics(t *testing.T) {
	t.Parallel()
	match, errHTTP := parseListQuery("cafe")
	require.Nil(t, errHTTP)
	assert.True(t, match("Café Ñoño"))

	match, errHTTP = parseListQuery("ÑOÑO")
	require.Nil(t, errHTTP)
	assert.True(t, match("Café nono"))
}

func TestParseListQuery_Regex(t *testing.T) {
	t.Parallel()
	match, errHTTP := parseListQuery("/f[io]re/")
	require.Nil(t, errHTTP)
	assert.True(t, match("FORE"))
	assert.False(t, match("fare"))

	_, errHTTP = parseListQuery("/(/")
	require.NotNil(t, errHTTP)
	assert.Equal(t, 400, errHTTP.Code)

	// A lone slash is a literal
	match, errHTTP = parseListQuery("/")
	require.Nil(t, errHTTP)
	assert.True(t, match("7:30/E"))
}

func TestListSummary(t *testing.T) {
	t.Parallel()
	entries := []imsdb.ReportEntry{
		{ID: 1, Text: "Changed state to new", Generated: true},
		{ID: 2, Text: "\r\n\nFirst real line\nsecond line"},
		{ID: 3, Text: "Later entry"},
	}
	assert.Equal(t, "Own summary", listSummary("Own summary", entries))
	assert.Equal(t, "First real line", listSummary("", entries))
	assert.Empty(t, listSummary("", entries[:1]))
	assert.Empty(t, listSummary("", nil))

	long := make([]byte, 500)
	for i := range long {
		long[i] = 'x'
	}
	summary := listSummary("", []imsdb.ReportEntry{{Text: string(long)}})
	assert.Less(t, len(summary), 300)
	assert.Equal(t, searchSnippetMarker, summary[len(summary)-len(searchSnippetMarker):])
}

func TestSortEntries(t *testing.T) {
	t.Parallel()
	entries := []imsdb.ReportEntry{
		{ID: 3, Created: 20},
		{ID: 2, Created: 10},
		{ID: 1, Created: 10},
	}
	sortEntries(entries)
	assert.Equal(t, []int32{1, 2, 3}, []int32{entries[0].ID, entries[1].ID, entries[2].ID})
}
