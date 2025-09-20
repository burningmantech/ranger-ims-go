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
	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"golang.org/x/sync/errgroup"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

type GetIncidents struct {
	imsDBQ             *store.DBQ
	userStore          *directory.UserStore
	imsAdmins          []string
	attachmentsEnabled bool
}

func (action GetIncidents) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getIncidents(req)
	if errHTTP != nil {
		errHTTP.From("[getIncidents]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}

func (action GetIncidents) getIncidents(req *http.Request) (imsjson.Incidents, *herr.HTTPError) {
	resp := make(imsjson.Incidents, 0)
	event, _, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return resp, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventReadIncidents == 0 {
		return nil, herr.Forbidden("The requestor does not have EventReadIncidents permission", nil)
	}
	err := req.ParseForm()
	if err != nil {
		return nil, herr.BadRequest("Failed to parse form", err)
	}
	includeSystemEntries := !strings.EqualFold(req.Form.Get("exclude_system_entries"), "true")

	// The Incidents and ReportEntries queries both request a lot of data, and we can query
	// and process those results concurrently.
	group, groupCtx := errgroup.WithContext(req.Context())

	entriesByIncident := make(map[int32][]imsdb.ReportEntry)
	group.Go(func() error {
		reportEntries, err := action.imsDBQ.Incidents_ReportEntries(
			groupCtx,
			action.imsDBQ,
			imsdb.Incidents_ReportEntriesParams{
				Event:     event.ID,
				Generated: includeSystemEntries,
			},
		)
		if err != nil {
			return herr.InternalServerError("Failed to fetch Incident Report Entries", err).From("[Incidents_ReportEntries]")
		}
		for _, row := range reportEntries {
			entriesByIncident[row.IncidentNumber] = append(
				entriesByIncident[row.IncidentNumber],
				row.ReportEntry,
			)
		}
		return nil
	})

	var incidentsRows []imsdb.IncidentsRow
	group.Go(func() error {
		var err error
		incidentsRows, err = action.imsDBQ.Incidents(groupCtx, action.imsDBQ, event.ID)
		if err != nil {
			return herr.InternalServerError("Failed to fetch Incidents", err).From("[Incidents]")
		}
		return nil
	})
	err = group.Wait()
	if err != nil {
		return resp, herr.AsHTTPError(err)
	}

	for _, r := range incidentsRows {
		// The conversion from IncidentsRow to IncidentRow works because the Incident and Incidents
		// query row structs currently have the same fields in the same order. If that changes in the
		// future, this won't compile, and we may need to duplicate the readExtraIncidentRowFields
		// function.
		incidentRow := imsdb.IncidentRow(r)

		// we don't bother looking up linked incidents for the GetIncidents call
		var emptyLinkedIncidents []imsdb.Incident_LinkedIncidentsRow

		incJSON, errHTTP := incidentToJSON(incidentRow, entriesByIncident[r.Incident.Number], emptyLinkedIncidents, event, action.attachmentsEnabled)
		if errHTTP != nil {
			return resp, errHTTP.From("[incidentToJSON]")
		}
		resp = append(resp, incJSON)
	}

	return resp, nil
}

type GetIncident struct {
	imsDBQ             *store.DBQ
	userStore          *directory.UserStore
	imsAdmins          []string
	attachmentsEnabled bool
}

func (action GetIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getIncident(req)
	if errHTTP != nil {
		errHTTP.From("[getIncident]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}

func (action GetIncident) getIncident(req *http.Request) (imsjson.Incident, *herr.HTTPError) {
	var resp imsjson.Incident

	event, jwt, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return resp, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventReadIncidents == 0 {
		return resp, herr.Forbidden("The requestor does not have EventReadIncidents permission on this Event", nil)
	}
	ctx := req.Context()

	incidentNumber, err := conv.ParseInt32(req.PathValue("incidentNumber"))
	if err != nil {
		return resp, herr.BadRequest("Failed to parse incident number", err)
	}

	storedRow, reportEntries, errHTTP := fetchIncident(ctx, action.imsDBQ, event.ID, incidentNumber)
	if errHTTP != nil {
		return resp, errHTTP.From("[fetchIncident]")
	}

	permsByEvent, errHTTP := permissionsByEvent(req.Context(), jwt, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return resp, errHTTP.From("[permissionsByEvent]")
	}

	linkedIncidents, err := action.imsDBQ.Incident_LinkedIncidents(ctx, action.imsDBQ, imsdb.Incident_LinkedIncidentsParams{
		Event1:          event.ID,
		IncidentNumber1: incidentNumber,
	})
	if err != nil {
		return resp, herr.InternalServerError("Failed to fetch linked incidents", err)
	}
	for i := range linkedIncidents {
		if permsByEvent[linkedIncidents[i].LinkedEvent]&authz.EventReadIncidents == 0 {
			linkedIncidents[i].LinkedIncidentSummary = sql.NullString{}
		}
	}

	resp, errHTTP = incidentToJSON(storedRow, reportEntries, linkedIncidents, event, action.attachmentsEnabled)
	if errHTTP != nil {
		return resp, errHTTP.From("[incidentToJSON]")
	}
	return resp, nil
}

func incidentToJSON(
	storedRow imsdb.IncidentRow,
	reportEntries []imsdb.ReportEntry,
	linkedIncidents []imsdb.Incident_LinkedIncidentsRow,
	event imsdb.Event,
	attachmentsEnabled bool,
) (imsjson.Incident, *herr.HTTPError) {
	var resp imsjson.Incident
	resultEntries := make([]imsjson.ReportEntry, 0)
	for _, re := range reportEntries {
		resultEntries = append(resultEntries, reportEntryToJSON(re, attachmentsEnabled))
	}

	linkedIncidentJson := make([]imsjson.LinkedIncident, 0)
	for _, li := range linkedIncidents {
		linkedIncidentJson = append(linkedIncidentJson, imsjson.LinkedIncident{
			EventID:   li.LinkedEvent,
			EventName: li.LinkedEventName,
			Number:    li.LinkedIncident,
			Summary:   li.LinkedIncidentSummary.String,
		})
	}

	rangerHandles, incidentTypeIDs, fieldReportNumbers, err := readExtraIncidentRowFields(storedRow)
	if err != nil {
		return resp, herr.InternalServerError("Failed to fetch Incident details", err).From("[readExtraIncidentRowFields]")
	}

	lastModified := conv.FloatToTime(storedRow.Incident.Created)
	for _, re := range resultEntries {
		if re.Created.After(lastModified) {
			lastModified = re.Created
		}
	}
	resp = imsjson.Incident{
		Event:        event.Name,
		EventID:      event.ID,
		Number:       storedRow.Incident.Number,
		Created:      conv.FloatToTime(storedRow.Incident.Created),
		LastModified: lastModified,
		State:        string(storedRow.Incident.State),
		Started:      conv.FloatToTime(storedRow.Incident.Started),
		Closed:       conv.NullFloatToTime(storedRow.Incident.Closed),
		Priority:     storedRow.Incident.Priority,
		Summary:      conv.SqlToString(storedRow.Incident.Summary),
		Location: imsjson.Location{
			Name:         conv.SqlToString(storedRow.Incident.LocationName),
			Concentric:   conv.SqlToString(storedRow.Incident.LocationConcentric),
			RadialHour:   conv.FormatSqlInt16(storedRow.Incident.LocationRadialHour),
			RadialMinute: conv.FormatSqlInt16(storedRow.Incident.LocationRadialMinute),
			Description:  conv.SqlToString(storedRow.Incident.LocationDescription),
		},
		IncidentTypeIDs: &incidentTypeIDs,
		FieldReports:    &fieldReportNumbers,
		RangerHandles:   &rangerHandles,
		ReportEntries:   resultEntries,
		LinkedIncidents: &linkedIncidentJson,
	}
	return resp, nil
}

func fetchIncident(ctx context.Context, imsDBQ *store.DBQ, eventID, incidentNumber int32) (
	imsdb.IncidentRow, []imsdb.ReportEntry, *herr.HTTPError,
) {
	var empty imsdb.IncidentRow
	var reportEntries []imsdb.ReportEntry
	incidentRow, err := imsDBQ.Incident(ctx, imsDBQ,
		imsdb.IncidentParams{
			Event:  eventID,
			Number: incidentNumber,
		},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return empty, nil, herr.NotFound("Incident not found", err).From("[Incident]")
		}
		return empty, nil, herr.InternalServerError("Failed to fetch Incident", err).From("[Incident]")
	}
	reportEntryRows, err := imsDBQ.Incident_ReportEntries(ctx, imsDBQ,
		imsdb.Incident_ReportEntriesParams{
			Event:          eventID,
			IncidentNumber: incidentNumber,
		},
	)
	if err != nil {
		return empty, nil, herr.InternalServerError("Failed to fetch report entries", err).From("[Incident_ReportEntries]")
	}
	for _, rer := range reportEntryRows {
		reportEntries = append(reportEntries, rer.ReportEntry)
	}
	return incidentRow, reportEntries, nil
}

func addIncidentReportEntry(
	ctx context.Context, db *store.DBQ, dbtx imsdb.DBTX,
	eventID, incidentNum int32, author, text string, generated bool,
	attachment, attachmentOriginalName, attachmentMediaType string,
) (int32, *herr.HTTPError) {
	reID64, err := db.CreateReportEntry(ctx, dbtx, imsdb.CreateReportEntryParams{
		Author:                   author,
		Text:                     text,
		Created:                  conv.TimeToFloat(time.Now()),
		Generated:                generated,
		Stricken:                 false,
		AttachedFile:             conv.StringToSql(&attachment, 128),
		AttachedFileOriginalName: conv.StringToSql(&attachmentOriginalName, 128),
		AttachedFileMediaType:    conv.StringToSql(&attachmentMediaType, 128),
	})
	if err != nil {
		return 0, herr.InternalServerError("Failed to create report entry", err).From("[MustInt32]")
	}
	// This column is an int32, so this is safe
	reID := conv.MustInt32(reID64)
	err = db.AttachReportEntryToIncident(ctx, dbtx, imsdb.AttachReportEntryToIncidentParams{
		Event:          eventID,
		IncidentNumber: incidentNum,
		ReportEntry:    reID,
	})
	if err != nil {
		return 0, herr.InternalServerError("Failed to attach report entry", err).From("[AttachReportEntryToIncident]")
	}
	return reID, nil
}

type NewIncident struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action NewIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	number, location, errHTTP := action.newIncident(req)
	if errHTTP != nil {
		errHTTP.From("[newIncident]").WriteResponse(w)
		return
	}

	w.Header().Set("IMS-Incident-Number", strconv.Itoa(int(number)))
	w.Header().Set("Location", location)
	herr.WriteCreatedResponse(w, http.StatusText(http.StatusCreated))
}
func (action NewIncident) newIncident(req *http.Request) (incidentNumber int32, location string, errHTTP *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return 0, "", errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteIncidents == 0 {
		return 0, "", herr.Forbidden("The requestor does not have EventWriteIncidents permission on this Event", nil)
	}
	ctx := req.Context()
	newIncident, errHTTP := readBodyAs[imsjson.Incident](req)
	if errHTTP != nil {
		return 0, "", errHTTP.From("[readBodyAs]")
	}

	author := jwtCtx.Claims.RangerHandle()

	// First create the incident, to lock in the incident number reservation
	newIncidentNumber, err := action.imsDBQ.NextIncidentNumber(ctx, action.imsDBQ, event.ID)
	if err != nil {
		return 0, "", herr.InternalServerError("Failed to find next Incident number", err).From("[NextIncidentNumber]")
	}
	newIncident.EventID = event.ID
	newIncident.Event = event.Name
	newIncident.Number = newIncidentNumber
	now := conv.TimeToFloat(time.Now())
	createTheIncident := imsdb.CreateIncidentParams{
		Event:    newIncident.EventID,
		Number:   newIncidentNumber,
		Created:  now,
		Started:  now,
		Priority: imsjson.IncidentPriorityNormal,
		State:    imsdb.IncidentStateNew,
	}
	_, err = action.imsDBQ.CreateIncident(ctx, action.imsDBQ, createTheIncident)
	if err != nil {
		return 0, "", herr.InternalServerError("Failed to create incident", err).From("[CreateIncident]")
	}

	errHTTP = updateIncident(ctx, action.imsDBQ, action.es, newIncident, author)
	if errHTTP != nil {
		return 0, "", errHTTP.From("[updateIncident]")
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

func readExtraIncidentRowFields(row imsdb.IncidentRow) (rangerHandles []string, incidentTypeIDs, fieldReportNumbers []int32, err error) {
	rangerHandles, err = unmarshalByteSlice[[]string](row.RangerHandles)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("[unmarshalByteSlice]: %w", err)
	}
	incidentTypeIDs, err = unmarshalByteSlice[[]int32](row.IncidentTypeIds)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("[unmarshalByteSlice]: %w", err)
	}
	fieldReportNumbers, err = unmarshalByteSlice[[]int32](row.FieldReportNumbers)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("[unmarshalByteSlice]: %w", err)
	}
	return rangerHandles, incidentTypeIDs, fieldReportNumbers, nil
}

func updateIncident(ctx context.Context, imsDBQ *store.DBQ, es *EventSourcerer, newIncident imsjson.Incident, author string,
) *herr.HTTPError {
	storedIncidentRow, err := imsDBQ.Incident(ctx, imsDBQ,
		imsdb.IncidentParams{
			Event:  newIncident.EventID,
			Number: newIncident.Number,
		},
	)
	if err != nil {
		return herr.InternalServerError("Failed to create incident", err).From("[Incident]")
	}
	storedIncident := storedIncidentRow.Incident

	allEvents, err := imsDBQ.Events(ctx, imsDBQ)
	if err != nil {
		return herr.InternalServerError("Failed to fetch events", err).From("[Events]")
	}
	eventNameById := make(map[int32]string)
	for _, event := range allEvents {
		eventNameById[event.Event.ID] = event.Event.Name
	}

	// Look up the incident types before starting the transaction, to avoid DB connection contention.
	var allIncidentTypes []imsdb.IncidentTypesRow
	if newIncident.IncidentTypeIDs != nil {
		allIncidentTypes, err = imsDBQ.IncidentTypes(ctx, imsDBQ)
		if err != nil {
			return herr.InternalServerError("Failed to get incident types", err).From("[IncidentTypes]")
		}
	}

	linkedIncidents, err := imsDBQ.Incident_LinkedIncidents(ctx, imsDBQ, imsdb.Incident_LinkedIncidentsParams{
		Event1:          storedIncident.Event,
		IncidentNumber1: storedIncident.Number,
	})
	if err != nil {
		return herr.InternalServerError("Failed to fetch linked incidents", err)
	}

	rangerHandles, incidentTypeIDs, fieldReportNumbers, err := readExtraIncidentRowFields(storedIncidentRow)
	_ = incidentTypeIDs
	if err != nil {
		return herr.InternalServerError("Failed to read incident details", err).From("[readExtraIncidentRowFields]")
	}

	// Be sure to do any data lookups prior to starting the transaction, as issues can arise
	// with database connection contention if you try to SELECT while in the process of a transaction.
	// This is specifically about the in-process MySQL, which only allows one DB connection.
	txn, err := imsDBQ.Begin()
	if err != nil {
		return herr.InternalServerError("Failed to start transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	update := imsdb.UpdateIncidentParams{
		Event:                storedIncident.Event,
		Number:               storedIncident.Number,
		Priority:             storedIncident.Priority,
		State:                storedIncident.State,
		Started:              storedIncident.Started,
		Closed:               storedIncident.Closed,
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
	if newState := imsdb.IncidentState(newIncident.State); newState.Valid() {
		update.State = newState
		logs = append(logs, fmt.Sprintf("Changed state: %v", update.State))
		if newState == imsdb.IncidentStateClosed {
			update.Closed = conv.TimeToNullFloat(time.Now())
		} else {
			update.Closed = sql.NullFloat64{}
		}
	}
	if !newIncident.Started.IsZero() {
		update.Started = conv.TimeToFloat(newIncident.Started)
		logs = append(logs, fmt.Sprintf("Changed start time: %v", newIncident.Started.In(time.UTC).Format(time.RFC3339)))
	}
	if newIncident.Summary != nil {
		update.Summary = conv.StringToSql(newIncident.Summary, 0)
		logs = append(logs, fmt.Sprintf("Changed summary: %v", update.Summary.String))
	}
	if newIncident.Location.Name != nil {
		update.LocationName = conv.StringToSql(newIncident.Location.Name, 0)
		logs = append(logs, fmt.Sprintf("Changed location name: %v", update.LocationName.String))
	}
	if newIncident.Location.Concentric != nil {
		update.LocationConcentric = conv.StringToSql(newIncident.Location.Concentric, 0)
		logs = append(logs, fmt.Sprintf("Changed location concentric: %v", update.LocationConcentric.String))
	}
	if newIncident.Location.RadialHour != nil {
		update.LocationRadialHour = conv.ParseSqlInt16(newIncident.Location.RadialHour)
		newValString := "(empty)"
		if update.LocationRadialHour.Valid {
			newValString = strconv.Itoa(int(update.LocationRadialHour.Int16))
		}
		logs = append(logs, fmt.Sprintf("Changed location radial hour: %v", newValString))
	}
	if newIncident.Location.RadialMinute != nil {
		update.LocationRadialMinute = conv.ParseSqlInt16(newIncident.Location.RadialMinute)
		newValString := "(empty)"
		if update.LocationRadialMinute.Valid {
			newValString = strconv.Itoa(int(update.LocationRadialMinute.Int16))
		}
		logs = append(logs, fmt.Sprintf("Changed location radial minute: %v", newValString))
	}
	if newIncident.Location.Description != nil {
		update.LocationDescription = conv.StringToSql(newIncident.Location.Description, 0)
		logs = append(logs, fmt.Sprintf("Changed location description: %v", update.LocationDescription.String))
	}
	err = imsDBQ.UpdateIncident(ctx, txn, update)
	if err != nil {
		return herr.InternalServerError("Failed to update incident", err).From("[UpdateIncident]")
	}

	if newIncident.RangerHandles != nil {
		add := sliceSubtract(*newIncident.RangerHandles, rangerHandles)
		sub := sliceSubtract(rangerHandles, *newIncident.RangerHandles)
		if len(add) > 0 {
			logs = append(logs, fmt.Sprintf("Added Ranger: %v", strings.Join(add, ", ")))
			for _, rh := range add {
				err = imsDBQ.AttachRangerHandleToIncident(ctx, txn, imsdb.AttachRangerHandleToIncidentParams{
					Event:          newIncident.EventID,
					IncidentNumber: newIncident.Number,
					RangerHandle:   rh,
				})
				if err != nil {
					return herr.InternalServerError("Failed to attach Ranger to Incident", err).From("[AttachRangerHandleToIncident]")
				}
			}
		}
		if len(sub) > 0 {
			logs = append(logs, fmt.Sprintf("Removed Ranger: %v", strings.Join(sub, ", ")))
			for _, rh := range sub {
				err = imsDBQ.DetachRangerHandleFromIncident(ctx, txn,
					imsdb.DetachRangerHandleFromIncidentParams{
						Event:          newIncident.EventID,
						IncidentNumber: newIncident.Number,
						RangerHandle:   rh,
					},
				)
				if err != nil {
					return herr.InternalServerError("Failed to detach Ranger from Incident", err).From("[DetachRangerHandleFromIncident]")
				}
			}
		}
	}
	if newIncident.IncidentTypeIDs != nil {
		add := sliceSubtract(*newIncident.IncidentTypeIDs, incidentTypeIDs)
		sub := sliceSubtract(incidentTypeIDs, *newIncident.IncidentTypeIDs)
		if len(add) > 0 {
			names := namesForIncidentTypes(allIncidentTypes, add)
			logs = append(logs, fmt.Sprintf("Added type: %v", names))
			for _, itype := range add {
				err = imsDBQ.AttachIncidentTypeToIncident(ctx, txn,
					imsdb.AttachIncidentTypeToIncidentParams{
						Event:          newIncident.EventID,
						IncidentNumber: newIncident.Number,
						IncidentType:   itype,
					},
				)
				if err != nil {
					return herr.InternalServerError("Failed to add Incident Type", err).From("[AttachIncidentTypeToIncident]")
				}
			}
		}
		if len(sub) > 0 {
			names := namesForIncidentTypes(allIncidentTypes, add)
			logs = append(logs, fmt.Sprintf("Removed type: %v", names))
			for _, rh := range sub {
				err = imsDBQ.DetachIncidentTypeFromIncident(ctx, txn,
					imsdb.DetachIncidentTypeFromIncidentParams{
						Event:          newIncident.EventID,
						IncidentNumber: newIncident.Number,
						IncidentType:   rh,
					},
				)
				if err != nil {
					return herr.InternalServerError("Failed to detach Incident Type", err).From("[AttachIncidentTypeToIncident]")
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
				err = imsDBQ.AttachFieldReportToIncident(ctx, txn,
					imsdb.AttachFieldReportToIncidentParams{
						Event:          newIncident.EventID,
						Number:         frNum,
						IncidentNumber: sql.NullInt32{Int32: newIncident.Number, Valid: true},
					},
				)
				if err != nil {
					return herr.InternalServerError("Failed to attach Field Report", err).From("[AttachFieldReportToIncident]")
				}
			}
		}
		if len(sub) > 0 {
			logs = append(logs, fmt.Sprintf("Field Report removed: %v", sub))
			for _, frNum := range sub {
				err = imsDBQ.AttachFieldReportToIncident(ctx, txn,
					imsdb.AttachFieldReportToIncidentParams{
						Event:          newIncident.EventID,
						Number:         frNum,
						IncidentNumber: sql.NullInt32{},
					},
				)
				if err != nil {
					return herr.InternalServerError("Failed to detach Field Report", err).From("[AttachFieldReportToIncident]")
				}
			}
		}
	}
	var updatedLinkedIncidents []imsjson.LinkedIncident
	if newIncident.LinkedIncidents != nil {
		var currentLinkedIncidents []imsjson.LinkedIncident
		for _, cli := range linkedIncidents {
			currentLinkedIncidents = append(currentLinkedIncidents, imsjson.LinkedIncident{
				EventID: cli.LinkedEvent,
				Number:  cli.LinkedIncident,
			})
		}
		var desiredLinkedIncidents []imsjson.LinkedIncident
		for _, dli := range *newIncident.LinkedIncidents {
			desiredLinkedIncidents = append(desiredLinkedIncidents, imsjson.LinkedIncident{
				EventID: dli.EventID,
				Number:  dli.Number,
			})
		}

		add := sliceSubtract(desiredLinkedIncidents, currentLinkedIncidents)
		sub := sliceSubtract(currentLinkedIncidents, desiredLinkedIncidents)
		updatedLinkedIncidents = append(updatedLinkedIncidents, add...)
		updatedLinkedIncidents = append(updatedLinkedIncidents, sub...)

		if len(add) > 0 {
			names := namesForLinkedIncidents(add, eventNameById)
			logs = append(logs, fmt.Sprintf("Incident linked: %v", names))
			for _, otherIncident := range add {
				err = imsDBQ.LinkIncidents(ctx, txn,
					imsdb.LinkIncidentsParams{
						Event1:          newIncident.EventID,
						IncidentNumber1: newIncident.Number,
						Event2:          otherIncident.EventID,
						IncidentNumber2: otherIncident.Number,
					},
				)
				if err != nil {
					// We'll just assume in this case that the problem is that the otherIncident ID
					// is invalid. This is probably the case...
					return herr.BadRequest(fmt.Sprintf("Failed to link Incident. There may be no IMS #%v for the given event.", otherIncident.Number), err).From("[LinkIncidents]")
				}
				err = imsDBQ.LinkIncidents(ctx, txn,
					imsdb.LinkIncidentsParams{
						Event2:          newIncident.EventID,
						IncidentNumber2: newIncident.Number,
						Event1:          otherIncident.EventID,
						IncidentNumber1: otherIncident.Number,
					},
				)
				if err != nil {
					return herr.InternalServerError("Failed to link Incident", err).From("[LinkIncidents]")
				}
				_, errHTTP := addIncidentReportEntry(
					ctx, imsDBQ, txn, otherIncident.EventID, otherIncident.Number,
					author, fmt.Sprintf("Incident linked: %v #%v", eventNameById[newIncident.EventID],
						newIncident.Number,
					), true, "", "", "",
				)
				if errHTTP != nil {
					return errHTTP.From("[addIncidentReportEntry]")
				}
			}
		}
		if len(sub) > 0 {
			names := namesForLinkedIncidents(sub, eventNameById)
			logs = append(logs, fmt.Sprintf("Incident unlinked: %v", names))
			for _, otherIncident := range sub {
				err = imsDBQ.UnlinkIncidents(ctx, txn,
					imsdb.UnlinkIncidentsParams{
						Event1:          newIncident.EventID,
						IncidentNumber1: newIncident.Number,
						Event2:          otherIncident.EventID,
						IncidentNumber2: otherIncident.Number,
					},
				)
				if err != nil {
					return herr.InternalServerError("Failed to unlink Incident", err).From("[UnlinkIncidents]")
				}
				err = imsDBQ.UnlinkIncidents(ctx, txn,
					imsdb.UnlinkIncidentsParams{
						Event2:          newIncident.EventID,
						IncidentNumber2: newIncident.Number,
						Event1:          otherIncident.EventID,
						IncidentNumber1: otherIncident.Number,
					},
				)
				if err != nil {
					return herr.InternalServerError("Failed to unlink Incident", err).From("[UnlinkIncidents]")
				}
				_, errHTTP := addIncidentReportEntry(
					ctx, imsDBQ, txn, otherIncident.EventID, otherIncident.Number,
					author, fmt.Sprintf("Incident unlinked: %v #%v", eventNameById[newIncident.EventID],
						newIncident.Number,
					), true, "", "", "",
				)
				if errHTTP != nil {
					return errHTTP.From("[addIncidentReportEntry]")
				}
			}
		}
	}

	if len(logs) > 0 {
		_, errHTTP := addIncidentReportEntry(ctx, imsDBQ, txn, newIncident.EventID, newIncident.Number, author, strings.Join(logs, "\n"), true, "", "", "")
		if errHTTP != nil {
			return errHTTP.From("[addIncidentReportEntry]")
		}
	}

	for _, entry := range newIncident.ReportEntries {
		if entry.Text == "" {
			continue
		}
		_, errHTTP := addIncidentReportEntry(ctx, imsDBQ, txn, newIncident.EventID, newIncident.Number, author, entry.Text, false, "", "", "")
		if errHTTP != nil {
			return errHTTP.From("[addIncidentReportEntry]")
		}
	}

	err = txn.Commit()
	if err != nil {
		return herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	es.notifyIncidentUpdateV2(newIncident.EventID, newIncident.Number)
	for _, fr := range updatedFieldReports {
		es.notifyFieldReportUpdateV2(newIncident.EventID, fr)
	}
	for _, inc := range updatedLinkedIncidents {
		es.notifyIncidentUpdateV2(inc.EventID, inc.Number)
	}

	return nil
}

func namesForIncidentTypes(rows []imsdb.IncidentTypesRow, typeIDs []int32) string {
	var names []string
	for _, row := range rows {
		if slices.Contains(typeIDs, row.IncidentType.ID) {
			names = append(names, row.IncidentType.Name)
		}
	}
	return strings.Join(names, ", ")
}

func namesForLinkedIncidents(linked []imsjson.LinkedIncident, eventNamesById map[int32]string) string {
	var names []string
	for _, link := range linked {
		names = append(names, fmt.Sprintf("%v #%v", eventNamesById[link.EventID], link.Number))
	}
	return strings.Join(names, ", ")
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
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action EditIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.editIncident(req)
	if errHTTP != nil {
		errHTTP.From("[editIncident]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action EditIncident) editIncident(req *http.Request) *herr.HTTPError {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteIncidents == 0 {
		return herr.Forbidden("The requestor does not have EventWriteIncidents permission for this Event", nil)
	}
	ctx := req.Context()

	incidentNumber, err := conv.ParseInt32(req.PathValue("incidentNumber"))
	if err != nil {
		return herr.BadRequest("Invalid Incident Number", err).From("[ParseInt32]")
	}
	newIncident, errHTTP := readBodyAs[imsjson.Incident](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}
	newIncident.Event = event.Name
	newIncident.EventID = event.ID
	newIncident.Number = incidentNumber

	author := jwtCtx.Claims.RangerHandle()

	errHTTP = updateIncident(ctx, action.imsDBQ, action.es, newIncident, author)
	if errHTTP != nil {
		return errHTTP.From("[updateIncident]")
	}

	return nil
}
