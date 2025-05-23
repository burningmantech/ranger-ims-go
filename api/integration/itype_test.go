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

package integration_test

import (
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestCreateIncidentTypes(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	// Make three new incident types
	typeA, typeB, typeC := rand.NonCryptoText(), rand.NonCryptoText(), rand.NonCryptoText()
	createTypes := imsjson.EditIncidentTypesRequest{
		Add:  imsjson.IncidentTypes{typeA, typeB, typeC},
		Hide: nil,
		Show: nil,
	}
	resp := apis.editTypes(ctx, createTypes)
	require.NoError(t, resp.Body.Close())

	// All three types should now be retrievable and non-hidden
	typesResp, resp := apis.getTypes(ctx, false)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, typesResp, typeA)
	require.Contains(t, typesResp, typeB)
	require.Contains(t, typesResp, typeC)

	// Hide one of those types
	hideOne := imsjson.EditIncidentTypesRequest{
		Hide: imsjson.IncidentTypes{typeA},
	}
	resp = apis.editTypes(ctx, hideOne)
	require.NoError(t, resp.Body.Close())

	// That type should no longer appear from the standard incident type query
	typesVisibleOnly, resp := apis.getTypes(ctx, false)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.NotContains(t, typesVisibleOnly, typeA)
	require.Contains(t, typesVisibleOnly, typeB)
	require.Contains(t, typesVisibleOnly, typeC)
	// but it will still appears when includeHidden=true
	typesIncludeHidden, resp := apis.getTypes(ctx, true)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, typesIncludeHidden, typeA)
	require.Contains(t, typesIncludeHidden, typeB)
	require.Contains(t, typesIncludeHidden, typeC)

	// Unhide that type we previously hid
	showItAgain := imsjson.EditIncidentTypesRequest{
		Show: imsjson.IncidentTypes{typeA, typeB},
	}
	resp = apis.editTypes(ctx, showItAgain)
	require.NoError(t, resp.Body.Close())
	// and see that it's back in the standard incident type query results
	typesVisibleOnly, resp = apis.getTypes(ctx, false)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, typesVisibleOnly, typeA)
	require.Contains(t, typesVisibleOnly, typeB)
	require.Contains(t, typesVisibleOnly, typeC)
}
