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
	"database/sql"
	"errors"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	clubhousequeries "github.com/burningmantech/ranger-ims-go/directory/clubhousedb"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/cache"
	"hash/fnv"
	"slices"
	"time"
)

type UserStore struct {
	testUsers   []conf.TestUser
	clubhouseDB *DB

	personCache          *cache.InMemory[[]clubhousequeries.RangersByIdRow]
	teamsCache           *cache.InMemory[[]clubhousequeries.TeamsRow]
	positionsCache       *cache.InMemory[[]clubhousequeries.PositionsRow]
	personTeamsCache     *cache.InMemory[[]clubhousequeries.PersonTeamsRow]
	personPositionsCache *cache.InMemory[[]clubhousequeries.PersonPosition]
}

func NewUserStore(testUsers []conf.TestUser, clubhouseDB *DB, cacheTTL time.Duration) (*UserStore, error) {
	if clubhouseDB == nil && testUsers == nil {
		return nil, errors.New("NewUserStore: exactly one of clubhouseDB or testUsers must be provided (got none)")
	}
	if clubhouseDB != nil && testUsers != nil {
		return nil, errors.New("NewUserStore: exactly one of clubhouseDB or testUsers must be provided (got both)")
	}

	us := &UserStore{
		testUsers:   testUsers,
		clubhouseDB: clubhouseDB,
	}
	us.personCache = cache.New[[]clubhousequeries.RangersByIdRow](cacheTTL, us.getRangersByIdRow)
	us.teamsCache = cache.New[[]clubhousequeries.TeamsRow](cacheTTL, us.getTeamsRows)
	us.positionsCache = cache.New[[]clubhousequeries.PositionsRow](cacheTTL, us.getPositionsRows)
	us.personTeamsCache = cache.New[[]clubhousequeries.PersonTeamsRow](cacheTTL, us.getPersonTeamsRows)
	us.personPositionsCache = cache.New[[]clubhousequeries.PersonPosition](cacheTTL, us.getPersonPositionsRows)
	return us, nil
}

func (store *UserStore) GetRangers(ctx context.Context) ([]imsjson.Person, error) {
	results, err := store.personCache.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("[personCache.Get] %w", err)
	}

	response := make([]imsjson.Person, 0, len(*results))
	for _, r := range *results {
		response = append(response, imsjson.Person{
			Handle:      r.Callsign,
			Email:       r.Email.String,
			Password:    r.Password.String,
			Status:      string(r.Status),
			Onsite:      r.OnSite,
			DirectoryID: r.ID,
		})
	}

	return response, nil
}

func (store *UserStore) GetUserPositionsTeams(ctx context.Context, userID int64) (positions, teams []string, err error) {
	teamRows, err := store.teamsCache.Get(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("[Teams]: %w", err)
	}
	positionRows, err := store.positionsCache.Get(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("[Positions]: %w", err)
	}
	personTeams, err := store.personTeamsCache.Get(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("[PersonTeams]: %w", err)
	}
	personPositions, err := store.personPositionsCache.Get(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("[PersonPositions]: %w", err)
	}

	var foundPositions []int64
	var foundPositionNames []string
	for _, pp := range *personPositions {
		if pp.PersonID == userID {
			foundPositions = append(foundPositions, pp.PositionID)
		}
	}
	for _, pos := range *positionRows {
		if slices.Contains(foundPositions, pos.ID) {
			foundPositionNames = append(foundPositionNames, pos.Title)
		}
	}

	var foundTeams []int64
	var foundTeamNames []string
	for _, pt := range *personTeams {
		if pt.PersonID == userID {
			foundTeams = append(foundTeams, pt.TeamID)
		}
	}
	for _, team := range *teamRows {
		if slices.Contains(foundTeams, team.ID) {
			foundTeamNames = append(foundTeamNames, team.Title)
		}
	}
	return foundPositionNames, foundTeamNames, nil
}

func (store *UserStore) getRangersByIdRow(ctx context.Context) ([]clubhousequeries.RangersByIdRow, error) {
	if store.testUsers == nil {
		return clubhousequeries.New(store.clubhouseDB).RangersById(ctx)
	}

	var rows []clubhousequeries.RangersByIdRow
	for _, tu := range store.testUsers {
		rows = append(rows, clubhousequeries.RangersByIdRow{
			ID:       tu.DirectoryID,
			Callsign: tu.Handle,
			Email:    sql.NullString{String: tu.Email, Valid: true},
			Status:   clubhousequeries.PersonStatus(tu.Status),
			OnSite:   tu.Onsite,
			Password: sql.NullString{String: tu.Password, Valid: true},
		})
	}
	return rows, nil
}

func (store *UserStore) getTeamsRows(ctx context.Context) ([]clubhousequeries.TeamsRow, error) {
	if store.testUsers == nil {
		return clubhousequeries.New(store.clubhouseDB).Teams(ctx)
	}

	var rows []clubhousequeries.TeamsRow
	for _, tu := range store.testUsers {
		for _, team := range tu.Teams {
			newRow := clubhousequeries.TeamsRow{
				ID:    nonCryptoHash(team),
				Title: team,
			}
			if !slices.Contains(rows, newRow) {
				rows = append(rows, newRow)
			}
		}
	}
	return rows, nil
}

func (store *UserStore) getPositionsRows(ctx context.Context) ([]clubhousequeries.PositionsRow, error) {
	if store.testUsers == nil {
		return clubhousequeries.New(store.clubhouseDB).Positions(ctx)
	}

	var rows []clubhousequeries.PositionsRow
	for _, tu := range store.testUsers {
		for _, pos := range tu.Positions {
			newRow := clubhousequeries.PositionsRow{
				ID:    nonCryptoHash(pos),
				Title: pos,
			}
			if !slices.Contains(rows, newRow) {
				rows = append(rows, newRow)
			}
		}
	}
	return rows, nil
}

func (store *UserStore) getPersonTeamsRows(ctx context.Context) ([]clubhousequeries.PersonTeamsRow, error) {
	if store.testUsers == nil {
		return clubhousequeries.New(store.clubhouseDB).PersonTeams(ctx)
	}

	var rows []clubhousequeries.PersonTeamsRow
	for _, tu := range store.testUsers {
		for _, team := range tu.Teams {
			newRow := clubhousequeries.PersonTeamsRow{
				PersonID: tu.DirectoryID,
				TeamID:   nonCryptoHash(team),
			}
			rows = append(rows, newRow)
		}
	}
	return rows, nil
}

func (store *UserStore) getPersonPositionsRows(ctx context.Context) ([]clubhousequeries.PersonPosition, error) {
	if store.testUsers == nil {
		return clubhousequeries.New(store.clubhouseDB).PersonPositions(ctx)
	}

	var rows []clubhousequeries.PersonPosition
	for _, tu := range store.testUsers {
		for _, position := range tu.Positions {
			newRow := clubhousequeries.PersonPosition{
				PersonID:   tu.DirectoryID,
				PositionID: nonCryptoHash(position),
			}
			rows = append(rows, newRow)
		}
	}
	return rows, nil
}

func nonCryptoHash(s string) int64 {
	hasher := fnv.New64()
	_, _ = hasher.Write([]byte(s))
	// allow twos-complement wraparound, because we just want any number
	return int64(hasher.Sum64())
}
