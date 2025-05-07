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
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"
)

type GetIncidentAttachment struct {
	imsDB            *store.DB
	attachmentsStore conf.AttachmentsStore
	imsAdmins        []string
}

type AttachToIncident struct {
	imsDB            *store.DB
	es               *EventSourcerer
	attachmentsStore conf.AttachmentsStore
	imsAdmins        []string
}

type GetFieldReportAttachment struct {
	imsDB            *store.DB
	attachmentsStore conf.AttachmentsStore
	imsAdmins        []string
}

type AttachToFieldReport struct {
	imsDB            *store.DB
	es               *EventSourcerer
	attachmentsStore conf.AttachmentsStore
	imsAdmins        []string
}

func (action GetIncidentAttachment) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	event, _, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&authz.EventReadIncidents == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have EventReadIncidents permission on this Event", nil)
		return
	}
	ctx := req.Context()

	incidentNumber, err := conv.ParseInt32(req.PathValue("incidentNumber"))
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Failed to parse incident number", err)
		return
	}
	attachmentNumber, err := conv.ParseInt32(req.PathValue("attachmentNumber"))
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Failed to parse attachment number", err)
		return
	}

	_, reportEntries, err := fetchIncident(ctx, action.imsDB, event.ID, incidentNumber)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			handleErr(w, req, http.StatusNotFound, "No such incident", err)
			return
		}
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Incident", err)
		return
	}

	var filename string
	for _, reportEntry := range reportEntries {
		if reportEntry.ID == attachmentNumber {
			filename = reportEntry.AttachedFile.String
			break
		}
	}
	if filename == "" {
		handleErr(w, req, http.StatusNotFound, "No attachment for this ID", err)
		return
	}

	var file io.ReadSeeker
	switch action.attachmentsStore.Type {
	case conf.AttachmentsStoreLocal:
		file, err = action.attachmentsStore.Local.Dir.Open(filename)
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to open file", err)
			return
		}
	case conf.AttachmentsStoreS3:
		fallthrough
	case conf.AttachmentsStoreNone:
		fallthrough
	default:
		handleErr(w, req, http.StatusNotFound, "Attachments are not currently supported", nil)
		return
	}

	http.ServeContent(w, req, "Attached File", time.Now(), file)
}

func (action GetFieldReportAttachment) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	event, jwtCtx, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&(authz.EventReadAllFieldReports|authz.EventReadOwnFieldReports) == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have permission to read Field Reports on this Event", nil)
		return
	}
	// i.e. the user has EventReadOwnFieldReports, but not EventReadAllFieldReports
	limitedAccess := eventPermissions&authz.EventReadAllFieldReports == 0
	ctx := req.Context()

	fieldReportNumber, err := conv.ParseInt32(req.PathValue("fieldReportNumber"))
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Invalid Field Report number", err)
		return
	}
	attachmentNumber, err := conv.ParseInt32(req.PathValue("attachmentNumber"))
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Failed to parse attachment number", err)
		return
	}

	_, reportEntries, err := fetchFieldReport(ctx, action.imsDB, event.ID, fieldReportNumber)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			handleErr(w, req, http.StatusNotFound, "No such Field Report", err)
			return
		}
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Field Report", err)
		return
	}

	if limitedAccess {
		if !containsAuthor(reportEntries, jwtCtx.Claims.RangerHandle()) {
			handleErr(w, req, http.StatusForbidden, "The requestor does not have permission to read this particular Field Report", nil)
			return
		}
	}

	var filename string
	for _, reportEntry := range reportEntries {
		if reportEntry.ID == attachmentNumber {
			filename = reportEntry.AttachedFile.String
			break
		}
	}
	if filename == "" {
		handleErr(w, req, http.StatusNotFound, "No attachment for this ID", err)
		return
	}

	var file io.ReadSeeker
	switch action.attachmentsStore.Type {
	case conf.AttachmentsStoreLocal:
		file, err = action.attachmentsStore.Local.Dir.Open(filename)
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to open file", err)
			return
		}
	case conf.AttachmentsStoreS3:
		fallthrough
	case conf.AttachmentsStoreNone:
		fallthrough
	default:
		handleErr(w, req, http.StatusNotFound, "Attachments are not currently supported", nil)
		return
	}

	http.ServeContent(w, req, "Attached File", time.Now(), file)
}

func (action AttachToIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	event, jwtCtx, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&authz.EventWriteIncidents == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have EventWriteIncidents permission on this Event", nil)
		return
	}
	ctx := req.Context()

	incidentNumber, err := conv.ParseInt32(req.PathValue("incidentNumber"))
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Failed to parse incident number", err)
		return
	}

	// this must match the key sent by the client
	fi, fiHead, err := req.FormFile("imsAttachment")
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Failed to parse file", err)
		return
	}
	defer shut(fi)

	// This must be >= http.sniffLen (it's a private field, so we can't read it)
	const sniffLen = 512
	head := make([]byte, sniffLen)
	if _, err = fi.ReadAt(head, 0); err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to read head of file", err)
		return
	}

	// We'll detect the contentType and file extension, rather than trust any value from the client.
	sniffedContentType := http.DetectContentType(head)
	var extension string
	extensions, err := mime.ExtensionsByType(sniffedContentType)
	if err == nil && len(extensions) > 0 {
		extension = extensions[0]
	} else {
		slog.Info("Unable to determine a good file type for attachment. We'll leave it extension-free.",
			"sniffedContentType", sniffedContentType,
			"error", err,
		)
	}

	newFileName := fmt.Sprintf("event_%05d_incident_%05d_%v%v", event.ID, incidentNumber, rand.Text(), extension)
	slog.Info("User is uploaded an incident attachment",
		"user", jwtCtx.Claims.RangerHandle(),
		"eventName", event.Name,
		"incidentNumber", incidentNumber,
		"originalName", fiHead.Filename,
		"newFileName", newFileName,
		"size", fiHead.Size,
		"contentType", sniffedContentType,
		"extension", extension,
	)

	switch action.attachmentsStore.Type {
	case conf.AttachmentsStoreLocal:
		outFi, err := action.attachmentsStore.Local.Dir.Create(newFileName)
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to create file", err)
			return
		}
		defer shut(outFi)
		if _, err = io.Copy(outFi, fi); err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to write file", err)
			return
		}
	case conf.AttachmentsStoreS3:
		fallthrough
	case conf.AttachmentsStoreNone:
		fallthrough
	default:
		handleErr(w, req, http.StatusNotFound, "Attachments are not currently supported", nil)
	}

	const MiB float64 = 1 << 20
	reText := fmt.Sprintf("%v uploaded a file\nOriginal name:%v\nType: %v\nSize: %.3f MiB",
		jwtCtx.Claims.RangerHandle(), fiHead.Filename, sniffedContentType, float64(fiHead.Size)/MiB)
	err = addIncidentReportEntry(ctx, imsdb.New(action.imsDB), event.ID, incidentNumber,
		jwtCtx.Claims.RangerHandle(), reText, false, newFileName)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to add incident report entry", err)
		return
	}

	slog.Info("Saved incident attachment")
	action.es.notifyIncidentUpdate(event.Name, incidentNumber)
}

func (action AttachToFieldReport) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	event, jwtCtx, eventPermissions, ok := mustGetEventPermissions(w, req, action.imsDB, action.imsAdmins)
	if !ok {
		return
	}
	if eventPermissions&(authz.EventWriteAllFieldReports|authz.EventWriteOwnFieldReports) == 0 {
		handleErr(w, req, http.StatusForbidden, "The requestor does not have EventWriteFieldReports permission on this Event", nil)
		return
	}
	// i.e. the user has EventReadOwnFieldReports, but not EventReadAllFieldReports
	limitedAccess := eventPermissions&authz.EventReadAllFieldReports == 0
	ctx := req.Context()

	fieldReportNumber, err := conv.ParseInt32(req.PathValue("fieldReportNumber"))
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Failed to parse Field Report number", err)
		return
	}

	fieldReport, entries, err := fetchFieldReport(ctx, action.imsDB, event.ID, fieldReportNumber)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to fetch Field Report", err)
		return
	}
	if limitedAccess {
		if !containsAuthor(entries, jwtCtx.Claims.RangerHandle()) {
			handleErr(w, req, http.StatusForbidden, "The requestor does not have permission to read this particular Field Report", nil)
			return
		}
	}

	// this must match the key sent by the client
	fi, fiHead, err := req.FormFile("imsAttachment")
	if err != nil {
		handleErr(w, req, http.StatusBadRequest, "Failed to parse file", err)
		return
	}
	defer shut(fi)

	// This must be >= http.sniffLen (it's a private field, so we can't read it directly)
	const sniffLen = 512
	head := make([]byte, sniffLen)
	if _, err = fi.ReadAt(head, 0); err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to read head of file", err)
		return
	}

	// We'll detect the contentType and file extension, rather than trust any value from the client.
	sniffedContentType := http.DetectContentType(head)
	var extension string
	extensions, err := mime.ExtensionsByType(sniffedContentType)
	if err == nil && len(extensions) > 0 {
		extension = extensions[0]
	} else {
		slog.Info("Unable to determine a good file type for attachment. We'll leave it extension-free.",
			"sniffedContentType", sniffedContentType,
			"error", err,
		)
	}

	newFileName := fmt.Sprintf("event_%05d_fieldreport_%05d_%v%v", event.ID, fieldReportNumber, rand.Text(), extension)
	slog.Info("User is uploaded a Field Report attachment",
		"user", jwtCtx.Claims.RangerHandle(),
		"eventName", event.Name,
		"fieldReportNumber", fieldReportNumber,
		"originalName", fiHead.Filename,
		"newFileName", newFileName,
		"size", fiHead.Size,
		"contentType", sniffedContentType,
		"extension", extension,
	)

	switch action.attachmentsStore.Type {
	case conf.AttachmentsStoreLocal:
		outFi, err := action.attachmentsStore.Local.Dir.Create(newFileName)
		if err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to create file", err)
			return
		}
		defer shut(outFi)
		if _, err = io.Copy(outFi, fi); err != nil {
			handleErr(w, req, http.StatusInternalServerError, "Failed to write file", err)
			return
		}
	case conf.AttachmentsStoreS3:
		fallthrough
	case conf.AttachmentsStoreNone:
		fallthrough
	default:
		handleErr(w, req, http.StatusNotFound, "Attachments are not currently supported", nil)
	}

	const MiB float64 = 1 << 20
	reText := fmt.Sprintf("%v uploaded a file\nOriginal name:%v\nType: %v\nSize: %.3f MiB",
		jwtCtx.Claims.RangerHandle(), fiHead.Filename, sniffedContentType, float64(fiHead.Size)/MiB)
	err = addFRReportEntry(ctx, imsdb.New(action.imsDB), event.ID, fieldReportNumber,
		jwtCtx.Claims.RangerHandle(), reText, false, newFileName)
	if err != nil {
		handleErr(w, req, http.StatusInternalServerError, "Failed to add Field Report report entry", err)
		return
	}

	slog.Info("Saved Field Report attachment")
	action.es.notifyFieldReportUpdate(event.Name, fieldReportNumber)
	if fieldReport.IncidentNumber.Valid {
		action.es.notifyIncidentUpdate(event.Name, fieldReport.IncidentNumber.Int32)
	}
}
