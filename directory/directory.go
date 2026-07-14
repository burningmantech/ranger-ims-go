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
	// FetchPositions returns all position names, keyed by position ID.
	FetchPositions(ctx context.Context) (map[int64]string, error)
	// FetchTeams returns all team names, keyed by team ID.
	FetchTeams(ctx context.Context) (map[int64]string, error)
}

type UserStore struct {
	source        Source
	userCache     *cache.InMemory[map[int64]*User]
	positionCache *cache.InMemory[map[int64]string]
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
	us := &UserStore{
		source: source,
	}
	us.userCache = cache.New(
		cacheTTL,
		source.FetchUsers,
	)
	us.positionCache = cache.New(
		cacheTTL,
		source.FetchPositions,
	)
	us.teamCache = cache.New(
		cacheTTL,
		source.FetchTeams,
	)

	return us
}

// Flush invalidates all cached directory data, so that the next read
// fetches fresh data from the Source. Call this after any write to the
// backing directory.
func (store *UserStore) Flush() {
	store.userCache.Invalidate()
	store.positionCache.Invalidate()
	store.teamCache.Invalidate()
}

// Ping checks that the backing directory Source is reachable and answering.
// It deliberately bypasses the caches, since a readiness check that reports
// on stale cached data would mask an unreachable directory.
func (store *UserStore) Ping(ctx context.Context) error {
	_, err := store.source.FetchPositions(ctx)
	if err != nil {
		return fmt.Errorf("[FetchPositions] %w", err)
	}
	return nil
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

func (store *UserStore) GetPositionsAndTeams(ctx context.Context) (positions, teams map[int64]string, err error) {
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
