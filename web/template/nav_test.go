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

package template_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/burningmantech/ranger-ims-go/web/template"
	"github.com/stretchr/testify/require"
)

func TestNavLogoLink(t *testing.T) {
	t.Parallel()

	logoLink := func(t *testing.T, eventName string) string {
		t.Helper()
		var sb strings.Builder
		require.NoError(t, template.Nav(eventName).Render(t.Context(), &sb))
		match := regexp.MustCompile(`<a id="ims-logo"[^>]*href="([^"]*)"`).FindStringSubmatch(sb.String())
		require.NotNil(t, match, "no ims-logo link found")
		return match[1]
	}

	t.Run("no event", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "/ims/app", logoLink(t, ""))
	})

	t.Run("event", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "/ims/app/events/2026/incidents", logoLink(t, "2026"))
	})
}
