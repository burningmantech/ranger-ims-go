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
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/stretchr/testify/require"
)

// rustTarget reports whether this run drives the experimental Rust port of
// the server (in rust/) instead of the in-process Go one. Set
// IMS_TEST_TARGET=rust and IMS_RUST_BIN to the built binary.
func rustTarget() bool {
	return os.Getenv("IMS_TEST_TARGET") == "rust"
}

// newRustServer starts the Rust server as a subprocess, configured through
// the same env vars as the Go server, on the package's shared databases.
func newRustServer(t *testing.T, cfg *conf.IMSConfig, imsDBQ *store.DBQ, userStore *directory.UserStore) testServer {
	t.Helper()
	var hookURL string
	if imsDBQ != shared.imsDBQ {
		interceptor, ok := imsDBQ.Querier.(casInterceptor)
		if !ok || imsDBQ.DB != shared.imsDBQ.DB {
			t.Skip("not supported against the Rust server: needs an in-process query interceptor")
		}
		hookServer := httptest.NewServer(queryHookHandler(t, interceptor))
		t.Cleanup(hookServer.Close)
		hookURL = hookServer.URL
	}
	if userStore != shared.userStore && cfg.Directory.Directory != conf.DirectoryTypeIMS {
		t.Skip("not supported against the Rust server: needs a custom in-process user store")
	}
	bin := os.Getenv("IMS_RUST_BIN")
	require.NotEmpty(t, bin, "IMS_RUST_BIN must name the Rust server binary")

	// The Rust server can't be handed a client, so the fake Burning Man API
	// listens on a real port instead.
	bmAPI := httptest.NewServer(http.HandlerFunc(fakeBMAPI))
	t.Cleanup(bmAPI.Close)

	envFile := filepath.Join(t.TempDir(), "empty.env")
	require.NoError(t, os.WriteFile(envFile, nil, 0o600))

	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"IMS_HOSTNAME=127.0.0.1",
		"IMS_PORT=0",
		"IMS_DEPLOYMENT=dev",
		"IMS_LOG_LEVEL=WARN",
		"IMS_JWT_SECRET=" + cfg.Core.JWTSecret,
		"IMS_TOKEN_LIFETIME=" + cfg.Core.TokenLifetime.String(),
		"IMS_CACHE_CONTROL_SHORT=" + cfg.Core.CacheControlShort.String(),
		"IMS_CACHE_CONTROL_LONG=" + cfg.Core.CacheControlLong.String(),
		"IMS_DIRECTORY_CACHE_TTL=" + cfg.Directory.InMemoryCacheTTL.String(),
		"IMS_ACTION_LOG_ENABLED=" + strconv.FormatBool(cfg.Core.ActionLogEnabled),
		"IMS_ERROR_LOG_ENABLED=" + strconv.FormatBool(cfg.Core.ErrorLogEnabled),
		"IMS_EVENT_DELETION_ENABLED=" + strconv.FormatBool(cfg.Core.EventDeletionEnabled),
		"IMS_INSECURE_COOKIES=" + strconv.FormatBool(cfg.Core.InsecureCookies),
		"IMS_BM_API_URL=" + bmAPI.URL,
		"IMS_BM_API_KEY=" + cfg.BurningManAPI.APIKey,
		"IMS_DIRECTORY=" + string(cfg.Directory.Directory),
		"IMS_DB_STORE_TYPE=mariadb",
		"IMS_DB_HOST_NAME=127.0.0.1",
		"IMS_DB_HOST_PORT=" + strconv.Itoa(int(cfg.Store.MariaDB.HostPort)),
		"IMS_DB_DATABASE=" + cfg.Store.MariaDB.Database,
		"IMS_DB_USER_NAME=" + cfg.Store.MariaDB.Username,
		"IMS_DB_PASSWORD=" + cfg.Store.MariaDB.Password,
		"IMS_DMS_HOSTNAME=127.0.0.1" + shared.cfg.Directory.ClubhouseDB.Hostname,
		"IMS_DMS_DATABASE=" + shared.cfg.Directory.ClubhouseDB.Database,
		"IMS_DMS_USERNAME=" + shared.cfg.Directory.ClubhouseDB.Username,
		"IMS_DMS_PASSWORD=" + shared.cfg.Directory.ClubhouseDB.Password,
		"IMS_ATTACHMENTS_STORE=" + string(cfg.AttachmentsStore.Type),
		// These aren't read by the Go server. See conf::TestHooks in the Rust port.
		"IMS_MAX_REQUEST_BYTES=" + strconv.FormatInt(cfg.Core.MaxRequestBytes, 10),
		"IMS_DB_MAX_OPEN_CONNS=3",
		"IMS_DMS_MAX_OPEN_CONNS=3",
		"IMS_TEST_PANIC_ROUTE=true",
		"IMS_TEST_SYNC_LOGS=true",
	}
	if len(cfg.Core.Admins) > 0 {
		env = append(env, "IMS_ADMINS="+strings.Join(cfg.Core.Admins, ","))
	}
	if hookURL != "" {
		env = append(env, "IMS_TEST_QUERY_HOOK_URL="+hookURL)
	}
	if cfg.AttachmentsStore.Local.Dir != nil {
		env = append(env, "IMS_ATTACHMENTS_LOCAL_DIR="+cfg.AttachmentsStore.Local.Dir.Name())
	}

	// #nosec G204 G702 // The binary is named by the test runner.
	cmd := exec.Command(bin, "serve", "--envfile", envFile, "--print-config=false")
	cmd.Env = env
	var stderr lockedBuffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			_ = cmd.Process.Kill()
		}
		if t.Failed() {
			t.Logf("Rust server stderr:\n%v", stderr.String())
		}
	})

	addrCh := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if addr, ok := strings.CutPrefix(scanner.Text(), "ims listening on "); ok {
				addrCh <- addr
				break
			}
		}
		_, _ = io.Copy(io.Discard, stdout)
	}()
	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(60 * time.Second):
		t.Fatalf("Rust server didn't start. stderr:\n%v", stderr.String())
	}
	serverURL, err := url.Parse("http://" + addr)
	require.NoError(t, err)
	return testServer{t: t, url: serverURL, client: &http.Client{}}
}

// queryHookCall is what the Rust server posts to its query hook just before
// running one of the queries a casInterceptor can intercept.
type queryHookCall struct {
	Query          string  `json:"query"`
	Event          int32   `json:"event"`
	Number         int32   `json:"number"`
	Version        int32   `json:"version"`
	Created        float64 `json:"created"`
	Started        float64 `json:"started"`
	Priority       int8    `json:"priority"`
	State          string  `json:"state"`
	Summary        *string `json:"summary"`
	IncidentNumber *int32  `json:"incident_number"`
}

// queryHookHandler runs a casInterceptor's hooks for the Rust server, which
// calls this before the corresponding queries, as the interceptor would be
// called from inside the Go server.
func queryHookHandler(t *testing.T, interceptor casInterceptor) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, req *http.Request) {
		var call queryHookCall
		err := json.NewDecoder(req.Body).Decode(&call)
		if err != nil {
			t.Errorf("bad query hook call: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		ctx := req.Context()
		switch call.Query {
		case "UpdateIncident":
			if interceptor.beforeUpdateIncident != nil {
				interceptor.beforeUpdateIncident(ctx, imsdb.UpdateIncidentParams{Event: call.Event, Number: call.Number, Version: call.Version})
			}
		case "UpdateFieldReport":
			if interceptor.beforeUpdateFieldReport != nil {
				interceptor.beforeUpdateFieldReport(ctx, imsdb.UpdateFieldReportParams{Event: call.Event, Number: call.Number, Version: call.Version})
			}
		case "UpdateVisit":
			if interceptor.beforeUpdateVisit != nil {
				interceptor.beforeUpdateVisit(ctx, imsdb.UpdateVisitParams{Event: call.Event, Number: call.Number, Version: call.Version})
			}
		case "CreateIncident":
			if interceptor.beforeCreateIncident != nil {
				interceptor.beforeCreateIncident(ctx, imsdb.CreateIncidentParams{
					Event:    call.Event,
					Number:   call.Number,
					Created:  call.Created,
					Priority: call.Priority,
					State:    imsdb.IncidentState(call.State),
					Started:  call.Started,
				})
			}
		case "CreateFieldReport":
			if interceptor.beforeCreateFieldReport != nil {
				params := imsdb.CreateFieldReportParams{Event: call.Event, Number: call.Number, Created: call.Created}
				if call.Summary != nil {
					params.Summary = sql.NullString{String: *call.Summary, Valid: true}
				}
				if call.IncidentNumber != nil {
					params.IncidentNumber = sql.NullInt32{Int32: *call.IncidentNumber, Valid: true}
				}
				interceptor.beforeCreateFieldReport(ctx, params)
			}
		case "CreateVisit":
			if interceptor.beforeCreateVisit != nil {
				interceptor.beforeCreateVisit(ctx, imsdb.CreateVisitParams{Event: call.Event, Number: call.Number, Created: call.Created})
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// lockedBuffer is a bytes.Buffer that the subprocess and the test can share.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p) //nolint:wrapcheck
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
