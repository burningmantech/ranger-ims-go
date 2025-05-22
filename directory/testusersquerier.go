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
	"github.com/burningmantech/ranger-ims-go/conf"
	clubhousequeries "github.com/burningmantech/ranger-ims-go/directory/clubhousedb"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"slices"
)

type TestUsersStore struct {
	TestUsers []conf.TestUser
}

var _ clubhousequeries.Querier = (*TestUsersStore)(nil)

func (t TestUsersStore) PersonPositions(ctx context.Context, db clubhousequeries.DBTX) ([]clubhousequeries.PersonPosition, error) {
	var rows []clubhousequeries.PersonPosition
	for _, tu := range t.TestUsers {
		for _, position := range tu.Positions {
			newRow := clubhousequeries.PersonPosition{
				PersonID:   tu.DirectoryID,
				PositionID: rand.NonCryptoHash64(position),
			}
			rows = append(rows, newRow)
		}
	}
	return rows, nil
}

func (t TestUsersStore) PersonTeams(ctx context.Context, db clubhousequeries.DBTX) ([]clubhousequeries.PersonTeamsRow, error) {
	var rows []clubhousequeries.PersonTeamsRow
	for _, tu := range t.TestUsers {
		for _, team := range tu.Teams {
			newRow := clubhousequeries.PersonTeamsRow{
				PersonID: tu.DirectoryID,
				TeamID:   rand.NonCryptoHash64(team),
			}
			rows = append(rows, newRow)
		}
	}
	return rows, nil
}

func (t TestUsersStore) Positions(ctx context.Context, db clubhousequeries.DBTX) ([]clubhousequeries.PositionsRow, error) {
	var rows []clubhousequeries.PositionsRow
	for _, tu := range t.TestUsers {
		for _, pos := range tu.Positions {
			newRow := clubhousequeries.PositionsRow{
				ID:    rand.NonCryptoHash64(pos),
				Title: pos,
			}
			if !slices.Contains(rows, newRow) {
				rows = append(rows, newRow)
			}
		}
	}
	return rows, nil
}

func (t TestUsersStore) RangersById(ctx context.Context, db clubhousequeries.DBTX) ([]clubhousequeries.RangersByIdRow, error) {
	var rows []clubhousequeries.RangersByIdRow
	for _, person := range t.TestUsers {
		rows = append(rows, clubhousequeries.RangersByIdRow{
			ID:       person.DirectoryID,
			Callsign: person.Handle,
			Email:    conv.StringToSql(&person.Email),
			Status:   clubhousequeries.PersonStatus(person.Status),
			OnSite:   person.Onsite,
			Password: conv.StringToSql(&person.Password),
		})
	}
	return rows, nil
}

func (t TestUsersStore) Teams(ctx context.Context, db clubhousequeries.DBTX) ([]clubhousequeries.TeamsRow, error) {
	var rows []clubhousequeries.TeamsRow
	for _, tu := range t.TestUsers {
		for _, team := range tu.Teams {
			newRow := clubhousequeries.TeamsRow{
				ID:    rand.NonCryptoHash64(team),
				Title: team,
			}
			if !slices.Contains(rows, newRow) {
				rows = append(rows, newRow)
			}
		}
	}
	return rows, nil
}
