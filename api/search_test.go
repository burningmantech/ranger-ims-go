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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndexFold(t *testing.T) {
	t.Parallel()

	// Case-insensitive match, at the correct byte offset.
	assert.Equal(t, 4, indexFold("the NEEDLE", "needle"))

	// No match.
	assert.Equal(t, -1, indexFold("haystack", "needle"))

	// The offset is valid in the original string even when lowercasing
	// changes rune byte lengths ("İ" is 2 bytes; ToLower maps it to the
	// 1-byte "i"). Indexing into ToLower(s) would give 100 here.
	assert.Equal(t, 200, indexFold(strings.Repeat("İ", 100)+"foo", "foo"))
}

func TestSearchSnippet(t *testing.T) {
	t.Parallel()

	// Short text comes back whole, with no ellipses.
	assert.Equal(t, "a broken taillight", searchSnippet("a broken taillight", "taillight"))

	// A match deep in a long text is excerpted, with ellipses at both ends.
	longText := strings.Repeat("a", 100) + "NEEDLE" + strings.Repeat("b", 300)
	snippet := searchSnippet(longText, "needle")
	assert.Contains(t, snippet, "NEEDLE")
	assert.True(t, strings.HasPrefix(snippet, searchSnippetMarker))
	assert.True(t, strings.HasSuffix(snippet, searchSnippetMarker))

	// When the query isn't found (the DB's collation matches more loosely
	// than we do), the excerpt comes from the start of the text.
	snippet = searchSnippet(longText, "zzz")
	assert.True(t, strings.HasPrefix(snippet, "aaa"))

	// Lowercasing "Ⱥ" grows it from 2 bytes to 3. A match beyond a run of
	// such characters used to produce an out-of-range offset and panic.
	growText := strings.Repeat("Ⱥ", 500) + " hello world"
	snippet = searchSnippet(growText, "hello")
	assert.Contains(t, snippet, "hello world")

	// Lowercasing "İ" shrinks it from 2 bytes to 1. A match beyond a run of
	// such characters used to yield a misaligned excerpt missing the match.
	shrinkText := strings.Repeat("İ", 100) + " hello world"
	snippet = searchSnippet(shrinkText, "hello")
	assert.Contains(t, snippet, "hello world")

	// Empty text yields an empty snippet.
	assert.Empty(t, searchSnippet("", "needle"))
}
