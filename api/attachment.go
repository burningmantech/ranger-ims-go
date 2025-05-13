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
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/lib/attachment"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/format"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

const IMSAttachmentFormKey = "imsAttachment"

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
	file, hErr := action.getIncidentAttachment(req)
	if hErr != nil {
		hErr.Src("[getIncidentAttachment]").WriteResponse(w)
		return
	}
	http.ServeContent(w, req, "Attached File", time.Now(), file)
}

func (action GetIncidentAttachment) getIncidentAttachment(req *http.Request) (io.ReadSeeker, *herr.HTTPError) {
	event, _, eventPermissions, errH := mustGetEventPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return nil, errH.Src("[mustGetEventPermissions]")
	}
	if eventPermissions&authz.EventReadIncidents == 0 {
		return nil, herr.S403("The requestor does not have EventReadIncidents permission on this Event", nil)
	}
	ctx := req.Context()

	incidentNumber, err := conv.ParseInt32(req.PathValue("incidentNumber"))
	if err != nil {
		return nil, herr.S400("Failed to parse incident number", err).Src("[ParseInt32]")
	}
	attachmentNumber, err := conv.ParseInt32(req.PathValue("attachmentNumber"))
	if err != nil {
		return nil, herr.S400("Failed to parse attachment number", err).Src("[ParseInt32]")
	}

	_, reportEntries, errH := fetchIncident(ctx, action.imsDB, event.ID, incidentNumber)
	if errH != nil {
		return nil, errH.Src("[fetchIncident]")
	}

	var filename string
	for _, reportEntry := range reportEntries {
		if reportEntry.ID == attachmentNumber {
			filename = reportEntry.AttachedFile.String
			break
		}
	}
	if filename == "" {
		return nil, herr.S404("No attachment for this ID", nil)
	}

	var file io.ReadSeeker
	switch action.attachmentsStore.Type {
	case conf.AttachmentsStoreLocal:
		file, err = action.attachmentsStore.Local.Dir.Open(filename)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, herr.S404("File does not exist", nil)
			}
			return nil, herr.S500("Failed to open file", err)
		}
	case conf.AttachmentsStoreS3:
		file, errH = mustGetS3File(ctx, action.attachmentsStore.S3.Bucket, action.attachmentsStore.S3.CommonKeyPrefix, filename)
		if errH != nil {
			return nil, errH.Src("[mustGetS3File]")
		}
	default:
		return nil, herr.S404("Attachments are not currently supported", nil)
	}

	return file, nil
}

func mustGetS3File(ctx context.Context, bucket, prefix, filename string) (io.ReadSeeker, *herr.HTTPError) {
	s3Name := prefix + filename
	var exists bool
	file, exists, err := attachment.GetObject(ctx, bucket, s3Name)
	if err != nil {
		return nil, herr.S500("Failed to get attachment", err).Src("[GetObject]")
	}
	if !exists {
		return nil, herr.S404("File does not exist", nil)
	}
	return file, nil
}

func (action GetFieldReportAttachment) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	file, hErr := action.getFieldReportAttachment(req)
	if hErr != nil {
		hErr.Src("[getFieldReportAttachment]").WriteResponse(w)
		return
	}
	http.ServeContent(w, req, "Attached File", time.Now(), file)
}

func (action GetFieldReportAttachment) getFieldReportAttachment(req *http.Request) (io.ReadSeeker, *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errH := mustGetEventPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return nil, errH.Src("[mustGetEventPermissions]")
	}
	if eventPermissions&(authz.EventReadAllFieldReports|authz.EventReadOwnFieldReports) == 0 {
		return nil, herr.S403("The requestor does not have permission to read Field Reports on this Event", nil)
	}
	// i.e. the user has EventReadOwnFieldReports, but not EventReadAllFieldReports
	limitedAccess := eventPermissions&authz.EventReadAllFieldReports == 0

	ctx := req.Context()

	fieldReportNumber, err := conv.ParseInt32(req.PathValue("fieldReportNumber"))
	if err != nil {
		return nil, herr.S400("Failed to parse Field Report number", err).Src("[ParseInt32]")
	}
	attachmentNumber, err := conv.ParseInt32(req.PathValue("attachmentNumber"))
	if err != nil {
		return nil, herr.S400("Failed to parse attachment number", err).Src("[ParseInt32]")
	}

	_, reportEntries, errH := fetchFieldReport(ctx, action.imsDB, event.ID, fieldReportNumber)
	if errH != nil {
		return nil, errH.Src("[fetchFieldReport]")
	}

	if limitedAccess {
		if !containsAuthor(reportEntries, jwtCtx.Claims.RangerHandle()) {
			return nil, herr.S403("The requestor does not have permission to read this particular Field Report", nil)
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
		return nil, herr.S404("No attachment for this ID", nil)
	}

	var file io.ReadSeeker
	switch action.attachmentsStore.Type {
	case conf.AttachmentsStoreLocal:
		file, err = action.attachmentsStore.Local.Dir.Open(filename)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, herr.S404("File does not exist", nil)
			}
			return nil, herr.S500("Failed to open file", err)
		}
	case conf.AttachmentsStoreS3:
		file, errH = mustGetS3File(ctx, action.attachmentsStore.S3.Bucket, action.attachmentsStore.S3.CommonKeyPrefix, filename)
		if errH != nil {
			return nil, errH.Src("[mustGetS3File]")
		}
	default:
		return nil, herr.S404("Attachments are not currently supported", nil)
	}
	return file, nil
}

func (action AttachToIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	reID, hErr := action.attachToIncident(req)
	if hErr != nil {
		hErr.Src("[attachToIncident]").WriteResponse(w)
		return
	}
	slog.Info("Saved Incident attachment")
	w.Header().Set("IMS-Report-Entry-Number", conv.FormatInt32(reID))
	http.Error(w, "Saved Incident attachment", http.StatusNoContent)
}

func (action AttachToIncident) attachToIncident(req *http.Request) (int32, *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errH := mustGetEventPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return 0, errH.Src("[mustGetEventPermissions]")
	}
	if eventPermissions&authz.EventWriteIncidents == 0 {
		return 0, herr.S403("The requestor does not have EventWriteIncidents permission on this Event", nil)
	}
	ctx := req.Context()

	incidentNumber, err := conv.ParseInt32(req.PathValue("incidentNumber"))
	if err != nil {
		return 0, herr.S400("Failed to parse incident number", err).Src("[ParseInt32]")
	}

	// this must match the key sent by the client
	fi, fiHead, err := req.FormFile(IMSAttachmentFormKey)
	if err != nil {
		return 0, herr.S400("Failed to parse file", err)
	}
	defer shut(fi)

	sniffedContentType, extension, errH := sniffFile(fi)
	if errH != nil {
		return 0, errH.Src("[sniffFile]")
	}

	newFileName := fmt.Sprintf("event_%05d_incident_%05d_%v%v", event.ID, incidentNumber, rand.Text(), extension)
	slog.Info("User uploaded an incident attachment",
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
			return 0, herr.S500("Failed to create file", err).Src("[Create]")
		}
		defer shut(outFi)
		if _, err = io.Copy(outFi, fi); err != nil {
			return 0, herr.S500("Failed to write file", err).Src("[Copy]")
		}
	case conf.AttachmentsStoreS3:
		s3Name := action.attachmentsStore.S3.CommonKeyPrefix + newFileName
		if err = attachment.UploadToS3(ctx, action.attachmentsStore.S3.Bucket, s3Name, fi); err != nil {
			return 0, herr.S500("Failed to upload file to S3", err).Src("[UploadToS3]")
		}
	default:
		return 0, herr.S404("Attachments are not currently supported", nil)
	}

	reText := fmt.Sprintf("%v uploaded a file\nOriginal name:%v\nType: %v\nSize: %v",
		jwtCtx.Claims.RangerHandle(), fiHead.Filename, sniffedContentType, format.HumanByteSize(fiHead.Size))
	reID, errH := addIncidentReportEntry(ctx, imsdb.New(action.imsDB), event.ID, incidentNumber,
		jwtCtx.Claims.RangerHandle(), reText, false, newFileName)
	if errH != nil {
		return 0, errH.Src("[addIncidentReportEntry]")
	}

	action.es.notifyIncidentUpdate(event.Name, incidentNumber)
	return reID, nil
}

func (action AttachToFieldReport) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	reID, hErr := action.attachToFieldReport(req)
	if hErr != nil {
		hErr.Src("[attachToFieldReport]").WriteResponse(w)
		return
	}
	slog.Info("Saved Field Report attachment")
	w.Header().Set("IMS-Report-Entry-Number", conv.FormatInt32(reID))
	http.Error(w, "Saved Field Report attachment", http.StatusNoContent)
}
func (action AttachToFieldReport) attachToFieldReport(req *http.Request) (int32, *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errH := mustGetEventPermissions(req, action.imsDB, action.imsAdmins)
	if errH != nil {
		return 0, errH.Src("[mustGetEventPermissions]")
	}
	if eventPermissions&(authz.EventWriteAllFieldReports|authz.EventWriteOwnFieldReports) == 0 {
		return 0, herr.S403("The requestor does not have permission to write Field Reports on this Event", nil)
	}
	// i.e. the user has EventReadOwnFieldReports, but not EventReadAllFieldReports
	limitedAccess := eventPermissions&authz.EventReadAllFieldReports == 0
	ctx := req.Context()

	fieldReportNumber, err := conv.ParseInt32(req.PathValue("fieldReportNumber"))
	if err != nil {
		return 0, herr.S400("Failed to parse Field Report number", err).Src("[ParseInt32]")
	}

	fieldReport, entries, errH := fetchFieldReport(ctx, action.imsDB, event.ID, fieldReportNumber)
	if errH != nil {
		return 0, errH.Src("[fetchFieldReport]")
	}
	if limitedAccess {
		if !containsAuthor(entries, jwtCtx.Claims.RangerHandle()) {
			return 0, herr.S403("The requestor does not have permission to read this particular Field Report", nil)
		}
	}

	// this must match the key sent by the client
	fi, fiHead, err := req.FormFile(IMSAttachmentFormKey)
	if err != nil {
		return 0, herr.S400("Failed to parse file", err)
	}
	defer shut(fi)

	sniffedContentType, extension, errH := sniffFile(fi)
	if errH != nil {
		return 0, errH.Src("[sniffFile]")
	}

	newFileName := fmt.Sprintf("event_%05d_fieldreport_%05d_%v%v", event.ID, fieldReportNumber, rand.Text(), extension)
	slog.Info("User uploaded a Field Report attachment",
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
			return 0, herr.S500("Failed to create file", err)
		}
		defer shut(outFi)
		if _, err = io.Copy(outFi, fi); err != nil {
			return 0, herr.S500("Failed to write file", err)
		}
	case conf.AttachmentsStoreS3:
		s3Name := action.attachmentsStore.S3.CommonKeyPrefix + newFileName
		if err = attachment.UploadToS3(ctx, action.attachmentsStore.S3.Bucket, s3Name, fi); err != nil {
			return 0, herr.S500("Failed to upload file to S3", err)
		}
	default:
		return 0, herr.S404("Attachments are not currently supported", nil)
	}

	reText := fmt.Sprintf("%v uploaded a file\nOriginal name:%v\nType: %v\nSize: %v",
		jwtCtx.Claims.RangerHandle(), fiHead.Filename, sniffedContentType, format.HumanByteSize(fiHead.Size))
	reID, errH := addFRReportEntry(ctx, imsdb.New(action.imsDB), event.ID, fieldReportNumber,
		jwtCtx.Claims.RangerHandle(), reText, false, newFileName)
	if errH != nil {
		return 0, errH.Src("[addFRReportEntry]")
	}

	action.es.notifyFieldReportUpdate(event.Name, fieldReportNumber)
	if fieldReport.IncidentNumber.Valid {
		action.es.notifyIncidentUpdate(event.Name, fieldReport.IncidentNumber.Int32)
	}
	return reID, nil
}

func sniffFile(fi multipart.File) (contentType string, extension string, errH *herr.HTTPError) {
	// This must be >= http.sniffLen (it's a private field, so we can't read it directly)
	const sniffLen = 512
	head := make([]byte, sniffLen)
	if n, err := fi.ReadAt(head, 0); err != nil {
		// It's fine if the file is less than sniffLen bytes long.
		// We just need to shorten the byte slice afterward to the actual file length.
		if errors.Is(err, io.EOF) {
			head = head[:n]
		} else {
			return "", "", herr.S500("Failed to detect content type", err).Src("[ReadAt]")
		}
	}

	// We'll detect the contentType and file extension, rather than trust any value from the client.
	sniffedContentType := http.DetectContentType(head)

	return sniffedContentType, extensionByType(sniffedContentType), nil
}

func extensionByType(contentType string) string {
	var extension string

	// We mostly rely on mime.ExtensionsByType, but just picking the first element of that list
	// often gives a weird extension. e.g. image/jpeg --> ".jpe". Hence, we special-case some
	// common MIME types below.
	switch contentType {
	case "image/jpeg", "image/jpg":
		extension = ".jpg"
	case "text/plain", "text/plain; charset=utf-8":
		extension = ".txt"
	default:
		extensions, _ := mime.ExtensionsByType(contentType)
		if len(extensions) > 0 {
			extension = extensions[0]
		}
	}
	return extension
}
