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
	"encoding/json"
	"errors"
	"fmt"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
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
	imsDB              *store.DB
	imsAdmins          []string
	attachmentsEnabled bool
}

func (action GetIncidents) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errH := action.getIncidents(req)
	if errH != nil {
		errH.Src("[getIncidents]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, resp)
}

func (action GetIncidents) getIncidents(req *http.Request) (imsjson.Incidents, *herr.HTTPError) {
	resp := make(imsjson.Incidents, 0)
	event, _, eventPermissions, errH := mustGetEventPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return resp, errH.Src("[mustGetEventPermissions]")
	}
	if eventPermissions&authz.EventReadIncidents == 0 {
		return nil, herr.S403("The requestor does not have EventReadIncidents permission", nil)
	}
	if err := req.ParseForm(); err != nil {
		return nil, herr.S400("Failed to parse form", err)
	}
	generatedLTE := req.Form.Get("exclude_system_entries") != "true" // false means to exclude

	reportEntries, err := imsdb.New(action.imsDB).Incidents_ReportEntries(req.Context(),
		imsdb.Incidents_ReportEntriesParams{
			Event:     event.ID,
			Generated: generatedLTE,
		})
	if err != nil {
		return resp, herr.S500("Failed to fetch Incident Report Entries", err)
	}

	entriesByIncident := make(map[int32][]imsjson.ReportEntry)
	for _, row := range reportEntries {
		re := row.ReportEntry
		entriesByIncident[row.IncidentNumber] = append(entriesByIncident[row.IncidentNumber], reportEntryToJSON(re, action.attachmentsEnabled))
	}

	incidentsRows, err := imsdb.New(action.imsDB).Incidents(req.Context(), event.ID)
	if err != nil {
		return resp, herr.S500("Failed to fetch Incidents", err).Src("[Incidents]")
	}

	for _, r := range incidentsRows {
		// The conversion from IncidentsRow to IncidentRow works because the Incident and Incidents
		// query row structs currently have the same fields in the same order. If that changes in the
		// future, this won't compile, and we may need to duplicate the readExtraIncidentRowFields
		// function.
		incidentTypes, rangerHandles, fieldReportNumbers, err := readExtraIncidentRowFields(imsdb.IncidentRow(r))
		if err != nil {
			return resp, herr.S500("Failed to fetch Incident details", err).Src("[readExtraIncidentRowFields]")
		}
		lastModified := int64(r.Incident.Created)
		for _, re := range entriesByIncident[r.Incident.Number] {
			lastModified = max(lastModified, re.Created.Unix())
		}
		resp = append(resp, imsjson.Incident{
			Event:        event.Name,
			EventID:      event.ID,
			Number:       r.Incident.Number,
			Created:      time.Unix(int64(r.Incident.Created), 0),
			LastModified: time.Unix(lastModified, 0),
			State:        string(r.Incident.State),
			Priority:     r.Incident.Priority,
			Summary:      conv.StringOrNil(r.Incident.Summary),
			Location: imsjson.Location{
				Name:         conv.StringOrNil(r.Incident.LocationName),
				Concentric:   conv.StringOrNil(r.Incident.LocationConcentric),
				RadialHour:   conv.FormatSqlInt16(r.Incident.LocationRadialHour),
				RadialMinute: conv.FormatSqlInt16(r.Incident.LocationRadialMinute),
				Description:  conv.StringOrNil(r.Incident.LocationDescription),
				Type:         garett,
			},
			IncidentTypes: &incidentTypes,
			FieldReports:  &fieldReportNumbers,
			RangerHandles: &rangerHandles,
			ReportEntries: entriesByIncident[r.Incident.Number],
		})
	}

	return resp, nil
}

type GetIncident struct {
	imsDB              *store.DB
	imsAdmins          []string
	attachmentsEnabled bool
}

func (action GetIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errH := action.getIncident(req)
	if errH != nil {
		errH.Src("[getIncident]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, resp)
}

func (action GetIncident) getIncident(req *http.Request) (imsjson.Incident, *herr.HTTPError) {
	var resp imsjson.Incident

	event, _, eventPermissions, errH := mustGetEventPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return resp, errH.Src("[mustGetEventPermissions]")
	}
	if eventPermissions&authz.EventReadIncidents == 0 {
		return resp, herr.S403("The requestor does not have EventReadIncidents permission on this Event", nil)
	}
	ctx := req.Context()

	incidentNumber, err := conv.ParseInt32(req.PathValue("incidentNumber"))
	if err != nil {
		return resp, herr.S400("Failed to parse incident number", err)
	}

	storedRow, reportEntries, errH := fetchIncident(ctx, action.imsDB, event.ID, incidentNumber)
	if errH != nil {
		return resp, errH.Src("[fetchIncident]")
	}

	resultEntries := make([]imsjson.ReportEntry, 0)
	for _, re := range reportEntries {
		resultEntries = append(resultEntries, reportEntryToJSON(re, action.attachmentsEnabled))
	}

	incidentTypes, rangerHandles, fieldReportNumbers, err := readExtraIncidentRowFields(storedRow)
	if err != nil {
		return resp, herr.S500("Failed to fetch Incident details", err).Src("[readExtraIncidentRowFields]")
	}

	lastModified := int64(storedRow.Incident.Created)
	for _, re := range resultEntries {
		lastModified = max(lastModified, re.Created.Unix())
	}
	resp = imsjson.Incident{
		Event:        event.Name,
		EventID:      event.ID,
		Number:       storedRow.Incident.Number,
		Created:      time.Unix(int64(storedRow.Incident.Created), 0),
		LastModified: time.Unix(lastModified, 0),
		State:        string(storedRow.Incident.State),
		Priority:     storedRow.Incident.Priority,
		Summary:      conv.StringOrNil(storedRow.Incident.Summary),
		Location: imsjson.Location{
			Name:         conv.StringOrNil(storedRow.Incident.LocationName),
			Concentric:   conv.StringOrNil(storedRow.Incident.LocationConcentric),
			RadialHour:   conv.FormatSqlInt16(storedRow.Incident.LocationRadialHour),
			RadialMinute: conv.FormatSqlInt16(storedRow.Incident.LocationRadialMinute),
			Description:  conv.StringOrNil(storedRow.Incident.LocationDescription),
			Type:         garett,
		},
		IncidentTypes: &incidentTypes,
		FieldReports:  &fieldReportNumbers,
		RangerHandles: &rangerHandles,
		ReportEntries: resultEntries,
	}
	return resp, nil
}

func fetchIncident(ctx context.Context, imsDB *store.DB, eventID, incidentNumber int32) (
	imsdb.IncidentRow, []imsdb.ReportEntry, *herr.HTTPError,
) {
	var empty imsdb.IncidentRow
	var reportEntries []imsdb.ReportEntry
	incidentRow, err := imsdb.New(imsDB).Incident(ctx, imsdb.IncidentParams{
		Event:  eventID,
		Number: incidentNumber,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return empty, nil, herr.S404("Incident not found", err).Src("[Incident]")
		}
		return empty, nil, herr.S500("Failed to fetch Incident", err).Src("[Incident]")
	}
	reportEntryRows, err := imsdb.New(imsDB).Incident_ReportEntries(ctx,
		imsdb.Incident_ReportEntriesParams{
			Event:          eventID,
			IncidentNumber: incidentNumber,
		},
	)
	if err != nil {
		return empty, nil, herr.S500("Failed to fetch report entries", err).Src("[Incident_ReportEntries]")
	}
	for _, rer := range reportEntryRows {
		reportEntries = append(reportEntries, rer.ReportEntry)
	}
	return incidentRow, reportEntries, nil
}

func addIncidentReportEntry(
	ctx context.Context, q *imsdb.Queries, eventID, incidentNum int32,
	author, text string, generated bool, attachment string,
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
		return 0, herr.S500("Failed to create report entry", err).Src("[MustInt32]")
	}
	err = q.AttachReportEntryToIncident(ctx, imsdb.AttachReportEntryToIncidentParams{
		Event:          eventID,
		IncidentNumber: incidentNum,
		ReportEntry:    reID,
	})
	if err != nil {
		return 0, herr.S500("Failed to attach report entry", err).Src("[AttachReportEntryToIncident]")
	}
	return reID, nil
}

type NewIncident struct {
	imsDB     *store.DB
	es        *EventSourcerer
	imsAdmins []string
}

func (action NewIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	number, location, errH := action.newIncident(req)
	if errH != nil {
		errH.Src("[newIncident]").WriteResponse(w)
		return
	}

	w.Header().Set("IMS-Incident-Number", strconv.Itoa(int(number)))
	w.Header().Set("Location", location)
	http.Error(w, http.StatusText(http.StatusCreated), http.StatusCreated)
}
func (action NewIncident) newIncident(req *http.Request) (incidentNumber int32, location string, errH *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errH := mustGetEventPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return 0, "", errH.Src("[mustGetEventPermissions]")
	}
	if eventPermissions&authz.EventWriteIncidents == 0 {
		return 0, "", herr.S403("The requestor does not have EventWriteIncidents permission on this Event", nil)
	}
	ctx := req.Context()
	newIncident, errH := mustReadBodyAs[imsjson.Incident](req)
	if errH != nil {
		return 0, "", errH.Src("[mustReadBodyAs]")
	}

	author := jwtCtx.Claims.RangerHandle()

	// First create the incident, to lock in the incident number reservation
	newIncidentNumber, err := imsdb.New(action.imsDB).NextIncidentNumber(ctx, event.ID)
	if err != nil {
		return 0, "", herr.S500("Failed to find next Incident number", err).Src("[NextIncidentNumber]")
	}
	newIncident.EventID = event.ID
	newIncident.Event = event.Name
	newIncident.Number = newIncidentNumber

	createTheIncident := imsdb.CreateIncidentParams{
		Event:    newIncident.EventID,
		Number:   newIncident.Number,
		Created:  float64(time.Now().Unix()),
		Priority: imsjson.IncidentPriorityNormal,
		State:    imsdb.IncidentStateNew,
	}
	_, err = imsdb.New(action.imsDB).CreateIncident(ctx, createTheIncident)
	if err != nil {
		return 0, "", herr.S500("Failed to create incident", err).Src("[CreateIncident]")
	}

	if errH = updateIncident(ctx, action.imsDB, action.es, newIncident, author); errH != nil {
		return 0, "", errH.Src("[updateIncident]")
	}

	return newIncident.Number, fmt.Sprintf("/ims/api/events/%v/incidents/%d", event.Name, newIncident.Number), nil
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

func updateIncident(ctx context.Context, imsDB *store.DB, es *EventSourcerer, newIncident imsjson.Incident, author string,
) *herr.HTTPError {
	storedIncidentRow, err := imsdb.New(imsDB).Incident(ctx, imsdb.IncidentParams{
		Event:  newIncident.EventID,
		Number: newIncident.Number,
	})
	if err != nil {
		return herr.S500("Failed to create incident", err).Src("[Incident]")
	}
	storedIncident := storedIncidentRow.Incident

	incidentTypes, rangerHandles, fieldReportNumbers, err := readExtraIncidentRowFields(storedIncidentRow)
	if err != nil {
		return herr.S500("Failed to read incident details", err).Src("[readExtraIncidentRowFields]")
	}

	txn, err := imsDB.Begin()
	if err != nil {
		return herr.S500("Failed to start transaction", err).Src("[Begin]")
	}
	defer rollback(txn)
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
		update.LocationRadialHour = conv.ParseSqlInt16(newIncident.Location.RadialHour)
		logs = append(logs, fmt.Sprintf("Changed location radial hour: %v", update.LocationRadialHour.Int16))
	}
	if newIncident.Location.RadialMinute != nil {
		update.LocationRadialMinute = conv.ParseSqlInt16(newIncident.Location.RadialMinute)
		logs = append(logs, fmt.Sprintf("Changed location radial minute: %v", update.LocationRadialMinute.Int16))
	}
	if newIncident.Location.Description != nil {
		update.LocationDescription = sqlNullString(newIncident.Location.Description)
		logs = append(logs, fmt.Sprintf("Changed location description: %v", update.LocationDescription.String))
	}
	err = dbTxn.UpdateIncident(ctx, update)
	if err != nil {
		return herr.S500("Failed to update incident", err).Src("[UpdateIncident]")
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
					return herr.S500("Failed to attach Ranger to Incident", err).Src("[AttachRangerHandleToIncident]")
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
					return herr.S500("Failed to detach Ranger from Incident", err).Src("[DetachRangerHandleFromIncident]")
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
					return herr.S500("Failed to add Incident Type", err).Src("[AttachIncidentTypeToIncident]")
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
					return herr.S500("Failed to detach Incident Type", err).Src("[AttachIncidentTypeToIncident]")
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
					return herr.S500("Failed to attach Field Report", err).Src("[AttachFieldReportToIncident]")
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
					return herr.S500("Failed to detach Field Report", err).Src("[AttachFieldReportToIncident]")
				}
			}
		}
	}

	if len(logs) > 0 {
		_, errH := addIncidentReportEntry(ctx, dbTxn, newIncident.EventID, newIncident.Number, author, strings.Join(logs, "\n"), true, "")
		if errH != nil {
			return errH.Src("[addIncidentReportEntry]")
		}
	}

	for _, entry := range newIncident.ReportEntries {
		if entry.Text == "" {
			continue
		}
		_, errH := addIncidentReportEntry(ctx, dbTxn, newIncident.EventID, newIncident.Number, author, entry.Text, false, "")
		if errH != nil {
			return errH.Src("[addIncidentReportEntry]")
		}
	}

	if err = txn.Commit(); err != nil {
		return herr.S500("Failed to commit transaction", err).Src("[Commit]")
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
	if hErr := action.editIncident(req); hErr != nil {
		hErr.Src("[editIncident]").WriteResponse(w)
		return
	}
	http.Error(w, "Success", http.StatusNoContent)
}

func (action EditIncident) editIncident(req *http.Request) *herr.HTTPError {
	event, jwtCtx, eventPermissions, errH := mustGetEventPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return errH.Src("[mustGetEventPermissions]")
	}
	if eventPermissions&authz.EventWriteIncidents == 0 {
		return herr.S403("The requestor does not have EventWriteIncidents permission for this Event", nil)
	}
	ctx := req.Context()

	incidentNumber, err := conv.ParseInt32(req.PathValue("incidentNumber"))
	if err != nil {
		return herr.S400("Invalid Incident Number", err).Src("[ParseInt32]")
	}
	newIncident, errH := mustReadBodyAs[imsjson.Incident](req)
	if errH != nil {
		return errH.Src("[mustReadBodyAs]")
	}
	newIncident.Event = event.Name
	newIncident.EventID = event.ID
	newIncident.Number = incidentNumber

	author := jwtCtx.Claims.RangerHandle()

	if errH = updateIncident(ctx, action.imsDB, action.es, newIncident, author); errH != nil {
		return errH.Src("[updateIncident]")
	}

	return nil
}
