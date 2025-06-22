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
	typeAID, resp := apis.editType(ctx, imsjson.IncidentType{Name: &typeA})
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, typeAID)
	typeBID, resp := apis.editType(ctx, imsjson.IncidentType{Name: &typeB})
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, typeBID)
	typeCID, resp := apis.editType(ctx, imsjson.IncidentType{Name: &typeC})
	require.NoError(t, resp.Body.Close())
	require.NotNil(t, typeCID)

	// All three types should now be retrievable and non-hidden
	typesResp, resp := apis.getTypes(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeAID, Name: &typeA, Hidden: ptr(false)})
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeBID, Name: &typeB, Hidden: ptr(false)})
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeCID, Name: &typeC, Hidden: ptr(false)})

	// Hide one of those types
	hideOne := imsjson.IncidentType{ID: *typeAID, Hidden: ptr(true)}
	_, resp = apis.editType(ctx, hideOne)
	require.NoError(t, resp.Body.Close())

	// That type should now be hidden
	typesResp, resp = apis.getTypes(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeAID, Name: &typeA, Hidden: ptr(true)})
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeBID, Name: &typeB, Hidden: ptr(false)})
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeCID, Name: &typeC, Hidden: ptr(false)})

	// Unhide that type we previously hid
	showItAgain := imsjson.IncidentType{ID: *typeAID, Hidden: ptr(false)}
	_, resp = apis.editType(ctx, showItAgain)
	require.NoError(t, resp.Body.Close())
	// and see that it's no longer hidden
	typesResp, resp = apis.getTypes(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeAID, Name: &typeA, Hidden: ptr(false)})
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeBID, Name: &typeB, Hidden: ptr(false)})
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeCID, Name: &typeC, Hidden: ptr(false)})
}
