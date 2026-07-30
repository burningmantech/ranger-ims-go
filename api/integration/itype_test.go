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

// An Incident Type with no name can't be shown to anyone, and lookups by name
// can't distinguish it from a type that doesn't exist, so the API refuses to
// make one.
func TestIncidentTypeNameMayNotBeEmpty(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apis := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	// Creating a type with an empty name is rejected.
	emptyName := ""
	typeID, resp := apis.editType(ctx, imsjson.IncidentType{Name: &emptyName})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Nil(t, typeID)
	require.NoError(t, resp.Body.Close())

	// So is one whose name is nothing but whitespace.
	blankName := "   "
	typeID, resp = apis.editType(ctx, imsjson.IncidentType{Name: &blankName})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.Nil(t, typeID)
	require.NoError(t, resp.Body.Close())

	// Renaming an existing type to an empty name is rejected too, so a type
	// can't lose its name after the fact.
	goodName := rand.NonCryptoText()
	typeID, resp = apis.editType(ctx, imsjson.IncidentType{Name: &goodName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NotNil(t, typeID)
	require.NoError(t, resp.Body.Close())

	_, resp = apis.editType(ctx, imsjson.IncidentType{ID: *typeID, Name: &emptyName})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// The type kept the name it had.
	typesResp, resp := apis.getTypes(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeID, Name: &goodName, Hidden: new(false)})

	// An edit that doesn't mention the name at all is still fine.
	_, resp = apis.editType(ctx, imsjson.IncidentType{ID: *typeID, Hidden: new(true)})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

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
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeAID, Name: &typeA, Hidden: new(false)})
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeBID, Name: &typeB, Hidden: new(false)})
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeCID, Name: &typeC, Hidden: new(false)})

	// Hide one of those types
	hideOne := imsjson.IncidentType{ID: *typeAID, Hidden: new(true)}
	_, resp = apis.editType(ctx, hideOne)
	require.NoError(t, resp.Body.Close())

	// That type should now be hidden
	typesResp, resp = apis.getTypes(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeAID, Name: &typeA, Hidden: new(true)})
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeBID, Name: &typeB, Hidden: new(false)})
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeCID, Name: &typeC, Hidden: new(false)})

	// Unhide that type we previously hid
	showItAgain := imsjson.IncidentType{ID: *typeAID, Hidden: new(false)}
	_, resp = apis.editType(ctx, showItAgain)
	require.NoError(t, resp.Body.Close())
	// and see that it's no longer hidden
	typesResp, resp = apis.getTypes(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeAID, Name: &typeA, Hidden: new(false)})
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeBID, Name: &typeB, Hidden: new(false)})
	require.Contains(t, typesResp, imsjson.IncidentType{ID: *typeCID, Name: &typeC, Hidden: new(false)})
}
