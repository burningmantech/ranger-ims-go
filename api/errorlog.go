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
	"net/http"
	"time"

	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
)

type GetErrorLogs struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

func (action GetErrorLogs) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getErrorLogs(req)
	if errHTTP != nil {
		errHTTP.From("[getErrorLogs]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}

func (action GetErrorLogs) getErrorLogs(req *http.Request) (imsjson.ErrorLogs, *herr.HTTPError) {
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return nil, errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateDebugging == 0 {
		return nil, herr.Forbidden("The requestor does not have GlobalAdministrateDebugging permission", nil)
	}

	// long ago
	minTime := 1e0
	// long from now
	maxTime := 1e100

	userName := req.FormValue("userName")
	path := req.FormValue("path")

	if req.FormValue("minTimeUnixMs") != "" {
		minTimeUnixMs, err := conv.ParseInt64(req.FormValue("minTimeUnixMs"))
		if err != nil {
			return nil, herr.BadRequest("minTimeUnixMs", err).From("[ParseInt64]")
		}
		minTime = float64(minTimeUnixMs) / 1e3
	}
	if req.FormValue("maxTimeUnixMs") != "" {
		maxTimeUnixMs, err := conv.ParseInt64(req.FormValue("maxTimeUnixMs"))
		if err != nil {
			return nil, herr.BadRequest("maxTimeUnixMs", err).From("[ParseInt64]")
		}
		maxTime = float64(maxTimeUnixMs) / 1e3
	}

	rows, err := action.imsDBQ.ErrorLogs(req.Context(), action.imsDBQ, imsdb.ErrorLogsParams{
		MinTime: minTime,
		MaxTime: maxTime,
	})
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch ErrorLogs", err).From("[ErrorLogs]")
	}

	resp := make(imsjson.ErrorLogs, 0)
	for _, row := range rows {
		el := row.ErrorLog
		if userName != "" && el.UserName.String != userName {
			continue
		}
		if path != "" && el.Path.String != path {
			continue
		}
		resp = append(resp, imsjson.ErrorLog{
			ID:              el.ID,
			CreatedAt:       conv.FloatToTime(el.CreatedAt),
			HttpStatus:      el.HttpStatus,
			ResponseMessage: el.ResponseMessage.String,
			InternalError:   el.InternalError.String,
			StackTrace:      el.StackTrace.String,
			Method:          el.Method.String,
			Path:            el.Path.String,
			Referrer:        el.Referrer.String,
			UserID:          el.UserID.Int64,
			UserName:        el.UserName.String,
			PositionID:      el.PositionID.Int64,
			PositionName:    el.PositionName.String,
			ClientAddress:   el.ClientAddress.String,
			Duration:        (time.Duration(el.DurationMicros.Int64) * time.Microsecond).String(),
		})
	}

	return resp, nil
}
