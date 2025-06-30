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

//go:build !nofakedb

package fakeclubhousedb

import (
	"context"
	_ "embed"
	"fmt"
	gms "github.com/dolthub/go-mysql-server"
	gmsmemory "github.com/dolthub/go-mysql-server/memory"
	gmsserver "github.com/dolthub/go-mysql-server/server"
	gmssql "github.com/dolthub/go-mysql-server/sql"
)

//go:embed seed.sql
var seedData string

func Start(
	ctx context.Context, dbName, dbAddr, username, password string,
) (actualDBAddr string, err error) {
	db := gmsmemory.NewDatabase(dbName)
	db.BaseDatabase.EnablePrimaryKeyIndexes()
	prov := gmsmemory.NewDBProvider(db)
	engine := gms.NewDefault(prov)

	addSuperUser(engine, username, password)

	session := gmsmemory.NewSession(gmssql.NewBaseSession(), prov)
	gmsCtx := gmssql.NewContext(ctx, gmssql.WithSession(session))
	gmsCtx.SetCurrentDatabase(dbName)

	// It's considered insecure to leave this setting empty
	// https://dev.mysql.com/doc/refman/8.4/en/server-system-variables.html#sysvar_secure_file_priv
	err = gmssql.SystemVariables.AssignValues(map[string]any{
		"secure_file_priv": "NULL",
	})
	if err != nil {
		return "", fmt.Errorf("[AssignValues]: %w", err)
	}

	config := gmsserver.Config{
		Protocol: "tcp",
		Address:  dbAddr,
	}
	s, err := gmsserver.NewServer(config, engine, gmssql.NewContext, gmsmemory.NewSessionBuilder(prov), nil)
	if err != nil {
		return "", fmt.Errorf("[NewServer]: %w", err)
	}
	go func() {
		err = s.Start()
		if err != nil {
			panic(err)
		}
	}()
	return s.Listener.Addr().String(), nil
}

func SeedData() string {
	return seedData
}

func addSuperUser(engine *gms.Engine, username string, password string) {
	mysqlDb := engine.Analyzer.Catalog.MySQLDb
	ed := mysqlDb.Editor()
	defer ed.Close()
	mysqlDb.AddEphemeralSuperUser(ed, username, "localhost", password)
}
