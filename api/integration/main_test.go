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
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/burningmantech/ranger-ims-go/api"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	chqueries "github.com/burningmantech/ranger-ims-go/directory/clubhousedb"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	_ "github.com/burningmantech/ranger-ims-go/lib/noopdb"
	"github.com/burningmantech/ranger-ims-go/lib/rand"
	"github.com/burningmantech/ranger-ims-go/lib/testctr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/actionlog"
	"github.com/burningmantech/ranger-ims-go/store/errorlog"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"golang.org/x/sync/errgroup"
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
	actionLogger *actionlog.Logger
	errorLogger  *errorlog.Logger
}

// bmAPIYearNoData and bmAPIYearBroken are the years the fake Burning Man API
// treats specially. See fakeBMAPI.
const (
	bmAPIYearNoData = "1999"
	bmAPIYearBroken = "1900"
)

// fakeBMAPI stands in for the public Burning Man API, which the places import
// endpoint calls. It answers from the request alone, holding no state of its
// own, so that the tests using it can still run in parallel.
func fakeBMAPI(w http.ResponseWriter, req *http.Request) {
	if req.Header.Get("X-API-Key") != shared.cfg.BurningManAPI.APIKey {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	kind := strings.TrimPrefix(req.URL.Path, "/api/")
	year := req.URL.Query().Get("year")
	w.Header().Set("Content-Type", "application/json")
	switch year {
	case bmAPIYearBroken:
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"detail": "the API is having a bad day"}`))
	case bmAPIYearNoData:
		// What the real API gives for a year it has nothing for.
		_, _ = w.Write([]byte(`[]`))
	default:
		// #nosec G705 // XSS via taint analysis. This test fake just echoes the
		// request it was given back as JSON.
		_, _ = fmt.Fprintf(w, `[
			{"uid": "%[1]v-1", "name": "%[1]v One", "location_string": "3:00 & A", "year": %[2]v},
			{"uid": "%[1]v-2", "name": "%[1]v Two", "location_string": "4:00 & B", "year": %[2]v}
		]`, kind, year)
	}
}

// panicPath is a test-only route that always panics, so that the error log
// tests can drive a genuine 500 through the whole middleware stack. There's no
// real endpoint that fails on demand.
const panicPath = "/ims/api/test/panic"

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
	// Each test server gets its own stand-in for the Burning Man API, reached
	// through an injected client rather than by address, so this URL is never
	// dialed. It's an unresolvable one, so a broken injection fails loudly
	// instead of calling the real API.
	shared.cfg.BurningManAPI = conf.BurningManAPI{
		URL:    "http://burning-man-api.invalid",
		APIKey: "bmapikey-" + rand.NonCryptoText(),
	}
	must(shared.cfg.Validate())

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
		clubhouseDBQ := directory.NewDBQ(clubhouseDB, chqueries.New())
		shared.userStore = directory.NewUserStore(
			directory.NewClubhouseSource(clubhouseDBQ),
			shared.cfg.Directory.InMemoryCacheTTL,
		)
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
	shared.errorLogger = errorlog.NewLogger(ctx, shared.imsDBQ, shared.cfg.Core.ErrorLogEnabled, true)
}

// testServer is one test's own IMS API server. httptest.NewTestServer ties a
// server's lifetime to the test that created it, so there's no sharing one
// across the package the way the MariaDB containers are shared. The server
// speaks over an in-memory network rather than a loopback port, so only the
// client it hands out can reach it.
type testServer struct {
	t      *testing.T
	server *httptest.Server
	url    *url.URL
	client *http.Client
}

// newServer starts a server for this test, using the package's standard
// config, IMS database and Clubhouse directory.
func newServer(t *testing.T) testServer {
	t.Helper()
	return newCustomServer(t, shared.cfg, shared.imsDBQ, shared.userStore)
}

// newCustomServer starts a server for a test that needs a different config,
// database or user directory than newServer provides.
func newCustomServer(
	t *testing.T, cfg *conf.IMSConfig, imsDBQ *store.DBQ, userStore *directory.UserStore,
) testServer {
	t.Helper()
	bmAPI := httptest.NewTestServer(t, http.HandlerFunc(fakeBMAPI))
	mux := api.AddToMux(
		nil, api.NewEventSourcerer(), cfg, imsDBQ, userStore, nil,
		shared.actionLogger, shared.errorLogger, bmAPI.Client(),
	)
	mux.Handle(http.MethodGet+" "+panicPath, api.Adapt(
		http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			panic("this handler always panics")
		}),
		api.RecordErrors(shared.errorLogger),
		api.RecoverFromPanic(),
		api.RequireAuthN(
			authz.JWTer{SecretKey: cfg.Core.JWTSecret},
			authz.TokenCookies{Insecure: cfg.Core.InsecureCookies},
			userStore,
		),
		api.LogRequest(api.LogNothing, shared.actionLogger, userStore),
	))
	server := httptest.NewTestServer(t, mux)
	client := *server.Client()
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	return testServer{t: t, server: server, url: serverURL, client: &client}
}

// unauthed returns a helper that sends no Authorization header.
func (s testServer) unauthed() ApiHelper {
	return ApiHelper{t: s.t, serverURL: s.url, client: s.client}
}

// withJWT returns a helper authenticated with an already-obtained token.
func (s testServer) withJWT(jwt string) ApiHelper {
	return ApiHelper{t: s.t, serverURL: s.url, client: s.client, jwt: jwt}
}

// admin returns a helper logged in as the package's IMS admin.
func (s testServer) admin(ctx context.Context) ApiHelper {
	s.t.Helper()
	return s.withJWT(s.login(ctx, userAdminEmail, userAdminPassword))
}

// alice returns a helper logged in as an ordinary, non-admin Ranger.
func (s testServer) alice(ctx context.Context) ApiHelper {
	s.t.Helper()
	return s.withJWT(s.login(ctx, userAliceEmail, userAlicePassword))
}

func (s testServer) login(ctx context.Context, identification, password string) string {
	s.t.Helper()
	statusCode, _, token := s.unauthed().postAuth(ctx, api.PostAuthRequest{
		Identification: identification,
		Password:       password,
	})
	require.Equal(s.t, http.StatusOK, statusCode)
	return token
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func shutdown(ctx context.Context, tempDir string) {
	_ = os.RemoveAll(tempDir)
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
