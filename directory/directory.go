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
	"github.com/burningmantech/ranger-ims-go/conf"
	clubhousequeries "github.com/burningmantech/ranger-ims-go/directory/clubhousedb"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"slices"
)

type UserStore struct {
	testUsers   []conf.TestUser
	clubhouseDB *DB
}

func NewUserStore(testUsers []conf.TestUser, clubhouseDB *DB) (*UserStore, error) {
	if clubhouseDB == nil && testUsers == nil {
		return nil, errors.New("NewUserStore: exactly one of clubhouseDB or testUsers must be provided (got none)")
	}
	if clubhouseDB != nil && testUsers != nil {
		return nil, errors.New("NewUserStore: exactly one of clubhouseDB or testUsers must be provided (got both)")
	}
	return &UserStore{
		testUsers:   testUsers,
		clubhouseDB: clubhouseDB,
	}, nil
}

func (users UserStore) GetRangers(ctx context.Context) ([]imsjson.Person, error) {
	if users.testUsers != nil {
		response := make([]imsjson.Person, 0, len(users.testUsers))
		for _, user := range users.testUsers {
			response = append(response, imsjson.Person{
				Handle:      user.Handle,
				Email:       user.Email,
				Password:    user.Password,
				Status:      user.Status,
				Onsite:      user.Onsite,
				DirectoryID: user.DirectoryID,
			})
		}
		return response, nil
	}

	results, err := clubhousequeries.New(users.clubhouseDB).RangersById(ctx)
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

func (users UserStore) GetUserPositionsTeams(ctx context.Context, userID int64) (positions, teams []string, err error) {
	if users.testUsers != nil {
		for _, user := range users.testUsers {
			if user.DirectoryID == userID {
				positions = append(positions, user.Positions...)
				teams = append(teams, user.Teams...)
				break
			}
		}
		return positions, teams, nil
	}

	teamRows, err := clubhousequeries.New(users.clubhouseDB).Teams(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("[Teams]: %w", err)
	}
	positionRows, err := clubhousequeries.New(users.clubhouseDB).Positions(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("[Positions]: %w", err)
	}
	personTeams, err := clubhousequeries.New(users.clubhouseDB).PersonTeams(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("[PersonTeams]: %w", err)
	}
	personPositions, err := clubhousequeries.New(users.clubhouseDB).PersonPositions(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("[PersonPositions]: %w", err)
	}

	var foundPositions []int64
	var foundPositionNames []string
	var foundTeams []int64
	var foundTeamNames []string
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
