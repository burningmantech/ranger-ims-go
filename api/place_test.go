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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlaceExternalData(t *testing.T) {
	t.Parallel()

	// An object that nobody needs to edit goes out exactly as stored.
	raw := placeExternalData("E", "camp", "Camp", []byte(` {"uid": "a1", "location": {"frontage": "E"}} `), false)
	rawJSON, ok := raw.(json.RawMessage)
	assert.True(t, ok)
	assert.JSONEq(t, `{"uid": "a1", "location": {"frontage": "E"}}`, string(rawJSON))

	// The same object is decoded when the caller is going to redact it.
	decoded := placeExternalData("E", "camp", "Camp", []byte(`{"uid": "a1"}`), true)
	assert.Equal(t, map[string]any{"uid": "a1"}, decoded)

	// SQL NULL is no external data.
	assert.Nil(t, placeExternalData("E", "camp", "Camp", nil, false))

	// Neither is anything that isn't an object, nor malformed JSON, which
	// would otherwise break encoding the whole response.
	assert.Nil(t, placeExternalData("E", "camp", "Camp", []byte(`[1,2]`), false))
	assert.Nil(t, placeExternalData("E", "camp", "Camp", []byte(`{"uid": `), false))
}
