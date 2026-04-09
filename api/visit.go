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
		Incident:     conv.SqlToInt32(storedRow.Visit.IncidentNumber),

		GuestPreferredName:   conv.SqlToString(storedRow.Visit.GuestPreferredName),
		GuestLegalName:       conv.SqlToString(storedRow.Visit.GuestLegalName),
		GuestDescription:     conv.SqlToString(storedRow.Visit.GuestDescription),
		GuestCampName:        conv.SqlToString(storedRow.Visit.GuestCampName),
		GuestCampAddress:     conv.SqlToString(storedRow.Visit.GuestCampAddress),
		GuestCampDescription: conv.SqlToString(storedRow.Visit.GuestCampDescription),

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
	number, location, errHTTP := action.newVisit(req)
	if errHTTP != nil {
		errHTTP.From("[newVisit]").WriteResponse(w)
		return
	}

	w.Header().Set("IMS-Visit-Number", strconv.Itoa(int(number)))
	w.Header().Set("Location", location)
	herr.WriteCreatedResponse(w, http.StatusText(http.StatusCreated))
}
func (action NewVisit) newVisit(req *http.Request) (visitNumber int32, location string, errHTTP *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return 0, "", errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteVisits == 0 {
		return 0, "", herr.Forbidden("The requestor does not have EventWriteVisits permission on this Event", nil)
	}
	ctx := req.Context()
	newVisit, errHTTP := readBodyAs[imsjson.Visit](req)
	if errHTTP != nil {
		return 0, "", errHTTP.From("[readBodyAs]")
	}

	author := jwtCtx.Claims.RangerHandle()

	// First create the visit, to lock in the visit number reservation
	newVisitNumber, err := action.imsDBQ.NextVisitNumber(ctx, action.imsDBQ, event.ID)
	if err != nil {
		return 0, "", herr.InternalServerError("Failed to find next Visit number", err).From("[NextVisitNumber]")
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
		return 0, "", herr.InternalServerError("Failed to create visit", err).From("[CreateVisit]")
	}

	errHTTP = updateVisit(ctx, action.imsDBQ, action.es, newVisit, author)
	if errHTTP != nil {
		return 0, "", errHTTP.From("[updateVisit]")
	}

	return newVisit.Number, fmt.Sprintf("/ims/api/events/%v/visits/%d", event.Name, newVisit.Number), nil
}

func updateVisit(ctx context.Context, imsDBQ *store.DBQ, es *EventSourcerer, newVisit imsjson.Visit, author string,
) *herr.HTTPError {
	storedVisitRow, err := imsDBQ.Visit(ctx, imsDBQ,
		imsdb.VisitParams{
			Event:  newVisit.EventID,
			Number: newVisit.Number,
		},
	)
	if err != nil {
		return herr.InternalServerError("Failed to fetch visit", err).From("[Visit]")
	}
	storedVisit := storedVisitRow.Visit

	allEvents, err := imsDBQ.Events(ctx, imsDBQ)
	if err != nil {
		return herr.InternalServerError("Failed to fetch events", err).From("[Events]")
	}
	eventNameById := make(map[int32]string)
	for _, event := range allEvents {
		eventNameById[event.Event.ID] = event.Event.Name
	}

	txn, err := imsDBQ.Begin()
	if err != nil {
		return herr.InternalServerError("Failed to start transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	update := imsdb.UpdateVisitParams{
		Event:                storedVisit.Event,
		Number:               storedVisit.Number,
		IncidentNumber:       storedVisit.IncidentNumber,
		GuestPreferredName:   storedVisit.GuestPreferredName,
		GuestLegalName:       storedVisit.GuestLegalName,
		GuestDescription:     storedVisit.GuestDescription,
		GuestCampName:        storedVisit.GuestCampName,
		GuestCampAddress:     storedVisit.GuestCampAddress,
		GuestCampDescription: storedVisit.GuestCampDescription,
		ArrivalTime:          storedVisit.ArrivalTime,
		ArrivalMethod:        storedVisit.ArrivalMethod,
		ArrivalState:         storedVisit.ArrivalState,
		ArrivalReason:        storedVisit.ArrivalReason,
		ArrivalBelongings:    storedVisit.ArrivalBelongings,
		DepartureTime:        storedVisit.DepartureTime,
		DepartureMethod:      storedVisit.DepartureMethod,
		DepartureState:       storedVisit.DepartureState,
		ResourceRest:         storedVisit.ResourceRest,
		ResourceClothes:      storedVisit.ResourceClothes,
		ResourcePogs:         storedVisit.ResourcePogs,
		ResourceFoodBev:      storedVisit.ResourceFoodBev,
		ResourceOther:        storedVisit.ResourceOther,
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

	if newVisit.GuestPreferredName != nil {
		update.GuestPreferredName = conv.StringToSql(newVisit.GuestPreferredName, 0)
		logs = append(logs, fmt.Sprintf("Changed GuestPreferredName: %v", update.GuestPreferredName.String))
	}
	if newVisit.GuestLegalName != nil {
		update.GuestLegalName = conv.StringToSql(newVisit.GuestLegalName, 0)
		logs = append(logs, fmt.Sprintf("Changed GuestLegalName: %v", update.GuestLegalName.String))
	}
	if newVisit.GuestDescription != nil {
		update.GuestDescription = conv.StringToSql(newVisit.GuestDescription, 0)
		logs = append(logs, fmt.Sprintf("Changed GuestDescription: %v", update.GuestDescription.String))
	}
	if newVisit.GuestCampName != nil {
		update.GuestCampName = conv.StringToSql(newVisit.GuestCampName, 0)
		logs = append(logs, fmt.Sprintf("Changed GuestCampName: %v", update.GuestCampName.String))
	}
	if newVisit.GuestCampAddress != nil {
		update.GuestCampAddress = conv.StringToSql(newVisit.GuestCampAddress, 0)
		logs = append(logs, fmt.Sprintf("Changed GuestCampAddress: %v", update.GuestCampAddress.String))
	}
	if newVisit.GuestCampDescription != nil {
		update.GuestCampDescription = conv.StringToSql(newVisit.GuestCampDescription, 0)
		logs = append(logs, fmt.Sprintf("Changed GuestCampDescription: %v", update.GuestCampDescription.String))
	}

	if newVisit.ArrivalTime != nil {
		update.ArrivalTime = conv.TimeToNullFloat(*newVisit.ArrivalTime)
		if update.ArrivalTime.Valid && update.DepartureTime.Valid && update.DepartureTime.Float64 < update.ArrivalTime.Float64 {
			return herr.BadRequest("Arrival time cannot be after departure time", errors.New("arrival time cannot be after departure time"))
		}
		logs = append(logs, fmt.Sprintf("Changed ArrivalTime: %v", newVisit.ArrivalTime.In(time.UTC).Format(time.RFC3339)))
	}
	if newVisit.ArrivalMethod != nil {
		update.ArrivalMethod = conv.StringToSql(newVisit.ArrivalMethod, 0)
		logs = append(logs, fmt.Sprintf("Changed ArrivalMethod: %v", update.ArrivalMethod.String))
	}
	if newVisit.ArrivalState != nil {
		update.ArrivalState = conv.StringToSql(newVisit.ArrivalState, 0)
		logs = append(logs, fmt.Sprintf("Changed ArrivalState: %v", update.ArrivalState.String))
	}
	if newVisit.ArrivalReason != nil {
		update.ArrivalReason = conv.StringToSql(newVisit.ArrivalReason, 0)
		logs = append(logs, fmt.Sprintf("Changed ArrivalReason: %v", update.ArrivalReason.String))
	}
	if newVisit.ArrivalBelongings != nil {
		update.ArrivalBelongings = conv.StringToSql(newVisit.ArrivalBelongings, 0)
		logs = append(logs, fmt.Sprintf("Changed ArrivalBelongings: %v", update.ArrivalBelongings.String))
	}

	if newVisit.DepartureTime != nil {
		update.DepartureTime = conv.TimeToNullFloat(*newVisit.DepartureTime)
		if update.ArrivalTime.Valid && update.DepartureTime.Valid && update.DepartureTime.Float64 < update.ArrivalTime.Float64 {
			return herr.BadRequest("Departure time cannot be before arrival time", errors.New("departure time cannot be before arrival time"))
		}
		logs = append(logs, fmt.Sprintf("Changed DepartureTime: %v", newVisit.DepartureTime.In(time.UTC).Format(time.RFC3339)))
	}
	if newVisit.DepartureMethod != nil {
		update.DepartureMethod = conv.StringToSql(newVisit.DepartureMethod, 0)
		logs = append(logs, fmt.Sprintf("Changed DepartureMethod: %v", update.DepartureMethod.String))
	}
	if newVisit.DepartureState != nil {
		update.DepartureState = conv.StringToSql(newVisit.DepartureState, 0)
		logs = append(logs, fmt.Sprintf("Changed DepartureState: %v", update.DepartureState.String))
	}

	if newVisit.ResourceRest != nil {
		update.ResourceRest = conv.StringToSql(newVisit.ResourceRest, 0)
		logs = append(logs, fmt.Sprintf("Changed ResourceRest: %v", update.ResourceRest.String))
	}
	if newVisit.ResourceClothes != nil {
		update.ResourceClothes = conv.StringToSql(newVisit.ResourceClothes, 0)
		logs = append(logs, fmt.Sprintf("Changed ResourceClothes: %v", update.ResourceClothes.String))
	}
	if newVisit.ResourcePogs != nil {
		update.ResourcePogs = conv.StringToSql(newVisit.ResourcePogs, 0)
		logs = append(logs, fmt.Sprintf("Changed ResourcePogs: %v", update.ResourcePogs.String))
	}
	if newVisit.ResourceFoodBev != nil {
		update.ResourceFoodBev = conv.StringToSql(newVisit.ResourceFoodBev, 0)
		logs = append(logs, fmt.Sprintf("Changed ResourceFoodBev: %v", update.ResourceFoodBev.String))
	}
	if newVisit.ResourceOther != nil {
		update.ResourceOther = conv.StringToSql(newVisit.ResourceOther, 0)
		logs = append(logs, fmt.Sprintf("Changed ResourceOther: %v", update.ResourceOther.String))
	}

	err = imsDBQ.UpdateVisit(ctx, txn, update)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		const mySQLErNoReferencedRow2 = 1452
		if errors.As(err, &mysqlErr) && mysqlErr.Number == mySQLErNoReferencedRow2 {
			// This is probably the source of the error, because there are no other foreign
			// keys updates within this function.
			return herr.NotFound("No such Incident", err).From("[UpdateVisit]")
		}
		return herr.InternalServerError("Failed to update visit", err).From("[UpdateVisit]")
	}

	if len(logs) > 0 {
		_, errHTTP := addVisitReportEntry(ctx, imsDBQ, txn, newVisit.EventID, newVisit.Number, author, strings.Join(logs, "\n"), true, "", "", "")
		if errHTTP != nil {
			return errHTTP.From("[addVisitReportEntry]")
		}
	}

	for _, entry := range newVisit.ReportEntries {
		if entry.Text == "" {
			continue
		}
		_, errHTTP := addVisitReportEntry(ctx, imsDBQ, txn, newVisit.EventID, newVisit.Number, author, entry.Text, false, "", "", "")
		if errHTTP != nil {
			return errHTTP.From("[addVisitReportEntry]")
		}
	}

	err = txn.Commit()
	if err != nil {
		return herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	es.notifyVisitUpdate(storedVisit.Event, storedVisit.Number)
	es.notifyIncidentUpdates(storedVisit.Event, storedVisit.IncidentNumber.Int32, update.IncidentNumber.Int32)

	return nil
}

func addVisitReportEntry(
	ctx context.Context, db *store.DBQ, dbtx imsdb.DBTX,
	eventID, visitNum int32, author, text string, generated bool,
	attachment, attachmentOriginalName, attachmentMediaType string,
) (int32, *herr.HTTPError) {
	reID64, err := db.CreateReportEntry(ctx, dbtx,
		imsdb.CreateReportEntryParams{
			Author:                   author,
			Text:                     text,
			Created:                  conv.TimeToFloat(time.Now()),
			Generated:                generated,
			Stricken:                 false,
			AttachedFile:             conv.StringToSql(&attachment, 128),
			AttachedFileOriginalName: conv.StringToSql(&attachmentOriginalName, 128),
			AttachedFileMediaType:    conv.StringToSql(&attachmentMediaType, 128),
		},
	)
	if err != nil {
		return 0, herr.InternalServerError("Failed to create report entry", err).From("[MustInt32]")
	}
	// This column is an int32, so this is safe
	reID := conv.MustInt32(reID64)
	err = db.AttachReportEntryToVisit(ctx, dbtx, imsdb.AttachReportEntryToVisitParams{
		Event:       eventID,
		VisitNumber: visitNum,
		ReportEntry: reID,
	})
	if err != nil {
		return 0, herr.InternalServerError("Failed to attach report entry", err).From("[AttachReportEntryToVisit]")
	}
	return reID, nil
}

type EditVisit struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action EditVisit) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.editVisit(req)
	if errHTTP != nil {
		errHTTP.From("[editVisit]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action EditVisit) editVisit(req *http.Request) *herr.HTTPError {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteVisits == 0 {
		return herr.Forbidden("The requestor does not have EventWriteVisits permission for this Event", nil)
	}
	ctx := req.Context()

	visitNumber, err := conv.ParseInt32(req.PathValue("visitNumber"))
	if err != nil {
		return herr.BadRequest("Invalid Visit Number", err).From("[ParseInt32]")
	}
	newVisit, errHTTP := readBodyAs[imsjson.Visit](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}
	newVisit.Event = event.Name
	newVisit.EventID = event.ID
	newVisit.Number = visitNumber

	author := jwtCtx.Claims.RangerHandle()

	errHTTP = updateVisit(ctx, action.imsDBQ, action.es, newVisit, author)
	if errHTTP != nil {
		return errHTTP.From("[updateVisit]")
	}

	return nil
}

type AttachRangerToVisit struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action AttachRangerToVisit) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.attachRanger(req)
	if errHTTP != nil {
		errHTTP.From("[attachRanger]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action AttachRangerToVisit) attachRanger(req *http.Request) *herr.HTTPError {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteVisits == 0 {
		return herr.Forbidden("The requestor does not have EventWriteVisits permission for this Event", nil)
	}
	ctx := req.Context()

	visitNumber, err := conv.ParseInt32(req.PathValue("visitNumber"))
	if err != nil {
		return herr.BadRequest("Invalid Visit Number", err).From("[ParseInt32]")
	}

	rangerName := req.PathValue("rangerName")
	if rangerName == "" {
		return herr.BadRequest("Empty Ranger Name", nil)
	}

	body, errHTTP := readBodyAs[imsjson.VisitRanger](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}
	txn, err := action.imsDBQ.Begin()
	if err != nil {
		return herr.InternalServerError("Failed to start transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	err = action.imsDBQ.DetachRangerFromVisit(ctx, txn, imsdb.DetachRangerFromVisitParams{
		Event:        event.ID,
		VisitNumber:  visitNumber,
		RangerHandle: rangerName,
	})
	if err != nil {
		return herr.InternalServerError("Failed to detach Ranger from Visit", err).From("[DetachRangerFromVisit]")
	}

	err = action.imsDBQ.AttachRangerToVisit(ctx, txn, imsdb.AttachRangerToVisitParams{
		Event:        event.ID,
		VisitNumber:  visitNumber,
		RangerHandle: rangerName,
		Role:         conv.StringToSql(body.Role, 128),
	})
	if err != nil {
		return herr.InternalServerError("Failed to attach Ranger to Visit", err).From("[AttachRangerToVisit]")
	}

	_, errHTTP = addVisitReportEntry(
		ctx, action.imsDBQ, txn, event.ID, visitNumber,
		jwtCtx.Claims.RangerHandle(), fmt.Sprintf("Added Ranger: %v", rangerName),
		true, "", "", "",
	)
	if errHTTP != nil {
		return errHTTP.From("[addVisitReportEntry]")
	}
	err = txn.Commit()
	if err != nil {
		return herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	action.es.notifyVisitUpdate(event.ID, visitNumber)

	return nil
}

type DetachRangerFromVisit struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action DetachRangerFromVisit) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.detachRanger(req)
	if errHTTP != nil {
		errHTTP.From("[detachRanger]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action DetachRangerFromVisit) detachRanger(req *http.Request) *herr.HTTPError {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteVisits == 0 {
		return herr.Forbidden("The requestor does not have EventWriteVisits permission for this Event", nil)
	}
	ctx := req.Context()

	visitNumber, err := conv.ParseInt32(req.PathValue("visitNumber"))
	if err != nil {
		return herr.BadRequest("Invalid Visit Number", err).From("[ParseInt32]")
	}

	rangerName := req.PathValue("rangerName")
	if rangerName == "" {
		return herr.BadRequest("Empty Ranger Name", nil)
	}

	txn, err := action.imsDBQ.Begin()
	if err != nil {
		return herr.InternalServerError("Failed to start transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	err = action.imsDBQ.DetachRangerFromVisit(ctx, txn, imsdb.DetachRangerFromVisitParams{
		Event:        event.ID,
		VisitNumber:  visitNumber,
		RangerHandle: rangerName,
	})
	if err != nil {
		return herr.InternalServerError("Failed to detach Ranger from Visit", err).From("[DetachRangerFromVisit]")
	}
	_, errHTTP = addVisitReportEntry(
		ctx, action.imsDBQ, txn, event.ID, visitNumber,
		jwtCtx.Claims.RangerHandle(), fmt.Sprintf("Removed Ranger: %v", rangerName),
		true, "", "", "",
	)
	if errHTTP != nil {
		return errHTTP.From("[addVisitReportEntry]")
	}

	err = txn.Commit()
	if err != nil {
		return herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	action.es.notifyVisitUpdate(event.ID, visitNumber)

	return nil
}
