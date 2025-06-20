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
	"fmt"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"log/slog"
	"net/http"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strings"
	"sync"
	"time"
)

type GetBuildInfo struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

type GetRuntimeMetrics struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
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
	w.Header().Set("Cache-Control", "no-cache")
	http.Error(w, bi.String(), http.StatusOK)
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
	w.Header().Set("Cache-Control", "no-cache")
	http.Error(w, metricsString, http.StatusOK)
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

func (action PerformGC) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.performGC(req)
	if errHTTP != nil {
		errHTTP.From("[performGC]").WriteResponse(w)
		return
	}
	http.Error(w, fmt.Sprintf("Performed GC at %v", time.Now().Truncate(time.Millisecond)), http.StatusOK)
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
