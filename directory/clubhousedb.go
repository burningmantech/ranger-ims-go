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
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	clubhousequeries "github.com/burningmantech/ranger-ims-go/directory/clubhousedb"
	"github.com/burningmantech/ranger-ims-go/lib/cache"
	"github.com/go-sql-driver/mysql"
	"log/slog"
	"strings"
	"time"
)

func MariaDB(ctx context.Context, imsCfg conf.ClubhouseDB) (*sql.DB, error) {
	slog.Info("Setting up Clubhouse DB connection")

	// Capture connection properties.
	cfg := mysql.NewConfig()
	cfg.User = imsCfg.Username
	cfg.Passwd = imsCfg.Password
	cfg.Net = "tcp"
	cfg.Addr = imsCfg.Hostname
	cfg.DBName = imsCfg.Database

	// Get a database handle.
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("[sql.Open]: %w", err)
	}
	// Some arbitrary value. We'll get errors from MariaDB if the server
	// hits the DB with too many parallel requests.
	db.SetMaxOpenConns(20)
	pingErr := db.PingContext(ctx)
	if pingErr != nil {
		return nil, fmt.Errorf("[PingContext]: %w", pingErr)
	}
	slog.Info("Connected to Clubhouse MariaDB")
	return db, nil
}

// DBQ combines the SQL database and the Querier for the Clubhouse datastore. It's convenient having those
// two types embedded in one struct, because it allows great flexibility in custom method overrides.
type DBQ struct {
	*sql.DB
	clubhousequeries.Querier

	rangersByIdCache     *cache.InMemory[[]clubhousequeries.RangersByIdRow]
	personPositionsCache *cache.InMemory[[]clubhousequeries.PersonPosition]
	personTeamsCache     *cache.InMemory[[]clubhousequeries.PersonTeamsRow]
	positionsCache       *cache.InMemory[[]clubhousequeries.PositionsRow]
	teamsCache           *cache.InMemory[[]clubhousequeries.TeamsRow]
}

// NewRealMariaDBQ creates a DBQ that uses a standard MariaDB Clubhouse database.
func NewRealMariaDBQ(db *sql.DB, cacheTTL time.Duration) *DBQ {
	return newDBQ(db, clubhousequeries.New(), cacheTTL)
}

// NewFakeTestUsersDBQ creates a DBQ that isn't actually connected to any database,
// but rather just returns values from a TestUsersStore. This actually sets the
// db to "nil", which is intentional, because it means we'll see a panic if
// something erroneously tries to talk to a DB rather than the TestUsersStore.
func NewFakeTestUsersDBQ(testUsers []conf.TestUser, cacheTTL time.Duration) *DBQ {
	return newDBQ(nil, TestUsersStore{TestUsers: testUsers}, cacheTTL)
}

func newDBQ(db *sql.DB, querier clubhousequeries.Querier, cacheTTL time.Duration) *DBQ {
	dbq := &DBQ{
		DB:      db,
		Querier: querier,
	}
	dbq.rangersByIdCache = cache.New[[]clubhousequeries.RangersByIdRow](
		cacheTTL,
		func(ctx context.Context) ([]clubhousequeries.RangersByIdRow, error) {
			return dbq.Querier.RangersById(ctx, dbq)
		},
	)
	dbq.personPositionsCache = cache.New[[]clubhousequeries.PersonPosition](
		cacheTTL,
		func(ctx context.Context) ([]clubhousequeries.PersonPosition, error) {
			return dbq.Querier.PersonPositions(ctx, dbq)
		},
	)
	dbq.personTeamsCache = cache.New[[]clubhousequeries.PersonTeamsRow](
		cacheTTL,
		func(ctx context.Context) ([]clubhousequeries.PersonTeamsRow, error) {
			return dbq.Querier.PersonTeams(ctx, dbq)
		},
	)
	dbq.positionsCache = cache.New[[]clubhousequeries.PositionsRow](
		cacheTTL,
		func(ctx context.Context) ([]clubhousequeries.PositionsRow, error) {
			return dbq.Querier.Positions(ctx, dbq)
		},
	)
	dbq.teamsCache = cache.New[[]clubhousequeries.TeamsRow](
		cacheTTL,
		func(ctx context.Context) ([]clubhousequeries.TeamsRow, error) {
			return dbq.Querier.Teams(ctx, dbq)
		},
	)
	return dbq
}

func (l DBQ) ExecContext(ctx context.Context, s string, i ...interface{}) (sql.Result, error) {
	start := time.Now()
	execContext, err := l.DB.ExecContext(ctx, s, i...)
	logQuery(s, start, err)
	return execContext, err
}

func (l DBQ) PrepareContext(ctx context.Context, s string) (*sql.Stmt, error) {
	start := time.Now()
	stmt, err := l.DB.PrepareContext(ctx, s)
	logQuery(s, start, err)
	return stmt, err
}

func (l DBQ) QueryContext(ctx context.Context, s string, i ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	rows, err := l.DB.QueryContext(ctx, s, i...)
	logQuery(s, start, err)
	return rows, err
}

func (l DBQ) QueryRowContext(ctx context.Context, s string, i ...interface{}) *sql.Row {
	start := time.Now()
	row := l.DB.QueryRowContext(ctx, s, i...)
	logQuery(s, start, nil)
	return row
}

func logQuery(s string, start time.Time, err error) {
	queryName, _, _ := strings.Cut(s, "\n")
	queryName = strings.TrimPrefix(queryName, "-- name: ")
	queryName = strings.Fields(queryName)[0]
	durationMS := float64(time.Since(start).Microseconds()) / 1000.0

	// Note that the duration(ish) is very misleading. It'll always be less than
	// the actual query time, often significantly. That's because most of the IO
	// takes place after we're able to log in this file, e.g. in the "for rows.Next()"
	// part of reading the results, and unfortunately that code is in the generated
	// sqlc package. It's a TODO for later to log actual query times.
	slog.Debug("CHQueryLog",
		"name", queryName,
		"durationish", fmt.Sprintf("%.3fms", durationMS),
		"err", err,
	)
}

func (l DBQ) PersonPositions(ctx context.Context, db clubhousequeries.DBTX) ([]clubhousequeries.PersonPosition, error) {
	rows, err := l.personPositionsCache.Get(ctx)
	return orNil(rows), err
}

func (l DBQ) PersonTeams(ctx context.Context, db clubhousequeries.DBTX) ([]clubhousequeries.PersonTeamsRow, error) {
	rows, err := l.personTeamsCache.Get(ctx)
	return orNil(rows), err
}

func (l DBQ) Positions(ctx context.Context, db clubhousequeries.DBTX) ([]clubhousequeries.PositionsRow, error) {
	rows, err := l.positionsCache.Get(ctx)
	return orNil(rows), err
}

func (l DBQ) RangersById(ctx context.Context, db clubhousequeries.DBTX) ([]clubhousequeries.RangersByIdRow, error) {
	rows, err := l.rangersByIdCache.Get(ctx)
	return orNil(rows), err
}

func (l DBQ) Teams(ctx context.Context, db clubhousequeries.DBTX) ([]clubhousequeries.TeamsRow, error) {
	rows, err := l.teamsCache.Get(ctx)
	return orNil(rows), err
}

func orNil[S ~*[]E, E any](sl S) []E {
	if sl == nil {
		return nil
	}
	return *sl
}
