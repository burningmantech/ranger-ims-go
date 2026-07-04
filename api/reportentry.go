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

// newReportEntry describes a report entry to be created and attached to an
// Incident, Field Report, or Visit.
type newReportEntry struct {
	author    string
	text      string
	generated bool

	// These fields are set only when the entry records a file attachment.
	attachedFile             string
	attachedFileOriginalName string
	attachedFileMediaType    string
}

// createReportEntry inserts a ReportEntry row and returns its ID. The caller must
// separately associate the entry with an Incident, Field Report, or Visit; see
// addIncidentReportEntry, addFRReportEntry, and addVisitReportEntry.
func createReportEntry(
	ctx context.Context, db *store.DBQ, dbtx imsdb.DBTX, entry newReportEntry,
) (int32, *herr.HTTPError) {
	reID64, err := db.CreateReportEntry(ctx, dbtx, imsdb.CreateReportEntryParams{
		Author:                   entry.author,
		Text:                     entry.text,
		Created:                  conv.TimeToFloat(time.Now()),
		Generated:                entry.generated,
		Stricken:                 false,
		AttachedFile:             conv.StringToSql(&entry.attachedFile, 128),
		AttachedFileOriginalName: conv.StringToSql(&entry.attachedFileOriginalName, 128),
		AttachedFileMediaType:    conv.StringToSql(&entry.attachedFileMediaType, 128),
	})
	if err != nil {
		return 0, herr.InternalServerError("Failed to create report entry", err).From("[CreateReportEntry]")
	}
	// This column is an int32, so this is safe
	return conv.MustInt32(reID64), nil
}

func addIncidentReportEntry(
	ctx context.Context, db *store.DBQ, dbtx imsdb.DBTX,
	eventID, incidentNum int32, entry newReportEntry,
) (int32, *herr.HTTPError) {
	reID, errHTTP := createReportEntry(ctx, db, dbtx, entry)
	if errHTTP != nil {
		return 0, errHTTP.From("[createReportEntry]")
	}
	err := db.AttachReportEntryToIncident(ctx, dbtx, imsdb.AttachReportEntryToIncidentParams{
		Event:          eventID,
		IncidentNumber: incidentNum,
		ReportEntry:    reID,
	})
	if err != nil {
		return 0, herr.InternalServerError("Failed to attach report entry", err).From("[AttachReportEntryToIncident]")
	}
	return reID, nil
}

func addFRReportEntry(
	ctx context.Context, db *store.DBQ, dbtx imsdb.DBTX,
	eventID, frNum int32, entry newReportEntry,
) (int32, *herr.HTTPError) {
	reID, errHTTP := createReportEntry(ctx, db, dbtx, entry)
	if errHTTP != nil {
		return 0, errHTTP.From("[createReportEntry]")
	}
	err := db.AttachReportEntryToFieldReport(ctx, dbtx, imsdb.AttachReportEntryToFieldReportParams{
		Event:             eventID,
		FieldReportNumber: frNum,
		ReportEntry:       reID,
	})
	if err != nil {
		return 0, herr.InternalServerError("Failed to attach report entry", err).From("[AttachReportEntryToFieldReport]")
	}
	return reID, nil
}

func addVisitReportEntry(
	ctx context.Context, db *store.DBQ, dbtx imsdb.DBTX,
	eventID, visitNum int32, entry newReportEntry,
) (int32, *herr.HTTPError) {
	reID, errHTTP := createReportEntry(ctx, db, dbtx, entry)
	if errHTTP != nil {
		return 0, errHTTP.From("[createReportEntry]")
	}
	err := db.AttachReportEntryToVisit(ctx, dbtx, imsdb.AttachReportEntryToVisitParams{
		Event:       eventID,
		VisitNumber: visitNum,
		ReportEntry: reID,
	})
	if err != nil {
		return 0, herr.InternalServerError("Failed to attach report entry", err).From("[AttachReportEntryToVisit]")
	}
	return reID, nil
}

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
	_, errHTTP = addFRReportEntry(ctx, action.imsDBQ, txn, event.ID, fieldReportNumber, newReportEntry{
		author:    author,
		text:      fmt.Sprintf("%v reportEntry %v", struckVerb, reportEntryId),
		generated: true,
	})
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
	_, errHTTP = addIncidentReportEntry(ctx, action.imsDBQ, txn, event.ID, incidentNumber, newReportEntry{
		author:    author,
		text:      fmt.Sprintf("%v reportEntry %v", struckVerb, reportEntryId),
		generated: true,
	})
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

type EditVisitReportEntry struct {
	imsDBQ      *store.DBQ
	userStore   *directory.UserStore
	eventSource *EventSourcerer
	imsAdmins   []string
}

func (action EditVisitReportEntry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.editVisitReportEntry(req)
	if errHTTP != nil {
		errHTTP.From("[editVisitReportEntry]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action EditVisitReportEntry) editVisitReportEntry(req *http.Request) *herr.HTTPError {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteVisits == 0 {
		return herr.Forbidden("The requestor does not have permission to write Visits on this Event", nil)
	}
	ctx := req.Context()

	author := jwtCtx.Claims.RangerHandle()

	visitNumber, err := conv.ParseInt32(req.PathValue("visitNumber"))
	if err != nil {
		return herr.BadRequest("Failed to parse visitNumber", err).From("[ParseInt32]")
	}
	reportEntryId, err := conv.ParseInt32(req.PathValue("reportEntryId"))
	if err != nil {
		return herr.BadRequest("Failed to parse reportEntryId", err).From("[ParseInt32]")
	}

	re, errHTTP := readBodyAs[imsjson.ReportEntry](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}

	_, err = action.imsDBQ.Visit(ctx, action.imsDBQ, imsdb.VisitParams{
		Event:  event.ID,
		Number: visitNumber,
	})
	if err != nil {
		return herr.NotFound("There is no Visit for the provided ID", err).From("[Visit]")
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

	err = action.imsDBQ.SetVisitReportEntryStricken(ctx, txn,
		imsdb.SetVisitReportEntryStrickenParams{
			Stricken:    *re.Stricken,
			Event:       event.ID,
			VisitNumber: visitNumber,
			ReportEntry: reportEntryId,
		},
	)
	if err != nil {
		return herr.InternalServerError("Error setting visit report entry", err).From("[SetVisitReportEntryStricken]")
	}
	struckVerb := "Struck"
	if !*re.Stricken {
		struckVerb = "Unstruck"
	}
	_, errHTTP = addVisitReportEntry(ctx, action.imsDBQ, txn, event.ID, visitNumber, newReportEntry{
		author:    author,
		text:      fmt.Sprintf("%v reportEntry %v", struckVerb, reportEntryId),
		generated: true,
	})
	if errHTTP != nil {
		return errHTTP.From("[addVisitReportEntry]")
	}
	err = txn.Commit()
	if err != nil {
		return herr.InternalServerError("Error committing transaction", err).From("[Commit]")
	}

	defer action.eventSource.notifyVisitUpdate(event.ID, visitNumber)

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
		Stricken:    new(re.Stricken),
		Attachment:  attachment,
	}
}
