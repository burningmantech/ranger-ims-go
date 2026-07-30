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
	"slices"

	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
)

// This file holds the sub-resource endpoints for an Incident's two
// set-membership relations: its Incident Types, and its links to other
// Incidents. Each endpoint expresses one gesture ("attach this type", "unlink
// that incident") rather than a replacement of the whole set, which is what the
// UI actually means and what the whole-list fields on EditIncident can't say.
//
// Consequences, which are the point of these endpoints:
//   - They're commutative, so two clients (or one client clicking twice) can
//     each change a different member without either losing the other's change.
//     A replacement computed from a stale snapshot silently reverts it.
//   - They're idempotent, so a retry is safe: a request that asks for the
//     membership the Incident already has is a true no-op, returning the
//     current version without moving it or writing a report entry.
//   - They therefore need no If-Match, like the Ranger roster endpoints in
//     ranger.go. They always report the resulting version as an ETag.
//
// The whole-list fields remain, for API compatibility and for creating an
// Incident that arrives with its types and links already populated.

// incidentRelationRequest holds the validated inputs common to all four
// endpoints: which Incident is being changed, and by whom.
type incidentRelationRequest struct {
	event  imsdb.Event
	number int32
	author string
}

// parseIncidentRelationRequest authorizes the request and resolves the Incident
// named in the path. requireJSON must be set for the POST handlers: they read no
// body, so they don't reach requireJSONContentType through readBodyAs, and that
// header check is what keeps a cross-origin HTML form from forging the request.
// The DELETE handlers don't need it, since a form can't issue a DELETE.
func parseIncidentRelationRequest(
	req *http.Request, requireJSON bool,
	imsDBQ *store.DBQ, userStore *directory.UserStore, imsAdmins []string,
) (incidentRelationRequest, *herr.HTTPError) {
	var empty incidentRelationRequest
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, imsDBQ, userStore, imsAdmins)
	if errHTTP != nil {
		return empty, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteIncidents == 0 {
		return empty, herr.Forbidden("The requestor does not have EventWriteIncidents permission for this Event", nil)
	}
	if requireJSON {
		errHTTP = requireJSONContentType(req)
		if errHTTP != nil {
			return empty, errHTTP.From("[requireJSONContentType]")
		}
	}

	number, err := conv.ParseInt32(req.PathValue("incidentNumber"))
	if err != nil {
		return empty, herr.BadRequest("Invalid Incident Number", err).From("[ParseInt32]")
	}

	return incidentRelationRequest{
		event:  event,
		number: number,
		author: jwtCtx.Claims.RangerHandle(),
	}, nil
}

// bumpIncidentVersionAndRead moves an Incident's optimistic-concurrency version
// within the caller's transaction and returns the new value, so that clients
// holding the old ETag notice the membership change.
//
// This near-duplicates bumpRosterVersion in ranger.go, which is parameterised
// over rangerRoster because it serves both Incidents and Visits.
func bumpIncidentVersionAndRead(
	ctx context.Context, imsDBQ *store.DBQ, dbtx imsdb.DBTX, eventID, number int32,
) (int32, *herr.HTTPError) {
	err := imsDBQ.BumpIncidentVersion(ctx, dbtx, imsdb.BumpIncidentVersionParams{
		Event:  eventID,
		Number: number,
	})
	if err != nil {
		return 0, herr.InternalServerError("Failed to update Incident", err).From("[BumpIncidentVersion]")
	}
	version, err := imsDBQ.IncidentVersion(ctx, dbtx, imsdb.IncidentVersionParams{
		Event:  eventID,
		Number: number,
	})
	if err != nil {
		return 0, herr.InternalServerError("Failed to fetch Incident", err).From("[IncidentVersion]")
	}
	return version, nil
}

// currentIncidentVersion reads an Incident's version without changing it, for
// the responses that report a request as a no-op.
func currentIncidentVersion(
	ctx context.Context, imsDBQ *store.DBQ, eventID, number int32,
) (int32, *herr.HTTPError) {
	version, err := imsDBQ.IncidentVersion(ctx, imsDBQ, imsdb.IncidentVersionParams{
		Event:  eventID,
		Number: number,
	})
	if err != nil {
		return 0, herr.InternalServerError("Failed to fetch Incident", err).From("[IncidentVersion]")
	}
	return version, nil
}

// readIncidentRow fetches the Incident named in the request, reporting a 404 for
// an Incident that doesn't exist.
func readIncidentRow(
	ctx context.Context, imsDBQ *store.DBQ, relReq incidentRelationRequest,
) (imsdb.IncidentRow, *herr.HTTPError) {
	row, err := imsDBQ.Incident(ctx, imsDBQ, imsdb.IncidentParams{
		Event:  relReq.event.ID,
		Number: relReq.number,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return row, herr.NotFound("Incident not found", err).From("[Incident]")
		}
		return row, herr.InternalServerError("Failed to fetch Incident", err).From("[Incident]")
	}
	return row, nil
}

// setIncidentType attaches or detaches one Incident Type, per attach. It returns
// the Incident's resulting version.
func setIncidentType(
	req *http.Request, attach bool,
	imsDBQ *store.DBQ, userStore *directory.UserStore, es *EventSourcerer, imsAdmins []string,
) (int32, *herr.HTTPError) {
	relReq, errHTTP := parseIncidentRelationRequest(req, attach, imsDBQ, userStore, imsAdmins)
	if errHTTP != nil {
		return 0, errHTTP.From("[parseIncidentRelationRequest]")
	}
	typeID, err := conv.ParseInt32(req.PathValue("incidentTypeId"))
	if err != nil {
		return 0, herr.BadRequest("Invalid Incident Type ID", err).From("[ParseInt32]")
	}
	ctx := req.Context()

	row, errHTTP := readIncidentRow(ctx, imsDBQ, relReq)
	if errHTTP != nil {
		return 0, errHTTP.From("[readIncidentRow]")
	}
	currentTypeIDs, _, _, err := readExtraIncidentRowFields(row)
	if err != nil {
		return 0, herr.InternalServerError("Failed to read Incident details", err).From("[readExtraIncidentRowFields]")
	}

	// The Incident already has the membership that was asked for, so there's
	// nothing to record and no reason to move the version. This is what makes
	// a repeated request safe to send.
	if slices.Contains(currentTypeIDs, typeID) == attach {
		return row.Incident.Version, nil
	}

	// Resolve the name for the change-log line. This also rejects an unknown
	// type id with a 400, rather than letting the foreign key fail as a 500.
	// Hidden types are allowed, matching the whole-list path.
	allIncidentTypes, err := imsDBQ.IncidentTypes(ctx, imsDBQ)
	if err != nil {
		return 0, herr.InternalServerError("Failed to get Incident Types", err).From("[IncidentTypes]")
	}
	typeName := namesForIncidentTypes(allIncidentTypes, []int32{typeID})
	if typeName == "" {
		return 0, herr.BadRequest(fmt.Sprintf("There is no Incident Type with id %v", typeID), nil)
	}

	txn, err := imsDBQ.Begin()
	if err != nil {
		return 0, herr.InternalServerError("Failed to start transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	logLine := fmt.Sprintf("Removed type: %v", typeName)
	if attach {
		logLine = fmt.Sprintf("Added type: %v", typeName)
		err = imsDBQ.AttachIncidentTypeToIncident(ctx, txn, imsdb.AttachIncidentTypeToIncidentParams{
			Event:          relReq.event.ID,
			IncidentNumber: relReq.number,
			IncidentType:   typeID,
		})
		if err != nil {
			// Another writer attached this same type between the membership
			// check above and this insert (the insert has no "on duplicate
			// key"). The Incident is now in the state this request asked for,
			// so this is the same no-op.
			if isDuplicateKeyError(err) {
				return currentIncidentVersion(ctx, imsDBQ, relReq.event.ID, relReq.number)
			}
			return 0, herr.InternalServerError("Failed to add Incident Type", err).From("[AttachIncidentTypeToIncident]")
		}
	} else {
		err = imsDBQ.DetachIncidentTypeFromIncident(ctx, txn, imsdb.DetachIncidentTypeFromIncidentParams{
			Event:          relReq.event.ID,
			IncidentNumber: relReq.number,
			IncidentType:   typeID,
		})
		if err != nil {
			return 0, herr.InternalServerError("Failed to detach Incident Type", err).From("[DetachIncidentTypeFromIncident]")
		}
	}

	version, errHTTP := bumpIncidentVersionAndRead(ctx, imsDBQ, txn, relReq.event.ID, relReq.number)
	if errHTTP != nil {
		return 0, errHTTP.From("[bumpIncidentVersionAndRead]")
	}

	_, errHTTP = addIncidentReportEntry(ctx, imsDBQ, txn, relReq.event.ID, relReq.number, newReportEntry{
		author:    relReq.author,
		text:      logLine,
		generated: true,
	})
	if errHTTP != nil {
		return 0, errHTTP.From("[addIncidentReportEntry]")
	}

	err = txn.Commit()
	if err != nil {
		return 0, herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	es.notifyIncidentUpdate(relReq.event.ID, relReq.number)

	return version, nil
}

// setIncidentLink links or unlinks one other Incident, per link. It returns the
// path Incident's resulting version.
//
// Links are symmetric, so both Incidents' rows, versions and change logs are
// updated. Only the path Event is permission-checked, matching what the
// whole-list path does today.
func setIncidentLink(
	req *http.Request, link bool,
	imsDBQ *store.DBQ, userStore *directory.UserStore, es *EventSourcerer, imsAdmins []string,
) (int32, *herr.HTTPError) {
	relReq, errHTTP := parseIncidentRelationRequest(req, link, imsDBQ, userStore, imsAdmins)
	if errHTTP != nil {
		return 0, errHTTP.From("[parseIncidentRelationRequest]")
	}
	peerEvent, errHTTP := getEvent(req, req.PathValue("linkedEventName"), imsDBQ)
	if errHTTP != nil {
		return 0, errHTTP.From("[getEvent]")
	}
	peerNumber, err := conv.ParseInt32(req.PathValue("linkedIncidentNumber"))
	if err != nil {
		return 0, herr.BadRequest("Invalid linked Incident Number", err).From("[ParseInt32]")
	}
	if peerEvent.ID == relReq.event.ID && peerNumber == relReq.number {
		return 0, herr.BadRequest("An Incident cannot be linked to itself", nil)
	}
	ctx := req.Context()

	row, errHTTP := readIncidentRow(ctx, imsDBQ, relReq)
	if errHTTP != nil {
		return 0, errHTTP.From("[readIncidentRow]")
	}
	currentLinks, err := imsDBQ.Incident_LinkedIncidents(ctx, imsDBQ, imsdb.Incident_LinkedIncidentsParams{
		Event1:          relReq.event.ID,
		IncidentNumber1: relReq.number,
	})
	if err != nil {
		return 0, herr.InternalServerError("Failed to fetch linked Incidents", err).From("[Incident_LinkedIncidents]")
	}
	alreadyLinked := slices.ContainsFunc(currentLinks, func(cli imsdb.Incident_LinkedIncidentsRow) bool {
		return cli.LinkedEvent == peerEvent.ID && cli.LinkedIncident == peerNumber
	})
	// As in setIncidentType: the link is already in the state that was asked
	// for, so this request has nothing to do.
	if alreadyLinked == link {
		return row.Incident.Version, nil
	}

	txn, err := imsDBQ.Begin()
	if err != nil {
		return 0, herr.InternalServerError("Failed to start transaction", err).From("[Begin]")
	}
	defer rollback(txn)

	// The LINKED_INCIDENT table holds each link twice, once per direction, so
	// that a lookup from either Incident finds it.
	selfLogLine := fmt.Sprintf("Incident unlinked: %v #%v", peerEvent.Name, peerNumber)
	peerLogLine := fmt.Sprintf("Incident unlinked: %v #%v", relReq.event.Name, relReq.number)
	if link {
		selfLogLine = fmt.Sprintf("Incident linked: %v #%v", peerEvent.Name, peerNumber)
		peerLogLine = fmt.Sprintf("Incident linked: %v #%v", relReq.event.Name, relReq.number)
		err = imsDBQ.LinkIncidents(ctx, txn, imsdb.LinkIncidentsParams{
			Event1:          relReq.event.ID,
			IncidentNumber1: relReq.number,
			Event2:          peerEvent.ID,
			IncidentNumber2: peerNumber,
		})
		if err != nil {
			// As in setIncidentType, a concurrent identical link is a no-op
			// rather than an error.
			if isDuplicateKeyError(err) {
				return currentIncidentVersion(ctx, imsDBQ, relReq.event.ID, relReq.number)
			}
			// Otherwise we'll just assume the problem is that the peer Incident
			// doesn't exist, and the insert broke the foreign key. This is
			// probably the case...
			return 0, herr.BadRequest(
				fmt.Sprintf("Failed to link Incident. There may be no IMS #%v for the given event.", peerNumber), err,
			).From("[LinkIncidents]")
		}
		err = imsDBQ.LinkIncidents(ctx, txn, imsdb.LinkIncidentsParams{
			Event2:          relReq.event.ID,
			IncidentNumber2: relReq.number,
			Event1:          peerEvent.ID,
			IncidentNumber1: peerNumber,
		})
		if err != nil {
			return 0, herr.InternalServerError("Failed to link Incident", err).From("[LinkIncidents]")
		}
	} else {
		err = imsDBQ.UnlinkIncidents(ctx, txn, imsdb.UnlinkIncidentsParams{
			Event1:          relReq.event.ID,
			IncidentNumber1: relReq.number,
			Event2:          peerEvent.ID,
			IncidentNumber2: peerNumber,
		})
		if err != nil {
			return 0, herr.InternalServerError("Failed to unlink Incident", err).From("[UnlinkIncidents]")
		}
		err = imsDBQ.UnlinkIncidents(ctx, txn, imsdb.UnlinkIncidentsParams{
			Event2:          relReq.event.ID,
			IncidentNumber2: relReq.number,
			Event1:          peerEvent.ID,
			IncidentNumber1: peerNumber,
		})
		if err != nil {
			return 0, herr.InternalServerError("Failed to unlink Incident", err).From("[UnlinkIncidents]")
		}
	}

	version, errHTTP := bumpIncidentVersionAndRead(ctx, imsDBQ, txn, relReq.event.ID, relReq.number)
	if errHTTP != nil {
		return 0, errHTTP.From("[bumpIncidentVersionAndRead]")
	}
	_, errHTTP = addIncidentReportEntry(ctx, imsDBQ, txn, relReq.event.ID, relReq.number, newReportEntry{
		author:    relReq.author,
		text:      selfLogLine,
		generated: true,
	})
	if errHTTP != nil {
		return 0, errHTTP.From("[addIncidentReportEntry]")
	}

	// The other Incident's representation changed too, so its version must move
	// for clients holding its ETag to notice, and its change log should say so.
	_, errHTTP = bumpIncidentVersionAndRead(ctx, imsDBQ, txn, peerEvent.ID, peerNumber)
	if errHTTP != nil {
		return 0, errHTTP.From("[bumpIncidentVersionAndRead]")
	}
	_, errHTTP = addIncidentReportEntry(ctx, imsDBQ, txn, peerEvent.ID, peerNumber, newReportEntry{
		author:    relReq.author,
		text:      peerLogLine,
		generated: true,
	})
	if errHTTP != nil {
		return 0, errHTTP.From("[addIncidentReportEntry]")
	}

	err = txn.Commit()
	if err != nil {
		return 0, herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
	}

	es.notifyIncidentUpdate(relReq.event.ID, relReq.number)
	es.notifyIncidentUpdate(peerEvent.ID, peerNumber)

	return version, nil
}

type AttachTypeToIncident struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action AttachTypeToIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	version, errHTTP := setIncidentType(req, true, action.imsDBQ, action.userStore, action.es, action.imsAdmins)
	if errHTTP != nil {
		errHTTP.From("[setIncidentType]").WriteResponse(w)
		return
	}
	setETag(w, version)
	herr.WriteNoContentResponse(w, "Success")
}

type DetachTypeFromIncident struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action DetachTypeFromIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	version, errHTTP := setIncidentType(req, false, action.imsDBQ, action.userStore, action.es, action.imsAdmins)
	if errHTTP != nil {
		errHTTP.From("[setIncidentType]").WriteResponse(w)
		return
	}
	setETag(w, version)
	herr.WriteNoContentResponse(w, "Success")
}

type LinkIncident struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action LinkIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	version, errHTTP := setIncidentLink(req, true, action.imsDBQ, action.userStore, action.es, action.imsAdmins)
	if errHTTP != nil {
		errHTTP.From("[setIncidentLink]").WriteResponse(w)
		return
	}
	setETag(w, version)
	herr.WriteNoContentResponse(w, "Success")
}

type UnlinkIncident struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action UnlinkIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	version, errHTTP := setIncidentLink(req, false, action.imsDBQ, action.userStore, action.es, action.imsAdmins)
	if errHTTP != nil {
		errHTTP.From("[setIncidentLink]").WriteResponse(w)
		return
	}
	setETag(w, version)
	herr.WriteNoContentResponse(w, "Success")
}
