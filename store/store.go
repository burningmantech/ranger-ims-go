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
	"github.com/burningmantech/ranger-ims-go/store/fakeimsdb"
	"github.com/go-sql-driver/mysql"
	"log/slog"
)

const (
	MariaDBVersion     = "10.5.27"
	MariaDBDockerImage = "mariadb:" + MariaDBVersion
)

//go:embed schema/current.sql
var CurrentSchema string

//go:embed schema/*-from-*.sql
var Migrations embed.FS

func SqlDB(ctx context.Context, dbStoreCfg conf.DBStore, migrateDB bool) (*sql.DB, error) {
	var mariaCfg conf.DBStoreMaria
	var err error
	switch dbStoreCfg.Type {
	case conf.DBStoreTypeNoOp:
		// This is a DB that does nothing and returns nothing on querying.
		// It's really only useful as a stand-in for testing.
		slog.Info("Using NoOp DB")
		return sql.Open("noop", "")
	case conf.DBStoreTypeFake:
		mariaCfg, err = startFakeDB(ctx, dbStoreCfg.Fake)
		if err != nil {
			return nil, fmt.Errorf("[startFakeDB]: %w", err)
		}
	case conf.DBStoreTypeMaria:
		fallthrough
	default:
		mariaCfg = dbStoreCfg.MariaDB
	}

	db, err := openDB(ctx, mariaCfg)
	if err != nil {
		return nil, fmt.Errorf("[openDB]: %w", err)
	}

	if migrateDB {
		err = MigrateDB(ctx, db)
		if err != nil {
			return nil, fmt.Errorf("[MigrateDB]: %w", err)
		}
	} else {
		slog.Info("IMS DB migration not requested")
	}

	slog.Info("Connected to IMS database")

	if dbStoreCfg.Type == conf.DBStoreTypeFake {
		_, err = db.ExecContext(ctx, fakeimsdb.SeedData())
		if err != nil {
			return nil, fmt.Errorf("[db.ExecContext]: %w", err)
		}
		slog.Info("Seeded volatile fake DB")
	}

	return db, nil
}

func openDB(ctx context.Context, mariaCfg conf.DBStoreMaria) (*sql.DB, error) {
	slog.Info("Setting up IMS DB connection")

	// Capture connection properties.
	cfg := mysql.NewConfig()
	cfg.User = mariaCfg.Username
	cfg.Passwd = mariaCfg.Password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%v:%v", mariaCfg.HostName, mariaCfg.HostPort)
	cfg.DBName = mariaCfg.Database
	cfg.MultiStatements = true

	// Get a database handle.
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("[sql.Open]: %w", err)
	}
	db.SetMaxOpenConns(int(mariaCfg.MaxOpenConns))
	pingErr := db.PingContext(ctx)
	if pingErr != nil {
		return nil, fmt.Errorf("[db.PingContext]: %w", pingErr)
	}
	return db, nil
}

func startFakeDB(ctx context.Context, mariaCfg conf.DBStoreMaria) (conf.DBStoreMaria, error) {
	port, err := fakeimsdb.Start(ctx,
		mariaCfg.Database,
		mariaCfg.HostName, mariaCfg.HostPort,
		mariaCfg.Username, mariaCfg.Password,
	)
	if err != nil {
		return mariaCfg, fmt.Errorf("[fakedb.Start]: %w", err)
	}
	mariaCfg.HostPort = int32(port)

	slog.Info("Started volatile fake DB", "config", mariaCfg)
	return mariaCfg, nil
}
