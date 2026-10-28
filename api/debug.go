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

package api

import (
	"bytes"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/dbinfo"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/go-sql-driver/mysql"
)

var serverStartTime = time.Now()

type GetBuildInfo struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
	timeZone  *time.Location
}

type GetRuntimeMetrics struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

type GetConfig struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
	cfg       *conf.IMSConfig
}

// GetDBInfo describes the IMS and Clubhouse database connections. With full
// set, it instead lists every variable and status value each server reports.
type GetDBInfo struct {
	imsDBQ      *store.DBQ
	userStore   *directory.UserStore
	imsAdmins   []string
	cfg         *conf.IMSConfig
	clubhouseDB *sql.DB
	full        bool
}

type PerformGC struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

func (action GetBuildInfo) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	bi, errHTTP := action.getBuildInfo(req)
	if errHTTP != nil {
		errHTTP.From("[getBuildInfo]").WriteResponse(w)
		return
	}
	schemaVersion := "unknown"
	vers, err := action.imsDBQ.SchemaVersion(req.Context(), action.imsDBQ)
	if err != nil {
		slog.Error("Failed to read DB schema version", "error", err)
	} else {
		schemaVersion = strconv.Itoa(int(vers))
	}
	serverTimeInfo := fmt.Sprintf("Server started at: %v\nUptime: %v\nDatabase schema version: %v\n\n",
		serverStartTime.In(action.timeZone).Format("Mon, 2006-01-02 15:04:05 MST"), time.Since(serverStartTime).Round(time.Second), schemaVersion)
	finalRespText := serverTimeInfo + bi.String()
	herr.WriteOKResponse(w, finalRespText)
}

func (action GetBuildInfo) getBuildInfo(req *http.Request) (debug.BuildInfo, *herr.HTTPError) {
	empty := debug.BuildInfo{}
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return empty, errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateDebugging == 0 {
		return empty, herr.Forbidden("The requestor does not have GlobalAdministrateDebugging permission", nil)
	}
	return buildInfo(), nil
}

var buildInfo = sync.OnceValue[debug.BuildInfo](func() debug.BuildInfo {
	bi, ok := debug.ReadBuildInfo()
	if ok {
		return *bi
	}
	// The conditions for this to happen aren't really possible, but returning an
	// empty struct instead is a good alternative. These values are just used for
	// informational purposes in the server anyway.
	slog.Info("Build info was unavailable, so an empty placeholder will be used instead")
	return debug.BuildInfo{}
})

func (action GetRuntimeMetrics) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	metricsString, errHTTP := action.getRuntimeMetrics(req)
	if errHTTP != nil {
		errHTTP.From("[getBuildInfo]").WriteResponse(w)
		return
	}
	herr.WriteOKResponse(w, metricsString)
}

func (action GetRuntimeMetrics) getRuntimeMetrics(req *http.Request) (string, *herr.HTTPError) {
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return "", errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateDebugging == 0 {
		return "", herr.Forbidden("The requestor does not have GlobalAdministrateDebugging permission", nil)
	}

	var buf bytes.Buffer

	bufPrintf := func(format string, a ...any) {
		_, _ = fmt.Fprintf(&buf, format, a...)
	}

	var samples []metrics.Sample
	for _, d := range metrics.All() {
		// These godebug metrics aren't of much use to us, and there are a lot of them.
		if strings.HasPrefix(d.Name, "/godebug/non-default-behavior/") {
			continue
		}
		// This metric is deprecated. Prefer the identical /sched/pauses/total/gc:seconds
		if d.Name == "/gc/pauses:seconds" {
			continue
		}
		samples = append(samples, metrics.Sample{Name: d.Name})
	}

	// Sample the metrics. Re-use the samples slice if you can!
	metrics.Read(samples)

	// Iterate over all results.
	for _, sample := range samples {
		// Pull out the name and value.
		name, value := sample.Name, sample.Value

		// Handle each sample.
		switch value.Kind() {
		case metrics.KindUint64:
			bufPrintf("%s: %d\n", name, value.Uint64())
		case metrics.KindFloat64:
			bufPrintf("%s: %f\n", name, value.Float64())
		case metrics.KindFloat64Histogram:
			// The histogram may be quite large, so let's just pull out
			// a crude estimate for the median for the sake of this example.
			bufPrintf("%s: %f\n", name, medianBucket(value.Float64Histogram()))
		default:
			bufPrintf("%s: unexpected metric Kind: %v\n", name, value.Kind())
		}
	}
	return buf.String(), nil
}

func medianBucket(h *metrics.Float64Histogram) float64 {
	total := uint64(0)
	for _, count := range h.Counts {
		total += count
	}
	thresh := total / 2
	total = 0
	for i, count := range h.Counts {
		total += count
		if total >= thresh {
			return h.Buckets[i]
		}
	}
	panic("should not happen")
}

func (action GetConfig) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	cfgString, errHTTP := action.getConfig(req)
	if errHTTP != nil {
		errHTTP.From("[getConfig]").WriteResponse(w)
		return
	}
	herr.WriteOKResponse(w, cfgString)
}

func (action GetConfig) getConfig(req *http.Request) (string, *herr.HTTPError) {
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return "", errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateDebugging == 0 {
		return "", herr.Forbidden("The requestor does not have GlobalAdministrateDebugging permission", nil)
	}
	return action.cfg.String(), nil
}

func (action GetDBInfo) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	infoString, errHTTP := action.getDBInfo(req)
	if errHTTP != nil {
		errHTTP.From("[getDBInfo]").WriteResponse(w)
		return
	}
	herr.WriteOKResponse(w, infoString)
}

func (action GetDBInfo) getDBInfo(req *http.Request) (string, *herr.HTTPError) {
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return "", errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateDebugging == 0 {
		return "", herr.Forbidden("The requestor does not have GlobalAdministrateDebugging permission", nil)
	}
	ctx := req.Context()
	describe := func(db *sql.DB, mysqlCfg *mysql.Config, connMaxLifetime time.Duration) string {
		if action.full {
			return dbinfo.All(ctx, db)
		}
		return dbinfo.Summary(ctx, db, mysqlCfg, connMaxLifetime)
	}

	var buf bytes.Buffer
	buf.WriteString("IMS database\n============\n\n")
	if action.cfg.Store.Type == conf.DBStoreTypeMaria {
		buf.WriteString(describe(action.imsDBQ.DB, store.MySQLConfig(action.cfg.Store.MariaDB), store.ConnMaxLifetime))
	} else {
		_, _ = fmt.Fprintf(&buf, "The store type is %v, so there's no database connection.\n", action.cfg.Store.Type)
	}

	buf.WriteString("\n\nDirectory database\n==================\n\n")
	switch {
	case action.cfg.Directory.Directory == conf.DirectoryTypeIMS:
		buf.WriteString("The directory lives in the IMS database, so there's no separate connection.\n")
	case action.cfg.Directory.Directory != conf.DirectoryTypeClubhouseDB:
		_, _ = fmt.Fprintf(&buf, "The directory type is %v, so there's no database connection.\n", action.cfg.Directory.Directory)
	case action.clubhouseDB == nil:
		buf.WriteString("This server has no Clubhouse database connection.\n")
	default:
		buf.WriteString(describe(action.clubhouseDB, directory.MySQLConfig(action.cfg.Directory.ClubhouseDB), directory.ConnMaxLifetime))
	}
	return buf.String(), nil
}

func (action PerformGC) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.performGC(req)
	if errHTTP != nil {
		errHTTP.From("[performGC]").WriteResponse(w)
		return
	}
	herr.WriteOKResponse(w, fmt.Sprintf("Performed GC at %v", time.Now().Truncate(time.Millisecond)))
}

func (action PerformGC) performGC(req *http.Request) *herr.HTTPError {
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateDebugging == 0 {
		return herr.Forbidden("The requestor does not have GlobalAdministrateDebugging permission", nil)
	}
	runtime.GC()
	return nil
}
