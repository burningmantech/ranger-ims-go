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
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"slices"
)

type UserStore struct {
	DBQ *DBQ
}

func NewUserStore(dbq *DBQ) *UserStore {
	return &UserStore{DBQ: dbq}
}

func (store *UserStore) GetRangers(ctx context.Context) ([]imsjson.Person, error) {
	results, err := store.DBQ.RangersById(ctx, store.DBQ)
	if err != nil {
		return nil, fmt.Errorf("[RangersById] %w", err)
	}
	response := make([]imsjson.Person, 0, len(results))
	for _, r := range results {
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
	var errs []error

	teamRows, err := store.DBQ.Teams(ctx, store.DBQ)
	errs = append(errs, err)
	positionRows, err := store.DBQ.Positions(ctx, store.DBQ)
	errs = append(errs, err)
	personTeams, err := store.DBQ.PersonTeams(ctx, store.DBQ)
	errs = append(errs, err)
	personPositions, err := store.DBQ.PersonPositions(ctx, store.DBQ)
	errs = append(errs, err)

	if err := errors.Join(errs...); err != nil {
		return nil, nil, fmt.Errorf("[Teams,Positions,PersonTeams,PersonPositions] %w", err)
	}

	var foundPositions []int64
	var foundPositionNames []string
	for _, pp := range personPositions {
		if pp.PersonID == userID {
			foundPositions = append(foundPositions, pp.PositionID)
		}
	}
	for _, pos := range positionRows {
		if slices.Contains(foundPositions, pos.ID) {
			foundPositionNames = append(foundPositionNames, pos.Title)
		}
	}

	var foundTeams []int64
	var foundTeamNames []string
	for _, pt := range personTeams {
		if pt.PersonID == userID {
			foundTeams = append(foundTeams, pt.TeamID)
		}
	}
	for _, team := range teamRows {
		if slices.Contains(foundTeams, team.ID) {
			foundTeamNames = append(foundTeamNames, team.Title)
		}
	}
	return foundPositionNames, foundTeamNames, nil
}

func (store *UserStore) GetUserPositionsTeamsIDs(ctx context.Context, userID int64) (positions, teams []int64, err error) {
	var errs []error

	personTeams, err := store.DBQ.PersonTeams(ctx, store.DBQ)
	errs = append(errs, err)
	personPositions, err := store.DBQ.PersonPositions(ctx, store.DBQ)
	errs = append(errs, err)

	if err := errors.Join(errs...); err != nil {
		return nil, nil, fmt.Errorf("[PersonTeams,PersonPositions] %w", err)
	}

	var foundPositions []int64
	for _, pp := range personPositions {
		if pp.PersonID == userID {
			foundPositions = append(foundPositions, pp.PositionID)
		}
	}

	var foundTeams []int64
	for _, pt := range personTeams {
		if pt.PersonID == userID {
			foundTeams = append(foundTeams, pt.TeamID)
		}
	}
	return foundPositions, foundTeams, nil
}

func (store *UserStore) GetPositionsAndTeams(ctx context.Context) (positions, teams map[int64]string, err error) {
	positionRows, err := store.DBQ.Positions(ctx, store.DBQ)
	if err != nil {
		return nil, nil, fmt.Errorf("[Positions]: %w", err)
	}
	teamRows, err := store.DBQ.Teams(ctx, store.DBQ)
	if err != nil {
		return nil, nil, fmt.Errorf("[Teams]: %w", err)
	}
	positions = make(map[int64]string, len(positionRows))
	teams = make(map[int64]string, len(teamRows))
	for _, row := range teamRows {
		teams[row.ID] = row.Title
	}
	for _, row := range positionRows {
		positions[row.ID] = row.Title
	}
	return positions, teams, nil
}
