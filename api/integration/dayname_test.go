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
	"net/http"
	"strings"
	"testing"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventDayNames(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	admin := srv.admin(ctx)

	eventName := rand.NonCryptoText()
	_, resp := admin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// A new event has no day names.
	dayNames, resp := admin.getDayNames(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Empty(t, dayNames)
	assert.Equal(t, "max-age=1200, private", resp.Header.Get("Cache-Control"))

	// Names come back sorted by date and trimmed.
	resp = admin.editDayNames(ctx, eventName, []imsjson.EventDayName{
		{Date: "2026-09-05", Name: "  Man Burn "},
		{Date: "2026-08-30", Name: "Gate Opens"},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	dayNames, resp = admin.getDayNames(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, []imsjson.EventDayName{
		{Date: "2026-08-30", Name: "Gate Opens"},
		{Date: "2026-09-05", Name: "Man Burn"},
	}, dayNames)

	// A save replaces the whole list.
	resp = admin.editDayNames(ctx, eventName, []imsjson.EventDayName{
		{Date: "2026-09-06", Name: "Temple Burn"},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	dayNames, resp = admin.getDayNames(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, []imsjson.EventDayName{{Date: "2026-09-06", Name: "Temple Burn"}}, dayNames)

	// So does an empty list.
	resp = admin.editDayNames(ctx, eventName, []imsjson.EventDayName{})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	dayNames, resp = admin.getDayNames(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Empty(t, dayNames)
}

func TestEventDayNamesValidation(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	admin := srv.admin(ctx)

	eventName := rand.NonCryptoText()
	_, resp := admin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = admin.editDayNames(ctx, eventName, []imsjson.EventDayName{{Date: "2026-09-05", Name: "Man Burn"}})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Not a date
	resp = admin.editDayNames(ctx, eventName, []imsjson.EventDayName{{Date: "Saturday", Name: "Man Burn"}})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Not zero-padded
	resp = admin.editDayNames(ctx, eventName, []imsjson.EventDayName{{Date: "2026-9-5", Name: "Man Burn"}})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Blank name
	resp = admin.editDayNames(ctx, eventName, []imsjson.EventDayName{{Date: "2026-09-05", Name: "   "}})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Name too long
	resp = admin.editDayNames(ctx, eventName, []imsjson.EventDayName{{Date: "2026-09-05", Name: strings.Repeat("x", 129)}})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// A line break inside the name
	resp = admin.editDayNames(ctx, eventName, []imsjson.EventDayName{{Date: "2026-09-05", Name: "Man Burn\n2026-09-06,Temple Burn"}})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// The same date twice
	resp = admin.editDayNames(ctx, eventName, []imsjson.EventDayName{
		{Date: "2026-09-05", Name: "Man Burn"},
		{Date: "2026-09-05", Name: "Burn Night"},
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// None of the rejected saves touched what was there.
	dayNames, resp := admin.getDayNames(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, []imsjson.EventDayName{{Date: "2026-09-05", Name: "Man Burn"}}, dayNames)
}

func TestEventDayNamesPermissions(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	admin := srv.admin(ctx)
	alice := srv.alice(ctx)

	eventName := rand.NonCryptoText()
	_, resp := admin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = admin.editDayNames(ctx, eventName, []imsjson.EventDayName{{Date: "2026-09-05", Name: "Man Burn"}})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Alice has no role on the event yet, so she can't read its day names.
	_, resp = alice.getDayNames(ctx, eventName)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Even a reporter, the least privileged role, can read them.
	resp = admin.addReporter(ctx, eventName, userAliceHandle)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	dayNames, resp := alice.getDayNames(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, []imsjson.EventDayName{{Date: "2026-09-05", Name: "Man Burn"}}, dayNames)

	// But only an admin can change them.
	resp = alice.editDayNames(ctx, eventName, []imsjson.EventDayName{})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}
