package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/srabraham/ranger-ims-go/auth"
	imsjson "github.com/srabraham/ranger-ims-go/json"
	"github.com/srabraham/ranger-ims-go/store"
	"github.com/srabraham/ranger-ims-go/store/imsdb"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

const (
	garett = "garett"
)

type GetIncidents struct {
	imsDB     *store.DB
	imsAdmins []string
}

func (action GetIncidents) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp := make(imsjson.Incidents, 0)
	event, _, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&auth.EventReadIncidents == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have EventReadIncidents permission on this Event", nil)
		return
	}
	if !mustParseForm(w, req) {
		return
	}
	generatedLTE := req.Form.Get("exclude_system_entries") != "true" // false means to exclude

	reportEntries, err := imsdb.New(action.imsDB).Incidents_ReportEntries(req.Context(),
		imsdb.Incidents_ReportEntriesParams{
			Event:     event.ID,
			Generated: generatedLTE,
		})
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Incident Report Entries", err)
		return
	}

	entriesByIncident := make(map[int32][]imsjson.ReportEntry)
	for _, row := range reportEntries {
		re := row.ReportEntry
		entriesByIncident[row.IncidentNumber] = append(entriesByIncident[row.IncidentNumber], reportEntryToJSON(re))
	}

	incidentsRows, err := imsdb.New(action.imsDB).Incidents(req.Context(), event.ID)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Incidents", err)
		return
	}

	for _, r := range incidentsRows {
		// The conversion from IncidentsRow to IncidentRow works because the Incident and Incidents
		// query row structs currently have the same fields in the same order. If that changes in the
		// future, this won't compile, and we may need to duplicate the readExtraIncidentRowFields
		// function.
		incidentTypes, rangerHandles, fieldReportNumbers, err := readExtraIncidentRowFields(imsdb.IncidentRow(r))
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Incident details", err)
			return
		}
		lastModified := time.Unix(int64(r.Incident.Created), 0)
		for _, re := range entriesByIncident[r.Incident.Number] {
			if re.Created.After(lastModified) {
				lastModified = re.Created
			}
		}
		resp = append(resp, imsjson.Incident{
			Event:        event.Name,
			EventID:      event.ID,
			Number:       r.Incident.Number,
			Created:      time.Unix(int64(r.Incident.Created), 0),
			LastModified: lastModified,
			State:        string(r.Incident.State),
			Priority:     r.Incident.Priority,
			Summary:      stringOrNil(r.Incident.Summary),
			Location: imsjson.Location{
				Name:         stringOrNil(r.Incident.LocationName),
				Concentric:   stringOrNil(r.Incident.LocationConcentric),
				RadialHour:   formatInt16(r.Incident.LocationRadialHour),
				RadialMinute: formatInt16(r.Incident.LocationRadialMinute),
				Description:  stringOrNil(r.Incident.LocationDescription),
				Type:         garett,
			},
			IncidentTypes: &incidentTypes,
			FieldReports:  &fieldReportNumbers,
			RangerHandles: &rangerHandles,
			ReportEntries: entriesByIncident[r.Incident.Number],
		})
	}

	mustWriteJSON(w, resp)
}

type GetIncident struct {
	imsDB     *store.DB
	imsAdmins []string
}

func (action GetIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	event, _, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&auth.EventReadIncidents == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have EventReadIncidents permission on this Event", nil)
		return
	}
	ctx := req.Context()

	incidentNumber, err := strconv.ParseInt(req.PathValue("incidentNumber"), 10, 32)
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Failed to parse incident number", err)
		return
	}

	storedRow, reportEntries, err := fetchIncident(ctx, action.imsDB, event.ID, int32(incidentNumber))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			handleErr(w, req, http.StatusNotFound, "No such incident", err)
			return
		}
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Incident", err)
		return
	}

	resultEntries := make([]imsjson.ReportEntry, 0)
	for _, re := range reportEntries {
		resultEntries = append(resultEntries, reportEntryToJSON(re))
	}

	incidentTypes, rangerHandles, fieldReportNumbers, err := readExtraIncidentRowFields(storedRow)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Incident details", err)
		return
	}

	lastModified := time.Unix(int64(storedRow.Incident.Created), 0)
	for _, re := range resultEntries {
		if re.Created.After(lastModified) {
			lastModified = re.Created
		}
	}
	result := imsjson.Incident{
		Event:        event.Name,
		EventID:      event.ID,
		Number:       storedRow.Incident.Number,
		Created:      time.Unix(int64(storedRow.Incident.Created), 0),
		LastModified: lastModified,
		State:        string(storedRow.Incident.State),
		Priority:     storedRow.Incident.Priority,
		Summary:      stringOrNil(storedRow.Incident.Summary),
		Location: imsjson.Location{
			Name:         stringOrNil(storedRow.Incident.LocationName),
			Concentric:   stringOrNil(storedRow.Incident.LocationConcentric),
			RadialHour:   formatInt16(storedRow.Incident.LocationRadialHour),
			RadialMinute: formatInt16(storedRow.Incident.LocationRadialMinute),
			Description:  stringOrNil(storedRow.Incident.LocationDescription),
			Type:         garett,
		},
		IncidentTypes: &incidentTypes,
		FieldReports:  &fieldReportNumbers,
		RangerHandles: &rangerHandles,
		ReportEntries: resultEntries,
	}

	mustWriteJSON(w, result)
}

func fetchIncident(ctx context.Context, imsDB *store.DB, eventID, incidentNumber int32) (
	incident imsdb.IncidentRow, reportEntries []imsdb.ReportEntry, err error,
) {
	incidentRow, err := imsdb.New(imsDB).Incident(ctx, imsdb.IncidentParams{
		Event:  eventID,
		Number: incidentNumber,
	})
	if err != nil {
		return imsdb.IncidentRow{}, nil, fmt.Errorf("[Incident]: %w", err)
	}
	reportEntryRows, err := imsdb.New(imsDB).Incident_ReportEntries(ctx,
		imsdb.Incident_ReportEntriesParams{
			Event:          eventID,
			IncidentNumber: incidentNumber,
		},
	)
	if err != nil {
		return imsdb.IncidentRow{}, nil, fmt.Errorf("[Incident_ReportEntries]: %w", err)
	}
	for _, rer := range reportEntryRows {
		reportEntries = append(reportEntries, rer.ReportEntry)
	}
	return incidentRow, reportEntries, nil
}

func addIncidentReportEntry(ctx context.Context, q *imsdb.Queries, eventID, incidentNum int32, author, text string, generated bool) error {
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
	err = q.AttachReportEntryToIncident(ctx, imsdb.AttachReportEntryToIncidentParams{
		Event:          eventID,
		IncidentNumber: incidentNum,
		ReportEntry:    int32(reID),
	})
	if err != nil {
		return fmt.Errorf("[AttachReportEntryToIncident]: %w", err)
	}
	return nil
}

type NewIncident struct {
	imsDB     *store.DB
	es        *EventSourcerer
	imsAdmins []string
}

func (action NewIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	event, jwtCtx, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&auth.EventWriteIncidents == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have EventWriteIncidents permission on this Event", nil)
		return
	}
	ctx := req.Context()
	newIncident, ok := mustReadBodyAs[imsjson.Incident](w, req)
	if !ok {
		return
	}

	author := jwtCtx.Claims.RangerHandle()

	// First create the incident, to lock in the incident number reservation
	numUntyped, _ := imsdb.New(action.imsDB).MaxIncidentNumber(ctx, event.ID)
	newIncident.EventID = event.ID
	newIncident.Event = event.Name
	newIncident.Number = int32(numUntyped.(int64)) + 1

	createTheIncident := imsdb.CreateIncidentParams{
		Event:    newIncident.EventID,
		Number:   newIncident.Number,
		Created:  float64(time.Now().Unix()),
		Priority: imsjson.IncidentPriorityNormal,
		State:    imsdb.IncidentStateNew,
	}
	_, err := imsdb.New(action.imsDB).CreateIncident(ctx, createTheIncident)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to create incident", err)
		return
	}

	if err = updateIncident(ctx, action.imsDB, action.es, newIncident, author); err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to update incident", err)
		return
	}

	w.Header().Set("X-IMS-Incident-Number", fmt.Sprint(newIncident.Number))
	w.Header().Set("Location", "/ims/api/events/"+event.Name+"/incidents/"+fmt.Sprint(newIncident.Number))
	http.Error(w, http.StatusText(http.StatusCreated), http.StatusCreated)
}

func unmarshalByteSlice[T any](isByteSlice any) (T, error) {
	var result T
	b, ok := isByteSlice.([]byte)
	if !ok {
		return result, fmt.Errorf("could not read object as []bytes. Was actually %T", b)
	}
	err := json.Unmarshal(b, &result)
	if err != nil {
		return result, fmt.Errorf("[Unmarshal]: %w", err)
	}
	return result, nil
}

func readExtraIncidentRowFields(row imsdb.IncidentRow) (incidentTypes, rangerHandles []string, fieldReportNumbers []int32, err error) {
	incidentTypes, err = unmarshalByteSlice[[]string](row.IncidentTypes)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("[unmarshalByteSlice]: %w", err)
	}
	rangerHandles, err = unmarshalByteSlice[[]string](row.RangerHandles)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("[unmarshalByteSlice]: %w", err)
	}
	fieldReportNumbers, err = unmarshalByteSlice[[]int32](row.FieldReportNumbers)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("[unmarshalByteSlice]: %w", err)
	}
	return incidentTypes, rangerHandles, fieldReportNumbers, nil
}

func updateIncident(ctx context.Context, imsDB *store.DB, es *EventSourcerer, newIncident imsjson.Incident, author string) error {
	storedIncidentRow, err := imsdb.New(imsDB).Incident(ctx, imsdb.IncidentParams{
		Event:  newIncident.EventID,
		Number: newIncident.Number,
	})
	if err != nil {
		return fmt.Errorf("[Incident]: %w", err)
	}
	storedIncident := storedIncidentRow.Incident

	incidentTypes, rangerHandles, fieldReportNumbers, err := readExtraIncidentRowFields(storedIncidentRow)
	if err != nil {
		return fmt.Errorf("[readExtraIncidentRowFields]: %w", err)
	}

	txn, err := imsDB.Begin()
	if err != nil {
		return fmt.Errorf("[Begin]: %w", err)
	}
	defer txn.Rollback()
	dbTxn := imsdb.New(txn)

	update := imsdb.UpdateIncidentParams{
		Event:                storedIncident.Event,
		Number:               storedIncident.Number,
		Created:              storedIncident.Created,
		Priority:             storedIncident.Priority,
		State:                storedIncident.State,
		Summary:              storedIncident.Summary,
		LocationName:         storedIncident.LocationName,
		LocationConcentric:   storedIncident.LocationConcentric,
		LocationRadialHour:   storedIncident.LocationRadialHour,
		LocationRadialMinute: storedIncident.LocationRadialMinute,
		LocationDescription:  storedIncident.LocationDescription,
	}

	var logs []string

	if newIncident.Priority != 0 {
		update.Priority = newIncident.Priority
		logs = append(logs, fmt.Sprintf("Changed priority: %v", update.Priority))
	}
	if imsdb.IncidentState(newIncident.State).Valid() {
		update.State = imsdb.IncidentState(newIncident.State)
		logs = append(logs, fmt.Sprintf("Changed state: %v", update.State))
	}
	if newIncident.Summary != nil {
		update.Summary = sqlNullString(newIncident.Summary)
		logs = append(logs, fmt.Sprintf("Changed summary: %v", update.Summary.String))
	}
	if newIncident.Location.Name != nil {
		update.LocationName = sqlNullString(newIncident.Location.Name)
		logs = append(logs, fmt.Sprintf("Changed location name: %v", update.LocationName.String))
	}
	if newIncident.Location.Concentric != nil {
		update.LocationConcentric = sqlNullString(newIncident.Location.Concentric)
		logs = append(logs, fmt.Sprintf("Changed location concentric: %v", update.LocationConcentric.String))
	}
	if newIncident.Location.RadialHour != nil {
		update.LocationRadialHour = parseInt16(newIncident.Location.RadialHour)
		logs = append(logs, fmt.Sprintf("Changed location radial hour: %v", update.LocationRadialHour.Int16))
	}
	if newIncident.Location.RadialMinute != nil {
		update.LocationRadialMinute = parseInt16(newIncident.Location.RadialMinute)
		logs = append(logs, fmt.Sprintf("Changed location radial minute: %v", update.LocationRadialMinute.Int16))
	}
	if newIncident.Location.Description != nil {
		update.LocationDescription = sqlNullString(newIncident.Location.Description)
		logs = append(logs, fmt.Sprintf("Changed location description: %v", update.LocationDescription.String))
	}
	err = dbTxn.UpdateIncident(ctx, update)
	if err != nil {
		return fmt.Errorf("[UpdateIncident]: %w", err)
	}

	if newIncident.RangerHandles != nil {
		add := sliceSubtract(*newIncident.RangerHandles, rangerHandles)
		sub := sliceSubtract(rangerHandles, *newIncident.RangerHandles)
		if len(add) > 0 {
			logs = append(logs, fmt.Sprintf("Added Ranger: %v", strings.Join(add, ", ")))
			for _, rh := range add {
				err = dbTxn.AttachRangerHandleToIncident(ctx, imsdb.AttachRangerHandleToIncidentParams{
					Event:          newIncident.EventID,
					IncidentNumber: newIncident.Number,
					RangerHandle:   rh,
				})
				if err != nil {
					return fmt.Errorf("[AttachRangerHandleToIncident]: %w", err)
				}
			}
		}
		if len(sub) > 0 {
			logs = append(logs, fmt.Sprintf("Removed Ranger: %v", strings.Join(sub, ", ")))
			for _, rh := range sub {
				err = dbTxn.DetachRangerHandleFromIncident(ctx, imsdb.DetachRangerHandleFromIncidentParams{
					Event:          newIncident.EventID,
					IncidentNumber: newIncident.Number,
					RangerHandle:   rh,
				})
				if err != nil {
					return fmt.Errorf("[DetachRangerHandleFromIncident]: %w", err)
				}
			}
		}
	}

	if newIncident.IncidentTypes != nil {
		add := sliceSubtract(*newIncident.IncidentTypes, incidentTypes)
		sub := sliceSubtract(incidentTypes, *newIncident.IncidentTypes)
		if len(add) > 0 {
			logs = append(logs, fmt.Sprintf("Added type: %v", strings.Join(add, ", ")))
			for _, itype := range add {
				err = dbTxn.AttachIncidentTypeToIncident(ctx, imsdb.AttachIncidentTypeToIncidentParams{
					Event:          newIncident.EventID,
					IncidentNumber: newIncident.Number,
					Name:           itype,
				})
				if err != nil {
					return fmt.Errorf("[AttachIncidentTypeToIncident]: %w", err)
				}
			}
		}
		if len(sub) > 0 {
			logs = append(logs, fmt.Sprintf("Removed type: %v", strings.Join(sub, ", ")))
			for _, rh := range sub {
				err = dbTxn.DetachIncidentTypeFromIncident(ctx, imsdb.DetachIncidentTypeFromIncidentParams{
					Event:          newIncident.EventID,
					IncidentNumber: newIncident.Number,
					Name:           rh,
				})
				if err != nil {
					return fmt.Errorf("[DetachIncidentTypeFromIncident]: %w", err)
				}
			}
		}
	}
	var updatedFieldReports []int32
	if newIncident.FieldReports != nil {
		add := sliceSubtract(*newIncident.FieldReports, fieldReportNumbers)
		sub := sliceSubtract(fieldReportNumbers, *newIncident.FieldReports)
		updatedFieldReports = append(updatedFieldReports, add...)
		updatedFieldReports = append(updatedFieldReports, sub...)

		if len(add) > 0 {
			logs = append(logs, fmt.Sprintf("Field Report added: %v", add))
			for _, frNum := range add {
				err = dbTxn.AttachFieldReportToIncident(ctx, imsdb.AttachFieldReportToIncidentParams{
					Event:          newIncident.EventID,
					Number:         frNum,
					IncidentNumber: sql.NullInt32{Int32: newIncident.Number, Valid: true},
				})
				if err != nil {
					return fmt.Errorf("[AttachIncidentTypeToIncident]: %w", err)
				}
			}
		}
		if len(sub) > 0 {
			logs = append(logs, fmt.Sprintf("Field Report removed: %v", sub))
			for _, frNum := range sub {
				err = dbTxn.AttachFieldReportToIncident(ctx, imsdb.AttachFieldReportToIncidentParams{
					Event:          newIncident.EventID,
					Number:         frNum,
					IncidentNumber: sql.NullInt32{},
				})
				if err != nil {
					return fmt.Errorf("[AttachFieldReportToIncident]: %w", err)
				}
			}
		}
	}

	if len(logs) > 0 {
		err = addIncidentReportEntry(ctx, dbTxn, newIncident.EventID, newIncident.Number, author, strings.Join(logs, "\n"), true)
		if err != nil {
			return fmt.Errorf("[addIncidentReportEntry]: %w", err)
		}
	}

	for _, entry := range newIncident.ReportEntries {
		if entry.Text == "" {
			continue
		}
		err = addIncidentReportEntry(ctx, dbTxn, newIncident.EventID, newIncident.Number, author, entry.Text, false)
		if err != nil {
			return fmt.Errorf("[addIncidentReportEntry]: %w", err)
		}
	}

	if err = txn.Commit(); err != nil {
		return fmt.Errorf("[Commit]: %w", err)
	}

	es.notifyIncidentUpdate(newIncident.Event, newIncident.Number)
	for _, fr := range updatedFieldReports {
		es.notifyFieldReportUpdate(newIncident.Event, fr)
	}

	return nil
}

func sliceSubtract[T comparable](a, b []T) []T {
	var ret []T
	for _, item := range a {
		if !slices.Contains(b, item) {
			ret = append(ret, item)
		}
	}
	return ret
}

type EditIncident struct {
	imsDB     *store.DB
	es        *EventSourcerer
	imsAdmins []string
}

func (action EditIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	event, jwtCtx, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&auth.EventWriteIncidents == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have EventWriteIncidents permission on this Event", nil)
		return
	}
	ctx := req.Context()

	incidentNumber, err := strconv.ParseInt(req.PathValue("incidentNumber"), 10, 32)
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Invalid Incident Number", err)
		return
	}
	newIncident, ok := mustReadBodyAs[imsjson.Incident](w, req)
	if !ok {
		return
	}
	newIncident.Event = event.Name
	newIncident.EventID = event.ID
	newIncident.Number = int32(incidentNumber)

	author := jwtCtx.Claims.RangerHandle()

	if err = updateIncident(ctx, action.imsDB, action.es, newIncident, author); err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to update Incident", err)
		return
	}

	http.Error(w, http.StatusText(http.StatusNoContent), http.StatusNoContent)

	return
}
