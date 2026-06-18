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
	"io"
	"net/http"
	"testing"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/stretchr/testify/require"
)

func TestGetAndEditEvent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	testEventName := rand.NonCryptoText()

	editEventReq := imsjson.Event{
		Name: &testEventName,
	}

	eventID, resp := apisAdmin.createEvent(ctx, editEventReq)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	editEventReq.ID = eventID
	editEventReq.MapURL = new("https://example.com/mymap")
	resp = apisAdmin.editEvent(ctx, editEventReq)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	accessReq := imsjson.EventsAccess{
		testEventName: {
			Writers: []imsjson.AccessRule{
				{
					Expression: "person:" + userAdminHandle,
					Validity:   "always",
				},
			},
		},
	}
	resp = apisAdmin.editAccess(ctx, accessReq)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	expectedAccessResult := imsjson.EventAccess{
		Writers:      accessReq[testEventName].Writers,
		Readers:      []imsjson.AccessRule{},
		Reporters:    []imsjson.AccessRule{},
		VisitWriters: []imsjson.AccessRule{},
	}
	expectedAccessResult.Writers[0].DebugInfo.MatchesUsers = []string{userAdminHandle}
	expectedAccessResult.Writers[0].DebugInfo.KnownTarget = true
	accessResult, httpResp := apisAdmin.getAccess(ctx)
	require.Equal(t, http.StatusOK, httpResp.StatusCode)
	require.Equal(t, expectedAccessResult, accessResult[testEventName])
	require.NoError(t, httpResp.Body.Close())

	events, resp := apisAdmin.getEvents(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	// The list may include events from other tests
	var foundEvent *imsjson.Event
	for _, event := range events {
		if event.ID == eventID {
			foundEvent = &event
		}
	}
	require.NotNil(t, foundEvent)
	require.NotNil(t, foundEvent.Name)
	require.Equal(t, testEventName, *foundEvent.Name)
	require.NotZero(t, foundEvent.ID)
}

func TestEditEvent_errors(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	testEventName := "This name is ugly (has spaces and parentheses)"

	editEventReq := imsjson.Event{
		Name: &testEventName,
	}

	// use editEvent rather than createEvent, because createEvent fails if it can't actually create the event
	resp := apisAdmin.editEvent(ctx, editEventReq)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Contains(t, string(b), "names must match the pattern")
}

// editEventBody POSTs an event edit and returns the status code and response body.
func editEventBody(ctx context.Context, t *testing.T, a ApiHelper, req imsjson.Event) (int, string) {
	t.Helper()
	resp := a.editEvent(ctx, req)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode, string(b)
}

func TestEventGroups(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	// Create an event group.
	groupName := rand.NonCryptoText()
	groupID, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &groupName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	status, body := editEventBody(ctx, t, apisAdmin, imsjson.Event{
		ID:      groupID,
		IsGroup: new(true),
	})
	require.Equal(t, http.StatusNoContent, status, body)

	// Create a regular child event.
	childName := rand.NonCryptoText()
	childID, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &childName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// Assign the child to the group.
	status, body = editEventBody(ctx, t, apisAdmin, imsjson.Event{
		ID:          childID,
		ParentGroup: new(groupID),
	})
	require.Equal(t, http.StatusNoContent, status, body)

	// By default, groups are excluded from the events listing.
	events, resp := apisAdmin.getEvents(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Nil(t, findEvent(events, groupID), "group should be excluded by default")
	child := findEvent(events, childID)
	require.NotNil(t, child)
	require.NotNil(t, child.ParentGroup)
	require.Equal(t, groupID, *child.ParentGroup)

	// With include_groups=true, the group appears.
	eventsWithGroups, resp := apisAdmin.getEventsIncludingGroups(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	group := findEvent(eventsWithGroups, groupID)
	require.NotNil(t, group, "group should be present when include_groups=true")
	require.NotNil(t, group.IsGroup)
	require.True(t, *group.IsGroup)

	// Clearing the child's parent group (ParentGroup <= 0) removes the association.
	status, body = editEventBody(ctx, t, apisAdmin, imsjson.Event{
		ID:          childID,
		ParentGroup: new(int32(0)),
	})
	require.Equal(t, http.StatusNoContent, status, body)
	events, resp = apisAdmin.getEvents(ctx)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	child = findEvent(events, childID)
	require.NotNil(t, child)
	require.Nil(t, child.ParentGroup, "parent group should be cleared")
}

func TestEventGroups_errors(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	apisAdmin := ApiHelper{t: t, serverURL: shared.serverURL, jwt: jwtForAdmin(ctx, t)}

	// A group to reference as a parent.
	groupName := rand.NonCryptoText()
	groupID, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &groupName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	status, body := editEventBody(ctx, t, apisAdmin, imsjson.Event{ID: groupID, IsGroup: new(true)})
	require.Equal(t, http.StatusNoContent, status, body)

	// A plain event to mutate.
	eventName := rand.NonCryptoText()
	eventID, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &eventName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	// An event cannot be its own parent group.
	status, body = editEventBody(ctx, t, apisAdmin, imsjson.Event{
		ID:          eventID,
		ParentGroup: new(eventID),
	})
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, body, "cannot be the same as the event itself")

	// A parent group must actually be a group, not a plain event.
	otherName := rand.NonCryptoText()
	otherID, resp := apisAdmin.createEvent(ctx, imsjson.Event{Name: &otherName})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	status, body = editEventBody(ctx, t, apisAdmin, imsjson.Event{
		ID:          eventID,
		ParentGroup: new(otherID),
	})
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, body, "must be an event group")

	// An event group cannot itself have a parent group. First give the event a
	// parent group, then try to promote it to a group.
	status, body = editEventBody(ctx, t, apisAdmin, imsjson.Event{
		ID:          eventID,
		ParentGroup: new(groupID),
	})
	require.Equal(t, http.StatusNoContent, status, body)
	status, body = editEventBody(ctx, t, apisAdmin, imsjson.Event{
		ID:      eventID,
		IsGroup: new(true),
	})
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, body, "cannot have a parent event group")

	// An event group cannot have a map URL.
	status, body = editEventBody(ctx, t, apisAdmin, imsjson.Event{
		ID:      groupID,
		MapURL:  new("https://example.com/mymap"),
		IsGroup: new(true),
	})
	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, body, "cannot have a map URL")
}

// findEvent returns a pointer to the event with the given ID, or nil if not present.
func findEvent(events imsjson.Events, id int32) *imsjson.Event {
	for i := range events {
		if events[i].ID == id {
			return &events[i]
		}
	}
	return nil
}
