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

package template

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strings"

	"github.com/a-h/templ"
)

// importMap points the page modules' imports of ims.js at the current build's
// copy of it, the same cache-busted URL that the page loads its own module from.
func importMap(cacheVersion string) templ.Component {
	return templ.JSONScript("importmap", map[string]any{
		"imports": map[string]string{
			"/ims/static/ims.js": "/ims/static/ims.js?v=" + cacheVersion[:8],
		},
	}).WithType("importmap")
}

// ImportMapHash returns the Content-Security-Policy source expression that
// allows the import map, which is the only inline script on any IMS page. It
// hashes the rendered element's contents rather than re-encoding the JSON, so
// it can't drift from what templ actually writes.
func ImportMapHash(cacheVersion string) string {
	var b strings.Builder
	err := importMap(cacheVersion).Render(context.Background(), &b)
	if err != nil {
		panic("this cannot happen, because strings.Builder can't return an error on write: " + err.Error())
	}
	_, script, _ := strings.Cut(b.String(), ">")
	script, _, _ = strings.Cut(script, "</script>")
	sum := sha256.Sum256([]byte(script))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}
