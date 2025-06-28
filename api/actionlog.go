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
	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"net/http"
	"time"
)

type GetActionLogs struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

func (action GetActionLogs) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getActionLogs(req)
	if errHTTP != nil {
		errHTTP.From("[getActionLogs]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}

func (action GetActionLogs) getActionLogs(req *http.Request) (imsjson.ActionLogs, *herr.HTTPError) {
	_, globalPermissions, errHTTP := getGlobalPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return nil, errHTTP.From("[getGlobalPermissions]")
	}
	if globalPermissions&authz.GlobalAdministrateDebugging == 0 {
		return nil, herr.Forbidden("The requestor does not have GlobalAdministrateDebugging permission", nil)
	}
	rows, err := action.imsDBQ.ActionLogs(req.Context(), action.imsDBQ, imsdb.ActionLogsParams{
		// long ago
		MinTime: 1e0,
		// long from now
		MaxTime: 1e100,
	})
	if err != nil {
		return nil, herr.InternalServerError("Failed to fetch ActionLogs", err).From("[ActionLogs]")
	}

	var resp imsjson.ActionLogs
	for _, row := range rows {
		al := row.ActionLog
		resp = append(resp, imsjson.ActionLog{
			ID:            al.ID,
			CreatedAt:     conv.FloatToTime(al.CreatedAt),
			ActionType:    al.ActionType,
			Method:        al.Method.String,
			Path:          al.Path.String,
			Referrer:      al.Referrer.String,
			UserID:        al.UserID.Int64,
			UserName:      al.UserName.String,
			PositionID:    al.PositionID.Int64,
			PositionName:  al.PositionName.String,
			ClientAddress: al.ClientAddress.String,
			HttpStatus:    al.HttpStatus.Int16,
			Duration:      (time.Duration(al.DurationMicros.Int64) * time.Microsecond).String(),
		})
	}

	return resp, nil
}
