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
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/burningmantech/ranger-ims-go/directory"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"golang.org/x/sync/errgroup"
)

// GetIncidents lists an Event's Incidents, slimmed down to what the Incidents
// table shows. See listFilter for the optional filters it takes.
type GetIncidents struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	imsAdmins []string
}

func (action GetIncidents) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getIncidents(req)
	if errHTTP != nil {
		errHTTP.From("[getIncidents]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}

func (action GetIncidents) getIncidents(req *http.Request) (imsjson.IncidentListItems, *herr.HTTPError) {
	resp := make(imsjson.IncidentListItems, 0)
	event, _, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return resp, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventReadIncidents == 0 {
		return nil, herr.Forbidden("The requestor does not have EventReadIncidents permission", nil)
	}
	filter, errHTTP := parseListFilter(req)
	if errHTTP != nil {
		return nil, errHTTP.From("[parseListFilter]")
	}

	if filter.hasNumber {
		item, errHTTP := action.getIncidentListItem(req.Context(), event, filter.number)
		if errHTTP != nil {
			return nil, errHTTP.From("[getIncidentListItem]")
		}
		return append(resp, item), nil
	}

	// These queries all request a lot of data, and we can query and process
	// those results concurrently.
	group, groupCtx := errgroup.WithContext(req.Context())

	entriesByIncident := make(map[int32][]imsdb.ReportEntry)
	group.Go(func() error {
		reportEntries, err := action.imsDBQ.Incidents_ReportEntries(
			groupCtx,
			action.imsDBQ,
			imsdb.Incidents_ReportEntriesParams{
				Event:     event.ID,
				Generated: true,
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

	rangersByIncident := make(map[int32][]imsdb.IncidentRanger)
	group.Go(func() error {
		rangersRows, err := action.imsDBQ.Incidents_Rangers(groupCtx, action.imsDBQ, event.ID)
		if err != nil {
			return herr.InternalServerError("Failed to fetch rangers", err).From("[Incidents_Rangers]")
		}
		for _, row := range rangersRows {
			rangersByIncident[row.IncidentRanger.IncidentNumber] = append(rangersByIncident[row.IncidentRanger.IncidentNumber], row.IncidentRanger)
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

	var corpus incidentSearchCorpus
	if filter.match != nil {
		corpus = action.fetchIncidentSearchCorpus(groupCtx, group, event.ID, eventPermissions)
	}

	err := group.Wait()
	if err != nil {
		return resp, herr.AsHTTPError(err)
	}

	for _, r := range incidentsRows {
		// The conversion from IncidentsRow to IncidentRow works because the Incident and Incidents
		// query row structs currently have the same fields in the same order. If that changes in the
		// future, this won't compile, and we may need to duplicate the readExtraIncidentRowFields
		// function.
		incidentRow := imsdb.IncidentRow(r)
		entries := entriesByIncident[r.Incident.Number]
		sortEntries(entries)

		item, errHTTP := incidentToListItem(incidentRow, rangersByIncident[r.Incident.Number], entries, event)
		if errHTTP != nil {
			return resp, errHTTP.From("[incidentToListItem]")
		}
		if filter.match != nil && !filter.match(corpus.searchText(incidentRow.Incident, item, entries)) {
			continue
		}
		resp = append(resp, item)
	}

	return resp, nil
}

func (action GetIncidents) getIncidentListItem(ctx context.Context, event imsdb.Event, incidentNumber int32) (
	imsjson.IncidentListItem, *herr.HTTPError,
) {
	storedRow, entries, errHTTP := fetchIncident(ctx, action.imsDBQ, event.ID, incidentNumber)
	if errHTTP != nil {
		return imsjson.IncidentListItem{}, errHTTP.From("[fetchIncident]")
	}
	rangersRows, err := action.imsDBQ.Incident_Rangers(ctx, action.imsDBQ, imsdb.Incident_RangersParams{
		Event:          event.ID,
		IncidentNumber: incidentNumber,
	})
	if err != nil {
		return imsjson.IncidentListItem{}, herr.InternalServerError("Failed to fetch rangers", err).From("[Incident_Rangers]")
	}
	rangers := make([]imsdb.IncidentRanger, len(rangersRows))
	for i, row := range rangersRows {
		rangers[i] = row.IncidentRanger
	}
	sortEntries(entries)
	return incidentToListItem(storedRow, rangers, entries, event)
}

// incidentToListItem slims an Incident down for the Incidents list. The
// entries must be sorted.
func incidentToListItem(storedRow imsdb.IncidentRow, incidentRangers []imsdb.IncidentRanger,
	entries []imsdb.ReportEntry, event imsdb.Event,
) (imsjson.IncidentListItem, *herr.HTTPError) {
	incidentTypeIDs, fieldReportNumbers, visitNumbers, err := readExtraIncidentRowFields(storedRow)
	if err != nil {
		return imsjson.IncidentListItem{}, herr.InternalServerError("Failed to fetch Incident details", err).From("[readExtraIncidentRowFields]")
	}
	rangersJSON := make([]imsjson.IncidentRanger, len(incidentRangers))
	for i, ir := range incidentRangers {
		rangersJSON[i] = imsjson.IncidentRanger{
			Handle: ir.RangerHandle,
			Role:   conv.SqlToString(ir.Role),
		}
	}
	inc := storedRow.Incident
	return imsjson.IncidentListItem{
		Event:        event.Name,
		EventID:      event.ID,
		Number:       inc.Number,
		Created:      conv.FloatToTime(inc.Created),
		LastModified: lastModified(inc.Created, entries),
		State:        string(inc.State),
		Started:      conv.FloatToTime(inc.Started),
		Closed:       conv.NullFloatToTime(inc.Closed),
		Priority:     inc.Priority,
		Summary:      listSummary(inc.Summary.String, entries),
		Location: imsjson.Location{
			Name:        conv.SqlToString(inc.LocationName),
			Address:     conv.SqlToString(inc.LocationAddress),
			Description: conv.SqlToString(inc.LocationDescription),
		},
		IncidentTypeIDs: incidentTypeIDs,
		FieldReports:    fieldReportNumbers,
		Visits:          visitNumbers,
		Rangers:         rangersJSON,
	}, nil
}

// incidentSearchCorpus is what an Incident search matches beyond the
// Incident's own fields: the names of its types, and the text of its attached
// Field Reports and Visits.
type incidentSearchCorpus struct {
	typeNames       map[int32]string
	fieldReportText map[int32]string
	visitText       map[int32]string
}

// fetchIncidentSearchCorpus adds the queries for an incidentSearchCorpus to
// group. The corpus is filled in once the group is done. Field Reports and
// Visits only contribute text when the requestor could read them anyway.
func (action GetIncidents) fetchIncidentSearchCorpus(
	ctx context.Context, group *errgroup.Group, eventID int32, eventPermissions authz.EventPermissionMask,
) incidentSearchCorpus {
	corpus := incidentSearchCorpus{
		typeNames:       make(map[int32]string),
		fieldReportText: make(map[int32]string),
		visitText:       make(map[int32]string),
	}
	group.Go(func() error {
		types, err := action.imsDBQ.IncidentTypes(ctx, action.imsDBQ)
		if err != nil {
			return herr.InternalServerError("Failed to fetch Incident Types", err).From("[IncidentTypes]")
		}
		for _, t := range types {
			corpus.typeNames[t.IncidentType.ID] = t.IncidentType.Name
		}
		return nil
	})
	if eventPermissions&authz.EventReadAllFieldReports != 0 {
		group.Go(func() error {
			frs, err := action.imsDBQ.FieldReports(ctx, action.imsDBQ, eventID)
			if err != nil {
				return herr.InternalServerError("Failed to fetch Field Reports", err).From("[FieldReports]")
			}
			entries, err := action.imsDBQ.FieldReports_ReportEntries(ctx, action.imsDBQ,
				imsdb.FieldReports_ReportEntriesParams{Event: eventID, Generated: false})
			if err != nil {
				return herr.InternalServerError("Failed to fetch FR report entries", err).From("[FieldReports_ReportEntries]")
			}
			docs := make(map[int32]*searchDoc)
			for _, fr := range frs {
				doc := &searchDoc{}
				doc.add(fr.FieldReport.Summary.String)
				docs[fr.FieldReport.Number] = doc
			}
			for _, row := range entries {
				if doc := docs[row.FieldReportNumber]; doc != nil {
					doc.addEntries([]imsdb.ReportEntry{row.ReportEntry})
				}
			}
			for number, doc := range docs {
				corpus.fieldReportText[number] = doc.String()
			}
			return nil
		})
	}
	if eventPermissions&authz.EventReadVisits != 0 {
		group.Go(func() error {
			visits, err := action.imsDBQ.Visits(ctx, action.imsDBQ, eventID)
			if err != nil {
				return herr.InternalServerError("Failed to fetch Visits", err).From("[Visits]")
			}
			entries, err := action.imsDBQ.Visits_ReportEntries(ctx, action.imsDBQ,
				imsdb.Visits_ReportEntriesParams{Event: eventID, Generated: false})
			if err != nil {
				return herr.InternalServerError("Failed to fetch Visit report entries", err).From("[Visits_ReportEntries]")
			}
			docs := make(map[int32]*searchDoc)
			for _, v := range visits {
				doc := &searchDoc{}
				doc.add(v.Visit.GuestPreferredName.String, v.Visit.GuestLegalName.String, v.Visit.GuestDescription.String)
				docs[v.Visit.Number] = doc
			}
			for _, row := range entries {
				if doc := docs[row.VisitNumber]; doc != nil {
					doc.addEntries([]imsdb.ReportEntry{row.ReportEntry})
				}
			}
			for number, doc := range docs {
				corpus.visitText[number] = doc.String()
			}
			return nil
		})
	}
	return corpus
}

// searchText is everything an Incident search looks through for this
// Incident, i.e. what the Incidents table used to search on the client.
func (c incidentSearchCorpus) searchText(
	inc imsdb.Incident, item imsjson.IncidentListItem, entries []imsdb.ReportEntry,
) string {
	var doc searchDoc
	doc.add(strconv.Itoa(int(inc.Number)), inc.Summary.String)
	doc.addEntries(entries)
	for _, typeID := range item.IncidentTypeIDs {
		doc.add(c.typeNames[typeID])
	}
	doc.add(inc.LocationName.String, inc.LocationAddress.String)
	for _, r := range item.Rangers {
		doc.add(r.Handle)
	}
	for _, frNumber := range item.FieldReports {
		doc.add(c.fieldReportText[frNumber])
	}
	for _, visitNumber := range item.Visits {
		doc.add(c.visitText[visitNumber])
	}
	return doc.String()
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

	rangersRows, err := action.imsDBQ.Incident_Rangers(ctx, action.imsDBQ, imsdb.Incident_RangersParams{
		Event:          event.ID,
		IncidentNumber: incidentNumber,
	})
	if err != nil {
		return resp, herr.InternalServerError("Failed to fetch rangers", err)
	}
	rangers := make([]imsdb.IncidentRanger, len(rangersRows))
	for i, row := range rangersRows {
		rangers[i] = row.IncidentRanger
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

	resp, errHTTP = incidentToJSON(storedRow, rangers, reportEntries, linkedIncidents, event, action.attachmentsEnabled)
	if errHTTP != nil {
		return resp, errHTTP.From("[incidentToJSON]")
	}
	return resp, nil
}

func incidentToJSON(storedRow imsdb.IncidentRow, incidentRangers []imsdb.IncidentRanger,
	reportEntries []imsdb.ReportEntry, linkedIncidents []imsdb.Incident_LinkedIncidentsRow,
	event imsdb.Event, attachmentsEnabled bool,
) (imsjson.Incident, *herr.HTTPError) {
	var resp imsjson.Incident
	resultEntries := make([]imsjson.ReportEntry, len(reportEntries))
	for i, re := range reportEntries {
		resultEntries[i] = reportEntryToJSON(re, attachmentsEnabled)
	}

	linkedIncidentJson := make([]imsjson.LinkedIncident, len(linkedIncidents))
	for i, li := range linkedIncidents {
		linkedIncidentJson[i] = imsjson.LinkedIncident{
			EventID:   li.LinkedEvent,
			EventName: li.LinkedEventName,
			Number:    li.LinkedIncident,
			Summary:   li.LinkedIncidentSummary.String,
		}
	}

	rangersJson := make([]imsjson.IncidentRanger, len(incidentRangers))
	for i, ir := range incidentRangers {
		rangersJson[i] = imsjson.IncidentRanger{
			Handle: ir.RangerHandle,
			Role:   conv.SqlToString(ir.Role),
		}
	}

	incidentTypeIDs, fieldReportNumbers, visitNumbers, err := readExtraIncidentRowFields(storedRow)
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
		Version:      storedRow.Incident.Version,
		State:        string(storedRow.Incident.State),
		Started:      conv.FloatToTime(storedRow.Incident.Started),
		Closed:       conv.NullFloatToTime(storedRow.Incident.Closed),
		Priority:     storedRow.Incident.Priority,
		Summary:      conv.SqlToString(storedRow.Incident.Summary),
		Location: imsjson.Location{
			Name:        conv.SqlToString(storedRow.Incident.LocationName),
			Address:     conv.SqlToString(storedRow.Incident.LocationAddress),
			Description: conv.SqlToString(storedRow.Incident.LocationDescription),
		},
		IncidentTypeIDs: &incidentTypeIDs,
		FieldReports:    &fieldReportNumbers,
		Visits:          &visitNumbers,
		Rangers:         &rangersJson,
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

	// First create the incident, to lock in the incident number reservation.
	// The number is allocated with a plain SELECT, so a concurrent creator in
	// the same event can claim the same number first; the (EVENT, NUMBER)
	// primary key turns that into a duplicate-key error, and the INSERT is
	// retried with a freshly allocated number.
	now := conv.TimeToFloat(time.Now())
	var newIncidentNumber int32
	for attempt := 1; ; attempt++ {
		var err error
		newIncidentNumber, err = action.imsDBQ.NextIncidentNumber(ctx, action.imsDBQ, event.ID)
		if err != nil {
			return 0, "", herr.InternalServerError("Failed to find next Incident number", err).From("[NextIncidentNumber]")
		}
		_, err = action.imsDBQ.CreateIncident(ctx, action.imsDBQ, imsdb.CreateIncidentParams{
			Event:    event.ID,
			Number:   newIncidentNumber,
			Created:  now,
			Started:  now,
			Priority: imsjson.IncidentPriorityNormal,
			State:    imsdb.IncidentStateNew,
		})
		if err == nil {
			break
		}
		if !isDuplicateKeyError(err) {
			return 0, "", herr.InternalServerError("Failed to create incident", err).From("[CreateIncident]")
		}
		if attempt == maxNumberAllocAttempts {
			return 0, "", herr.Conflict("Incidents are being created concurrently. Please try again.", err).From("[CreateIncident]")
		}
	}
	newIncident.EventID = event.ID
	newIncident.Event = event.Name
	newIncident.Number = newIncidentNumber

	errHTTP = updateIncident(ctx, action.imsDBQ, action.es, newIncident, author, event.NormalizeAddresses)
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

func readExtraIncidentRowFields(row imsdb.IncidentRow) (incidentTypeIDs, fieldReportNumbers, visitNumbers []int32, err error) {
	incidentTypeIDs, err = unmarshalByteSlice[[]int32](row.IncidentTypeIds)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("[unmarshalByteSlice]: %w", err)
	}
	fieldReportNumbers, err = unmarshalByteSlice[[]int32](row.FieldReportNumbers)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("[unmarshalByteSlice]: %w", err)
	}
	visitNumbers, err = unmarshalByteSlice[[]int32](row.VisitNumbers)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("[unmarshalByteSlice]: %w", err)
	}
	return incidentTypeIDs, fieldReportNumbers, visitNumbers, nil
}

// maxCASAttempts bounds the read-merge-write retries an edit makes when a
// concurrent writer moves the record's version out from under it.
const maxCASAttempts = 3

// rejectSetReplacement refuses an edit body that carries one of the Incident's
// set-valued fields. These used to be applied by diffing the client's list
// against stored state, which silently undid a concurrent writer's add or
// remove whenever the client's list was built from a stale read. Each is now
// mutated one member at a time through its own endpoint, so those requests
// commute and need no such diff. The fields remain in the response body.
//
// Rejecting is deliberate: ignoring them would report success for a change
// that never happened.
func rejectSetReplacement(newIncident imsjson.Incident) *herr.HTTPError {
	var msg string
	switch {
	case newIncident.IncidentTypeIDs != nil:
		msg = "incident_type_ids is read-only; use POST/DELETE .../incidents/{number}/incident_types/{incidentTypeId}"
	case newIncident.LinkedIncidents != nil:
		msg = "linked_incidents is read-only; use POST/DELETE .../incidents/{number}/linked_incidents/{eventName}/{number}"
	case newIncident.FieldReports != nil:
		msg = "field_reports is read-only; attach or detach via the Field Report endpoint"
	case newIncident.Visits != nil:
		msg = "visits is read-only; set the Visit's incident field via the Visit endpoint"
	default:
		return nil
	}
	return herr.BadRequest(msg, nil).SetExpectedError()
}

func updateIncident(ctx context.Context, imsDBQ *store.DBQ, es *EventSourcerer, newIncident imsjson.Incident, author string,
	normalizeAddresses bool,
) *herr.HTTPError {
	errHTTP := rejectSetReplacement(newIncident)
	if errHTTP != nil {
		return errHTTP.From("[rejectSetReplacement]")
	}
	for range maxCASAttempts {
		conflict, errHTTP := retryOnDeadlock(func() (bool, *herr.HTTPError) {
			return updateIncidentAttempt(ctx, imsDBQ, es, newIncident, author, normalizeAddresses)
		})
		if errHTTP != nil {
			return errHTTP.From("[updateIncidentAttempt]")
		}
		if !conflict {
			return nil
		}
	}
	return herr.Conflict("The incident is being modified concurrently. Please try again.", nil)
}

func updateIncidentAttempt(ctx context.Context, imsDBQ *store.DBQ, es *EventSourcerer, newIncident imsjson.Incident, author string,
	normalizeAddresses bool,
) (conflict bool, errHTTP *herr.HTTPError) {
	storedIncidentRow, err := imsDBQ.Incident(ctx, imsDBQ,
		imsdb.IncidentParams{
			Event:  newIncident.EventID,
			Number: newIncident.Number,
		},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, herr.NotFound("Incident not found", err).From("[Incident]")
		}
		return false, herr.InternalServerError("Failed to fetch incident", err).From("[Incident]")
	}
	storedIncident := storedIncidentRow.Incident

	txn, err := imsDBQ.Begin()
	if err != nil {
		return false, herr.InternalServerError("Failed to start transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	update, logs := buildIncidentUpdate(storedIncident, newIncident, normalizeAddresses)

	// A request that only appends report entries is applied without the guarded
	// update below: appends can't lose data, so they must not conflict with
	// concurrent field edits.
	if len(logs) > 0 {
		// The version-guarded update is the concurrency gate for everything in
		// this transaction: if another writer committed since the read above,
		// it affects zero rows and the whole edit is retried rather than
		// clobbering the other writer's changes. Once it succeeds, the row lock
		// it takes serializes any competing writers until commit.
		rows, err := imsDBQ.UpdateIncident(ctx, txn, update)
		if err != nil {
			return false, herr.InternalServerError("Failed to update incident", err).From("[UpdateIncident]")
		}
		if rows == 0 {
			// Stale version or vanished row; re-read to tell which.
			_, err = imsDBQ.IncidentVersion(ctx, imsDBQ, imsdb.IncidentVersionParams{
				Event:  newIncident.EventID,
				Number: newIncident.Number,
			})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return false, herr.NotFound("Incident not found", err).From("[IncidentVersion]")
				}
				return false, herr.InternalServerError("Failed to fetch incident", err).From("[IncidentVersion]")
			}
			return true, nil
		}
	}

	errHTTP = addChangeReportEntries(ctx, imsDBQ, txn, newIncident.EventID, newIncident.Number, author,
		logs, newIncident.ReportEntries, addIncidentReportEntry)
	if errHTTP != nil {
		return false, errHTTP.From("[addChangeReportEntries]")
	}

	err = txn.Commit()
	if err != nil {
		return false, herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	es.notifyIncidentUpdate(newIncident.EventID, newIncident.Number)

	return false, nil
}

// buildIncidentUpdate merges the client-provided fields of newIncident over the
// stored Incident, returning the update parameters along with change-log lines
// describing each modified field.
func buildIncidentUpdate(stored imsdb.Incident, newIncident imsjson.Incident, normalizeAddresses bool,
) (imsdb.UpdateIncidentParams, []string) {
	update := imsdb.UpdateIncidentParams{
		Event:               stored.Event,
		Number:              stored.Number,
		Version:             stored.Version,
		Priority:            stored.Priority,
		State:               stored.State,
		Started:             stored.Started,
		Closed:              stored.Closed,
		Summary:             stored.Summary,
		LocationName:        stored.LocationName,
		LocationAddress:     stored.LocationAddress,
		LocationDescription: stored.LocationDescription,
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
	applyStringChange(&update.Summary, newIncident.Summary, "summary", &logs)
	applyStringChange(&update.LocationName, newIncident.Location.Name, "location name", &logs)
	applyStringChange(&update.LocationAddress, normalizedAddress(newIncident.Location.Address, normalizeAddresses), "location address", &logs)
	applyStringChange(&update.LocationDescription, newIncident.Location.Description, "location description", &logs)

	return update, logs
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

	errHTTP = updateIncident(ctx, action.imsDBQ, action.es, newIncident, author, event.NormalizeAddresses)
	if errHTTP != nil {
		return errHTTP.From("[updateIncident]")
	}

	return nil
}
