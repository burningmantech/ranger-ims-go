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
	"net/http"
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
	"github.com/go-sql-driver/mysql"
	"golang.org/x/sync/errgroup"
)

type GetVisits struct {
	imsDBQ             *store.DBQ
	userStore          *directory.UserStore
	imsAdmins          []string
	attachmentsEnabled bool
}

func (action GetVisits) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getVisits(req)
	if errHTTP != nil {
		errHTTP.From("[getVisits]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}

func (action GetVisits) getVisits(req *http.Request) (imsjson.Visits, *herr.HTTPError) {
	resp := make(imsjson.Visits, 0)
	event, _, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return resp, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventReadVisits == 0 {
		return nil, herr.Forbidden("The requestor does not have EventReadVisits permission", nil)
	}
	err := req.ParseForm()
	if err != nil {
		return nil, herr.BadRequest("Failed to parse form", err)
	}
	includeSystemEntries := !strings.EqualFold(req.Form.Get("exclude_system_entries"), "true")

	// The Visits and ReportEntries queries both request a lot of data, and we can query
	// and process those results concurrently.
	group, groupCtx := errgroup.WithContext(req.Context())

	entriesByVisit := make(map[int32][]imsdb.ReportEntry)
	group.Go(func() error {
		reportEntries, err := action.imsDBQ.Visits_ReportEntries(
			groupCtx,
			action.imsDBQ,
			imsdb.Visits_ReportEntriesParams{
				Event:     event.ID,
				Generated: includeSystemEntries,
			},
		)
		if err != nil {
			return herr.InternalServerError("Failed to fetch Visit Report Entries", err).From("[Visits_ReportEntries]")
		}
		for _, row := range reportEntries {
			entriesByVisit[row.VisitNumber] = append(
				entriesByVisit[row.VisitNumber],
				row.ReportEntry,
			)
		}
		return nil
	})

	rangersByVisit := make(map[int32][]imsdb.VisitRanger)
	group.Go(func() error {
		rangersRows, err := action.imsDBQ.Visits_Rangers(groupCtx, action.imsDBQ, event.ID)
		if err != nil {
			return herr.InternalServerError("Failed to fetch rangers", err).From("[Visits_Rangers]")
		}
		for _, row := range rangersRows {
			rangersByVisit[row.VisitRanger.VisitNumber] = append(rangersByVisit[row.VisitRanger.VisitNumber], row.VisitRanger)
		}
		return nil
	})

	var visitsRows []imsdb.VisitsRow
	group.Go(func() error {
		var err error
		visitsRows, err = action.imsDBQ.Visits(groupCtx, action.imsDBQ, event.ID)
		if err != nil {
			return herr.InternalServerError("Failed to fetch Visits", err).From("[Visits]")
		}
		return nil
	})
	err = group.Wait()
	if err != nil {
		return resp, herr.AsHTTPError(err)
	}

	for _, r := range visitsRows {
		// The conversion from VisitsRow to VisitRow works because the Visit and Visits
		// query row structs currently have the same fields in the same order.
		visitRow := imsdb.VisitRow(r)

		visitJSON, errHTTP := visitToJSON(visitRow, rangersByVisit[r.Visit.Number], entriesByVisit[r.Visit.Number], event, action.attachmentsEnabled)
		if errHTTP != nil {
			return resp, errHTTP.From("[visitToJSON]")
		}
		resp = append(resp, visitJSON)
	}

	return resp, nil
}

type GetVisit struct {
	imsDBQ             *store.DBQ
	userStore          *directory.UserStore
	imsAdmins          []string
	attachmentsEnabled bool
}

func (action GetVisit) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getVisit(req)
	if errHTTP != nil {
		errHTTP.From("[getVisit]").WriteResponse(w)
		return
	}
	setETag(w, resp.Version)
	mustWriteJSON(w, req, resp)
}

func (action GetVisit) getVisit(req *http.Request) (imsjson.Visit, *herr.HTTPError) {
	var resp imsjson.Visit

	event, _, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return resp, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventReadVisits == 0 {
		return resp, herr.Forbidden("The requestor does not have EventReadVisits permission on this Event", nil)
	}
	ctx := req.Context()

	visitNumber, err := conv.ParseInt32(req.PathValue("visitNumber"))
	if err != nil {
		return resp, herr.BadRequest("Failed to parse visit number", err)
	}

	storedRow, reportEntries, errHTTP := fetchVisit(ctx, action.imsDBQ, event.ID, visitNumber)
	if errHTTP != nil {
		return resp, errHTTP.From("[fetchVisit]")
	}

	rangersRows, err := action.imsDBQ.Visit_Rangers(ctx, action.imsDBQ, imsdb.Visit_RangersParams{
		Event:       event.ID,
		VisitNumber: visitNumber,
	})
	if err != nil {
		return resp, herr.InternalServerError("Failed to fetch rangers", err)
	}
	rangers := make([]imsdb.VisitRanger, len(rangersRows))
	for i, row := range rangersRows {
		rangers[i] = row.VisitRanger
	}

	resp, errHTTP = visitToJSON(storedRow, rangers, reportEntries, event, action.attachmentsEnabled)
	if errHTTP != nil {
		return resp, errHTTP.From("[visitToJSON]")
	}
	return resp, nil
}

func fetchVisit(ctx context.Context, imsDBQ *store.DBQ, eventID, visitNumber int32) (
	imsdb.VisitRow, []imsdb.ReportEntry, *herr.HTTPError,
) {
	var empty imsdb.VisitRow
	var reportEntries []imsdb.ReportEntry
	visitRow, err := imsDBQ.Visit(ctx, imsDBQ,
		imsdb.VisitParams{
			Event:  eventID,
			Number: visitNumber,
		},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return empty, nil, herr.NotFound("Visit not found", err).From("[Visit]")
		}
		return empty, nil, herr.InternalServerError("Failed to fetch Visit", err).From("[Visit]")
	}
	reportEntryRows, err := imsDBQ.Visit_ReportEntries(ctx, imsDBQ,
		imsdb.Visit_ReportEntriesParams{
			Event:       eventID,
			VisitNumber: visitNumber,
		},
	)
	if err != nil {
		return empty, nil, herr.InternalServerError("Failed to fetch report entries", err).From("[Visit_ReportEntries]")
	}
	for _, rer := range reportEntryRows {
		reportEntries = append(reportEntries, rer.ReportEntry)
	}
	return visitRow, reportEntries, nil
}

func visitToJSON(storedRow imsdb.VisitRow, visitRangers []imsdb.VisitRanger,
	reportEntries []imsdb.ReportEntry, event imsdb.Event, attachmentsEnabled bool,
) (imsjson.Visit, *herr.HTTPError) {
	var resp imsjson.Visit
	resultEntries := make([]imsjson.ReportEntry, len(reportEntries))
	for i, re := range reportEntries {
		resultEntries[i] = reportEntryToJSON(re, attachmentsEnabled)
	}

	rangersJson := make([]imsjson.VisitRanger, len(visitRangers))
	for i, ir := range visitRangers {
		rangersJson[i] = imsjson.VisitRanger{
			Handle: ir.RangerHandle,
			Role:   conv.SqlToString(ir.Role),
		}
	}

	lastModified := conv.FloatToTime(storedRow.Visit.Created)
	for _, re := range resultEntries {
		if re.Created.After(lastModified) {
			lastModified = re.Created
		}
	}
	resp = imsjson.Visit{
		Event:        event.Name,
		EventID:      event.ID,
		Number:       storedRow.Visit.Number,
		Created:      conv.FloatToTime(storedRow.Visit.Created),
		LastModified: lastModified,
		Version:      storedRow.Visit.Version,
		Incident:     conv.SqlToInt32(storedRow.Visit.IncidentNumber),

		GuestPreferredName:   conv.SqlToString(storedRow.Visit.GuestPreferredName),
		GuestLegalName:       conv.SqlToString(storedRow.Visit.GuestLegalName),
		GuestDescription:     conv.SqlToString(storedRow.Visit.GuestDescription),
		GuestActionPlan:      conv.SqlToString(storedRow.Visit.GuestActionPlan),
		ResourceBedID:        conv.SqlToString(storedRow.Visit.ResourceBedID),
		GuestCampName:        conv.SqlToString(storedRow.Visit.GuestCampName),
		GuestCampAddress:     conv.SqlToString(storedRow.Visit.GuestCampAddress),
		GuestCampDescription: conv.SqlToString(storedRow.Visit.GuestCampDescription),
		GuestCampContacts:    conv.SqlToString(storedRow.Visit.GuestCampContacts),
		ResourceSitter:       conv.SqlToString(storedRow.Visit.ResourceSitter),

		ArrivalTime:       conv.NullFloatToTimePtr(storedRow.Visit.ArrivalTime),
		ArrivalMethod:     conv.SqlToString(storedRow.Visit.ArrivalMethod),
		ArrivalState:      conv.SqlToString(storedRow.Visit.ArrivalState),
		ArrivalReason:     conv.SqlToString(storedRow.Visit.ArrivalReason),
		ArrivalBelongings: conv.SqlToString(storedRow.Visit.ArrivalBelongings),

		DepartureTime:   conv.NullFloatToTimePtr(storedRow.Visit.DepartureTime),
		DepartureMethod: conv.SqlToString(storedRow.Visit.DepartureMethod),
		DepartureState:  conv.SqlToString(storedRow.Visit.DepartureState),

		ResourceRest:    conv.SqlToString(storedRow.Visit.ResourceRest),
		ResourceClothes: conv.SqlToString(storedRow.Visit.ResourceClothes),
		ResourcePogs:    conv.SqlToString(storedRow.Visit.ResourcePogs),
		ResourceFoodBev: conv.SqlToString(storedRow.Visit.ResourceFoodBev),
		ResourceOther:   conv.SqlToString(storedRow.Visit.ResourceOther),

		Rangers:       &rangersJson,
		ReportEntries: resultEntries,
	}
	return resp, nil
}

type NewVisit struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action NewVisit) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	number, version, location, errHTTP := action.newVisit(req)
	if errHTTP != nil {
		errHTTP.From("[newVisit]").WriteResponse(w)
		return
	}

	w.Header().Set("IMS-Visit-Number", strconv.Itoa(int(number)))
	w.Header().Set("Location", location)
	setETag(w, version)
	herr.WriteCreatedResponse(w, http.StatusText(http.StatusCreated))
}
func (action NewVisit) newVisit(req *http.Request) (visitNumber, version int32, location string, errHTTP *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return 0, 0, "", errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteVisits == 0 {
		return 0, 0, "", herr.Forbidden("The requestor does not have EventWriteVisits permission on this Event", nil)
	}
	ctx := req.Context()
	newVisit, errHTTP := readBodyAs[imsjson.Visit](req)
	if errHTTP != nil {
		return 0, 0, "", errHTTP.From("[readBodyAs]")
	}

	author := jwtCtx.Claims.RangerHandle()

	// First create the visit, to lock in the visit number reservation
	newVisitNumber, err := action.imsDBQ.NextVisitNumber(ctx, action.imsDBQ, event.ID)
	if err != nil {
		return 0, 0, "", herr.InternalServerError("Failed to find next Visit number", err).From("[NextVisitNumber]")
	}
	newVisit.EventID = event.ID
	newVisit.Event = event.Name
	newVisit.Number = newVisitNumber
	now := conv.TimeToFloat(time.Now())
	createTheVisit := imsdb.CreateVisitParams{
		Event:   newVisit.EventID,
		Number:  newVisitNumber,
		Created: now,
	}
	_, err = action.imsDBQ.CreateVisit(ctx, action.imsDBQ, createTheVisit)
	if err != nil {
		return 0, 0, "", herr.InternalServerError("Failed to create visit", err).From("[CreateVisit]")
	}

	version, errHTTP = updateVisit(ctx, action.imsDBQ, action.es, newVisit, author, nil)
	if errHTTP != nil {
		return 0, 0, "", errHTTP.From("[updateVisit]")
	}

	return newVisit.Number, version, fmt.Sprintf("/ims/api/events/%v/visits/%d", event.Name, newVisit.Number), nil
}

func updateVisit(ctx context.Context, imsDBQ *store.DBQ, es *EventSourcerer, newVisit imsjson.Visit, author string,
	ifMatch *int32,
) (newVersion int32, errHTTP *herr.HTTPError) {
	attempts := maxCASAttempts
	if ifMatch != nil {
		attempts = 1
	}
	for range attempts {
		version, conflict, errHTTP := updateVisitAttempt(ctx, imsDBQ, es, newVisit, author, ifMatch)
		if errHTTP != nil {
			return 0, errHTTP.From("[updateVisitAttempt]")
		}
		if !conflict {
			return version, nil
		}
		if ifMatch != nil {
			return 0, herr.PreconditionFailed(
				"This visit was changed by someone else while you were editing it", nil,
			).SetExpectedError()
		}
	}
	return 0, herr.Conflict("The visit is being modified concurrently. Please try again.", nil)
}

func updateVisitAttempt(ctx context.Context, imsDBQ *store.DBQ, es *EventSourcerer, newVisit imsjson.Visit, author string,
	ifMatch *int32,
) (newVersion int32, conflict bool, errHTTP *herr.HTTPError) {
	storedVisitRow, err := imsDBQ.Visit(ctx, imsDBQ,
		imsdb.VisitParams{
			Event:  newVisit.EventID,
			Number: newVisit.Number,
		},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, herr.NotFound("Visit not found", err).From("[Visit]")
		}
		return 0, false, herr.InternalServerError("Failed to fetch visit", err).From("[Visit]")
	}
	storedVisit := storedVisitRow.Visit
	expectedVersion := storedVisit.Version
	if ifMatch != nil && *ifMatch != expectedVersion {
		return 0, true, nil
	}

	txn, err := imsDBQ.Begin()
	if err != nil {
		return 0, false, herr.InternalServerError("Failed to start transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	update, logs, errHTTP := buildVisitUpdate(storedVisit, newVisit)
	if errHTTP != nil {
		return 0, false, errHTTP.From("[buildVisitUpdate]")
	}

	// A request that only appends report entries is applied without the
	// guarded update below; see updateIncidentAttempt.
	changesVisit := len(logs) > 0
	newVersion = expectedVersion
	if changesVisit {
		// The version-guarded update is the concurrency gate; see updateIncidentAttempt.
		rows, err := imsDBQ.UpdateVisit(ctx, txn, update)
		if err != nil {
			const mySQLErNoReferencedRow2 = 1452
			mysqlErr, ok := errors.AsType[*mysql.MySQLError](err)
			if ok && mysqlErr.Number == mySQLErNoReferencedRow2 {
				// This is probably the source of the error, because there are no other foreign
				// keys updates within this function.
				return 0, false, herr.NotFound("No such Incident", err).From("[UpdateVisit]")
			}
			return 0, false, herr.InternalServerError("Failed to update visit", err).From("[UpdateVisit]")
		}
		if rows == 0 {
			// Stale version or vanished row; re-read to tell which.
			_, err = imsDBQ.VisitVersion(ctx, imsDBQ, imsdb.VisitVersionParams{
				Event:  newVisit.EventID,
				Number: newVisit.Number,
			})
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return 0, false, herr.NotFound("Visit not found", err).From("[VisitVersion]")
				}
				return 0, false, herr.InternalServerError("Failed to fetch visit", err).From("[VisitVersion]")
			}
			return 0, true, nil
		}
		newVersion = expectedVersion + 1

		// If the visit was reassigned, the affected incidents' visit lists
		// changed, so their versions must move too.
		if storedVisit.IncidentNumber != update.IncidentNumber {
			for _, incidentNumber := range []sql.NullInt32{storedVisit.IncidentNumber, update.IncidentNumber} {
				if !incidentNumber.Valid {
					continue
				}
				err = imsDBQ.BumpIncidentVersion(ctx, txn, imsdb.BumpIncidentVersionParams{
					Event:  newVisit.EventID,
					Number: incidentNumber.Int32,
				})
				if err != nil {
					return 0, false, herr.InternalServerError("Failed to update incident", err).From("[BumpIncidentVersion]")
				}
			}
		}
	}

	errHTTP = addChangeReportEntries(ctx, imsDBQ, txn, newVisit.EventID, newVisit.Number, author,
		logs, newVisit.ReportEntries, addVisitReportEntry)
	if errHTTP != nil {
		return 0, false, errHTTP.From("[addChangeReportEntries]")
	}

	err = txn.Commit()
	if err != nil {
		return 0, false, herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	es.notifyVisitUpdate(storedVisit.Event, storedVisit.Number)
	es.notifyIncidentUpdates(storedVisit.Event, storedVisit.IncidentNumber.Int32, update.IncidentNumber.Int32)

	return newVersion, false, nil
}

// buildVisitUpdate merges the client-provided fields of newVisit over the
// stored Visit, returning the update parameters along with change-log lines
// describing each modified field. It rejects updates that would put the
// arrival time after the departure time.
func buildVisitUpdate(stored imsdb.Visit, newVisit imsjson.Visit) (imsdb.UpdateVisitParams, []string, *herr.HTTPError) {
	update := imsdb.UpdateVisitParams{
		Event:                stored.Event,
		Number:               stored.Number,
		Version:              stored.Version,
		IncidentNumber:       stored.IncidentNumber,
		GuestPreferredName:   stored.GuestPreferredName,
		GuestLegalName:       stored.GuestLegalName,
		GuestDescription:     stored.GuestDescription,
		GuestActionPlan:      stored.GuestActionPlan,
		ResourceBedID:        stored.ResourceBedID,
		GuestCampName:        stored.GuestCampName,
		GuestCampAddress:     stored.GuestCampAddress,
		GuestCampDescription: stored.GuestCampDescription,
		GuestCampContacts:    stored.GuestCampContacts,
		ResourceSitter:       stored.ResourceSitter,
		ArrivalTime:          stored.ArrivalTime,
		ArrivalMethod:        stored.ArrivalMethod,
		ArrivalState:         stored.ArrivalState,
		ArrivalReason:        stored.ArrivalReason,
		ArrivalBelongings:    stored.ArrivalBelongings,
		DepartureTime:        stored.DepartureTime,
		DepartureMethod:      stored.DepartureMethod,
		DepartureState:       stored.DepartureState,
		ResourceRest:         stored.ResourceRest,
		ResourceClothes:      stored.ResourceClothes,
		ResourcePogs:         stored.ResourcePogs,
		ResourceFoodBev:      stored.ResourceFoodBev,
		ResourceOther:        stored.ResourceOther,
	}

	var logs []string

	if newVisit.Incident != nil {
		newIncNum := sql.NullInt32{
			Int32: *newVisit.Incident,
			// Nonpositive numbers unassign the Visit from the Incident
			Valid: *newVisit.Incident > 0,
		}
		update.IncidentNumber = newIncNum
		if newIncNum.Valid {
			logs = append(logs, fmt.Sprintf("Changed incident number: %v", *newVisit.Incident))
		} else {
			logs = append(logs, "Changed incident number: (unassigned)")
		}
	}

	applyStringChange(&update.GuestPreferredName, newVisit.GuestPreferredName, "GuestPreferredName", &logs)
	applyStringChange(&update.GuestLegalName, newVisit.GuestLegalName, "GuestLegalName", &logs)
	applyStringChange(&update.GuestDescription, newVisit.GuestDescription, "GuestDescription", &logs)
	applyStringChange(&update.GuestActionPlan, newVisit.GuestActionPlan, "GuestActionPlan", &logs)
	applyStringChange(&update.ResourceBedID, newVisit.ResourceBedID, "ResourceBedID", &logs)
	applyStringChange(&update.GuestCampName, newVisit.GuestCampName, "GuestCampName", &logs)
	applyStringChange(&update.GuestCampAddress, newVisit.GuestCampAddress, "GuestCampAddress", &logs)
	applyStringChange(&update.GuestCampDescription, newVisit.GuestCampDescription, "GuestCampDescription", &logs)
	applyStringChange(&update.GuestCampContacts, newVisit.GuestCampContacts, "GuestCampContacts", &logs)
	applyStringChange(&update.ResourceSitter, newVisit.ResourceSitter, "ResourceSitter", &logs)

	if newVisit.ArrivalTime != nil {
		update.ArrivalTime = conv.TimeToNullFloat(*newVisit.ArrivalTime)
		if update.ArrivalTime.Valid && update.DepartureTime.Valid && update.DepartureTime.Float64 < update.ArrivalTime.Float64 {
			return update, nil, herr.BadRequest("Arrival time cannot be after departure time", errors.New("arrival time cannot be after departure time"))
		}
		logs = append(logs, fmt.Sprintf("Changed ArrivalTime: %v", newVisit.ArrivalTime.In(time.UTC).Format(time.RFC3339)))
	}
	applyStringChange(&update.ArrivalMethod, newVisit.ArrivalMethod, "ArrivalMethod", &logs)
	applyStringChange(&update.ArrivalState, newVisit.ArrivalState, "ArrivalState", &logs)
	applyStringChange(&update.ArrivalReason, newVisit.ArrivalReason, "ArrivalReason", &logs)
	applyStringChange(&update.ArrivalBelongings, newVisit.ArrivalBelongings, "ArrivalBelongings", &logs)

	if newVisit.DepartureTime != nil {
		update.DepartureTime = conv.TimeToNullFloat(*newVisit.DepartureTime)
		if update.ArrivalTime.Valid && update.DepartureTime.Valid && update.DepartureTime.Float64 < update.ArrivalTime.Float64 {
			return update, nil, herr.BadRequest("Departure time cannot be before arrival time", errors.New("departure time cannot be before arrival time"))
		}
		logs = append(logs, fmt.Sprintf("Changed DepartureTime: %v", newVisit.DepartureTime.In(time.UTC).Format(time.RFC3339)))
	}
	applyStringChange(&update.DepartureMethod, newVisit.DepartureMethod, "DepartureMethod", &logs)
	applyStringChange(&update.DepartureState, newVisit.DepartureState, "DepartureState", &logs)

	applyStringChange(&update.ResourceRest, newVisit.ResourceRest, "ResourceRest", &logs)
	applyStringChange(&update.ResourceClothes, newVisit.ResourceClothes, "ResourceClothes", &logs)
	applyStringChange(&update.ResourcePogs, newVisit.ResourcePogs, "ResourcePogs", &logs)
	applyStringChange(&update.ResourceFoodBev, newVisit.ResourceFoodBev, "ResourceFoodBev", &logs)
	applyStringChange(&update.ResourceOther, newVisit.ResourceOther, "ResourceOther", &logs)

	return update, logs, nil
}

type EditVisit struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action EditVisit) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	version, errHTTP := action.editVisit(req)
	if errHTTP != nil {
		errHTTP.From("[editVisit]").WriteResponse(w)
		return
	}
	setETag(w, version)
	herr.WriteNoContentResponse(w, "Success")
}

func (action EditVisit) editVisit(req *http.Request) (int32, *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return 0, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteVisits == 0 {
		return 0, herr.Forbidden("The requestor does not have EventWriteVisits permission for this Event", nil)
	}
	ctx := req.Context()

	visitNumber, err := conv.ParseInt32(req.PathValue("visitNumber"))
	if err != nil {
		return 0, herr.BadRequest("Invalid Visit Number", err).From("[ParseInt32]")
	}
	ifMatch, errHTTP := parseIfMatch(req)
	if errHTTP != nil {
		return 0, errHTTP.From("[parseIfMatch]")
	}
	newVisit, errHTTP := readBodyAs[imsjson.Visit](req)
	if errHTTP != nil {
		return 0, errHTTP.From("[readBodyAs]")
	}
	newVisit.Event = event.Name
	newVisit.EventID = event.ID
	newVisit.Number = visitNumber

	author := jwtCtx.Claims.RangerHandle()

	version, errHTTP := updateVisit(ctx, action.imsDBQ, action.es, newVisit, author, ifMatch)
	if errHTTP != nil {
		return 0, errHTTP.From("[updateVisit]")
	}

	return version, nil
}
