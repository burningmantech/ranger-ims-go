package api

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/auth"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

type GetFieldReports struct {
	imsDB     *store.DB
	imsAdmins []string
}

func (action GetFieldReports) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var resp imsjson.FieldReports
	event, jwtCtx, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&(auth.EventReadAllFieldReports|auth.EventReadOwnFieldReports) == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have permission to read Field Reports on this Event", nil)
		return
	}
	// i.e. the user has EventReadOwnFieldReports, but not EventReadAllFieldReports
	limitedAccess := eventPermissions&auth.EventReadAllFieldReports == 0

	if ok = mustParseForm(w, req); !ok {
		return
	}
	generatedLTE := req.Form.Get("exclude_system_entries") != "true" // false means to exclude

	reportEntries, err := imsdb.New(action.imsDB).FieldReports_ReportEntries(req.Context(),
		imsdb.FieldReports_ReportEntriesParams{
			Event:     event.ID,
			Generated: generatedLTE,
		})
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to get FR report entries", err)
		return
	}

	entriesByFR := make(map[int32][]imsjson.ReportEntry)
	for _, row := range reportEntries {
		re := row.ReportEntry
		entriesByFR[row.FieldReportNumber] = append(entriesByFR[row.FieldReportNumber], imsjson.ReportEntry{
			ID:            re.ID,
			Created:       time.Unix(int64(re.Created), 0),
			Author:        re.Author,
			SystemEntry:   re.Generated,
			Text:          re.Text,
			Stricken:      re.Stricken,
			HasAttachment: re.AttachedFile.String != "",
		})
	}

	storedFRs, err := imsdb.New(action.imsDB).FieldReports(req.Context(), event.ID)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Field Reports", err)
		return
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

	resp = make(imsjson.FieldReports, 0, len(authorizedFRs))
	for _, fr := range authorizedFRs {
		resp = append(resp, imsjson.FieldReport{
			Event:         event.Name,
			Number:        fr.FieldReport.Number,
			Created:       time.Unix(int64(fr.FieldReport.Created), 0),
			Summary:       stringOrNil(fr.FieldReport.Summary),
			Incident:      fr.FieldReport.IncidentNumber.Int32,
			ReportEntries: entriesByFR[fr.FieldReport.Number],
		})
	}

	mustWriteJSON(w, resp)
}

func containsAuthor(entries []imsjson.ReportEntry, author string) bool {
	for _, e := range entries {
		if e.Author == author {
			return true
		}
	}
	return false
}

type GetFieldReport struct {
	imsDB     *store.DB
	imsAdmins []string
}

func (action GetFieldReport) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var response imsjson.FieldReport

	event, jwtCtx, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&(auth.EventReadAllFieldReports|auth.EventReadOwnFieldReports) == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have permission to read Field Reports on this Event", nil)
		return
	}
	// i.e. they have EventReadOwnFieldReports, but not EventReadAllFieldReports
	limitedAccess := eventPermissions&auth.EventReadAllFieldReports == 0

	ctx := req.Context()

	fieldReportNumber, err := strconv.ParseInt(req.PathValue("fieldReportNumber"), 10, 32)
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Invalid field report number", err)
		return
	}

	reportEntryRows, err := imsdb.New(action.imsDB).FieldReport_ReportEntries(ctx,
		imsdb.FieldReport_ReportEntriesParams{
			Event:             event.ID,
			FieldReportNumber: int32(fieldReportNumber),
		})
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to get FR report entries", err)
		return
	}
	entries := make([]imsjson.ReportEntry, 0)
	for _, rer := range reportEntryRows {
		re := rer.ReportEntry
		entries = append(entries, imsjson.ReportEntry{
			ID:            re.ID,
			Created:       time.Unix(int64(re.Created), 0),
			Author:        re.Author,
			SystemEntry:   re.Generated,
			Text:          re.Text,
			Stricken:      re.Stricken,
			HasAttachment: re.AttachedFile.String != "",
		})
	}
	if limitedAccess {
		if !containsAuthor(entries, jwtCtx.Claims.RangerHandle()) {
			handleErr(w, req, http.StatusForbidden, "The requestor does not have permission to read this Field Report", nil)
			return
		}
	}

	frRow, err := imsdb.New(action.imsDB).FieldReport(ctx, imsdb.FieldReportParams{
		Event:  event.ID,
		Number: int32(fieldReportNumber),
	})
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Field Report", err)
		return
	}
	fr := frRow.FieldReport

	response = imsjson.FieldReport{
		Event:         event.Name,
		Number:        fr.Number,
		Created:       time.Unix(int64(fr.Created), 0),
		Summary:       stringOrNil(fr.Summary),
		Incident:      fr.IncidentNumber.Int32,
		ReportEntries: []imsjson.ReportEntry{},
	}
	response.ReportEntries = entries
	mustWriteJSON(w, response)
}

type EditFieldReport struct {
	imsDB       *store.DB
	eventSource *EventSourcerer
	imsAdmins   []string
}

func (action EditFieldReport) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	event, jwt, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&(auth.EventWriteAllFieldReports|auth.EventWriteOwnFieldReports) == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have permission to edit Field Reports on this Event", nil)
		return
	}
	// i.e. they have EventWriteOwnFieldReports, but not EventWriteAllFieldReports
	limitedAccess := eventPermissions&auth.EventWriteAllFieldReports == 0

	ctx := req.Context()
	if ok = mustParseForm(w, req); !ok {
		return
	}
	fieldReportNumber64, err := strconv.ParseInt(req.PathValue("fieldReportNumber"), 10, 32)
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Invalid field report number", err)
		return
	}
	fieldReportNumber := int32(fieldReportNumber64)
	author := jwt.Claims.RangerHandle()
	if limitedAccess {
		if ok = action.mustCheckIfPreviousAuthor(w, req, event.ID, fieldReportNumber, author); !ok {
			return
		}
	}

	frr, err := imsdb.New(action.imsDB).FieldReport(ctx, imsdb.FieldReportParams{
		Event:  event.ID,
		Number: fieldReportNumber,
	})
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Field Report", err)
		return
	}
	storedFR := frr.FieldReport

	queryAction := req.FormValue("action")
	if queryAction != "" {
		previousIncident := storedFR.IncidentNumber

		var newIncident sql.NullInt32
		entryText := ""
		switch queryAction {
		case "attach":
			num, err := strconv.ParseInt(req.FormValue("incident"), 10, 32)
			if err != nil {
				handleErr(w, req, http.StatusBadRequest, "Invalid incident number for attachment of FR", err)
				return
			}
			newIncident = sql.NullInt32{Int32: int32(num), Valid: true}
			entryText = fmt.Sprintf("Attached to incident: %v", num)
		case "detach":
			newIncident = sql.NullInt32{Valid: false}
			entryText = fmt.Sprintf("Detached from incident: %v", previousIncident.Int32)
		default:
			handleErr(w, req, http.StatusBadRequest, "Invalid action", fmt.Errorf("provided bad action was %v", queryAction))
			return
		}
		err = imsdb.New(action.imsDB).AttachFieldReportToIncident(ctx, imsdb.AttachFieldReportToIncidentParams{
			IncidentNumber: newIncident,
			Event:          event.ID,
			Number:         fieldReportNumber,
		})
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to attach Field Report to Incident", err)
			return
		}
		err = addFRReportEntry(ctx, imsdb.New(action.imsDB), event.ID, fieldReportNumber, author, entryText, true)
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to attach Report Entry to Field Report", err)
			return
		}
		defer action.eventSource.notifyFieldReportUpdate(event.Name, fieldReportNumber)
		defer action.eventSource.notifyIncidentUpdate(event.Name, previousIncident.Int32)
		defer action.eventSource.notifyIncidentUpdate(event.Name, newIncident.Int32)
		slog.Info("Attached Field Report to newIncident", "event", event.ID, "newIncident", newIncident.Int32, "previousIncident", previousIncident.Int32, "field report", fieldReportNumber)
	}

	requestFR, ok := mustReadBodyAs[imsjson.FieldReport](w, req)
	if !ok {
		return
	}
	// This is fine, as it may be that only an attach/detach was requested
	if requestFR.Number == 0 {
		slog.Debug("No field report number provided")
		http.Error(w, "OK", http.StatusNoContent)
		return
	}

	txn, err := action.imsDB.Begin()
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to start transaction", err)
		return
	}
	defer txn.Rollback()
	dbTxn := imsdb.New(txn)

	if requestFR.Summary != nil {
		storedFR.Summary = sqlNullString(requestFR.Summary)
		text := "Changed summary to: " + *requestFR.Summary
		err = addFRReportEntry(ctx, dbTxn, event.ID, storedFR.Number, author, text, true)
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Error adding system Field Report Report Entry", err)
			return
		}
	}
	err = dbTxn.UpdateFieldReport(ctx, imsdb.UpdateFieldReportParams{
		Event:          storedFR.Event,
		Number:         storedFR.Number,
		Summary:        storedFR.Summary,
		IncidentNumber: storedFR.IncidentNumber,
	})
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to update Field Report", err)
		return
	}
	for _, entry := range requestFR.ReportEntries {
		if entry.Text == "" {
			continue
		}
		err = addFRReportEntry(ctx, dbTxn, event.ID, storedFR.Number, author, entry.Text, false)
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Error adding Field Report Report Entry", err)
			return
		}
	}

	if err = txn.Commit(); err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to commit transaction", err)
		return
	}

	defer action.eventSource.notifyFieldReportUpdate(event.Name, storedFR.Number)

	http.Error(w, "Success", http.StatusNoContent)
}

func (action EditFieldReport) mustCheckIfPreviousAuthor(
	w http.ResponseWriter,
	req *http.Request,
	eventID int32,
	fieldReportNumber int32,
	author string,
) (isPreviousAuthor bool) {
	entries, err := imsdb.New(action.imsDB).FieldReport_ReportEntries(req.Context(),
		imsdb.FieldReport_ReportEntriesParams{
			Event:             eventID,
			FieldReportNumber: fieldReportNumber,
		})
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Field Report Report Entries", err)
		return false
	}
	authorMatch := false
	for _, entry := range entries {
		if entry.ReportEntry.Author == author {
			authorMatch = true
			break
		}
	}
	if !authorMatch {
		handleErr(w, req, http.StatusForbidden, "EditFieldReport denied to user who is not a previous author on this FieldReport", nil)
		return false
	}
	return true
}

type NewFieldReport struct {
	imsDB       *store.DB
	eventSource *EventSourcerer
	imsAdmins   []string
}

func (action NewFieldReport) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	event, jwtCtx, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&(auth.EventWriteAllFieldReports|auth.EventWriteOwnFieldReports) == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have permission to write Field Reports on this Event", nil)
		return
	}
	ctx := req.Context()

	fr, ok := mustReadBodyAs[imsjson.FieldReport](w, req)
	if !ok {
		return
	}

	if fr.Incident != 0 {
		handleErr(w, req, http.StatusBadRequest, "A new Field Report may not be attached to an incident", nil)
		return
	}

	author := jwtCtx.Claims.RangerHandle()
	numUntyped, err := imsdb.New(action.imsDB).MaxFieldReportNumber(ctx, event.ID)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to find next Field Report number", err)
		return
	}
	newFrNum := numUntyped.(int64) + 1

	txn, err := action.imsDB.Begin()
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to start transaction", err)
		return
	}
	defer txn.Rollback()
	dbTxn := imsdb.New(txn)

	err = dbTxn.CreateFieldReport(ctx, imsdb.CreateFieldReportParams{
		Event:          event.ID,
		Number:         int32(newFrNum),
		Created:        float64(time.Now().Unix()),
		Summary:        sqlNullString(fr.Summary),
		IncidentNumber: sql.NullInt32{},
	})
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to create Field Report", err)
		return
	}

	for _, entry := range fr.ReportEntries {
		if entry.Text == "" {
			continue
		}
		err = addFRReportEntry(ctx, dbTxn, event.ID, int32(newFrNum), author, entry.Text, false)
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Error adding Report Entry", err)
			return
		}
	}

	if fr.Summary != nil {
		text := "Changed summary to: " + *fr.Summary
		err = addFRReportEntry(ctx, dbTxn, event.ID, int32(newFrNum), author, text, true)
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Error changing Field Report summary", err)
			return
		}
	}

	if err = txn.Commit(); err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to commit transaction", err)
		return
	}

	loc := fmt.Sprintf("/ims/api/events/%v/field_reports/%v", event.Name, newFrNum)
	w.Header().Set("X-IMS-Field-Report-Number", fmt.Sprint(newFrNum))
	w.Header().Set("Location", loc)
	defer action.eventSource.notifyFieldReportUpdate(event.Name, int32(newFrNum))

	http.Error(w, http.StatusText(http.StatusCreated), http.StatusCreated)
}

func addFRReportEntry(ctx context.Context, q *imsdb.Queries, eventID, frNum int32, author, text string, generated bool) error {
	reID, err := q.CreateReportEntry(ctx, imsdb.CreateReportEntryParams{
		Author:       author,
		Text:         text,
		Created:      float64(time.Now().Unix()),
		Generated:    generated,
		Stricken:     false,
		AttachedFile: sql.NullString{},
	})
	if err != nil {
		return fmt.Errorf("[CreateReportEntry]: %w", err)
	}
	err = q.AttachReportEntryToFieldReport(ctx, imsdb.AttachReportEntryToFieldReportParams{
		Event:             eventID,
		FieldReportNumber: frNum,
		ReportEntry:       int32(reID),
	})
	if err != nil {
		return fmt.Errorf("[AttachReportEntryToFieldReport]: %w", err)
	}
	return nil
}

func sqlNullString(s *string) sql.NullString {
	if s == nil || *s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}
