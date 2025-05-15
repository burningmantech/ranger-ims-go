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
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestExtensionByType(t *testing.T) {
	t.Parallel()
	assert.Equal(t, ".bmp", extensionByType("image/bmp"))
	assert.Equal(t, ".csv", extensionByType("text/csv"))
	assert.Equal(t, ".gif", extensionByType("image/gif"))
	assert.Equal(t, ".html", extensionByType("text/html"))
	assert.Equal(t, ".jpg", extensionByType("image/jpeg"))
	assert.Equal(t, ".mp4", extensionByType("video/mp4"))
	assert.Equal(t, ".pdf", extensionByType("application/pdf"))
	assert.Equal(t, ".png", extensionByType("image/png"))
	assert.Equal(t, ".txt", extensionByType("text/plain"))
	assert.Equal(t, ".zip", extensionByType("application/zip"))

	assert.Empty(t, extensionByType("notta/mime"))
}
