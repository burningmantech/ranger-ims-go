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
	"github.com/burningmantech/ranger-ims-go/directory/clubhousedb"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/cache"
	"time"
)

type UserStore struct {
	DBQ           *DBQ
	userCache     *cache.InMemory[map[int64]*User]
	positionCache *cache.InMemory[map[int64]string]
	teamCache     *cache.InMemory[map[int64]string]
}

type User struct {
	Person        clubhousedb.RangersByIdRow
	PositionIDs   []int64
	PositionNames []string
	TeamIDs       []int64
	TeamNames     []string
}

func NewUserStore(dbq *DBQ, cacheTTL time.Duration) *UserStore {
	us := &UserStore{
		DBQ: dbq,
	}
	us.userCache = cache.New(
		cacheTTL,
		us.refreshUserCache,
	)
	us.positionCache = cache.New(
		cacheTTL,
		us.refreshPositionCache,
	)
	us.teamCache = cache.New(
		cacheTTL,
		us.refreshTeamCache,
	)

	return us
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
			Handle:      r.Person.Callsign,
			Email:       r.Person.Email.String,
			Password:    r.Person.Password.String,
			Status:      string(r.Person.Status),
			Onsite:      r.Person.OnSite,
			DirectoryID: r.Person.ID,
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
	if errors.Join(errs...) != nil {
		return nil, nil, fmt.Errorf("[GetPositionsAndTeams] %w", err)
	}
	return *posMap, *teamMap, nil
}

func (store *UserStore) refreshUserCache(ctx context.Context) (map[int64]*User, error) {
	var errs []error
	rangers, err := store.DBQ.RangersById(ctx, store.DBQ)
	errs = append(errs, err)
	teamRows, err := store.DBQ.Teams(ctx, store.DBQ)
	errs = append(errs, err)
	positionRows, err := store.DBQ.Positions(ctx, store.DBQ)
	errs = append(errs, err)
	personTeams, err := store.DBQ.PersonTeams(ctx, store.DBQ)
	errs = append(errs, err)
	personPositions, err := store.DBQ.PersonPositions(ctx, store.DBQ)
	errs = append(errs, err)
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("[Teams,Positions,PersonTeams,PersonPositions] %w", err)
	}

	m := make(map[int64]*User, len(rangers))
	for _, ranger := range rangers {
		m[ranger.ID] = &User{Person: ranger}
	}
	positions := make(map[int64]string, len(positionRows))
	for _, positionRow := range positionRows {
		positions[positionRow.ID] = positionRow.Title
	}
	teams := make(map[int64]string, len(teamRows))
	for _, teamRow := range teamRows {
		teams[teamRow.ID] = teamRow.Title
	}
	for _, pp := range personPositions {
		if _, ok := m[pp.PersonID]; ok {
			person := m[pp.PersonID]
			person.PositionIDs = append(person.PositionIDs, pp.PositionID)
			person.PositionNames = append(person.PositionNames, positions[pp.PositionID])
		}
	}
	for _, pt := range personTeams {
		if _, ok := m[pt.PersonID]; ok {
			person := m[pt.PersonID]
			person.TeamIDs = append(person.TeamIDs, pt.TeamID)
			person.TeamNames = append(person.TeamNames, teams[pt.TeamID])
		}
	}
	return m, nil
}

func (store *UserStore) refreshPositionCache(ctx context.Context) (map[int64]string, error) {
	positionRows, err := store.DBQ.Positions(ctx, store.DBQ)
	if err != nil {
		return nil, fmt.Errorf("[Positions]: %w", err)
	}
	positions := make(map[int64]string, len(positionRows))
	for _, row := range positionRows {
		positions[row.ID] = row.Title
	}
	return positions, nil
}

func (store *UserStore) refreshTeamCache(ctx context.Context) (map[int64]string, error) {
	teamRows, err := store.DBQ.Teams(ctx, store.DBQ)
	if err != nil {
		return nil, fmt.Errorf("[Teams]: %w", err)
	}
	teams := make(map[int64]string, len(teamRows))
	for _, row := range teamRows {
		teams[row.ID] = row.Title
	}
	return teams, nil
}
