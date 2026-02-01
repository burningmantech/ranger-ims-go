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
	"time"

	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
)

type EditFieldReportReportEntry struct {
	imsDBQ      *store.DBQ
	userStore   *directory.UserStore
	eventSource *EventSourcerer
	imsAdmins   []string
}

func (action EditFieldReportReportEntry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.editFieldReportEntry(req)
	if errHTTP != nil {
		errHTTP.From("[editFieldReportEntry]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action EditFieldReportReportEntry) editFieldReportEntry(req *http.Request) *herr.HTTPError {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&(authz.EventWriteAllFieldReports|authz.EventWriteOwnFieldReports) == 0 {
		return herr.Forbidden("The requestor does not have permission to write Field Reports on this Event", nil)
	}
	ctx := req.Context()

	author := jwtCtx.Claims.RangerHandle()

	fieldReportNumber, err := conv.ParseInt32(req.PathValue("fieldReportNumber"))
	if err != nil {
		return herr.BadRequest("Failed to parse fieldReportNumber", err).From("[ParseInt32]")
	}
	reportEntryId, err := conv.ParseInt32(req.PathValue("reportEntryId"))
	if err != nil {
		return herr.BadRequest("Failed to parse reportEntryId", err).From("[ParseInt32]")
	}

	re, errHTTP := readBodyAs[imsjson.ReportEntry](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}

	_, err = action.imsDBQ.FieldReport(ctx, action.imsDBQ, imsdb.FieldReportParams{
		Event:  event.ID,
		Number: fieldReportNumber,
	})
	if err != nil {
		return herr.NotFound("There is no Field Report for the provided ID", err).From("[FieldReport]")
	}

	if re.Stricken == nil {
		// Nothing to do if no Stricken value is set, since Stricken is the only field this endpoint can modify
		return nil
	}

	txn, err := action.imsDBQ.Begin()
	if err != nil {
		return herr.InternalServerError("Error beginning transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	err = action.imsDBQ.SetFieldReportReportEntryStricken(ctx, txn,
		imsdb.SetFieldReportReportEntryStrickenParams{
			Stricken:          *re.Stricken,
			Event:             event.ID,
			FieldReportNumber: fieldReportNumber,
			ReportEntry:       reportEntryId,
		},
	)
	if err != nil {
		return herr.InternalServerError("Error setting field report entry", err).From("[SetFieldReportReportEntryStricken]")
	}
	struckVerb := "Struck"
	if !*re.Stricken {
		struckVerb = "Unstruck"
	}
	_, errHTTP = addFRReportEntry(ctx, action.imsDBQ, txn, event.ID, fieldReportNumber, author, fmt.Sprintf("%v reportEntry %v", struckVerb, reportEntryId), true, "", "", "")
	if errHTTP != nil {
		return errHTTP.From("[addFRReportEntry]")
	}
	err = txn.Commit()
	if err != nil {
		return herr.InternalServerError("Error committing transaction", err).From("[Commit]")
	}

	defer action.eventSource.notifyFieldReportUpdate(event.ID, fieldReportNumber)

	return nil
}

type EditIncidentReportEntry struct {
	imsDBQ      *store.DBQ
	userStore   *directory.UserStore
	eventSource *EventSourcerer
	imsAdmins   []string
}

func (action EditIncidentReportEntry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.editIncidentReportEntry(req)
	if errHTTP != nil {
		errHTTP.From("[editIncidentReportEntry]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action EditIncidentReportEntry) editIncidentReportEntry(req *http.Request) *herr.HTTPError {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&(authz.EventWriteIncidents) == 0 {
		return herr.Forbidden("The requestor does not have permission to write Report Entries on this Event", nil)
	}
	ctx := req.Context()

	author := jwtCtx.Claims.RangerHandle()

	incidentNumber, err := conv.ParseInt32(req.PathValue("incidentNumber"))
	if err != nil {
		return herr.BadRequest("Failed to parse incidentNumber", err).From("[ParseInt32]")
	}
	reportEntryId, err := conv.ParseInt32(req.PathValue("reportEntryId"))
	if err != nil {
		return herr.BadRequest("Failed to parse reportEntryId", err).From("[ParseInt32]")
	}

	re, errHTTP := readBodyAs[imsjson.ReportEntry](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}

	if re.Stricken == nil {
		// Nothing to do if no Stricken value is set, since Stricken is the only field this endpoint can modify
		return nil
	}

	txn, err := action.imsDBQ.Begin()
	if err != nil {
		return herr.InternalServerError("Error beginning transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	err = action.imsDBQ.SetIncidentReportEntryStricken(ctx, txn,
		imsdb.SetIncidentReportEntryStrickenParams{
			Stricken:       *re.Stricken,
			Event:          event.ID,
			IncidentNumber: incidentNumber,
			ReportEntry:    reportEntryId,
		},
	)
	if err != nil {
		return herr.InternalServerError("Error setting incident report entry", err).From("[SetIncidentReportEntryStricken]")
	}
	struckVerb := "Struck"
	if !*re.Stricken {
		struckVerb = "Unstruck"
	}
	_, errHTTP = addIncidentReportEntry(ctx, action.imsDBQ, txn, event.ID, incidentNumber, author, fmt.Sprintf("%v reportEntry %v", struckVerb, reportEntryId), true, "", "", "")
	if errHTTP != nil {
		return errHTTP.From("[addIncidentReportEntry]")
	}
	err = txn.Commit()
	if err != nil {
		return herr.InternalServerError("Error committing transaction", err).From("[Commit]")
	}

	defer action.eventSource.notifyIncidentUpdate(event.ID, incidentNumber)
	return nil
}

type EditStayReportEntry struct {
	imsDBQ      *store.DBQ
	userStore   *directory.UserStore
	eventSource *EventSourcerer
	imsAdmins   []string
}

func (action EditStayReportEntry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.editStayReportEntry(req)
	if errHTTP != nil {
		errHTTP.From("[editStayReportEntry]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action EditStayReportEntry) editStayReportEntry(req *http.Request) *herr.HTTPError {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteStays == 0 {
		return herr.Forbidden("The requestor does not have permission to write Stays on this Event", nil)
	}
	ctx := req.Context()

	author := jwtCtx.Claims.RangerHandle()

	stayNumber, err := conv.ParseInt32(req.PathValue("stayNumber"))
	if err != nil {
		return herr.BadRequest("Failed to parse stayNumber", err).From("[ParseInt32]")
	}
	reportEntryId, err := conv.ParseInt32(req.PathValue("reportEntryId"))
	if err != nil {
		return herr.BadRequest("Failed to parse reportEntryId", err).From("[ParseInt32]")
	}

	re, errHTTP := readBodyAs[imsjson.ReportEntry](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}

	_, err = action.imsDBQ.Stay(ctx, action.imsDBQ, imsdb.StayParams{
		Event:  event.ID,
		Number: stayNumber,
	})
	if err != nil {
		return herr.NotFound("There is no Stay for the provided ID", err).From("[Stay]")
	}

	if re.Stricken == nil {
		// Nothing to do if no Stricken value is set, since Stricken is the only field this endpoint can modify
		return nil
	}

	txn, err := action.imsDBQ.Begin()
	if err != nil {
		return herr.InternalServerError("Error beginning transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	err = action.imsDBQ.SetStayReportEntryStricken(ctx, txn,
		imsdb.SetStayReportEntryStrickenParams{
			Stricken:    *re.Stricken,
			Event:       event.ID,
			StayNumber:  stayNumber,
			ReportEntry: reportEntryId,
		},
	)
	if err != nil {
		return herr.InternalServerError("Error setting stay report entry", err).From("[SetStayReportEntryStricken]")
	}
	struckVerb := "Struck"
	if !*re.Stricken {
		struckVerb = "Unstruck"
	}
	_, errHTTP = addStayReportEntry(ctx, action.imsDBQ, txn, event.ID, stayNumber, author, fmt.Sprintf("%v reportEntry %v", struckVerb, reportEntryId), true, "", "", "")
	if errHTTP != nil {
		return errHTTP.From("[addStayReportEntry]")
	}
	err = txn.Commit()
	if err != nil {
		return herr.InternalServerError("Error committing transaction", err).From("[Commit]")
	}

	defer action.eventSource.notifyStayUpdate(event.ID, stayNumber)

	return nil
}

func reportEntryToJSON(re imsdb.ReportEntry, attachmentsEnabled bool) imsjson.ReportEntry {
	var attachment imsjson.Attachment
	if attachmentsEnabled && re.AttachedFileOriginalName.Valid {
		attachment.Name = re.AttachedFileOriginalName.String
		attachment.Previewable = previewableContentType(re.AttachedFileMediaType.String)
	}
	return imsjson.ReportEntry{
		ID:          re.ID,
		Created:     time.Unix(int64(re.Created), 0),
		Author:      re.Author,
		SystemEntry: re.Generated,
		Text:        re.Text,
		Stricken:    ptr(re.Stricken),
		Attachment:  attachment,
	}
}

func ptr[T any](s T) *T {
	return &s
}
