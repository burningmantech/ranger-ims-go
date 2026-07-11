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
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
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

func TestSearchQueryError(t *testing.T) {
	t.Parallel()

	// A database-side regexp error means the client sent a bad pattern.
	regexpErr := &mysql.MySQLError{Number: 1139, Message: "Regex error 'missing closing parenthesis'"}
	httpErr := searchQueryError("Incidents", regexpErr)
	assert.Equal(t, http.StatusBadRequest, httpErr.Code)

	// Hitting the search deadline is reported as unavailability, not a 500.
	deadlineErr := fmt.Errorf("query: %w", context.DeadlineExceeded)
	httpErr = searchQueryError("Incidents", deadlineErr)
	assert.Equal(t, http.StatusServiceUnavailable, httpErr.Code)

	// Anything else is an internal error.
	httpErr = searchQueryError("Incidents", errors.New("connection lost"))
	assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
}

func TestSearchSnippet(t *testing.T) {
	t.Parallel()

	// Short text comes back whole, with no ellipses.
	text := "a broken taillight"
	assert.Equal(t, "a broken taillight", searchSnippet(text, indexFold(text, "taillight")))

	// A match deep in a long text is excerpted, with ellipses at both ends.
	longText := strings.Repeat("a", 100) + "NEEDLE" + strings.Repeat("b", 300)
	snippet := searchSnippet(longText, indexFold(longText, "needle"))
	assert.Contains(t, snippet, "NEEDLE")
	assert.True(t, strings.HasPrefix(snippet, searchSnippetMarker))
	assert.True(t, strings.HasSuffix(snippet, searchSnippetMarker))

	// When the match wasn't found (the DB's collation matches more loosely
	// than we do), the excerpt comes from the start of the text.
	snippet = searchSnippet(longText, indexFold(longText, "zzz"))
	assert.True(t, strings.HasPrefix(snippet, "aaa"))

	// A match located by a regexp is excerpted the same way.
	loc := regexp.MustCompile(`(?i)ne+dle`).FindStringIndex(longText)
	assert.NotNil(t, loc)
	snippet = searchSnippet(longText, loc[0])
	assert.Contains(t, snippet, "NEEDLE")
	assert.True(t, strings.HasPrefix(snippet, searchSnippetMarker))

	// Lowercasing "Ⱥ" grows it from 2 bytes to 3. A match beyond a run of
	// such characters used to produce an out-of-range offset and panic.
	growText := strings.Repeat("Ⱥ", 500) + " hello world"
	snippet = searchSnippet(growText, indexFold(growText, "hello"))
	assert.Contains(t, snippet, "hello world")

	// Lowercasing "İ" shrinks it from 2 bytes to 1. A match beyond a run of
	// such characters used to yield a misaligned excerpt missing the match.
	shrinkText := strings.Repeat("İ", 100) + " hello world"
	snippet = searchSnippet(shrinkText, indexFold(shrinkText, "hello"))
	assert.Contains(t, snippet, "hello world")

	// Empty text yields an empty snippet.
	assert.Empty(t, searchSnippet("", -1))
}
