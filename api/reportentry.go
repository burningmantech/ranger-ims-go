package api

import (
	"fmt"
	"github.com/burningmantech/ranger-ims-go/auth"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"net/http"
	"time"
)

type EditFieldReportReportEntry struct {
	imsDB       *store.DB
	eventSource *EventSourcerer
	imsAdmins   []string
}

func (action EditFieldReportReportEntry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	event, jwtCtx, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&(auth.EventWriteAllFieldReports|auth.EventWriteOwnFieldReports) == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have permission to write Field Reports on this Event", nil)
		return
	}
	ctx := req.Context()

	author := jwtCtx.Claims.RangerHandle()

	var fieldReportNumber int32
	if isInt32(req.PathValue("fieldReportNumber")) {
		fieldReportNumber = toInt32(req.PathValue("fieldReportNumber"))
	} else {
		handleErr(w, req, http.StatusBadRequest, "Got a nonnumeric Field Report Number", nil)
		return
	}
	var reportEntryId int32
	if isInt32(req.PathValue("reportEntryId")) {
		reportEntryId = toInt32(req.PathValue("reportEntryId"))
	} else {
		handleErr(w, req, http.StatusBadRequest, "Got a nonnumeric Report Entry ID", nil)
		return
	}

	re, ok := mustReadBodyAs[imsjson.ReportEntry](w, req)
	if !ok {
		return
	}

	txn, err := action.imsDB.Begin()
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Error starting transaction", err)
		return
	}
	defer txn.Rollback()
	dbTxn := imsdb.New(txn)

	err = dbTxn.SetFieldReportReportEntryStricken(ctx, imsdb.SetFieldReportReportEntryStrickenParams{
		Stricken:          re.Stricken,
		Event:             event.ID,
		FieldReportNumber: fieldReportNumber,
		ReportEntry:       reportEntryId,
	})
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Error setting field report entry", err)
		return
	}
	struckVerb := "Struck"
	if !re.Stricken {
		struckVerb = "Unstruck"
	}
	err = addFRReportEntry(ctx, dbTxn, event.ID, fieldReportNumber, author, fmt.Sprintf("%v reportEntry %v", struckVerb, reportEntryId), true)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Error adding report entry", err)
		return
	}
	if err = txn.Commit(); err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Error committing transaction", err)
		return
	}

	defer action.eventSource.notifyFieldReportUpdate(event.Name, fieldReportNumber)

	http.Error(w, http.StatusText(http.StatusNoContent), http.StatusNoContent)
}

type EditIncidentReportEntry struct {
	imsDB       *store.DB
	eventSource *EventSourcerer
	imsAdmins   []string
}

func (action EditIncidentReportEntry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	event, jwtCtx, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&(auth.EventWriteIncidents) == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have permission to write Report Entries on this Event", nil)
		return
	}
	ctx := req.Context()

	author := jwtCtx.Claims.RangerHandle()

	var incidentNumber int32
	if isInt32(req.PathValue("incidentNumber")) {
		incidentNumber = toInt32(req.PathValue("incidentNumber"))
	} else {
		handleErr(w, req, http.StatusBadRequest, "Got a nonnumeric Incident Number", nil)
		return
	}
	var reportEntryId int32
	if isInt32(req.PathValue("reportEntryId")) {
		reportEntryId = toInt32(req.PathValue("reportEntryId"))
	} else {
		handleErr(w, req, http.StatusBadRequest, "Got a nonnumeric Report Entry ID", nil)
		return
	}

	re, ok := mustReadBodyAs[imsjson.ReportEntry](w, req)
	if !ok {
		return
	}

	txn, err := action.imsDB.Begin()
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Error starting transaction", err)
		return
	}
	defer txn.Rollback()
	dbTxn := imsdb.New(txn)

	err = dbTxn.SetIncidentReportEntryStricken(ctx, imsdb.SetIncidentReportEntryStrickenParams{
		Stricken:       re.Stricken,
		Event:          event.ID,
		IncidentNumber: incidentNumber,
		ReportEntry:    reportEntryId,
	})
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Error setting incident report entry", err)
		return
	}
	struckVerb := "Struck"
	if !re.Stricken {
		struckVerb = "Unstruck"
	}
	err = addIncidentReportEntry(ctx, dbTxn, event.ID, incidentNumber, author, fmt.Sprintf("%v reportEntry %v", struckVerb, reportEntryId), true)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Error adding report entry", err)
		return
	}
	if err = txn.Commit(); err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Error committing transaction", err)
		return
	}

	defer action.eventSource.notifyIncidentUpdate(event.Name, incidentNumber)

	http.Error(w, http.StatusText(http.StatusNoContent), http.StatusNoContent)
}

func reportEntryToJSON(re imsdb.ReportEntry) imsjson.ReportEntry {
	return imsjson.ReportEntry{
		ID:            re.ID,
		Created:       time.Unix(int64(re.Created), 0),
		Author:        re.Author,
		SystemEntry:   re.Generated,
		Text:          re.Text,
		Stricken:      re.Stricken,
		HasAttachment: re.AttachedFile.String != "",
	}
}
