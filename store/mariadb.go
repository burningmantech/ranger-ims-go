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

package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	_ "github.com/burningmantech/ranger-ims-go/lib/noopdb"
	"github.com/go-sql-driver/mysql"
	"log/slog"
	"strings"
	"time"
)

const (
	MariaDBVersion     = "10.5.27"
	MariaDBDockerImage = "mariadb:" + MariaDBVersion
)

//go:embed schema/current.sql
var CurrentSchema string

//go:embed schema/*-from-*.sql
var Migrations embed.FS

func IMSDB(ctx context.Context, dbStoreCfg conf.DBStore, migrateDB bool) (*sql.DB, error) {
	if dbStoreCfg.Type == conf.DBStoreTypeNoOp {
		// This is a DB that does nothing and returns nothing on querying.
		// It's really only useful as a stand-in for testing.
		slog.Info("Using NoOp DB")
		return sql.Open("noop", "")
	}
	slog.Info("Setting up IMS DB connection")
	mariaCfg := dbStoreCfg.MariaDB

	// Capture connection properties.
	cfg := mysql.NewConfig()
	cfg.User = mariaCfg.Username
	cfg.Passwd = mariaCfg.Password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%v:%v", mariaCfg.HostName, mariaCfg.HostPort)
	cfg.DBName = mariaCfg.Database

	// Get a database handle.
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("[sql.Open]: %ws", err)
	}
	// Some arbitrary value. We'll get errors from MariaDB if the server
	// hits the DB with too many parallel requests.
	db.SetMaxOpenConns(20)
	pingErr := db.PingContext(ctx)
	if pingErr != nil {
		return nil, fmt.Errorf("[db.PingContext]: %ws", pingErr)
	}

	if migrateDB {
		if err = MigrateDB(ctx, db); err != nil {
			return nil, fmt.Errorf("[MigrateDB]: %w", err)
		}
	} else {
		slog.Info("IMS DB migration not requested")
	}

	slog.Info("Connected to IMS MariaDB")
	return db, nil
}

type DB struct {
	*sql.DB
}

func (l DB) ExecContext(ctx context.Context, s string, i ...interface{}) (sql.Result, error) {
	start := time.Now()
	result, err := l.DB.ExecContext(ctx, s, i...)
	logQuery(s, start, err)
	return result, err
}

func (l DB) PrepareContext(ctx context.Context, s string) (*sql.Stmt, error) {
	start := time.Now()
	stmt, err := l.DB.PrepareContext(ctx, s)
	logQuery(s, start, err)
	return stmt, err
}

func (l DB) QueryContext(ctx context.Context, s string, i ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	rows, err := l.DB.QueryContext(ctx, s, i...)
	logQuery(s, start, err)
	return rows, err
}

func (l DB) QueryRowContext(ctx context.Context, s string, i ...interface{}) *sql.Row {
	start := time.Now()
	row := l.DB.QueryRowContext(ctx, s, i...)
	logQuery(s, start, nil)
	return row
}

func logQuery(s string, start time.Time, err error) {
	queryName, _, _ := strings.Cut(s, "\n")
	queryName = strings.TrimPrefix(queryName, "-- name: ")
	queryName = strings.Fields(queryName)[0]
	timeMS := float64(time.Since(start).Microseconds()) / 1000.0

	// Note that the duration(ish) is very misleading. It'll always be less than
	// the actual query time, often significantly. That's because most of the IO
	// takes place after we're able to log in this file, e.g. in the "for rows.Next()"
	// part of reading the results, and unfortunately that code is in the generated
	// sqlc package. It's a TODO for later to log actual query times.
	slog.Debug("QueryLog",
		"name", queryName,
		"durationish", fmt.Sprintf("%.3fms", timeMS),
		"err", err,
	)
}
