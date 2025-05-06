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
	"database/sql"
	"errors"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/store"
	"net/http"
	"os"
	"strconv"
	"time"
)

type AttachToIncident struct{}
type AttachToFieldReport struct{}
type GetIncidentAttachment struct {
	imsDB              *store.DB
	localAttachmentDir *os.Root
	imsAdmins          []string
}

type GetFieldReportAttachment struct{}

func (action GetIncidentAttachment) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	event, _, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&authz.EventReadIncidents == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have EventReadIncidents permission on this Event", nil)
		return
	}
	ctx := req.Context()

	incidentNumber, err := strconv.ParseInt(req.PathValue("incidentNumber"), 10, 32)
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Failed to parse incident number", err)
		return
	}
	attachmentNumber64, err := strconv.ParseInt(req.PathValue("attachmentNumber"), 10, 32)
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Failed to parse attachment number", err)
		return
	}
	attachmentNumber := int32(attachmentNumber64)

	_, reportEntries, err := fetchIncident(ctx, action.imsDB, event.ID, int32(incidentNumber))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			handleErr(w, req, http.StatusNotFound, "No such incident", err)
			return
		}
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Incident", err)
		return
	}

	var filename string
	for _, reportEntry := range reportEntries {
		if reportEntry.ID == attachmentNumber {
			filename = reportEntry.AttachedFile.String
			break
		}
	}
	if filename == "" {
		handleErr(w, req, http.StatusNotFound, "No attachment for this ID", err)
		return
	}
	file, err := action.localAttachmentDir.Open(filename)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to open file", err)
		return
	}
	http.ServeContent(w, req, "some attachment", time.Now(), file)
}
