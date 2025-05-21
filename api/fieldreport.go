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
	"database/sql"
	"errors"
	"fmt"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type GetFieldReports struct {
	imsDBQ             *store.DBQ
	imsAdmins          []string
	attachmentsEnabled bool
}

func (action GetFieldReports) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getFieldReports(req)
	if errHTTP != nil {
		errHTTP.From("[getFieldReports]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}
func (action GetFieldReports) getFieldReports(req *http.Request) (imsjson.FieldReports, *herr.HTTPError) {
	resp := make(imsjson.FieldReports, 0)
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.imsAdmins)
	if errHTTP != nil {
		return resp, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&(authz.EventReadAllFieldReports|authz.EventReadOwnFieldReports) == 0 {
		return resp, herr.Forbidden("The requestor does not have permission to read Field Reports on this Event", nil)
	}
	// i.e. the user has EventReadOwnFieldReports, but not EventReadAllFieldReports
	limitedAccess := eventPermissions&authz.EventReadAllFieldReports == 0

	if err := req.ParseForm(); err != nil {
		return resp, herr.BadRequest("Failed to parse form", err).From("[ParseForm]")
	}

	includeSystemEntries := !strings.EqualFold(req.Form.Get("exclude_system_entries"), "true")

	reportEntries, err := action.imsDBQ.FieldReports_ReportEntries(
		req.Context(),
		action.imsDBQ,
		imsdb.FieldReports_ReportEntriesParams{
			Event:     event.ID,
			Generated: includeSystemEntries,
		},
	)
	if err != nil {
		return resp, herr.InternalServerError("Failed to get FR report entries", err).From("[FieldReports_ReportEntries]")
	}

	entriesByFR := make(map[int32][]imsdb.ReportEntry)
	for _, row := range reportEntries {
		entriesByFR[row.FieldReportNumber] = append(entriesByFR[row.FieldReportNumber], row.ReportEntry)
	}

	storedFRs, err := action.imsDBQ.FieldReports(req.Context(), action.imsDBQ, event.ID)
	if err != nil {
		return resp, herr.InternalServerError("Failed to fetch Field Reports", err).From("[FieldReports]")
	}

	var authorizedFRs []imsdb.FieldReportsRow
	if limitedAccess {
		for _, storedFR := range storedFRs {
			entries := entriesByFR[storedFR.FieldReport.Number]
			if containsAuthor(entries, jwtCtx.Claims.RangerHandle()) {
				authorizedFRs = append(authorizedFRs, storedFR)
			}
		}
	} else {
		authorizedFRs = storedFRs
	}

	entryJSONsByFR := make(map[int32][]imsdb.ReportEntry)
	for frNum, entries := range entriesByFR {
		for _, entry := range entries {
			entryJSONsByFR[frNum] = append(entryJSONsByFR[frNum], entry)
		}
	}

	resp = make(imsjson.FieldReports, 0, len(authorizedFRs))
	for _, fr := range authorizedFRs {
		resp = append(
			resp,
			fieldReportToJSON(
				fr.FieldReport,
				entryJSONsByFR[fr.FieldReport.Number],
				event,
				action.attachmentsEnabled,
			),
		)
	}

	return resp, nil
}

func containsAuthor(entries []imsdb.ReportEntry, author string) bool {
	for _, e := range entries {
		if e.Author == author {
			return true
		}
	}
	return false
}

type GetFieldReport struct {
	imsDBQ             *store.DBQ
	imsAdmins          []string
	attachmentsEnabled bool
}

func (action GetFieldReport) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getFieldReport(req)
	if errHTTP != nil {
		errHTTP.From("[getFieldReport]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}

func (action GetFieldReport) getFieldReport(req *http.Request) (imsjson.FieldReport, *herr.HTTPError) {
	var response imsjson.FieldReport

	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.imsAdmins)
	if errHTTP != nil {
		return response, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&(authz.EventReadAllFieldReports|authz.EventReadOwnFieldReports) == 0 {
		return response, herr.Forbidden("The requestor does not have permission to read Field Reports on this Event", nil)
	}
	// i.e. they have EventReadOwnFieldReports, but not EventReadAllFieldReports
	limitedAccess := eventPermissions&authz.EventReadAllFieldReports == 0

	ctx := req.Context()

	fieldReportNumber, err := conv.ParseInt32(req.PathValue("fieldReportNumber"))
	if err != nil {
		return response, herr.BadRequest("Invalid field report number", err).From("[ParseInt32]")
	}

	fr, reportEntries, errHTTP := fetchFieldReport(ctx, action.imsDBQ, event.ID, fieldReportNumber)
	if errHTTP != nil {
		return response, errHTTP.From("[fetchFieldReport]")
	}

	if limitedAccess {
		if !containsAuthor(reportEntries, jwtCtx.Claims.RangerHandle()) {
			return response, herr.Forbidden("The requestor does not have permission to access this particular Field Report", nil)
		}
	}

	return fieldReportToJSON(fr, reportEntries, event, action.attachmentsEnabled), nil
}

func fieldReportToJSON(
	fr imsdb.FieldReport, reportEntries []imsdb.ReportEntry, event imsdb.Event, attachmentsEnabled bool,
) imsjson.FieldReport {
	entries := make([]imsjson.ReportEntry, 0)
	for _, re := range reportEntries {
		entries = append(entries, reportEntryToJSON(re, attachmentsEnabled))
	}
	return imsjson.FieldReport{
		Event:         event.Name,
		Number:        fr.Number,
		Created:       conv.Float64UnixSeconds(fr.Created),
		Summary:       conv.StringOrNil(fr.Summary),
		Incident:      conv.Int32OrNil(fr.IncidentNumber),
		ReportEntries: entries,
	}
}

func fetchFieldReport(ctx context.Context, imsDBQ *store.DBQ, eventID, fieldReportNumber int32) (
	imsdb.FieldReport, []imsdb.ReportEntry, *herr.HTTPError,
) {
	frRow, err := imsDBQ.FieldReport(ctx, imsDBQ,
		imsdb.FieldReportParams{
			Event:  eventID,
			Number: fieldReportNumber,
		},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return imsdb.FieldReport{}, nil, herr.NotFound("Field Report does not exist", err).From("[FieldReport]")
		}
		return imsdb.FieldReport{}, nil, herr.InternalServerError("Failed to fetch Field Report", err).From("[FieldReport]")
	}
	reportEntryRows, err := imsDBQ.FieldReport_ReportEntries(ctx, imsDBQ,
		imsdb.FieldReport_ReportEntriesParams{
			Event:             eventID,
			FieldReportNumber: fieldReportNumber,
		})
	if err != nil {
		return imsdb.FieldReport{}, nil, herr.InternalServerError("Failed to fetch Report Entries", err).From("[FieldReport_ReportEntries]")
	}
	var reportEntries []imsdb.ReportEntry
	for _, rer := range reportEntryRows {
		reportEntries = append(reportEntries, rer.ReportEntry)
	}
	return frRow.FieldReport, reportEntries, nil
}

type EditFieldReport struct {
	imsDBQ      *store.DBQ
	eventSource *EventSourcerer
	imsAdmins   []string
}

func (action EditFieldReport) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.editFieldReport(req)
	if errHTTP != nil {
		errHTTP.From("[editFieldReport]").WriteResponse(w)
		return
	}
	http.Error(w, "Success", http.StatusNoContent)
}
func (action EditFieldReport) editFieldReport(req *http.Request) *herr.HTTPError {
	event, jwt, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&(authz.EventWriteAllFieldReports|authz.EventWriteOwnFieldReports) == 0 {
		return herr.Forbidden("The requestor does not have permission to edit Field Reports on this Event", nil)
	}
	// i.e. they have EventWriteOwnFieldReports, but not EventWriteAllFieldReports
	limitedAccess := eventPermissions&authz.EventWriteAllFieldReports == 0

	ctx := req.Context()
	if err := req.ParseForm(); err != nil {
		return herr.BadRequest("Failed to parse form data", err).From("[ParseForm]")
	}
	fieldReportNumber, err := conv.ParseInt32(req.PathValue("fieldReportNumber"))
	if err != nil {
		return herr.BadRequest("Invalid field report number", err).From("[ParseInt32]")
	}
	author := jwt.Claims.RangerHandle()
	if limitedAccess {
		isPrevAuthor, errHTTP := action.isPreviousAuthor(req, event.ID, fieldReportNumber, author)
		if errHTTP != nil {
			return errHTTP.From("[isPreviousAuthor]")
		}
		if !isPrevAuthor {
			return herr.Forbidden("The requestor does not have permission to edit this Field Report", nil)
		}
	}

	frr, err := action.imsDBQ.FieldReport(ctx, action.imsDBQ,
		imsdb.FieldReportParams{
			Event:  event.ID,
			Number: fieldReportNumber,
		},
	)
	if err != nil {
		return herr.InternalServerError("Failed to fetch Field Report", err).From("[FieldReport]")
	}
	storedFR := frr.FieldReport

	// If there's an "action" in the form, we're either linking or unlinking this FR from an Incident.
	if queryAction := req.FormValue("action"); queryAction != "" {
		targetIncidentVal := req.FormValue("incident")
		errHTTP = action.handleLinkToIncident(ctx, storedFR, event, queryAction, targetIncidentVal, author)
		if errHTTP != nil {
			return errHTTP.From("[handleLinkToIncident]")
		}
	}

	requestFR, errHTTP := readBodyAs[imsjson.FieldReport](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}
	// This is fine, as it may be that only a link/unlink was requested
	if requestFR.Number == 0 {
		slog.Debug("No field report number provided")
		return nil
	}

	txn, err := action.imsDBQ.Begin()
	if err != nil {
		return herr.InternalServerError("Failed to begin transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	if requestFR.Summary != nil {
		storedFR.Summary = conv.ParseSqlNullString(requestFR.Summary)
		text := "Changed summary to: " + *requestFR.Summary
		_, errHTTP := addFRReportEntry(
			ctx, action.imsDBQ, txn, event.ID, storedFR.Number, author, text, true, "",
		)
		if errHTTP != nil {
			return errHTTP.From("[addFRReportEntry]")
		}
	}
	err = action.imsDBQ.UpdateFieldReport(ctx, txn,
		imsdb.UpdateFieldReportParams{
			Event:          storedFR.Event,
			Number:         storedFR.Number,
			Summary:        storedFR.Summary,
			IncidentNumber: storedFR.IncidentNumber,
		},
	)
	if err != nil {
		return herr.InternalServerError("Failed to update Field Report", err).From("[UpdateFieldReport]")
	}
	for _, entry := range requestFR.ReportEntries {
		if entry.Text == "" {
			continue
		}
		_, errHTTP := addFRReportEntry(
			ctx, action.imsDBQ, txn, event.ID, storedFR.Number, author, entry.Text, false, "",
		)
		if errHTTP != nil {
			return errHTTP.From("[addFRReportEntry]")
		}
	}

	if err = txn.Commit(); err != nil {
		return herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	defer action.eventSource.notifyFieldReportUpdate(event.Name, storedFR.Number)
	return nil
}

func (action EditFieldReport) handleLinkToIncident(
	ctx context.Context,
	storedFR imsdb.FieldReport,
	event imsdb.Event,
	queryAction string,
	targetIncidentVal string,
	actor string,
) *herr.HTTPError {
	previousIncident := storedFR.IncidentNumber
	fieldReportNumber := storedFR.Number

	var newIncident sql.NullInt32
	var entryText string
	switch queryAction {
	case "attach":
		num, err := conv.ParseInt32(targetIncidentVal)
		if err != nil {
			return herr.BadRequest("Invalid incident number for attachment of FR", err).From("[ParseInt32]")
		}
		newIncident = sql.NullInt32{Int32: num, Valid: true}
		entryText = fmt.Sprintf("Attached to incident: %v", num)
	case "detach":
		newIncident = sql.NullInt32{Valid: false}
		entryText = fmt.Sprintf("Detached from incident: %v", previousIncident.Int32)
	default:
		return herr.BadRequest("Invalid action", fmt.Errorf("provided bad action was %v", queryAction))
	}
	err := action.imsDBQ.AttachFieldReportToIncident(ctx, action.imsDBQ,
		imsdb.AttachFieldReportToIncidentParams{
			IncidentNumber: newIncident,
			Event:          event.ID,
			Number:         fieldReportNumber,
		},
	)
	if err != nil {
		return herr.InternalServerError("Failed to attach Field Report to incident", err).From("[AttachFieldReportToIncident]")
	}
	_, errHTTP := addFRReportEntry(
		ctx, action.imsDBQ, action.imsDBQ, event.ID, fieldReportNumber,
		actor, entryText, true, "",
	)
	if errHTTP != nil {
		return errHTTP.From("[addFRReportEntry]")
	}
	defer action.eventSource.notifyFieldReportUpdate(event.Name, fieldReportNumber)
	defer action.eventSource.notifyIncidentUpdate(event.Name, previousIncident.Int32)
	defer action.eventSource.notifyIncidentUpdate(event.Name, newIncident.Int32)
	slog.Info("Attached Field Report to newIncident",
		"event", event.ID,
		"newIncident", newIncident.Int32,
		"previousIncident", previousIncident.Int32,
		"field report", fieldReportNumber,
	)
	return nil
}

func (action EditFieldReport) isPreviousAuthor(
	req *http.Request,
	eventID int32,
	fieldReportNumber int32,
	author string,
) (isPreviousAuthor bool, errHTTP *herr.HTTPError) {
	entries, err := action.imsDBQ.FieldReport_ReportEntries(req.Context(), action.imsDBQ,
		imsdb.FieldReport_ReportEntriesParams{
			Event:             eventID,
			FieldReportNumber: fieldReportNumber,
		},
	)
	if err != nil {
		return false, herr.InternalServerError("Failed to fetch Field Report ReportEntries", err).From("[FieldReport_ReportEntries]")
	}
	authorMatch := false
	for _, entry := range entries {
		if entry.ReportEntry.Author == author {
			authorMatch = true
			break
		}
	}
	return authorMatch, nil
}

type NewFieldReport struct {
	imsDBQ      *store.DBQ
	eventSource *EventSourcerer
	imsAdmins   []string
}

func (action NewFieldReport) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	number, location, errHTTP := action.newFieldReport(req)
	if errHTTP != nil {
		errHTTP.From("[newFieldReport]").WriteResponse(w)
		return
	}

	w.Header().Set("IMS-Field-Report-Number", strconv.Itoa(int(number)))
	w.Header().Set("Location", location)
	http.Error(w, http.StatusText(http.StatusCreated), http.StatusCreated)
}

func (action NewFieldReport) newFieldReport(req *http.Request) (incidentNumber int32, location string, errHTTP *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.imsAdmins)
	if errHTTP != nil {
		return 0, "", errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&(authz.EventWriteAllFieldReports|authz.EventWriteOwnFieldReports) == 0 {
		return 0, "", herr.Forbidden("The requestor does not have permission to write Field Reports on this Event", nil)
	}
	ctx := req.Context()

	fr, errHTTP := readBodyAs[imsjson.FieldReport](req)
	if errHTTP != nil {
		return 0, "", errHTTP.From("[readBodyAs]")
	}

	if fr.Incident != nil {
		return 0, "", herr.BadRequest("A new Field Report may not be attached to an incident", nil)
	}

	author := jwtCtx.Claims.RangerHandle()

	newFrNum, err := action.imsDBQ.NextFieldReportNumber(ctx, action.imsDBQ, event.ID)
	if err != nil {
		return 0, "", herr.InternalServerError("Failed to find next Field Report number", err).From("[NextFieldReportNumber]")
	}
	fr.Number = newFrNum

	err = action.imsDBQ.CreateFieldReport(ctx, action.imsDBQ,
		imsdb.CreateFieldReportParams{
			Event:          event.ID,
			Number:         newFrNum,
			Created:        conv.TimeFloat64(time.Now()),
			Summary:        conv.ParseSqlNullString(fr.Summary),
			IncidentNumber: sql.NullInt32{},
		},
	)
	if err != nil {
		return 0, "", herr.InternalServerError("Failed to create Field Report", err).From("[CreateFieldReport]")
	}

	txn, err := action.imsDBQ.Begin()
	if err != nil {
		return 0, "", herr.InternalServerError("Failed to begin transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	if fr.Summary != nil {
		text := "Changed summary to: " + *fr.Summary
		_, errHTTP := addFRReportEntry(ctx, action.imsDBQ, txn, event.ID, fr.Number, author, text, true, "")
		if errHTTP != nil {
			return 0, "", errHTTP.From("[addFRReportEntry]")
		}
	}

	for _, entry := range fr.ReportEntries {
		if entry.Text == "" {
			continue
		}
		_, errHTTP := addFRReportEntry(ctx, action.imsDBQ, txn, event.ID, fr.Number, author, entry.Text, false, "")
		if errHTTP != nil {
			return 0, "", errHTTP.From("[addFRReportEntry]")
		}
	}

	if err = txn.Commit(); err != nil {
		return 0, "", herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	loc := fmt.Sprintf("/ims/api/events/%v/field_reports/%v", event.Name, fr.Number)
	defer action.eventSource.notifyFieldReportUpdate(event.Name, fr.Number)
	return fr.Number, loc, nil
}

func addFRReportEntry(
	ctx context.Context, imsDBQ *store.DBQ, dbtx imsdb.DBTX, eventID, frNum int32, author, text string, generated bool, attachment string,
) (int32, *herr.HTTPError) {
	reID64, err := imsDBQ.CreateReportEntry(ctx,
		dbtx,
		imsdb.CreateReportEntryParams{
			Author:       author,
			Text:         text,
			Created:      conv.TimeFloat64(time.Now()),
			Generated:    generated,
			Stricken:     false,
			AttachedFile: conv.ParseSqlNullString(&attachment),
		},
	)
	// This column is an int32, so this is safe
	reID := conv.MustInt32(reID64)
	if err != nil {
		return 0, herr.InternalServerError("Failed to create report entry", err).From("[CreateReportEntry]")
	}
	err = imsDBQ.AttachReportEntryToFieldReport(ctx, dbtx,
		imsdb.AttachReportEntryToFieldReportParams{
			Event:             eventID,
			FieldReportNumber: frNum,
			ReportEntry:       reID,
		},
	)
	if err != nil {
		return 0, herr.InternalServerError("Failed to attach report entry", err).From("[AttachReportEntryToFieldReport]")
	}
	return reID, nil
}
