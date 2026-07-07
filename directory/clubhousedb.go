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
	_ "embed"
	"errors"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	chqueries "github.com/burningmantech/ranger-ims-go/directory/clubhousedb"
	"github.com/go-sql-driver/mysql"
	"log/slog"
)

//go:embed schema/current.sql
var CurrentSchema string

func MariaDB(ctx context.Context, directoryCfg conf.Directory) (*sql.DB, error) {
	var chDBCfg conf.ClubhouseDB
	var err error
	switch directoryCfg.Directory {
	case conf.DirectoryTypeNoOp:
		return sql.Open("noop", "")
	case conf.DirectoryTypeClubhouseDB:
		fallthrough
	default:
		chDBCfg = directoryCfg.ClubhouseDB
	}
	slog.Info("Setting up Clubhouse DB connection")

	// Capture connection properties.
	cfg := mysql.NewConfig()
	cfg.User = chDBCfg.Username
	cfg.Passwd = chDBCfg.Password
	cfg.Net = "tcp"
	cfg.Addr = chDBCfg.Hostname
	cfg.DBName = chDBCfg.Database
	cfg.MultiStatements = true

	// Get a database handle.
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("[sql.Open]: %w", err)
	}
	// Some arbitrary value. We'll get errors from MariaDB if the server
	// hits the DB with too many parallel requests.
	db.SetMaxOpenConns(int(chDBCfg.MaxOpenConns))
	pingErr := db.PingContext(ctx)
	if pingErr != nil {
		return nil, fmt.Errorf("[PingContext]: %w", pingErr)
	}
	slog.Info("Connected to Clubhouse MariaDB")

	return db, nil
}

// DBQ combines the SQL database and the Querier for the Clubhouse datastore.
type DBQ struct {
	*sql.DB

	q chqueries.Querier
}

var _ chqueries.Querier = (*DBQ)(nil)

func NewDBQ(db *sql.DB, querier chqueries.Querier) *DBQ {
	dbq := &DBQ{
		DB: db,
		q:  querier,
	}
	return dbq
}

// ClubhouseSource is a directory Source backed by a Ranger Clubhouse database.
//
// This is the directory used by the Black Rock Rangers. Users, positions, and
// teams come from the Clubhouse tables, and a user's on-duty position is
// derived from the Clubhouse timesheet.
type ClubhouseSource struct {
	dbq *DBQ
}

var _ Source = (*ClubhouseSource)(nil)

func NewClubhouseSource(dbq *DBQ) *ClubhouseSource {
	return &ClubhouseSource{dbq: dbq}
}

func (s *ClubhouseSource) FetchUsers(ctx context.Context) (map[int64]*User, error) {
	var errs []error
	persons, err := s.dbq.Persons(ctx, s.dbq)
	errs = append(errs, err)
	teamRows, err := s.dbq.Teams(ctx, s.dbq)
	errs = append(errs, err)
	positionRows, err := s.dbq.Positions(ctx, s.dbq)
	errs = append(errs, err)
	personTeams, err := s.dbq.PersonTeams(ctx, s.dbq)
	errs = append(errs, err)
	personPositions, err := s.dbq.PersonPositions(ctx, s.dbq)
	errs = append(errs, err)
	personsOnDuty, err := s.dbq.PersonsOnDuty(ctx, s.dbq)
	errs = append(errs, err)
	err = errors.Join(errs...)
	if err != nil {
		return nil, fmt.Errorf("[Teams,Positions,PersonTeams,PersonPositions] %w", err)
	}

	m := make(map[int64]*User, len(persons))
	for _, person := range persons {
		m[person.ID] = &User{
			ID:       person.ID,
			Handle:   person.Callsign,
			Email:    person.Email.String,
			Status:   string(person.Status),
			Onsite:   person.OnSite,
			Password: person.Password.String,
		}
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
	for _, pod := range personsOnDuty {
		if _, ok := m[int64(pod.PersonID)]; ok {
			posID := int64(pod.PositionID)
			m[int64(pod.PersonID)].OnDutyPositionID = &posID
			if pos, ok := positions[int64(pod.PositionID)]; ok {
				m[int64(pod.PersonID)].OnDutyPositionName = &pos
			}
		}
	}
	return m, nil
}

func (s *ClubhouseSource) FetchPositions(ctx context.Context) (map[int64]string, error) {
	positionRows, err := s.dbq.Positions(ctx, s.dbq)
	if err != nil {
		return nil, fmt.Errorf("[Positions]: %w", err)
	}
	positions := make(map[int64]string, len(positionRows))
	for _, row := range positionRows {
		positions[row.ID] = row.Title
	}
	return positions, nil
}

func (s *ClubhouseSource) FetchTeams(ctx context.Context) (map[int64]string, error) {
	teamRows, err := s.dbq.Teams(ctx, s.dbq)
	if err != nil {
		return nil, fmt.Errorf("[Teams]: %w", err)
	}
	teams := make(map[int64]string, len(teamRows))
	for _, row := range teamRows {
		teams[row.ID] = row.Title
	}
	return teams, nil
}

func (l DBQ) PersonPositions(ctx context.Context, db chqueries.DBTX) ([]chqueries.PersonPosition, error) {
	return l.q.PersonPositions(ctx, db)
}

func (l DBQ) PersonTeams(ctx context.Context, db chqueries.DBTX) ([]chqueries.PersonTeamsRow, error) {
	return l.q.PersonTeams(ctx, db)
}

func (l DBQ) Positions(ctx context.Context, db chqueries.DBTX) ([]chqueries.PositionsRow, error) {
	return l.q.Positions(ctx, db)
}

func (l DBQ) Persons(ctx context.Context, db chqueries.DBTX) ([]chqueries.PersonsRow, error) {
	return l.q.Persons(ctx, db)
}

func (l DBQ) PersonsOnDuty(ctx context.Context, db chqueries.DBTX) ([]chqueries.PersonsOnDutyRow, error) {
	return l.q.PersonsOnDuty(ctx, db)
}

func (l DBQ) Teams(ctx context.Context, db chqueries.DBTX) ([]chqueries.TeamsRow, error) {
	return l.q.Teams(ctx, db)
}
