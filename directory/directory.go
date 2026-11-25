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

package directory

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/cache"
)

// Source provides user, position, and team data from some backing directory,
// e.g. a Clubhouse database or IMS's own directory tables. Implementations
// should return fully-populated data on each call; caching is handled by
// UserStore, not by the Source.
type Source interface {
	// FetchUsers returns all directory users who may use IMS, keyed by
	// directory ID, with team and position memberships populated.
	FetchUsers(ctx context.Context) (map[int64]*User, error)
	// FetchPositions returns all positions, keyed by position ID.
	FetchPositions(ctx context.Context) (map[int64]Position, error)
	// FetchTeams returns all team names, keyed by team ID.
	FetchTeams(ctx context.Context) (map[int64]string, error)
}

// OnDutySource is implemented by the Sources that know who is currently signed
// in to a position. Only the Clubhouse has a timesheet.
type OnDutySource interface {
	// FetchOnDuty returns the shifts that were open at the given time, oldest
	// first. It's read live, since it changes minute to minute.
	FetchOnDuty(ctx context.Context, at time.Time) ([]OnDutyShift, error)
	// FetchPodsOnDuty returns the pods that were out at the given time, oldest
	// first, each with its members who were on duty then.
	FetchPodsOnDuty(ctx context.Context, at time.Time) ([]OnDutyPod, error)
}

type Position struct {
	Name string
	// AvailableToAllRangers is the Clubhouse's all_rangers flag: anyone may
	// sign in to the position, without being granted it.
	AvailableToAllRangers bool
}

type OnDutyShift struct {
	PersonID   int64
	PositionID int64
}

// Pod is a Clubhouse pod: a few Rangers sent out together on a shift.
type Pod struct {
	ID int64
	// Type is the Clubhouse's alpha, mentor, mitten, or shift.
	Type string
	// Transport is foot, bicycle, or vehicle.
	Transport string
	Location  string
}

type OnDutyPod struct {
	Pod       Pod
	PersonIDs []int64
}

type OnDutyRanger struct {
	Handle   string
	Position string
	// Pod is the pod the Ranger was out in, if any.
	Pod *Pod
}

type UserStore struct {
	source        Source
	userCache     *cache.InMemory[map[int64]*User]
	positionCache *cache.InMemory[map[int64]Position]
	teamCache     *cache.InMemory[map[int64]string]
}

type User struct {
	ID     int64
	Handle string
	Email  string
	Status string
	Onsite bool
	// #nosec G117 // Exported secret struct field
	Password           string
	PositionIDs        []int64
	PositionNames      []string
	TeamIDs            []int64
	TeamNames          []string
	OnDutyPositionID   *int64
	OnDutyPositionName *string
}

func NewUserStore(source Source, cacheTTL time.Duration) *UserStore {
	return &UserStore{
		source:        source,
		userCache:     cache.New(cacheTTL, source.FetchUsers),
		positionCache: cache.New(cacheTTL, source.FetchPositions),
		teamCache:     cache.New(cacheTTL, source.FetchTeams),
	}
}

// Flush invalidates all cached directory data, so that the next read
// fetches fresh data from the Source. Call this after any write to the
// backing directory.
func (store *UserStore) Flush() {
	store.userCache.Invalidate()
	store.positionCache.Invalidate()
	store.teamCache.Invalidate()
}

func (store *UserStore) GetAllUsers(ctx context.Context) (map[int64]*User, error) {
	users, err := store.userCache.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("[userCache.Get] %w", err)
	}
	return *users, nil
}

func (store *UserStore) GetRangers(ctx context.Context) ([]imsjson.Person, error) {
	usersPtr, err := store.userCache.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("[userCache.Get] %w", err)
	}
	users := *usersPtr

	response := make([]imsjson.Person, 0, len(users))
	for _, r := range users {
		response = append(response, imsjson.Person{
			Handle:      r.Handle,
			Email:       r.Email,
			Password:    r.Password,
			Status:      r.Status,
			Onsite:      r.Onsite,
			DirectoryID: r.ID,
		})
	}

	return response, nil
}

// OnDutyAvailable reports whether this directory knows who is on duty.
func (store *UserStore) OnDutyAvailable() bool {
	_, ok := store.source.(OnDutySource)
	return ok
}

// OnDuty returns the Rangers on duty at the given time, each once and sorted
// by handle, or nothing if the directory doesn't track that.
func (store *UserStore) OnDuty(ctx context.Context, at time.Time) ([]OnDutyRanger, error) {
	src, ok := store.source.(OnDutySource)
	if !ok {
		return nil, nil
	}
	shifts, err := src.FetchOnDuty(ctx, at)
	if err != nil {
		return nil, fmt.Errorf("[FetchOnDuty] %w", err)
	}
	users, err := store.GetAllUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("[GetAllUsers] %w", err)
	}
	positions, err := store.positionCache.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("[positionCache.Get] %w", err)
	}
	pods, err := src.FetchPodsOnDuty(ctx, at)
	if err != nil {
		return nil, fmt.Errorf("[FetchPodsOnDuty] %w", err)
	}
	// Someone with more than one open shift (a missed sign-out) is listed once,
	// in their latest position. The shifts come oldest first. Persons missing
	// from the user cache have statuses IMS doesn't deal with.
	byHandle := make(map[string]string, len(shifts))
	for _, shift := range shifts {
		user, ok := users[shift.PersonID]
		if !ok {
			continue
		}
		// This returns an empty position name if there is no name for the ID. That's fine.
		byHandle[user.Handle] = (*positions)[shift.PositionID].Name
	}
	// Someone in more than one pod at once is shown in the newest. The pods
	// come oldest first.
	podByHandle := make(map[string]*Pod)
	for i := range pods {
		for _, personID := range pods[i].PersonIDs {
			if user, ok := users[personID]; ok {
				podByHandle[user.Handle] = &pods[i].Pod
			}
		}
	}
	onDuty := make([]OnDutyRanger, 0, len(byHandle))
	for handle, position := range byHandle {
		onDuty = append(onDuty, OnDutyRanger{Handle: handle, Position: position, Pod: podByHandle[handle]})
	}
	slices.SortFunc(onDuty, func(a, b OnDutyRanger) int {
		return strings.Compare(a.Handle, b.Handle)
	})
	return onDuty, nil
}

func (store *UserStore) GetPositionsAndTeams(ctx context.Context) (positions map[int64]Position, teams map[int64]string, err error) {
	var errs []error
	posMap, err := store.positionCache.Get(ctx)
	errs = append(errs, err)
	teamMap, err := store.teamCache.Get(ctx)
	errs = append(errs, err)
	err = errors.Join(errs...)
	if err != nil {
		return nil, nil, fmt.Errorf("[GetPositionsAndTeams] %w", err)
	}
	return *posMap, *teamMap, nil
}

func (store *UserStore) PositionsForRanger(ctx context.Context, personID int64) ([]string, error) {
	users, err := store.GetAllUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("[GetAllUsers] %w", err)
	}
	user, ok := users[personID]
	if !ok {
		return nil, nil
	}
	return user.PositionNames, nil
}

func (store *UserStore) TeamsForRanger(ctx context.Context, personID int64) ([]string, error) {
	users, err := store.GetAllUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("[GetAllUsers] %w", err)
	}
	user, ok := users[personID]
	if !ok {
		return nil, nil
	}
	return user.TeamNames, nil
}

func (store *UserStore) OnSiteForRanger(ctx context.Context, personID int64) (bool, error) {
	users, err := store.GetAllUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("[GetAllUsers] %w", err)
	}
	user, ok := users[personID]
	if !ok {
		return false, nil
	}
	return user.Onsite, nil
}

func (store *UserStore) OnDutyForRanger(ctx context.Context, personID int64) (id int64, name string, err error) {
	users, err := store.GetAllUsers(ctx)
	if err != nil {
		return 0, "", fmt.Errorf("[GetAllUsers] %w", err)
	}
	user, ok := users[personID]
	if !ok {
		return 0, "", nil
	}
	posID := int64(0)
	posName := ""
	if user.OnDutyPositionID != nil {
		posID = *user.OnDutyPositionID
	}
	if user.OnDutyPositionName != nil {
		posName = *user.OnDutyPositionName
	}
	return posID, posName, nil
}
