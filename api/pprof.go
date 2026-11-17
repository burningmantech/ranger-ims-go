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
	"fmt"
	"net/http"
	"net/http/pprof"
	"strconv"

	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
)

// Importing net/http/pprof registers its handlers on http.DefaultServeMux.
// That's harmless only because IMS never serves the default mux, which the
// forbidigo rules in .golangci.yml enforce. These endpoints are the only way
// to reach pprof.

// PprofProfiles are the profiles served under /ims/api/debug/pprof/. "profile"
// is the CPU profile and "trace" the execution trace; the rest are
// runtime/pprof's named profiles. "block" and "mutex" stay empty unless their
// sampling rates get set, which IMS doesn't do.
var PprofProfiles = []string{
	"profile",
	"trace",
	"heap",
	"allocs",
	"goroutine",
	"block",
	"mutex",
	"threadcreate",
}

// maxPprofSeconds caps the "seconds" parameter, so that no one request can
// hold the CPU profiler or tracer for long.
const maxPprofSeconds = 60

// GetPprof serves one pprof profile, only to holders of
// GlobalAdministrateDebugging.
type GetPprof struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
	profile   string
}

func (action GetPprof) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.validate(req)
	if errHTTP != nil {
		errHTTP.From("[validate]").WriteResponse(w)
		return
	}
	switch action.profile {
	case "profile":
		pprof.Profile(w, req)
	case "trace":
		pprof.Trace(w, req)
	default:
		pprof.Handler(action.profile).ServeHTTP(w, req)
	}
}

func (action GetPprof) validate(req *http.Request) *herr.HTTPError {
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateDebugging == 0 {
		return herr.Forbidden("The requestor does not have GlobalAdministrateDebugging permission", nil)
	}
	if s := req.FormValue("seconds"); s != "" {
		seconds, err := strconv.Atoi(s)
		if err != nil || seconds > maxPprofSeconds {
			return herr.BadRequest(fmt.Sprintf("seconds must be an integer no greater than %v", maxPprofSeconds), err)
		}
	}
	return nil
}
