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
	"errors"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/go-sql-driver/mysql"
	"log/slog"
	"strings"
)

type schemaVersion int16

func repoSchemaVersion() (schemaVersion, error) {
	// Find the line
	// `insert into SCHEMA_INFO (VERSION) values (123);`
	// and extract the 123

	// after this
	insert := "insert into SCHEMA_INFO (VERSION) values ("
	afterInsert := strings.SplitN(CurrentSchema, insert, 2)
	if len(afterInsert) != 2 {
		return 0, errors.New("couldn't find SCHEMA_INFO insert in current.sql")
	}
	// and before ")"
	endParen := strings.SplitN(afterInsert[1], ")", 2)
	if len(afterInsert) != 2 {
		return 0, errors.New("couldn't find `)` after SCHEMA_INFO insert in current.sql")
	}

	vers, err := conv.ParseInt16(strings.TrimSpace(endParen[0]))
	return schemaVersion(vers), err
}

func dbSchemaVersion(ctx context.Context, db *sql.DB) (schemaVersion, error) {
	dbq := New(db, imsdb.New())
	result, err := dbq.SchemaVersion(ctx, db)
	if err == nil {
		return schemaVersion(result), nil
	}

	const tableUnknownError = 1146
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == tableUnknownError {
		slog.Info("No SCHEMA_INFO table found. This must be a new database.")
		return 0, nil
	}
	return 0, fmt.Errorf("[schemaVersion]: %w", err)
}

func runScript(ctx context.Context, imsDBQ *sql.DB, script string) error {
	_, err := imsDBQ.ExecContext(ctx, script)
	if err != nil {
		return fmt.Errorf("[ExecContext]: %w", err)
	}
	return nil
}

func migrate(ctx context.Context, imsDBQ *sql.DB, to, from schemaVersion) error {
	if from == 0 {
		if err := runScript(ctx, imsDBQ, CurrentSchema); err != nil {
			return fmt.Errorf("[runScript]: %w", err)
		}
		slog.Info("Migrated schema version", "to", to, "from", from)
		return nil
	}
	for step := from + 1; step <= to; step++ {
		b, err := Migrations.ReadFile(fmt.Sprintf("schema/%02d-from-%02d.sql", step, step-1))
		if err != nil {
			return fmt.Errorf("[ReadFile]: %w", err)
		}
		if err := runScript(ctx, imsDBQ, string(b)); err != nil {
			return fmt.Errorf("[runScript]: %w", err)
		}
		slog.Info("Migrated schema version", "to", step, "from", step-1)
	}
	return nil
}

func MigrateDB(ctx context.Context, imsDBQ *sql.DB) error {
	dbVersion, err := dbSchemaVersion(ctx, imsDBQ)
	if err != nil {
		return fmt.Errorf("[dbSchemaVersion]: %w", err)
	}
	repoVersion, err := repoSchemaVersion()
	if err != nil {
		return fmt.Errorf("[repoSchemaVersion]: %w", err)
	}
	slog.Info("Read schema versions", "repoVersion", repoVersion, "dbVersion", dbVersion)
	if dbVersion == repoVersion {
		// DB is up-to-date. Move along.
		return nil
	}
	if dbVersion > repoVersion {
		return fmt.Errorf("the DB schema is ahead of the schema in the code (%v > %v). Something is wrong", dbVersion, repoVersion)
	}
	if err = migrate(ctx, imsDBQ, repoVersion, dbVersion); err != nil {
		return fmt.Errorf("[migrate]: %w", err)
	}

	// We should be done now. Check to be sure the schema version was updated.
	dbVersion, err = dbSchemaVersion(ctx, imsDBQ)
	if err != nil {
		return fmt.Errorf("[repoSchemaVersion]: %w", err)
	}
	if dbVersion != repoVersion {
		return fmt.Errorf("failed to migrate to schema version %v. Database reports a version of %v", repoVersion, dbVersion)
	}
	return nil
}
