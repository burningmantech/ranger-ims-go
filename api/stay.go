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
	"golang.org/x/sync/errgroup"
)

type GetStays struct {
	imsDBQ             *store.DBQ
	userStore          *directory.UserStore
	imsAdmins          []string
	attachmentsEnabled bool
}

func (action GetStays) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getStays(req)
	if errHTTP != nil {
		errHTTP.From("[getStays]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}

func (action GetStays) getStays(req *http.Request) (imsjson.Stays, *herr.HTTPError) {
	resp := make(imsjson.Stays, 0)
	event, _, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return resp, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventReadStays == 0 {
		return nil, herr.Forbidden("The requestor does not have EventReadStays permission", nil)
	}
	err := req.ParseForm()
	if err != nil {
		return nil, herr.BadRequest("Failed to parse form", err)
	}
	includeSystemEntries := !strings.EqualFold(req.Form.Get("exclude_system_entries"), "true")

	// The Stays and ReportEntries queries both request a lot of data, and we can query
	// and process those results concurrently.
	group, groupCtx := errgroup.WithContext(req.Context())

	entriesByStay := make(map[int32][]imsdb.ReportEntry)
	group.Go(func() error {
		reportEntries, err := action.imsDBQ.Stays_ReportEntries(
			groupCtx,
			action.imsDBQ,
			imsdb.Stays_ReportEntriesParams{
				Event:     event.ID,
				Generated: includeSystemEntries,
			},
		)
		if err != nil {
			return herr.InternalServerError("Failed to fetch Stay Report Entries", err).From("[Stays_ReportEntries]")
		}
		for _, row := range reportEntries {
			entriesByStay[row.StayNumber] = append(
				entriesByStay[row.StayNumber],
				row.ReportEntry,
			)
		}
		return nil
	})

	rangersByStay := make(map[int32][]imsdb.StayRanger)
	group.Go(func() error {
		rangersRows, err := action.imsDBQ.Stays_Rangers(groupCtx, action.imsDBQ, event.ID)
		if err != nil {
			return herr.InternalServerError("Failed to fetch rangers", err).From("[Stays_Rangers]")
		}
		for _, row := range rangersRows {
			rangersByStay[row.StayRanger.StayNumber] = append(rangersByStay[row.StayRanger.StayNumber], row.StayRanger)
		}
		return nil
	})

	var staysRows []imsdb.StaysRow
	group.Go(func() error {
		var err error
		staysRows, err = action.imsDBQ.Stays(groupCtx, action.imsDBQ, event.ID)
		if err != nil {
			return herr.InternalServerError("Failed to fetch Stays", err).From("[Stays]")
		}
		return nil
	})
	err = group.Wait()
	if err != nil {
		return resp, herr.AsHTTPError(err)
	}

	for _, r := range staysRows {
		// The conversion from StaysRow to StayRow works because the Stay and Stays
		// query row structs currently have the same fields in the same order.
		stayRow := imsdb.StayRow(r)

		stayJSON, errHTTP := stayToJSON(stayRow, rangersByStay[r.Stay.Number], entriesByStay[r.Stay.Number], event, action.attachmentsEnabled)
		if errHTTP != nil {
			return resp, errHTTP.From("[stayToJSON]")
		}
		resp = append(resp, stayJSON)
	}

	return resp, nil
}

type GetStay struct {
	imsDBQ             *store.DBQ
	userStore          *directory.UserStore
	imsAdmins          []string
	attachmentsEnabled bool
}

func (action GetStay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resp, errHTTP := action.getStay(req)
	if errHTTP != nil {
		errHTTP.From("[getStay]").WriteResponse(w)
		return
	}
	mustWriteJSON(w, req, resp)
}

func (action GetStay) getStay(req *http.Request) (imsjson.Stay, *herr.HTTPError) {
	var resp imsjson.Stay

	event, _, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return resp, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventReadStays == 0 {
		return resp, herr.Forbidden("The requestor does not have EventReadStays permission on this Event", nil)
	}
	ctx := req.Context()

	stayNumber, err := conv.ParseInt32(req.PathValue("stayNumber"))
	if err != nil {
		return resp, herr.BadRequest("Failed to parse stay number", err)
	}

	storedRow, reportEntries, errHTTP := fetchStay(ctx, action.imsDBQ, event.ID, stayNumber)
	if errHTTP != nil {
		return resp, errHTTP.From("[fetchStay]")
	}

	rangersRows, err := action.imsDBQ.Stay_Rangers(ctx, action.imsDBQ, imsdb.Stay_RangersParams{
		Event:      event.ID,
		StayNumber: stayNumber,
	})
	if err != nil {
		return resp, herr.InternalServerError("Failed to fetch rangers", err)
	}
	rangers := make([]imsdb.StayRanger, len(rangersRows))
	for i, row := range rangersRows {
		rangers[i] = row.StayRanger
	}

	resp, errHTTP = stayToJSON(storedRow, rangers, reportEntries, event, action.attachmentsEnabled)
	if errHTTP != nil {
		return resp, errHTTP.From("[stayToJSON]")
	}
	return resp, nil
}

func fetchStay(ctx context.Context, imsDBQ *store.DBQ, eventID, stayNumber int32) (
	imsdb.StayRow, []imsdb.ReportEntry, *herr.HTTPError,
) {
	var empty imsdb.StayRow
	var reportEntries []imsdb.ReportEntry
	stayRow, err := imsDBQ.Stay(ctx, imsDBQ,
		imsdb.StayParams{
			Event:  eventID,
			Number: stayNumber,
		},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return empty, nil, herr.NotFound("Stay not found", err).From("[Stay]")
		}
		return empty, nil, herr.InternalServerError("Failed to fetch Stay", err).From("[Stay]")
	}
	reportEntryRows, err := imsDBQ.Stay_ReportEntries(ctx, imsDBQ,
		imsdb.Stay_ReportEntriesParams{
			Event:      eventID,
			StayNumber: stayNumber,
		},
	)
	if err != nil {
		return empty, nil, herr.InternalServerError("Failed to fetch report entries", err).From("[Stay_ReportEntries]")
	}
	for _, rer := range reportEntryRows {
		reportEntries = append(reportEntries, rer.ReportEntry)
	}
	return stayRow, reportEntries, nil
}

func stayToJSON(storedRow imsdb.StayRow, stayRangers []imsdb.StayRanger,
	reportEntries []imsdb.ReportEntry, event imsdb.Event, attachmentsEnabled bool,
) (imsjson.Stay, *herr.HTTPError) {
	var resp imsjson.Stay
	resultEntries := make([]imsjson.ReportEntry, len(reportEntries))
	for i, re := range reportEntries {
		resultEntries[i] = reportEntryToJSON(re, attachmentsEnabled)
	}

	rangersJson := make([]imsjson.StayRanger, len(stayRangers))
	for i, ir := range stayRangers {
		rangersJson[i] = imsjson.StayRanger{
			Handle: ir.RangerHandle,
			Role:   conv.SqlToString(ir.Role),
		}
	}

	lastModified := conv.FloatToTime(storedRow.Stay.Created)
	for _, re := range resultEntries {
		if re.Created.After(lastModified) {
			lastModified = re.Created
		}
	}
	resp = imsjson.Stay{
		Event:          event.Name,
		EventID:        event.ID,
		Number:         storedRow.Stay.Number,
		Created:        conv.FloatToTime(storedRow.Stay.Created),
		LastModified:   lastModified,
		IncidentNumber: conv.SqlToInt32(storedRow.Stay.IncidentNumber),

		GuestPreferredName:   conv.SqlToString(storedRow.Stay.GuestPreferredName),
		GuestLegalName:       conv.SqlToString(storedRow.Stay.GuestLegalName),
		GuestDescription:     conv.SqlToString(storedRow.Stay.GuestDescription),
		GuestCampName:        conv.SqlToString(storedRow.Stay.GuestCampName),
		GuestCampAddress:     conv.SqlToString(storedRow.Stay.GuestCampAddress),
		GuestCampDescription: conv.SqlToString(storedRow.Stay.GuestCampDescription),

		ArrivalTime:       conv.NullFloatToTimePtr(storedRow.Stay.ArrivalTime),
		ArrivalMethod:     conv.SqlToString(storedRow.Stay.ArrivalMethod),
		ArrivalState:      conv.SqlToString(storedRow.Stay.ArrivalState),
		ArrivalReason:     conv.SqlToString(storedRow.Stay.ArrivalReason),
		ArrivalBelongings: conv.SqlToString(storedRow.Stay.ArrivalBelongings),

		DepartureTime:   conv.NullFloatToTimePtr(storedRow.Stay.DepartureTime),
		DepartureMethod: conv.SqlToString(storedRow.Stay.DepartureMethod),
		DepartureState:  conv.SqlToString(storedRow.Stay.DepartureState),

		ResourceRest:    conv.SqlToString(storedRow.Stay.ResourceRest),
		ResourceClothes: conv.SqlToString(storedRow.Stay.ResourceClothes),
		ResourcePogs:    conv.SqlToString(storedRow.Stay.ResourcePogs),
		ResourceFoodBev: conv.SqlToString(storedRow.Stay.ResourceFoodBev),
		ResourceOther:   conv.SqlToString(storedRow.Stay.ResourceOther),

		Rangers:       &rangersJson,
		ReportEntries: resultEntries,
	}
	return resp, nil
}

type NewStay struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action NewStay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	number, location, errHTTP := action.newStay(req)
	if errHTTP != nil {
		errHTTP.From("[newStay]").WriteResponse(w)
		return
	}

	w.Header().Set("IMS-Stay-Number", strconv.Itoa(int(number)))
	w.Header().Set("Location", location)
	herr.WriteCreatedResponse(w, http.StatusText(http.StatusCreated))
}
func (action NewStay) newStay(req *http.Request) (stayNumber int32, location string, errHTTP *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return 0, "", errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteStays == 0 {
		return 0, "", herr.Forbidden("The requestor does not have EventWriteStays permission on this Event", nil)
	}
	ctx := req.Context()
	newStay, errHTTP := readBodyAs[imsjson.Stay](req)
	if errHTTP != nil {
		return 0, "", errHTTP.From("[readBodyAs]")
	}

	author := jwtCtx.Claims.RangerHandle()

	// First create the incident, to lock in the incident number reservation
	newStayNumber, err := action.imsDBQ.NextStayNumber(ctx, action.imsDBQ, event.ID)
	if err != nil {
		return 0, "", herr.InternalServerError("Failed to find next Stay number", err).From("[NextStayNumber]")
	}
	newStay.EventID = event.ID
	newStay.Event = event.Name
	newStay.Number = newStayNumber
	now := conv.TimeToFloat(time.Now())
	createTheStay := imsdb.CreateStayParams{
		Event:   newStay.EventID,
		Number:  newStayNumber,
		Created: now,
	}
	_, err = action.imsDBQ.CreateStay(ctx, action.imsDBQ, createTheStay)
	if err != nil {
		return 0, "", herr.InternalServerError("Failed to create stay", err).From("[CreateStay]")
	}

	errHTTP = updateStay(ctx, action.imsDBQ, action.es, newStay, author)
	if errHTTP != nil {
		return 0, "", errHTTP.From("[updateStay]")
	}

	return newStay.Number, fmt.Sprintf("/ims/api/events/%v/stays/%d", event.Name, newStay.Number), nil
}

func updateStay(ctx context.Context, imsDBQ *store.DBQ, es *EventSourcerer, newStay imsjson.Stay, author string,
) *herr.HTTPError {
	storedStayRow, err := imsDBQ.Stay(ctx, imsDBQ,
		imsdb.StayParams{
			Event:  newStay.EventID,
			Number: newStay.Number,
		},
	)
	if err != nil {
		return herr.InternalServerError("Failed to fetch stay", err).From("[Stay]")
	}
	storedStay := storedStayRow.Stay

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

	update := imsdb.UpdateStayParams{
		Event:                storedStay.Event,
		Number:               storedStay.Number,
		IncidentNumber:       storedStay.IncidentNumber,
		GuestPreferredName:   storedStay.GuestPreferredName,
		GuestLegalName:       storedStay.GuestLegalName,
		GuestDescription:     storedStay.GuestDescription,
		GuestCampName:        storedStay.GuestCampName,
		GuestCampAddress:     storedStay.GuestCampAddress,
		GuestCampDescription: storedStay.GuestCampDescription,
		ArrivalTime:          storedStay.ArrivalTime,
		ArrivalMethod:        storedStay.ArrivalMethod,
		ArrivalState:         storedStay.ArrivalState,
		ArrivalReason:        storedStay.ArrivalReason,
		ArrivalBelongings:    storedStay.ArrivalBelongings,
		DepartureTime:        storedStay.DepartureTime,
		DepartureMethod:      storedStay.DepartureMethod,
		DepartureState:       storedStay.DepartureState,
		ResourceRest:         storedStay.ResourceRest,
		ResourceClothes:      storedStay.ResourceClothes,
		ResourcePogs:         storedStay.ResourcePogs,
		ResourceFoodBev:      storedStay.ResourceFoodBev,
		ResourceOther:        storedStay.ResourceOther,
	}

	var logs []string

	if newStay.IncidentNumber != nil {
		newIncNum := sql.NullInt32{
			Int32: *newStay.IncidentNumber,
			// we treat a value of 0 as unassigning the incident
			Valid: *newStay.IncidentNumber != 0,
		}
		update.IncidentNumber = newIncNum
		logs = append(logs, fmt.Sprintf("Changed incident number: %v", newStay.IncidentNumber))
	}

	if newStay.GuestPreferredName != nil {
		update.GuestPreferredName = conv.StringToSql(newStay.GuestPreferredName, 0)
		logs = append(logs, fmt.Sprintf("Changed GuestPreferredName: %v", update.GuestPreferredName.String))
	}
	if newStay.GuestLegalName != nil {
		update.GuestLegalName = conv.StringToSql(newStay.GuestLegalName, 0)
		logs = append(logs, fmt.Sprintf("Changed GuestLegalName: %v", update.GuestLegalName.String))
	}
	if newStay.GuestDescription != nil {
		update.GuestDescription = conv.StringToSql(newStay.GuestDescription, 0)
		logs = append(logs, fmt.Sprintf("Changed GuestDescription: %v", update.GuestDescription.String))
	}
	if newStay.GuestCampName != nil {
		update.GuestCampName = conv.StringToSql(newStay.GuestCampName, 0)
		logs = append(logs, fmt.Sprintf("Changed GuestCampName: %v", update.GuestCampName.String))
	}
	if newStay.GuestCampAddress != nil {
		update.GuestCampAddress = conv.StringToSql(newStay.GuestCampAddress, 0)
		logs = append(logs, fmt.Sprintf("Changed GuestCampAddress: %v", update.GuestCampAddress.String))
	}
	if newStay.GuestCampDescription != nil {
		update.GuestCampDescription = conv.StringToSql(newStay.GuestCampDescription, 0)
		logs = append(logs, fmt.Sprintf("Changed GuestCampDescription: %v", update.GuestCampDescription.String))
	}

	if newStay.ArrivalTime != nil {
		update.ArrivalTime = conv.TimeToNullFloat(*newStay.ArrivalTime)
		logs = append(logs, fmt.Sprintf("Changed ArrivalTime: %v", newStay.ArrivalTime.In(time.UTC).Format(time.RFC3339)))
	}
	if newStay.ArrivalMethod != nil {
		update.ArrivalMethod = conv.StringToSql(newStay.ArrivalMethod, 0)
		logs = append(logs, fmt.Sprintf("Changed ArrivalMethod: %v", update.ArrivalMethod.String))
	}
	if newStay.ArrivalState != nil {
		update.ArrivalState = conv.StringToSql(newStay.ArrivalState, 0)
		logs = append(logs, fmt.Sprintf("Changed ArrivalState: %v", update.ArrivalState.String))
	}
	if newStay.ArrivalReason != nil {
		update.ArrivalReason = conv.StringToSql(newStay.ArrivalReason, 0)
		logs = append(logs, fmt.Sprintf("Changed ArrivalReason: %v", update.ArrivalReason.String))
	}
	if newStay.ArrivalBelongings != nil {
		update.ArrivalBelongings = conv.StringToSql(newStay.ArrivalBelongings, 0)
		logs = append(logs, fmt.Sprintf("Changed ArrivalBelongings: %v", update.ArrivalBelongings.String))
	}

	if newStay.DepartureTime != nil {
		update.DepartureTime = conv.TimeToNullFloat(*newStay.DepartureTime)
		logs = append(logs, fmt.Sprintf("Changed DepartureTime: %v", newStay.DepartureTime.In(time.UTC).Format(time.RFC3339)))
	}
	if newStay.DepartureMethod != nil {
		update.DepartureMethod = conv.StringToSql(newStay.DepartureMethod, 0)
		logs = append(logs, fmt.Sprintf("Changed DepartureMethod: %v", update.DepartureMethod.String))
	}
	if newStay.DepartureState != nil {
		update.DepartureState = conv.StringToSql(newStay.DepartureState, 0)
		logs = append(logs, fmt.Sprintf("Changed DepartureState: %v", update.DepartureState.String))
	}

	if newStay.ResourceRest != nil {
		update.ResourceRest = conv.StringToSql(newStay.ResourceRest, 0)
		logs = append(logs, fmt.Sprintf("Changed ResourceRest: %v", update.ResourceRest.String))
	}
	if newStay.ResourceClothes != nil {
		update.ResourceClothes = conv.StringToSql(newStay.ResourceClothes, 0)
		logs = append(logs, fmt.Sprintf("Changed ResourceClothes: %v", update.ResourceClothes.String))
	}
	if newStay.ResourcePogs != nil {
		update.ResourcePogs = conv.StringToSql(newStay.ResourcePogs, 0)
		logs = append(logs, fmt.Sprintf("Changed ResourcePogs: %v", update.ResourcePogs.String))
	}
	if newStay.ResourceFoodBev != nil {
		update.ResourceFoodBev = conv.StringToSql(newStay.ResourceFoodBev, 0)
		logs = append(logs, fmt.Sprintf("Changed ResourceFoodBev: %v", update.ResourceFoodBev.String))
	}
	if newStay.ResourceOther != nil {
		update.ResourceOther = conv.StringToSql(newStay.ResourceOther, 0)
		logs = append(logs, fmt.Sprintf("Changed ResourceOther: %v", update.ResourceOther.String))
	}

	err = imsDBQ.UpdateStay(ctx, txn, update)
	if err != nil {
		return herr.InternalServerError("Failed to update stay", err).From("[UpdateStay]")
	}

	if len(logs) > 0 {
		_, errHTTP := addStayReportEntry(ctx, imsDBQ, txn, newStay.EventID, newStay.Number, author, strings.Join(logs, "\n"), true, "", "", "")
		if errHTTP != nil {
			return errHTTP.From("[addStayReportEntry]")
		}
	}

	for _, entry := range newStay.ReportEntries {
		if entry.Text == "" {
			continue
		}
		_, errHTTP := addStayReportEntry(ctx, imsDBQ, txn, newStay.EventID, newStay.Number, author, entry.Text, false, "", "", "")
		if errHTTP != nil {
			return errHTTP.From("[addStayReportEntry]")
		}
	}

	err = txn.Commit()
	if err != nil {
		return herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	es.notifyStayUpdate(storedStay.Event, storedStay.Number)
	es.notifyIncidentUpdates(storedStay.Event, storedStay.IncidentNumber.Int32, update.IncidentNumber.Int32)

	return nil
}

func addStayReportEntry(
	ctx context.Context, db *store.DBQ, dbtx imsdb.DBTX,
	eventID, stayNum int32, author, text string, generated bool,
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
	err = db.AttachReportEntryToStay(ctx, dbtx, imsdb.AttachReportEntryToStayParams{
		Event:       eventID,
		StayNumber:  stayNum,
		ReportEntry: reID,
	})
	if err != nil {
		return 0, herr.InternalServerError("Failed to attach report entry", err).From("[AttachReportEntryToStay]")
	}
	return reID, nil
}

type EditStay struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action EditStay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.editStay(req)
	if errHTTP != nil {
		errHTTP.From("[editStay]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action EditStay) editStay(req *http.Request) *herr.HTTPError {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteStays == 0 {
		return herr.Forbidden("The requestor does not have EventWriteStays permission for this Event", nil)
	}
	ctx := req.Context()

	stayNumber, err := conv.ParseInt32(req.PathValue("stayNumber"))
	if err != nil {
		return herr.BadRequest("Invalid Stay Number", err).From("[ParseInt32]")
	}
	newStay, errHTTP := readBodyAs[imsjson.Stay](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}
	newStay.Event = event.Name
	newStay.EventID = event.ID
	newStay.Number = stayNumber

	author := jwtCtx.Claims.RangerHandle()

	errHTTP = updateStay(ctx, action.imsDBQ, action.es, newStay, author)
	if errHTTP != nil {
		return errHTTP.From("[updateStay]")
	}

	return nil
}

type AttachRangerToStay struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action AttachRangerToStay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.attachRanger(req)
	if errHTTP != nil {
		errHTTP.From("[attachRanger]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action AttachRangerToStay) attachRanger(req *http.Request) *herr.HTTPError {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteStays == 0 {
		return herr.Forbidden("The requestor does not have EventWriteStays permission for this Event", nil)
	}
	ctx := req.Context()

	stayNumber, err := conv.ParseInt32(req.PathValue("stayNumber"))
	if err != nil {
		return herr.BadRequest("Invalid Stay Number", err).From("[ParseInt32]")
	}

	rangerName := req.PathValue("rangerName")
	if rangerName == "" {
		return herr.BadRequest("Empty Ranger Name", nil)
	}

	body, errHTTP := readBodyAs[imsjson.StayRanger](req)
	if errHTTP != nil {
		return errHTTP.From("[readBodyAs]")
	}
	txn, err := action.imsDBQ.Begin()
	if err != nil {
		return herr.InternalServerError("Failed to start transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	err = action.imsDBQ.DetachRangerFromStay(ctx, txn, imsdb.DetachRangerFromStayParams{
		Event:        event.ID,
		StayNumber:   stayNumber,
		RangerHandle: rangerName,
	})
	if err != nil {
		return herr.InternalServerError("Failed to detach Ranger from Stay", err).From("[DetachRangerHandleFromIncident]")
	}

	err = action.imsDBQ.AttachRangerToStay(ctx, txn, imsdb.AttachRangerToStayParams{
		Event:        event.ID,
		StayNumber:   stayNumber,
		RangerHandle: rangerName,
		Role:         conv.StringToSql(body.Role, 128),
	})
	if err != nil {
		return herr.InternalServerError("Failed to attach Ranger to Stay", err).From("[AttachRangerToStay]")
	}

	_, errHTTP = addStayReportEntry(
		ctx, action.imsDBQ, txn, event.ID, stayNumber,
		jwtCtx.Claims.RangerHandle(), fmt.Sprintf("Added Ranger: %v", rangerName),
		true, "", "", "",
	)
	if errHTTP != nil {
		return errHTTP.From("[addStayReportEntry]")
	}
	err = txn.Commit()
	if err != nil {
		return herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	action.es.notifyStayUpdate(event.ID, stayNumber)

	return nil
}

type DetachRangerFromStay struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action DetachRangerFromStay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := action.detachRanger(req)
	if errHTTP != nil {
		errHTTP.From("[detachRanger]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

func (action DetachRangerFromStay) detachRanger(req *http.Request) *herr.HTTPError {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteStays == 0 {
		return herr.Forbidden("The requestor does not have EventWriteStays permission for this Event", nil)
	}
	ctx := req.Context()

	stayNumber, err := conv.ParseInt32(req.PathValue("stayNumber"))
	if err != nil {
		return herr.BadRequest("Invalid Stay Number", err).From("[ParseInt32]")
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

	err = action.imsDBQ.DetachRangerFromStay(ctx, txn, imsdb.DetachRangerFromStayParams{
		Event:        event.ID,
		StayNumber:   stayNumber,
		RangerHandle: rangerName,
	})
	if err != nil {
		return herr.InternalServerError("Failed to detach Ranger from Stay", err).From("[DetachRangerHandleFromIncident]")
	}
	_, errHTTP = addStayReportEntry(
		ctx, action.imsDBQ, txn, event.ID, stayNumber,
		jwtCtx.Claims.RangerHandle(), fmt.Sprintf("Removed Ranger: %v", rangerName),
		true, "", "", "",
	)
	if errHTTP != nil {
		return errHTTP.From("[addStayReportEntry]")
	}

	err = txn.Commit()
	if err != nil {
		return herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	action.es.notifyStayUpdate(event.ID, stayNumber)

	return nil
}
