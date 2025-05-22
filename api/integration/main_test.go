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
	"github.com/burningmantech/ranger-ims-go/api"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/authn"
	_ "github.com/burningmantech/ranger-ims-go/lib/noopdb"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"log"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

// mainTestInternal contains fields to be used only within main_test.go.
var mainTestInternal struct {
	dbCtr testcontainers.Container
}

// shared contains fields that may be used by any test in the integration package.
// These are fields from the common setup performed in main_test.go.
var shared struct {
	cfg        *conf.IMSConfig
	imsDBQ     *store.DBQ
	userStore  *directory.UserStore
	es         *api.EventSourcerer
	testServer *httptest.Server
	serverURL  *url.URL
}

const (
	userAdminHandle   = "AdminTestRanger"
	userAdminEmail    = "admintestranger@rangers.brc"
	userAdminPassword = ")'("

	userAliceHandle   = "AliceTestRanger"
	userAliceEmail    = "alicetestranger@rangers.brc"
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
			log.Println("Recovered from panic")
			shutdown(ctx, tempDir)
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
	shared.cfg.Store.MariaDB.Database = "ims-" + rand.NonCryptoText()
	shared.cfg.Store.MariaDB.Username = "rangers-" + rand.NonCryptoText()
	shared.cfg.Store.MariaDB.Password = "password-" + rand.NonCryptoText()
	shared.cfg.Directory.Directory = conf.DirectoryTypeTestUsers
	shared.cfg.Directory.TestUsers = []conf.TestUser{
		{
			Handle:      userAliceHandle,
			Email:       userAliceEmail,
			Status:      "active",
			DirectoryID: 80808,
			Password:    authn.NewSalted("password"),
			Onsite:      true,
			Positions:   []string{"Nooperator"},
			Teams:       nil,
		},
		{
			Handle:      userAdminHandle,
			Email:       userAdminEmail,
			Status:      "active",
			DirectoryID: 70707,
			Password:    authn.NewSalted(")'("),
			Onsite:      true,
			Positions:   nil,
			Teams:       []string{"Brown Dot"},
		},
	}
	must(shared.cfg.Validate())
	shared.es = api.NewEventSourcerer()
	clubhouseDBQ := directory.NewFakeTestUsersDBQ(
		shared.cfg.Directory.TestUsers,
		shared.cfg.Directory.InMemoryCacheTTL,
	)
	shared.userStore = directory.NewUserStore(clubhouseDBQ)
	mainTestInternal.dbCtr, err = testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        store.MariaDBDockerImage,
				ExposedPorts: []string{"3306/tcp"},
				WaitingFor:   wait.ForLog("port: 3306  mariadb.org binary distribution"),
				Env: map[string]string{
					"MARIADB_RANDOM_ROOT_PASSWORD": "true",
					"MARIADB_DATABASE":             shared.cfg.Store.MariaDB.Database,
					"MARIADB_USER":                 shared.cfg.Store.MariaDB.Username,
					"MARIADB_PASSWORD":             shared.cfg.Store.MariaDB.Password,
				},
			},
			Started: true,
		},
	)
	must(err)
	port, err := mainTestInternal.dbCtr.MappedPort(ctx, "3306/tcp")
	must(err)
	shared.cfg.Store.MariaDB.HostPort = int32(port.Int())
	db, err := store.SqlDB(ctx, shared.cfg.Store, true)
	must(err)
	shared.imsDBQ = store.New(db, imsdb.New())
	shared.testServer = httptest.NewServer(api.AddToMux(nil, shared.es, shared.cfg, shared.imsDBQ, shared.userStore, nil))
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
	if mainTestInternal.dbCtr != nil {
		err := mainTestInternal.dbCtr.Terminate(ctx)
		if err != nil {
			// log and continue
			slog.Error("Failed to terminate container", "error", err)
		}
	}
}
