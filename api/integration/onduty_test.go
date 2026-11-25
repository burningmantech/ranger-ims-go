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
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/require"
)

func TestGetOnDutyRangers(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	srv := newServer(t)
	apisAdmin := srv.admin(ctx)
	apisAlice := srv.alice(ctx)

	eventName := rand.NonCryptoText()
	_, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = apisAdmin.editAccess(ctx, imsjson.EventsAccess{
		eventName: imsjson.EventAccess{
			Readers: []imsjson.AccessRule{{
				Expression: "person:" + userAliceHandle,
				Validity:   "always",
			}},
		},
	})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	shiftPod := &imsjson.OnDutyPod{ID: 9000, Type: "shift", Transport: "bicycle", Location: "3:00 & C"}

	// The admin has two open shifts in clubhousedb_test_seed.sql, and is listed
	// once, in the newer one's all_rangers position. Both are in the shift pod.
	// The admin's other pods have disbanded or been left.
	onDuty, resp := apisAlice.getOnDuty(ctx, eventName)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []imsjson.OnDutyRanger{
		{Handle: userAdminHandle, Position: "Dirt", Pod: shiftPod},
		{Handle: userAliceHandle, Position: "Nooperator", Pod: shiftPod},
	}, onDuty)

	// 90 minutes ago, only the admin's older shift had started. Their mentor
	// pod had already disbanded.
	onDuty, resp = apisAlice.getOnDutyAt(ctx, eventName, time.Now().Add(-90*time.Minute))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []imsjson.OnDutyRanger{
		{Handle: userAdminHandle, Position: "Nooperator", Pod: shiftPod},
	}, onDuty)

	// 20 minutes ago, the admin hadn't yet left the mitten pod, which is newer
	// than the shift pod they were also in.
	onDuty, resp = apisAlice.getOnDutyAt(ctx, eventName, time.Now().Add(-20*time.Minute))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []imsjson.OnDutyRanger{
		{Handle: userAdminHandle, Position: "Dirt", Pod: &imsjson.OnDutyPod{ID: 9002, Type: "mitten", Transport: "vehicle"}},
	}, onDuty)

	// 4 hours ago, Alice was midway through a shift she has since finished,
	// before she joined a pod.
	onDuty, resp = apisAlice.getOnDutyAt(ctx, eventName, time.Now().Add(-4*time.Hour))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []imsjson.OnDutyRanger{
		{Handle: userAliceHandle, Position: "Nooperator"},
	}, onDuty)

	// A day ago, nobody was on duty.
	onDuty, resp = apisAlice.getOnDutyAt(ctx, eventName, time.Now().Add(-24*time.Hour))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Empty(t, onDuty)

	// A malformed time is rejected.
	path := apisAlice.serverURL.JoinPath("/ims/api/events/", eventName, "/on_duty").String() + "?at=yesterday"
	_, resp = apisAlice.imsGet[[]imsjson.OnDutyRanger](ctx, path)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
}

func TestOnDutyPositionForRanger(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	users, err := shared.userStore.GetAllUsers(ctx)
	require.NoError(t, err)
	var adminID, aliceID int64
	for id, u := range users {
		switch u.Handle {
		case userAdminHandle:
			adminID = id
		case userAliceHandle:
			aliceID = id
		}
	}

	// The admin's newer open shift is in Dirt, an all_rangers position.
	posID, posName, err := shared.userStore.OnDutyForRanger(ctx, adminID)
	require.NoError(t, err)
	require.Equal(t, int64(7001), posID)
	require.Equal(t, "Dirt", posName)

	posID, posName, err = shared.userStore.OnDutyForRanger(ctx, aliceID)
	require.NoError(t, err)
	require.Equal(t, int64(7000), posID)
	require.Equal(t, "Nooperator", posName)
}

func (a ApiHelper) getOnDuty(ctx context.Context, eventName string) ([]imsjson.OnDutyRanger, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/on_duty").String()
	return a.imsGet[[]imsjson.OnDutyRanger](ctx, path)
}

func (a ApiHelper) getOnDutyAt(ctx context.Context, eventName string, at time.Time) ([]imsjson.OnDutyRanger, *http.Response) {
	a.t.Helper()
	path := a.serverURL.JoinPath("/ims/api/events/", eventName, "/on_duty").String() + "?at=" + url.QueryEscape(at.Format(time.RFC3339))
	return a.imsGet[[]imsjson.OnDutyRanger](ctx, path)
}
