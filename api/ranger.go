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
	"fmt"
	"net/http"

	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
)

// rangerRoster describes the pieces of the Ranger attach/detach flow that differ
// between Incidents and Visits. Everything else about those flows is identical
// and lives in attachRanger and detachRanger below.
type rangerRoster struct {
	writePermission     authz.EventPermissionMask
	writePermissionName string
	numberPathKey       string
	noun                string

	detach         func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32, rangerHandle string) error
	attach         func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32, rangerHandle string, role sql.NullString) error
	addReportEntry func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32, entry newReportEntry) (int32, *herr.HTTPError)
	// bumpVersion moves the parent record's optimistic-concurrency version, so
	// that clients holding its ETag notice the roster change. getVersion reads
	// the resulting version for the response's ETag header.
	bumpVersion  func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32) error
	getVersion   func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32) (int32, error)
	notifyUpdate func(eventID, number int32)
}

func incidentRangerRoster(imsDBQ *store.DBQ, es *EventSourcerer) rangerRoster {
	return rangerRoster{
		writePermission:     authz.EventWriteIncidents,
		writePermissionName: "EventWriteIncidents",
		numberPathKey:       "incidentNumber",
		noun:                "Incident",
		detach: func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32, rangerHandle string) error {
			return imsDBQ.DetachRangerHandleFromIncident(ctx, dbtx, imsdb.DetachRangerHandleFromIncidentParams{
				Event:          eventID,
				IncidentNumber: number,
				RangerHandle:   rangerHandle,
			})
		},
		attach: func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32, rangerHandle string, role sql.NullString) error {
			return imsDBQ.AttachRangerHandleToIncident(ctx, dbtx, imsdb.AttachRangerHandleToIncidentParams{
				Event:          eventID,
				IncidentNumber: number,
				RangerHandle:   rangerHandle,
				Role:           role,
			})
		},
		addReportEntry: func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32, entry newReportEntry) (int32, *herr.HTTPError) {
			return addIncidentReportEntry(ctx, imsDBQ, dbtx, eventID, number, entry)
		},
		bumpVersion: func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32) error {
			return imsDBQ.BumpIncidentVersion(ctx, dbtx, imsdb.BumpIncidentVersionParams{
				Event:  eventID,
				Number: number,
			})
		},
		getVersion: func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32) (int32, error) {
			return imsDBQ.IncidentVersion(ctx, dbtx, imsdb.IncidentVersionParams{
				Event:  eventID,
				Number: number,
			})
		},
		notifyUpdate: es.notifyIncidentUpdate,
	}
}

func visitRangerRoster(imsDBQ *store.DBQ, es *EventSourcerer) rangerRoster {
	return rangerRoster{
		writePermission:     authz.EventWriteVisits,
		writePermissionName: "EventWriteVisits",
		numberPathKey:       "visitNumber",
		noun:                "Visit",
		detach: func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32, rangerHandle string) error {
			return imsDBQ.DetachRangerFromVisit(ctx, dbtx, imsdb.DetachRangerFromVisitParams{
				Event:        eventID,
				VisitNumber:  number,
				RangerHandle: rangerHandle,
			})
		},
		attach: func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32, rangerHandle string, role sql.NullString) error {
			return imsDBQ.AttachRangerToVisit(ctx, dbtx, imsdb.AttachRangerToVisitParams{
				Event:        eventID,
				VisitNumber:  number,
				RangerHandle: rangerHandle,
				Role:         role,
			})
		},
		addReportEntry: func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32, entry newReportEntry) (int32, *herr.HTTPError) {
			return addVisitReportEntry(ctx, imsDBQ, dbtx, eventID, number, entry)
		},
		bumpVersion: func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32) error {
			return imsDBQ.BumpVisitVersion(ctx, dbtx, imsdb.BumpVisitVersionParams{
				Event:  eventID,
				Number: number,
			})
		},
		getVersion: func(ctx context.Context, dbtx imsdb.DBTX, eventID, number int32) (int32, error) {
			return imsDBQ.VisitVersion(ctx, dbtx, imsdb.VisitVersionParams{
				Event:  eventID,
				Number: number,
			})
		},
		notifyUpdate: es.notifyVisitUpdate,
	}
}

// rangerRosterRequest holds the validated, entity-independent inputs common to
// the attach and detach endpoints.
type rangerRosterRequest struct {
	event      imsdb.Event
	number     int32
	rangerName string
	author     string
}

func parseRangerRosterRequest(
	req *http.Request, roster rangerRoster,
	imsDBQ *store.DBQ, userStore *directory.UserStore, imsAdmins []string,
) (rangerRosterRequest, *herr.HTTPError) {
	var empty rangerRosterRequest
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, imsDBQ, userStore, imsAdmins)
	if errHTTP != nil {
		return empty, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&roster.writePermission == 0 {
		return empty, herr.Forbidden(
			fmt.Sprintf("The requestor does not have %v permission for this Event", roster.writePermissionName), nil)
	}

	number, err := conv.ParseInt32(req.PathValue(roster.numberPathKey))
	if err != nil {
		return empty, herr.BadRequest(fmt.Sprintf("Invalid %v Number", roster.noun), err).From("[ParseInt32]")
	}

	rangerName := req.PathValue("rangerName")
	if rangerName == "" {
		return empty, herr.BadRequest("Empty Ranger Name", nil)
	}

	return rangerRosterRequest{
		event:      event,
		number:     number,
		rangerName: rangerName,
		author:     jwtCtx.Claims.RangerHandle(),
	}, nil
}

// rangerRosterBody is the request body for the attach-Ranger endpoints. It has
// the same shape as imsjson.IncidentRanger and imsjson.VisitRanger, which are
// identical to each other.
type rangerRosterBody struct {
	Role *string `json:"role"`
}

func attachRanger(
	req *http.Request, roster rangerRoster,
	imsDBQ *store.DBQ, userStore *directory.UserStore, imsAdmins []string,
) (int32, *herr.HTTPError) {
	rosterReq, errHTTP := parseRangerRosterRequest(req, roster, imsDBQ, userStore, imsAdmins)
	if errHTTP != nil {
		return 0, errHTTP.From("[parseRangerRosterRequest]")
	}
	body, errHTTP := readBodyAs[rangerRosterBody](req)
	if errHTTP != nil {
		return 0, errHTTP.From("[readBodyAs]")
	}
	ctx := req.Context()

	txn, err := imsDBQ.Begin()
	if err != nil {
		return 0, herr.InternalServerError("Failed to start transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	// Detach first, so that attaching a Ranger who is already on the roster
	// updates their role rather than failing.
	err = roster.detach(ctx, txn, rosterReq.event.ID, rosterReq.number, rosterReq.rangerName)
	if err != nil {
		return 0, herr.InternalServerError(fmt.Sprintf("Failed to detach Ranger from %v", roster.noun), err).From("[detach]")
	}

	err = roster.attach(ctx, txn, rosterReq.event.ID, rosterReq.number, rosterReq.rangerName, conv.StringToSql(body.Role, 128))
	if err != nil {
		return 0, herr.InternalServerError(fmt.Sprintf("Failed to attach Ranger to %v", roster.noun), err).From("[attach]")
	}

	version, errHTTP := bumpRosterVersion(ctx, txn, roster, rosterReq)
	if errHTTP != nil {
		return 0, errHTTP.From("[bumpRosterVersion]")
	}

	_, errHTTP = roster.addReportEntry(ctx, txn, rosterReq.event.ID, rosterReq.number, newReportEntry{
		author:    rosterReq.author,
		text:      fmt.Sprintf("Added Ranger: %v", rosterReq.rangerName),
		generated: true,
	})
	if errHTTP != nil {
		return 0, errHTTP.From("[addReportEntry]")
	}
	err = txn.Commit()
	if err != nil {
		return 0, herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	roster.notifyUpdate(rosterReq.event.ID, rosterReq.number)

	return version, nil
}

// bumpRosterVersion moves the parent record's version within the roster
// transaction and returns the new version for the response's ETag.
func bumpRosterVersion(
	ctx context.Context, txn *sql.Tx, roster rangerRoster, rosterReq rangerRosterRequest,
) (int32, *herr.HTTPError) {
	err := roster.bumpVersion(ctx, txn, rosterReq.event.ID, rosterReq.number)
	if err != nil {
		return 0, herr.InternalServerError(fmt.Sprintf("Failed to update %v", roster.noun), err).From("[bumpVersion]")
	}
	version, err := roster.getVersion(ctx, txn, rosterReq.event.ID, rosterReq.number)
	if err != nil {
		return 0, herr.InternalServerError(fmt.Sprintf("Failed to fetch %v", roster.noun), err).From("[getVersion]")
	}
	return version, nil
}

func detachRanger(
	req *http.Request, roster rangerRoster,
	imsDBQ *store.DBQ, userStore *directory.UserStore, imsAdmins []string,
) (int32, *herr.HTTPError) {
	rosterReq, errHTTP := parseRangerRosterRequest(req, roster, imsDBQ, userStore, imsAdmins)
	if errHTTP != nil {
		return 0, errHTTP.From("[parseRangerRosterRequest]")
	}
	ctx := req.Context()

	txn, err := imsDBQ.Begin()
	if err != nil {
		return 0, herr.InternalServerError("Failed to start transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	err = roster.detach(ctx, txn, rosterReq.event.ID, rosterReq.number, rosterReq.rangerName)
	if err != nil {
		return 0, herr.InternalServerError(fmt.Sprintf("Failed to detach Ranger from %v", roster.noun), err).From("[detach]")
	}

	version, errHTTP := bumpRosterVersion(ctx, txn, roster, rosterReq)
	if errHTTP != nil {
		return 0, errHTTP.From("[bumpRosterVersion]")
	}

	_, errHTTP = roster.addReportEntry(ctx, txn, rosterReq.event.ID, rosterReq.number, newReportEntry{
		author:    rosterReq.author,
		text:      fmt.Sprintf("Removed Ranger: %v", rosterReq.rangerName),
		generated: true,
	})
	if errHTTP != nil {
		return 0, errHTTP.From("[addReportEntry]")
	}

	err = txn.Commit()
	if err != nil {
		return 0, herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	roster.notifyUpdate(rosterReq.event.ID, rosterReq.number)

	return version, nil
}

type AttachRangerToIncident struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action AttachRangerToIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	version, errHTTP := attachRanger(req, incidentRangerRoster(action.imsDBQ, action.es), action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		errHTTP.From("[attachRanger]").WriteResponse(w)
		return
	}
	setETag(w, version)
	herr.WriteNoContentResponse(w, "Success")
}

type DetachRangerFromIncident struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action DetachRangerFromIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	version, errHTTP := detachRanger(req, incidentRangerRoster(action.imsDBQ, action.es), action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		errHTTP.From("[detachRanger]").WriteResponse(w)
		return
	}
	setETag(w, version)
	herr.WriteNoContentResponse(w, "Success")
}

type AttachRangerToVisit struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action AttachRangerToVisit) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	version, errHTTP := attachRanger(req, visitRangerRoster(action.imsDBQ, action.es), action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		errHTTP.From("[attachRanger]").WriteResponse(w)
		return
	}
	setETag(w, version)
	herr.WriteNoContentResponse(w, "Success")
}

type DetachRangerFromVisit struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action DetachRangerFromVisit) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	version, errHTTP := detachRanger(req, visitRangerRoster(action.imsDBQ, action.es), action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		errHTTP.From("[detachRanger]").WriteResponse(w)
		return
	}
	setETag(w, version)
	herr.WriteNoContentResponse(w, "Success")
}
