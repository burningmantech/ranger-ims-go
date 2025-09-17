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
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	chqueries "github.com/burningmantech/ranger-ims-go/directory/clubhousedb"
	"github.com/go-sql-driver/mysql"
	"log/slog"
	"time"
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

func NewDBQ(db *sql.DB, querier chqueries.Querier, cacheTTL time.Duration) *DBQ {
	dbq := &DBQ{
		DB: db,
		q:  querier,
	}
	return dbq
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
