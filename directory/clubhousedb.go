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
	"github.com/burningmantech/ranger-ims-go/directory/fakeclubhousedb"
	"github.com/burningmantech/ranger-ims-go/lib/cache"
	"github.com/go-sql-driver/mysql"
	"log/slog"
	"time"
)

//go:embed schema/current.sql
var CurrentSchema string

func MariaDB(ctx context.Context, directoryCfg conf.Directory) (*sql.DB, error) {
	var imsCfg conf.ClubhouseDB
	var err error
	switch directoryCfg.Directory {
	case conf.DirectoryTypeNoOp:
		// This is a DB that does nothing and returns nothing on querying.
		// It's really only useful as a stand-in for testing.
		slog.Info("Using NoOp DB")
		return sql.Open("noop", "")
	case conf.DirectoryTypeFake:
		imsCfg, err = startFakeDB(ctx, directoryCfg.FakeDB)
		if err != nil {
			return nil, fmt.Errorf("[startFakeDB]: %w", err)
		}
	case conf.DirectoryTypeClubhouseDB:
		fallthrough
	default:
		imsCfg = directoryCfg.ClubhouseDB
	}
	slog.Info("Setting up Clubhouse DB connection")

	// Capture connection properties.
	cfg := mysql.NewConfig()
	cfg.User = imsCfg.Username
	cfg.Passwd = imsCfg.Password
	cfg.Net = "tcp"
	cfg.Addr = imsCfg.Hostname
	cfg.DBName = imsCfg.Database
	cfg.MultiStatements = true

	// Get a database handle.
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("[sql.Open]: %w", err)
	}
	// Some arbitrary value. We'll get errors from MariaDB if the server
	// hits the DB with too many parallel requests.
	db.SetMaxOpenConns(int(imsCfg.MaxOpenConns))
	pingErr := db.PingContext(ctx)
	if pingErr != nil {
		return nil, fmt.Errorf("[PingContext] dsn=%v: %w", cfg.FormatDSN(), pingErr)
	}
	slog.Info("Connected to Clubhouse MariaDB")

	if directoryCfg.Directory == conf.DirectoryTypeFake {
		_, err = db.ExecContext(ctx, CurrentSchema)
		if err != nil {
			return nil, fmt.Errorf("[db.ExecContext]: %w", err)
		}
		_, err = db.ExecContext(ctx, fakeclubhousedb.SeedData())
		if err != nil {
			return nil, fmt.Errorf("[db.ExecContext]: %w", err)
		}
		slog.Info("Seeded volatile fake Clubhouse DB")
	}

	return db, nil
}

// DBQ combines the SQL database and the Querier for the Clubhouse datastore.
type DBQ struct {
	*sql.DB
	q chqueries.Querier

	rangersByIdCache     *cache.InMemory[[]chqueries.RangersByIdRow]
	personPositionsCache *cache.InMemory[[]chqueries.PersonPosition]
	personTeamsCache     *cache.InMemory[[]chqueries.PersonTeamsRow]
	positionsCache       *cache.InMemory[[]chqueries.PositionsRow]
	teamsCache           *cache.InMemory[[]chqueries.TeamsRow]
}

var _ chqueries.Querier = (*DBQ)(nil)

// NewMariaDBQ creates a DBQ that uses a standard MariaDB Clubhouse database.
func NewMariaDBQ(db *sql.DB, cacheTTL time.Duration) *DBQ {
	return newDBQ(db, chqueries.New(), cacheTTL)
}

func newDBQ(db *sql.DB, querier chqueries.Querier, cacheTTL time.Duration) *DBQ {
	dbq := &DBQ{
		DB: db,
		q:  querier,
	}
	dbq.rangersByIdCache = cachemaker(dbq, cacheTTL, "RangersById", dbq.q.RangersById)
	dbq.personPositionsCache = cachemaker(dbq, cacheTTL, "PersonPositions", dbq.q.PersonPositions)
	dbq.personTeamsCache = cachemaker(dbq, cacheTTL, "PersonTeams", dbq.q.PersonTeams)
	dbq.positionsCache = cachemaker(dbq, cacheTTL, "Positions", dbq.q.Positions)
	dbq.teamsCache = cachemaker(dbq, cacheTTL, "Teams", dbq.q.Teams)
	return dbq
}

// cachemaker cachemaker makes me a cache.
func cachemaker[T any](
	dbtx chqueries.DBTX,
	valTTL time.Duration,
	queryName string,
	refresher func(context.Context, chqueries.DBTX) (T, error),
) *cache.InMemory[T] {
	return cache.New[T](
		valTTL,
		func(ctx context.Context) (T, error) {
			start := time.Now()
			t, err := refresher(ctx, dbtx)
			logQuery(queryName, start, err)
			return t, err
		},
	)
}

func logQuery(queryName string, start time.Time, err error) {
	durationMS := float64(time.Since(start).Microseconds()) / 1000.0
	slog.Debug("Ran Clubhouse SQL: "+queryName,
		"durationish", fmt.Sprintf("%.3fms", durationMS),
		"err", err,
	)
}

func (l DBQ) PersonPositions(ctx context.Context, db chqueries.DBTX) ([]chqueries.PersonPosition, error) {
	rows, err := l.personPositionsCache.Get(ctx)
	return orNil(rows), err
}

func (l DBQ) PersonTeams(ctx context.Context, db chqueries.DBTX) ([]chqueries.PersonTeamsRow, error) {
	rows, err := l.personTeamsCache.Get(ctx)
	return orNil(rows), err
}

func (l DBQ) Positions(ctx context.Context, db chqueries.DBTX) ([]chqueries.PositionsRow, error) {
	rows, err := l.positionsCache.Get(ctx)
	return orNil(rows), err
}

func (l DBQ) RangersById(ctx context.Context, db chqueries.DBTX) ([]chqueries.RangersByIdRow, error) {
	rows, err := l.rangersByIdCache.Get(ctx)
	return orNil(rows), err
}

func (l DBQ) Teams(ctx context.Context, db chqueries.DBTX) ([]chqueries.TeamsRow, error) {
	rows, err := l.teamsCache.Get(ctx)
	return orNil(rows), err
}

// orNil takes something like *[]string and returns it as either []string or nil.
func orNil[S ~*[]E, E any](sl S) []E {
	if sl == nil {
		return nil
	}
	return *sl
}

func startFakeDB(ctx context.Context, mariaCfg conf.ClubhouseDB) (conf.ClubhouseDB, error) {
	addr, err := fakeclubhousedb.Start(ctx,
		mariaCfg.Database, mariaCfg.Hostname,
		mariaCfg.Username, mariaCfg.Password,
	)
	if err != nil {
		return mariaCfg, fmt.Errorf("[fakedb.Start]: %w", err)
	}
	mariaCfg.Hostname = addr

	slog.Info("Started volatile fake DB", "config", mariaCfg)
	return mariaCfg, nil
}
