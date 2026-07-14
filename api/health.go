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
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
)

// readinessTimeout bounds the whole readiness probe, so that a hung
// dependency turns into a prompt 503 rather than a hung health check.
const readinessTimeout = 5 * time.Second

// GetReadiness reports whether this process is able to serve real traffic:
// the IMS database must be reachable with the schema version this binary
// expects, and the directory source must be answering. Load balancer and
// container health checks should target this endpoint, not the liveness one.
type GetReadiness struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
}

func (action GetReadiness) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx, cancel := context.WithTimeout(req.Context(), readinessTimeout)
	defer cancel()

	err := store.CheckSchemaCurrent(ctx, action.imsDBQ.DB)
	if err != nil {
		slog.Error("Readiness check failed on the IMS database", "err", err)
		http.Error(w, "IMS database is not ready", http.StatusServiceUnavailable)
		return
	}
	err = action.userStore.Ping(ctx)
	if err != nil {
		slog.Error("Readiness check failed on the directory source", "err", err)
		http.Error(w, "directory source is not ready", http.StatusServiceUnavailable)
		return
	}
	herr.WriteOKResponse(w, "ready")
}
