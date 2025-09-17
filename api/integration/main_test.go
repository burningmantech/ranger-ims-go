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

package integration_test

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/api"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	chqueries "github.com/burningmantech/ranger-ims-go/directory/clubhousedb"
	_ "github.com/burningmantech/ranger-ims-go/lib/noopdb"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/burningmantech/ranger-ims-go/lib/testctr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/actionlog"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/testcontainers/testcontainers-go"
	"golang.org/x/sync/errgroup"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

//go:embed clubhousedb_test_seed.sql
var clubhouseDBTestSeed string

// mainTestInternal contains fields to be used only within main_test.go.
var mainTestInternal struct {
	dbCtr                 testcontainers.Container
	dbCtrCleanup          func()
	clubhouseDbCtr        testcontainers.Container
	clubhouseDbCtrCleanup func()
}

// shared contains fields that may be used by any test in the integration package.
// These are fields from the common setup performed in main_test.go.
var shared struct {
	cfg          *conf.IMSConfig
	imsDBQ       *store.DBQ
	userStore    *directory.UserStore
	es           *api.EventSourcerer
	testServer   *httptest.Server
	serverURL    *url.URL
	actionLogger *actionlog.Logger
}

// These values must align with those in clubhousedb_test_seed.sql.
const (
	userAdminHandle   = "AdminTestRanger"
	userAdminEmail    = "admintestranger@example.com"
	userAdminPassword = ")'("

	userAliceHandle   = "AliceTestRanger"
	userAliceEmail    = "alicetestranger@example.com"
	userAlicePassword = "password"
)

// TestMain does the common setup and teardown for all tests in this package.
// It's slow to start up a MariaDB container, so we want to only have to do
// that once for the whole suite of test files.
func TestMain(m *testing.M) {
	ctx := context.Background()
	tempDir, err := os.MkdirTemp("", "imstest-*")
	must(err)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic: %v", r)
			shutdown(ctx, tempDir)
			os.Exit(1)
		}
	}()
	setup(ctx, tempDir)
	code := m.Run()
	shutdown(ctx, tempDir)
	os.Exit(code)
}

func setup(ctx context.Context, tempDir string) {
	tempRoot, err := os.OpenRoot(tempDir)
	must(err)

	shared.cfg = conf.DefaultIMS()
	shared.cfg.Core.JWTSecret = "jwtsecret-" + rand.NonCryptoText()
	shared.cfg.Core.Admins = []string{userAdminHandle}
	// 100 KiB, much lower than we'd use outside tests, since we want to test error cases
	// when requests are too large.
	shared.cfg.Core.MaxRequestBytes = 100 << 10
	shared.cfg.AttachmentsStore.Type = conf.AttachmentsStoreLocal
	shared.cfg.AttachmentsStore.Local = conf.LocalAttachments{
		Dir: tempRoot,
	}
	shared.cfg.Store.Type = conf.DBStoreTypeMaria
	shared.cfg.Store.MariaDB.Database = "ims-" + rand.NonCryptoText()
	shared.cfg.Store.MariaDB.Username = "rangers-" + rand.NonCryptoText()
	shared.cfg.Store.MariaDB.Password = "password-" + rand.NonCryptoText()
	shared.cfg.Directory.Directory = conf.DirectoryTypeClubhouseDB
	shared.cfg.Directory.ClubhouseDB.Database = "clubhouse-" + rand.NonCryptoText()
	shared.cfg.Directory.ClubhouseDB.Username = "rangers-" + rand.NonCryptoText()
	shared.cfg.Directory.ClubhouseDB.Password = "password-" + rand.NonCryptoText()
	must(shared.cfg.Validate())
	shared.es = api.NewEventSourcerer()

	// Do IMS and Clubhouse DB setup in parallel, since the container startup takes a few seconds each
	g := errgroup.Group{}
	g.Go(func() error {
		chCtr, chCleanup, chDbHostPort, err := testctr.MariaDBContainer(
			ctx,
			shared.cfg.Directory.ClubhouseDB.Database,
			shared.cfg.Directory.ClubhouseDB.Username,
			shared.cfg.Directory.ClubhouseDB.Password,
		)
		if err != nil {
			return err
		}
		mainTestInternal.clubhouseDbCtr = chCtr
		mainTestInternal.clubhouseDbCtrCleanup = chCleanup
		shared.cfg.Directory.ClubhouseDB.Hostname = fmt.Sprintf(":%d", chDbHostPort)
		clubhouseDB, err := directory.MariaDB(ctx, shared.cfg.Directory)
		if err != nil {
			return err
		}
		_, err = clubhouseDB.ExecContext(ctx, directory.CurrentSchema)
		if err != nil {
			return err
		}
		_, err = clubhouseDB.ExecContext(ctx, clubhouseDBTestSeed)
		if err != nil {
			return err
		}
		clubhouseDBQ := directory.NewDBQ(
			clubhouseDB,
			chqueries.New(),
			shared.cfg.Directory.InMemoryCacheTTL,
		)
		shared.userStore = directory.NewUserStore(clubhouseDBQ, shared.cfg.Directory.InMemoryCacheTTL)
		return nil
	})
	g.Go(func() error {
		ctr, cleanup, dbHostPort, err := testctr.MariaDBContainer(
			ctx,
			shared.cfg.Store.MariaDB.Database,
			shared.cfg.Store.MariaDB.Username,
			shared.cfg.Store.MariaDB.Password,
		)
		if err != nil {
			return err
		}
		mainTestInternal.dbCtr = ctr
		mainTestInternal.dbCtrCleanup = cleanup
		shared.cfg.Store.MariaDB.HostPort = dbHostPort
		db, err := store.SqlDB(ctx, shared.cfg.Store, true)
		if err != nil {
			return err
		}
		shared.imsDBQ = store.NewDBQ(db, imsdb.New())
		return nil
	})
	must(g.Wait())

	shared.actionLogger = actionlog.NewLogger(ctx, shared.imsDBQ, shared.cfg.Core.ActionLogEnabled, true)
	shared.testServer = httptest.NewServer(
		api.AddToMux(nil, shared.es, shared.cfg, shared.imsDBQ, shared.userStore, nil, shared.actionLogger),
	)
	shared.serverURL, err = url.Parse(shared.testServer.URL)
	must(err)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func shutdown(ctx context.Context, tempDir string) {
	_ = os.RemoveAll(tempDir)
	if shared.testServer != nil {
		shared.testServer.Close()
	}
	if shared.imsDBQ != nil {
		_ = shared.imsDBQ.Close()
	}
	if mainTestInternal.dbCtrCleanup != nil {
		mainTestInternal.dbCtrCleanup()
	}
	if mainTestInternal.clubhouseDbCtrCleanup != nil {
		mainTestInternal.clubhouseDbCtrCleanup()
	}
}
