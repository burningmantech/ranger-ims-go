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
	"crypto/rand"
	"github.com/burningmantech/ranger-ims-go/api"
	"github.com/burningmantech/ranger-ims-go/auth/password"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"log"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
)

// mainTestInternal contains fields to be used only within main_test.go.
var mainTestInternal struct {
	imsDBContainer testcontainers.Container
}

// shared contains fields that may be used by any test in the integration package.
// These are fields from the common setup performed in main_test.go.
var shared struct {
	cfg        *conf.IMSConfig
	imsDB      *store.DB
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
	defer func() {
		if r := recover(); r != nil {
			log.Println("Recovered from panic")
			shutdown(ctx)
		}
	}()
	setup(ctx)
	code := m.Run()
	shutdown(ctx)
	os.Exit(code)
}

func setup(ctx context.Context) {
	shared.cfg = conf.DefaultIMS()
	shared.cfg.Core.JWTSecret = rand.Text()
	shared.cfg.Core.Admins = []string{userAdminHandle}
	shared.cfg.Store.MySQL.Database = "ims"
	shared.cfg.Store.MySQL.Username = "rangers"
	shared.cfg.Store.MySQL.Password = rand.Text()
	shared.cfg.Directory.Directory = conf.DirectoryTypeTestUsers
	shared.cfg.Directory.TestUsers = []conf.TestUser{
		{
			Handle:      userAliceHandle,
			Email:       userAliceEmail,
			Status:      "active",
			DirectoryID: 80808,
			Password:    password.NewSalted("password"),
			Onsite:      true,
			Positions:   nil,
			Teams:       nil,
		},
		{
			Handle:      userAdminHandle,
			Email:       userAdminEmail,
			Status:      "active",
			DirectoryID: 70707,
			Password:    password.NewSalted(")'("),
			Onsite:      true,
			Positions:   nil,
			Teams:       nil,
		},
	}
	shared.es = api.NewEventSourcerer()
	userStore, err := directory.NewUserStore(shared.cfg.Directory.TestUsers, nil)
	if err != nil {
		panic(err)
	}
	shared.userStore = userStore
	req := testcontainers.ContainerRequest{
		Image:        "mariadb:10.5.27",
		ExposedPorts: []string{"3306/tcp"},
		WaitingFor:   wait.ForListeningPort("3306/tcp"),
		Env: map[string]string{
			"MARIADB_RANDOM_ROOT_PASSWORD": "true",
			"MARIADB_DATABASE":             shared.cfg.Store.MySQL.Database,
			"MARIADB_USER":                 shared.cfg.Store.MySQL.Username,
			"MARIADB_PASSWORD":             shared.cfg.Store.MySQL.Password,
		},
	}
	mainTestInternal.imsDBContainer, err = testcontainers.GenericContainer(ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
			// This logging is useful for debugging container startup issues
			//Logger:           log.New(os.Stdout, "MariaDB ", log.LstdFlags),
		},
	)
	if err != nil {
		panic(err)
	}
	endpoint, err := mainTestInternal.imsDBContainer.Endpoint(ctx, "")
	if err != nil {
		panic(err)
	}
	port, _ := strconv.Atoi(strings.TrimPrefix(endpoint, "localhost:"))
	shared.cfg.Store.MySQL.HostPort = int32(port)
	db := store.MariaDB(ctx, shared.cfg)
	shared.imsDB = &store.DB{DB: db}
	// Use faster/less-secure UUID generation for tests
	uuid.EnableRandPool()
	shared.testServer = httptest.NewServer(api.AddToMux(nil, shared.es, shared.cfg, shared.imsDB, shared.userStore))
	shared.serverURL, err = url.Parse(shared.testServer.URL)
	if err != nil {
		panic(err)
	}
}

func shutdown(ctx context.Context) {
	shared.testServer.Close()
	_ = shared.imsDB.Close()
	err := mainTestInternal.imsDBContainer.Terminate(ctx)
	if err != nil {
		// log and continue
		slog.Error("Failed to terminate container", "error", err)
	}
}
