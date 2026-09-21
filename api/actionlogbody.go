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
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
)

const (
	// maxLoggedBodyBytes caps what one action log row keeps of a request body.
	// The column holds 64KiB, and no legitimate mutation comes close to this.
	maxLoggedBodyBytes = 16 * 1024

	redacted         = "[redacted]"
	truncationMarker = "…[truncated]"
)

// passwordValue matches a JSON password field and its string value, for
// redacting a body that json.Unmarshal wouldn't take (a truncated or malformed
// one). The structured path below handles everything that parses.
var passwordValue = regexp.MustCompile(`(?i)("[^"]*password[^"]*"\s*:\s*)"(?:[^"\\]|\\.)*"`)

// bodyCapture is the request body with a copy of the first maxLoggedBodyBytes
// kept aside for the action log. It records what the handler actually reads,
// rather than reading the body itself, so a handler that ignores its body (or
// rejects it as too large) costs nothing.
type bodyCapture struct {
	io.ReadCloser

	kept      strings.Builder
	truncated bool
}

func (b *bodyCapture) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if room := maxLoggedBodyBytes - b.kept.Len(); n > 0 && room > 0 {
		if n > room {
			b.truncated = true
			n = room
		}
		b.kept.Write(p[:n])
	} else if n > 0 {
		b.truncated = true
	}
	return n, err //nolint:wrapcheck // pass the underlying body's error through untouched
}

// logged returns the captured body as it should be stored: passwords redacted,
// JSON compacted, and the whole thing valid UTF-8 so MariaDB will take it. It
// returns nil when there was no body to keep.
func (b *bodyCapture) logged() *string {
	if b.kept.Len() == 0 {
		return nil
	}
	raw := b.kept.String()

	var parsed any
	if !b.truncated && json.Unmarshal([]byte(raw), &parsed) == nil {
		compacted, err := json.Marshal(redactPasswords(parsed))
		if err == nil {
			s := string(compacted)
			return &s
		}
	}

	// Not JSON we can work with, so fall back to the raw text. The regex is
	// the only redaction available here, and the marker says why it's odd.
	s := strings.ToValidUTF8(passwordValue.ReplaceAllString(raw, `${1}"`+redacted+`"`), "")
	if b.truncated {
		s += truncationMarker
	}
	return &s
}

// redactPasswords replaces the value of any object field whose name mentions a
// password. Action logs are readable by any IMS admin, and a password set
// through the directory endpoints has no business being in there.
func redactPasswords(v any) any {
	switch typed := v.(type) {
	case map[string]any:
		for k, val := range typed {
			if strings.Contains(strings.ToLower(k), "password") {
				typed[k] = redacted
			} else {
				typed[k] = redactPasswords(val)
			}
		}
		return typed
	case []any:
		for i, val := range typed {
			typed[i] = redactPasswords(val)
		}
		return typed
	default:
		return v
	}
}

// isJSONContentType reports whether the request declares a JSON body, which is
// the only kind worth capturing. Attachment uploads aren't logged at all, but
// this keeps any other multipart body out of the action log too.
func isJSONContentType(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && strings.EqualFold(mediaType, "application/json")
}
