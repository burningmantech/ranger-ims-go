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
	imsDB              *store.DB
	imsAdmins          []string
	attachmentsEnabled bool
}

func (action GetFieldReports) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errH := action.getFieldReports(req)
	if errH != nil {
		errH.Src("[getFieldReports]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, resp)
}
func (action GetFieldReports) getFieldReports(req *http.Request) (imsjson.FieldReports, *herr.HTTPError) {
	resp := make(imsjson.FieldReports, 0)
	event, jwtCtx, eventPermissions, errH := mustGetEventPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return resp, errH.Src("[mustGetEventPermissions]")
	}
	if eventPermissions&(authz.EventReadAllFieldReports|authz.EventReadOwnFieldReports) == 0 {
		return resp, herr.S403("The requestor does not have permission to read Field Reports on this Event", nil)
	}
	// i.e. the user has EventReadOwnFieldReports, but not EventReadAllFieldReports
	limitedAccess := eventPermissions&authz.EventReadAllFieldReports == 0

	if err := req.ParseForm(); err != nil {
		return resp, herr.S400("Failed to parse form", err).Src("[ParseForm]")
	}
	generatedLTE := !strings.EqualFold("exclude_system_entries", "true") // false means to exclude

	reportEntries, err := imsdb.New(action.imsDB).FieldReports_ReportEntries(req.Context(),
		imsdb.FieldReports_ReportEntriesParams{
			Event:     event.ID,
			Generated: generatedLTE,
		})
	if err != nil {
		return resp, herr.S500("Failed to get FR report entries", err).Src("[FieldReports_ReportEntries]")
	}

	entriesByFR := make(map[int32][]imsdb.ReportEntry)
	for _, row := range reportEntries {
		entriesByFR[row.FieldReportNumber] = append(entriesByFR[row.FieldReportNumber], row.ReportEntry)
	}

	storedFRs, err := imsdb.New(action.imsDB).FieldReports(req.Context(), event.ID)
	if err != nil {
		return resp, herr.S500("Failed to fetch Field Reports", err).Src("[FieldReports]")
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

	entryJSONsByFR := make(map[int32][]imsjson.ReportEntry)
	for frNum, entries := range entriesByFR {
		for _, entry := range entries {
			entryJSONsByFR[frNum] = append(entryJSONsByFR[frNum], reportEntryToJSON(entry, action.attachmentsEnabled))
		}
	}

	resp = make(imsjson.FieldReports, 0, len(authorizedFRs))
	for _, fr := range authorizedFRs {
		resp = append(resp, imsjson.FieldReport{
			Event:         event.Name,
			Number:        fr.FieldReport.Number,
			Created:       time.Unix(int64(fr.FieldReport.Created), 0),
			Summary:       conv.StringOrNil(fr.FieldReport.Summary),
			Incident:      conv.Int32OrNil(fr.FieldReport.IncidentNumber),
			ReportEntries: entryJSONsByFR[fr.FieldReport.Number],
		})
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
	imsDB              *store.DB
	imsAdmins          []string
	attachmentsEnabled bool
}

func (action GetFieldReport) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errH := action.getFieldReport(req)
	if errH != nil {
		errH.Src("[getFieldReport]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, resp)
}

func (action GetFieldReport) getFieldReport(req *http.Request) (imsjson.FieldReport, *herr.HTTPError) {
	var response imsjson.FieldReport

	event, jwtCtx, eventPermissions, errH := mustGetEventPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return response, errH.Src("[mustGetEventPermissions]")
	}
	if eventPermissions&(authz.EventReadAllFieldReports|authz.EventReadOwnFieldReports) == 0 {
		return response, herr.S403("The requestor does not have permission to read Field Reports on this Event", nil)
	}
	// i.e. they have EventReadOwnFieldReports, but not EventReadAllFieldReports
	limitedAccess := eventPermissions&authz.EventReadAllFieldReports == 0

	ctx := req.Context()

	fieldReportNumber, err := conv.ParseInt32(req.PathValue("fieldReportNumber"))
	if err != nil {
		return response, herr.S400("Invalid field report number", err).Src("[ParseInt32]")
	}

	fr, reportEntries, errH := fetchFieldReport(ctx, action.imsDB, event.ID, fieldReportNumber)
	if errH != nil {
		return response, errH.Src("[fetchFieldReport]")
	}

	if limitedAccess {
		if !containsAuthor(reportEntries, jwtCtx.Claims.RangerHandle()) {
			return response, herr.S403("The requestor does not have permission to access this particular Field Report", nil)
		}
	}

	entries := make([]imsjson.ReportEntry, 0)
	for _, re := range reportEntries {
		entries = append(entries, reportEntryToJSON(re, action.attachmentsEnabled))
	}

	response = imsjson.FieldReport{
		Event:         event.Name,
		Number:        fr.Number,
		Created:       time.Unix(int64(fr.Created), 0),
		Summary:       conv.StringOrNil(fr.Summary),
		Incident:      conv.Int32OrNil(fr.IncidentNumber),
		ReportEntries: []imsjson.ReportEntry{},
	}
	response.ReportEntries = entries
	return response, nil
}

func fetchFieldReport(ctx context.Context, imsDB *store.DB, eventID, fieldReportNumber int32) (
	imsdb.FieldReport, []imsdb.ReportEntry, *herr.HTTPError,
) {
	frRow, err := imsdb.New(imsDB).FieldReport(ctx, imsdb.FieldReportParams{
		Event:  eventID,
		Number: fieldReportNumber,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return imsdb.FieldReport{}, nil, herr.S404("Field Report does not exist", err).Src("[FieldReport]")
		}
		return imsdb.FieldReport{}, nil, herr.S500("Failed to fetch Field Report", err).Src("[FieldReport]")
	}
	reportEntryRows, err := imsdb.New(imsDB).FieldReport_ReportEntries(ctx,
		imsdb.FieldReport_ReportEntriesParams{
			Event:             eventID,
			FieldReportNumber: fieldReportNumber,
		})
	if err != nil {
		return imsdb.FieldReport{}, nil, herr.S500("Failed to fetch Report Entries", err).Src("[FieldReport_ReportEntries]")
	}
	var reportEntries []imsdb.ReportEntry
	for _, rer := range reportEntryRows {
		reportEntries = append(reportEntries, rer.ReportEntry)
	}
	return frRow.FieldReport, reportEntries, nil
}

type EditFieldReport struct {
	imsDB       *store.DB
	eventSource *EventSourcerer
	imsAdmins   []string
}

func (action EditFieldReport) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errH := action.editFieldReport(req)
	if errH != nil {
		errH.Src("[editFieldReport]").WriteResponse(w)
		return
	}
	http.Error(w, "Success", http.StatusNoContent)
}
func (action EditFieldReport) editFieldReport(req *http.Request) *herr.HTTPError {
	event, jwt, eventPermissions, errH := mustGetEventPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return errH.Src("[mustGetEventPermissions]")
	}
	if eventPermissions&(authz.EventWriteAllFieldReports|authz.EventWriteOwnFieldReports) == 0 {
		return herr.S403("The requestor does not have permission to edit Field Reports on this Event", nil)
	}
	// i.e. they have EventWriteOwnFieldReports, but not EventWriteAllFieldReports
	limitedAccess := eventPermissions&authz.EventWriteAllFieldReports == 0

	ctx := req.Context()
	if err := req.ParseForm(); err != nil {
		return herr.S400("Failed to parse form data", err).Src("[ParseForm]")
	}
	fieldReportNumber, err := conv.ParseInt32(req.PathValue("fieldReportNumber"))
	if err != nil {
		return herr.S400("Invalid field report number", err).Src("[ParseInt32]")
	}
	author := jwt.Claims.RangerHandle()
	if limitedAccess {
		isPrevAuthor, errH := action.isPreviousAuthor(req, event.ID, fieldReportNumber, author)
		if errH != nil {
			return errH.Src("[isPreviousAuthor]")
		}
		if !isPrevAuthor {
			return herr.S403("The requestor does not have permission to edit this Field Report", nil)
		}
	}

	frr, err := imsdb.New(action.imsDB).FieldReport(ctx, imsdb.FieldReportParams{
		Event:  event.ID,
		Number: fieldReportNumber,
	})
	if err != nil {
		return herr.S500("Failed to fetch Field Report", err).Src("[FieldReport]")
	}
	storedFR := frr.FieldReport

	queryAction := req.FormValue("action")
	if queryAction != "" {
		previousIncident := storedFR.IncidentNumber

		var newIncident sql.NullInt32
		var entryText string
		switch queryAction {
		case "attach":
			num, err := conv.ParseInt32(req.FormValue("incident"))
			if err != nil {
				return herr.S400("Invalid incident number for attachment of FR", err).Src("[ParseInt32]")
			}
			newIncident = sql.NullInt32{Int32: num, Valid: true}
			entryText = fmt.Sprintf("Attached to incident: %v", num)
		case "detach":
			newIncident = sql.NullInt32{Valid: false}
			entryText = fmt.Sprintf("Detached from incident: %v", previousIncident.Int32)
		default:
			return herr.S400("Invalid action", fmt.Errorf("provided bad action was %v", queryAction))
		}
		err = imsdb.New(action.imsDB).AttachFieldReportToIncident(ctx, imsdb.AttachFieldReportToIncidentParams{
			IncidentNumber: newIncident,
			Event:          event.ID,
			Number:         fieldReportNumber,
		})
		if err != nil {
			return herr.S500("Failed to attach Field Report to incident", err).Src("[AttachFieldReportToIncident]")
		}
		_, errH := addFRReportEntry(ctx, imsdb.New(action.imsDB), event.ID, fieldReportNumber, author, entryText, true, "")
		if errH != nil {
			return errH.Src("[addFRReportEntry]")
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
	}

	requestFR, errH := mustReadBodyAs[imsjson.FieldReport](req)
	if errH != nil {
		return errH.Src("[readBodyAs2]")
	}
	// This is fine, as it may be that only an attach/detach was requested
	if requestFR.Number == 0 {
		slog.Debug("No field report number provided")
		return nil
	}

	txn, err := action.imsDB.Begin()
	if err != nil {
		return herr.S500("Failed to begin transaction", err).Src("[Begin]")
	}
	defer rollback(txn)
	dbTxn := imsdb.New(txn)

	if requestFR.Summary != nil {
		storedFR.Summary = sqlNullString(requestFR.Summary)
		text := "Changed summary to: " + *requestFR.Summary
		_, errH := addFRReportEntry(ctx, dbTxn, event.ID, storedFR.Number, author, text, true, "")
		if errH != nil {
			return errH.Src("[addFRReportEntry]")
		}
	}
	err = dbTxn.UpdateFieldReport(ctx, imsdb.UpdateFieldReportParams{
		Event:          storedFR.Event,
		Number:         storedFR.Number,
		Summary:        storedFR.Summary,
		IncidentNumber: storedFR.IncidentNumber,
	})
	if err != nil {
		return herr.S500("Failed to update Field Report", err).Src("[UpdateFieldReport]")
	}
	for _, entry := range requestFR.ReportEntries {
		if entry.Text == "" {
			continue
		}
		_, errH := addFRReportEntry(ctx, dbTxn, event.ID, storedFR.Number, author, entry.Text, false, "")
		if errH != nil {
			return errH.Src("[addFRReportEntry]")
		}
	}

	if err = txn.Commit(); err != nil {
		return herr.S500("Failed to commit transaction", err).Src("[Commit]")
	}

	defer action.eventSource.notifyFieldReportUpdate(event.Name, storedFR.Number)
	return nil
}

func (action EditFieldReport) isPreviousAuthor(
	req *http.Request,
	eventID int32,
	fieldReportNumber int32,
	author string,
) (isPreviousAuthor bool, errH *herr.HTTPError) {
	entries, err := imsdb.New(action.imsDB).FieldReport_ReportEntries(req.Context(),
		imsdb.FieldReport_ReportEntriesParams{
			Event:             eventID,
			FieldReportNumber: fieldReportNumber,
		},
	)
	if err != nil {
		return false, herr.S500("Failed to fetch Field Report ReportEntries", err).Src("[FieldReport_ReportEntries]")
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
	imsDB       *store.DB
	eventSource *EventSourcerer
	imsAdmins   []string
}

func (action NewFieldReport) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	number, location, errH := action.newFieldReport(req)
	if errH != nil {
		errH.Src("[newFieldReport]").WriteResponse(w)
		return
	}

	w.Header().Set("IMS-Field-Report-Number", strconv.Itoa(int(number)))
	w.Header().Set("Location", location)
	http.Error(w, http.StatusText(http.StatusCreated), http.StatusCreated)
}
func (action NewFieldReport) newFieldReport(req *http.Request) (incidentNumber int32, location string, errH *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errH := mustGetEventPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return 0, "", errH.Src("[mustGetEventPermissions]")
	}
	if eventPermissions&(authz.EventWriteAllFieldReports|authz.EventWriteOwnFieldReports) == 0 {
		return 0, "", herr.S403("The requestor does not have permission to write Field Reports on this Event", nil)
	}
	ctx := req.Context()

	fr, errH := mustReadBodyAs[imsjson.FieldReport](req)
	if errH != nil {
		return 0, "", errH.Src("[mustReadBodyAs]")
	}

	if fr.Incident != nil {
		return 0, "", herr.S400("A new Field Report may not be attached to an incident", nil)
	}

	author := jwtCtx.Claims.RangerHandle()
	newFrNum, err := imsdb.New(action.imsDB).NextFieldReportNumber(ctx, event.ID)
	if err != nil {
		return 0, "", herr.S500("Failed to find next Field Report number", err).Src("[NextFieldReportNumber]")
	}

	txn, err := action.imsDB.Begin()
	if err != nil {
		return 0, "", herr.S500("Failed to begin transaction", err).Src("[Begin]")
	}
	defer rollback(txn)
	dbTxn := imsdb.New(txn)

	err = dbTxn.CreateFieldReport(ctx, imsdb.CreateFieldReportParams{
		Event:          event.ID,
		Number:         newFrNum,
		Created:        float64(time.Now().Unix()),
		Summary:        sqlNullString(fr.Summary),
		IncidentNumber: sql.NullInt32{},
	})
	if err != nil {
		return 0, "", herr.S500("Failed to create Field Report", err).Src("[CreateFieldReport]")
	}

	if fr.Summary != nil {
		text := "Changed summary to: " + *fr.Summary
		_, errH := addFRReportEntry(ctx, dbTxn, event.ID, newFrNum, author, text, true, "")
		if errH != nil {
			return 0, "", errH.Src("[addFRReportEntry]")
		}
	}

	for _, entry := range fr.ReportEntries {
		if entry.Text == "" {
			continue
		}
		_, errH := addFRReportEntry(ctx, dbTxn, event.ID, newFrNum, author, entry.Text, false, "")
		if errH != nil {
			return 0, "", errH.Src("[addFRReportEntry]")
		}
	}

	if err = txn.Commit(); err != nil {
		return 0, "", herr.S500("Failed to commit transaction", err).Src("[Commit]")
	}

	loc := fmt.Sprintf("/ims/api/events/%v/field_reports/%v", event.Name, newFrNum)
	defer action.eventSource.notifyFieldReportUpdate(event.Name, newFrNum)
	return newFrNum, loc, nil
}

func addFRReportEntry(
	ctx context.Context, q *imsdb.Queries, eventID, frNum int32, author, text string, generated bool, attachment string,
) (int32, *herr.HTTPError) {
	reID64, err := q.CreateReportEntry(ctx, imsdb.CreateReportEntryParams{
		Author:       author,
		Text:         text,
		Created:      float64(time.Now().Unix()),
		Generated:    generated,
		Stricken:     false,
		AttachedFile: sqlNullString(&attachment),
	})
	// This column is an int32, so this is safe
	reID := conv.MustInt32(reID64)
	if err != nil {
		return 0, herr.S500("Failed to create report entry", err).Src("[CreateReportEntry]")
	}
	err = q.AttachReportEntryToFieldReport(ctx, imsdb.AttachReportEntryToFieldReportParams{
		Event:             eventID,
		FieldReportNumber: frNum,
		ReportEntry:       reID,
	})
	if err != nil {
		return 0, herr.S500("Failed to attach report entry", err).Src("[AttachReportEntryToFieldReport]")
	}
	return reID, nil
}

func sqlNullString(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}
