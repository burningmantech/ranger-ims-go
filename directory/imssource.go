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

	"github.com/burningmantech/ranger-ims-go/store"
)

// IMSUserStatus is the status reported for every user from the IMS-native
// directory. The native directory has no Clubhouse-style status taxonomy,
// just an active flag, and inactive users are never returned at all.
const IMSUserStatus = "active"

// IMSSource is a directory Source backed by IMS's own DIRECTORY_* tables,
// which live in the IMS database. It is used when a deployment has no
// Clubhouse database, i.e. when IMS_DIRECTORY is "ims".
//
// Users from this source never have an on-duty position, so "onduty:"
// access expressions never match them.
type IMSSource struct {
	imsDBQ *store.DBQ
}

var _ Source = (*IMSSource)(nil)

func NewIMSSource(imsDBQ *store.DBQ) *IMSSource {
	return &IMSSource{imsDBQ: imsDBQ}
}

func (s *IMSSource) FetchUsers(ctx context.Context) (map[int64]*User, error) {
	var errs []error
	persons, err := s.imsDBQ.DirectoryActivePersons(ctx, s.imsDBQ)
	errs = append(errs, err)
	positionRows, err := s.imsDBQ.DirectoryActivePositions(ctx, s.imsDBQ)
	errs = append(errs, err)
	teamRows, err := s.imsDBQ.DirectoryActiveTeams(ctx, s.imsDBQ)
	errs = append(errs, err)
	personPositions, err := s.imsDBQ.DirectoryPersonPositions(ctx, s.imsDBQ)
	errs = append(errs, err)
	personTeams, err := s.imsDBQ.DirectoryPersonTeams(ctx, s.imsDBQ)
	errs = append(errs, err)
	err = errors.Join(errs...)
	if err != nil {
		return nil, fmt.Errorf("[DirectoryPersons,Positions,Teams,Memberships] %w", err)
	}

	m := make(map[int64]*User, len(persons))
	for _, person := range persons {
		m[person.ID] = &User{
			ID:       person.ID,
			Handle:   person.Handle,
			Email:    person.Email.String,
			Status:   IMSUserStatus,
			Onsite:   person.Onsite,
			Password: person.Password,
		}
	}
	positions := make(map[int64]string, len(positionRows))
	for _, row := range positionRows {
		positions[row.ID] = row.Title
	}
	teams := make(map[int64]string, len(teamRows))
	for _, row := range teamRows {
		teams[row.ID] = row.Title
	}
	for _, pp := range personPositions {
		person, personOK := m[pp.PersonID]
		positionName, positionOK := positions[pp.PositionID]
		if personOK && positionOK {
			person.PositionIDs = append(person.PositionIDs, pp.PositionID)
			person.PositionNames = append(person.PositionNames, positionName)
		}
	}
	for _, pt := range personTeams {
		person, personOK := m[pt.PersonID]
		teamName, teamOK := teams[pt.TeamID]
		if personOK && teamOK {
			person.TeamIDs = append(person.TeamIDs, pt.TeamID)
			person.TeamNames = append(person.TeamNames, teamName)
		}
	}
	return m, nil
}

func (s *IMSSource) FetchPositions(ctx context.Context) (map[int64]string, error) {
	positionRows, err := s.imsDBQ.DirectoryActivePositions(ctx, s.imsDBQ)
	if err != nil {
		return nil, fmt.Errorf("[DirectoryActivePositions]: %w", err)
	}
	positions := make(map[int64]string, len(positionRows))
	for _, row := range positionRows {
		positions[row.ID] = row.Title
	}
	return positions, nil
}

func (s *IMSSource) FetchTeams(ctx context.Context) (map[int64]string, error) {
	teamRows, err := s.imsDBQ.DirectoryActiveTeams(ctx, s.imsDBQ)
	if err != nil {
		return nil, fmt.Errorf("[DirectoryActiveTeams]: %w", err)
	}
	teams := make(map[int64]string, len(teamRows))
	for _, row := range teamRows {
		teams[row.ID] = row.Title
	}
	return teams, nil
}
