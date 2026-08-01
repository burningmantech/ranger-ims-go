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
//     membership the Incident already has is a true no-op, writing no report
//     entry.
//
// None of this moves the Incident's version. That counter is the CAS gate for
// the Incident row's own columns, which nothing here writes, so bumping it would
// only make a concurrent field edit retry for nothing — the same reason a report
// entry append doesn't move it.
//
// The corresponding whole-list fields on EditIncident are response-only; see
// rejectSetReplacement in incident.go.

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

// setIncidentType attaches or detaches one Incident Type, per attach.
func setIncidentType(
	req *http.Request, attach bool,
	imsDBQ *store.DBQ, userStore *directory.UserStore, es *EventSourcerer, imsAdmins []string,
) *herr.HTTPError {
	relReq, errHTTP := parseIncidentRelationRequest(req, attach, imsDBQ, userStore, imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[parseIncidentRelationRequest]")
	}
	typeID, err := conv.ParseInt32(req.PathValue("incidentTypeId"))
	if err != nil {
		return herr.BadRequest("Invalid Incident Type ID", err).From("[ParseInt32]")
	}
	ctx := req.Context()

	row, errHTTP := readIncidentRow(ctx, imsDBQ, relReq)
	if errHTTP != nil {
		return errHTTP.From("[readIncidentRow]")
	}
	currentTypeIDs, _, _, err := readExtraIncidentRowFields(row)
	if err != nil {
		return herr.InternalServerError("Failed to read Incident details", err).From("[readExtraIncidentRowFields]")
	}

	// The Incident already has the membership that was asked for, so there's
	// nothing to record. This is what makes a repeated request safe to send.
	if slices.Contains(currentTypeIDs, typeID) == attach {
		return nil
	}

	// Resolve the name for the change-log line. This also rejects an unknown
	// type id with a 400, rather than letting the foreign key fail as a 500.
	// Hidden types are allowed, matching the whole-list path.
	allIncidentTypes, err := imsDBQ.IncidentTypes(ctx, imsDBQ)
	if err != nil {
		return herr.InternalServerError("Failed to get Incident Types", err).From("[IncidentTypes]")
	}
	typeName := namesForIncidentTypes(allIncidentTypes, []int32{typeID})
	if typeName == "" {
		return herr.BadRequest(fmt.Sprintf("There is no Incident Type with id %v", typeID), nil)
	}

	errHTTP = retryOnDeadlockErr(func() *herr.HTTPError {
		txn, err := imsDBQ.Begin()
		if err != nil {
			return herr.InternalServerError("Failed to start transaction", err).From("[Begin]")
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
				// so this is the same no-op: return without committing, and this
				// transaction's report entry goes with it.
				if isDuplicateKeyError(err) {
					return nil
				}
				return herr.InternalServerError("Failed to add Incident Type", err).From("[AttachIncidentTypeToIncident]")
			}
		} else {
			err = imsDBQ.DetachIncidentTypeFromIncident(ctx, txn, imsdb.DetachIncidentTypeFromIncidentParams{
				Event:          relReq.event.ID,
				IncidentNumber: relReq.number,
				IncidentType:   typeID,
			})
			if err != nil {
				return herr.InternalServerError("Failed to detach Incident Type", err).From("[DetachIncidentTypeFromIncident]")
			}
		}

		_, errHTTP = addIncidentReportEntry(ctx, imsDBQ, txn, relReq.event.ID, relReq.number, newReportEntry{
			author:    relReq.author,
			text:      logLine,
			generated: true,
		})
		if errHTTP != nil {
			return errHTTP.From("[addIncidentReportEntry]")
		}

		err = txn.Commit()
		if err != nil {
			return herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
		}
		return nil
	})
	if errHTTP != nil {
		return errHTTP
	}

	es.notifyIncidentUpdate(relReq.event.ID, relReq.number)

	return nil
}

// setIncidentLink links or unlinks one other Incident, per link.
//
// Links are symmetric, so both Incidents' rows and change logs are updated. Only
// the path Event is permission-checked, matching what the whole-list path does
// today.
func setIncidentLink(
	req *http.Request, link bool,
	imsDBQ *store.DBQ, userStore *directory.UserStore, es *EventSourcerer, imsAdmins []string,
) *herr.HTTPError {
	relReq, errHTTP := parseIncidentRelationRequest(req, link, imsDBQ, userStore, imsAdmins)
	if errHTTP != nil {
		return errHTTP.From("[parseIncidentRelationRequest]")
	}
	peerEvent, errHTTP := getEvent(req, req.PathValue("linkedEventName"), imsDBQ)
	if errHTTP != nil {
		return errHTTP.From("[getEvent]")
	}
	peerNumber, err := conv.ParseInt32(req.PathValue("linkedIncidentNumber"))
	if err != nil {
		return herr.BadRequest("Invalid linked Incident Number", err).From("[ParseInt32]")
	}
	if peerEvent.ID == relReq.event.ID && peerNumber == relReq.number {
		return herr.BadRequest("An Incident cannot be linked to itself", nil)
	}
	ctx := req.Context()

	// Read for its 404: the path Incident has to exist before either end of the
	// link is touched.
	_, errHTTP = readIncidentRow(ctx, imsDBQ, relReq)
	if errHTTP != nil {
		return errHTTP.From("[readIncidentRow]")
	}
	currentLinks, err := imsDBQ.Incident_LinkedIncidents(ctx, imsDBQ, imsdb.Incident_LinkedIncidentsParams{
		Event1:          relReq.event.ID,
		IncidentNumber1: relReq.number,
	})
	if err != nil {
		return herr.InternalServerError("Failed to fetch linked Incidents", err).From("[Incident_LinkedIncidents]")
	}
	alreadyLinked := slices.ContainsFunc(currentLinks, func(cli imsdb.Incident_LinkedIncidentsRow) bool {
		return cli.LinkedEvent == peerEvent.ID && cli.LinkedIncident == peerNumber
	})
	// As in setIncidentType: the link is already in the state that was asked
	// for, so this request has nothing to do.
	if alreadyLinked == link {
		return nil
	}

	errHTTP = retryOnDeadlockErr(func() *herr.HTTPError {
		txn, err := imsDBQ.Begin()
		if err != nil {
			return herr.InternalServerError("Failed to start transaction", err).From("[Begin]")
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
					return nil
				}
				// Otherwise the insert broke a foreign key, which means there's no
				// such peer Incident. That's the client's mistake, not a 500.
				return herr.BadRequest(
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
				return herr.InternalServerError("Failed to link Incident", err).From("[LinkIncidents]")
			}
		} else {
			err = imsDBQ.UnlinkIncidents(ctx, txn, imsdb.UnlinkIncidentsParams{
				Event1:          relReq.event.ID,
				IncidentNumber1: relReq.number,
				Event2:          peerEvent.ID,
				IncidentNumber2: peerNumber,
			})
			if err != nil {
				return herr.InternalServerError("Failed to unlink Incident", err).From("[UnlinkIncidents]")
			}
			err = imsDBQ.UnlinkIncidents(ctx, txn, imsdb.UnlinkIncidentsParams{
				Event2:          relReq.event.ID,
				IncidentNumber2: relReq.number,
				Event1:          peerEvent.ID,
				IncidentNumber1: peerNumber,
			})
			if err != nil {
				return herr.InternalServerError("Failed to unlink Incident", err).From("[UnlinkIncidents]")
			}
		}

		_, errHTTP = addIncidentReportEntry(ctx, imsDBQ, txn, relReq.event.ID, relReq.number, newReportEntry{
			author:    relReq.author,
			text:      selfLogLine,
			generated: true,
		})
		if errHTTP != nil {
			return errHTTP.From("[addIncidentReportEntry]")
		}

		// The other Incident's change log should say what happened to it too.
		_, errHTTP = addIncidentReportEntry(ctx, imsDBQ, txn, peerEvent.ID, peerNumber, newReportEntry{
			author:    relReq.author,
			text:      peerLogLine,
			generated: true,
		})
		if errHTTP != nil {
			return errHTTP.From("[addIncidentReportEntry]")
		}

		err = txn.Commit()
		if err != nil {
			return herr.InternalServerError("Failed to commit transaction", err).From("[Commit]")
		}
		return nil
	})
	if errHTTP != nil {
		return errHTTP
	}

	es.notifyIncidentUpdate(relReq.event.ID, relReq.number)
	es.notifyIncidentUpdate(peerEvent.ID, peerNumber)

	return nil
}

type AttachTypeToIncident struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action AttachTypeToIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := setIncidentType(req, true, action.imsDBQ, action.userStore, action.es, action.imsAdmins)
	if errHTTP != nil {
		errHTTP.From("[setIncidentType]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

type DetachTypeFromIncident struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action DetachTypeFromIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := setIncidentType(req, false, action.imsDBQ, action.userStore, action.es, action.imsAdmins)
	if errHTTP != nil {
		errHTTP.From("[setIncidentType]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

type LinkIncident struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action LinkIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := setIncidentLink(req, true, action.imsDBQ, action.userStore, action.es, action.imsAdmins)
	if errHTTP != nil {
		errHTTP.From("[setIncidentLink]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}

type UnlinkIncident struct {
	imsDBQ    *store.DBQ
	userStore *directory.UserStore
	es        *EventSourcerer
	imsAdmins []string
}

func (action UnlinkIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	errHTTP := setIncidentLink(req, false, action.imsDBQ, action.userStore, action.es, action.imsAdmins)
	if errHTTP != nil {
		errHTTP.From("[setIncidentLink]").WriteResponse(w)
		return
	}
	herr.WriteNoContentResponse(w, "Success")
}
