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
	imsDBQ           *store.DBQ
	attachmentsStore conf.AttachmentsStore
	s3Client         *attachment.S3Client
	imsAdmins        []string
}

type AttachToIncident struct {
	imsDBQ           *store.DBQ
	es               *EventSourcerer
	attachmentsStore conf.AttachmentsStore
	s3Client         *attachment.S3Client
	imsAdmins        []string
}

type GetFieldReportAttachment struct {
	imsDBQ           *store.DBQ
	attachmentsStore conf.AttachmentsStore
	s3Client         *attachment.S3Client
	imsAdmins        []string
}

type AttachToFieldReport struct {
	imsDBQ           *store.DBQ
	es               *EventSourcerer
	attachmentsStore conf.AttachmentsStore
	s3Client         *attachment.S3Client
	imsAdmins        []string
}

func (action GetIncidentAttachment) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	file, errHTTP := action.getIncidentAttachment(req)
	if errHTTP != nil {
		errHTTP.From("[getIncidentAttachment]").WriteResponse(w)
		return
	}
	http.ServeContent(w, req, "Attached File", time.Now(), file)
}

func (action GetIncidentAttachment) getIncidentAttachment(req *http.Request) (io.ReadSeeker, *herr.HTTPError) {
	event, _, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.imsAdmins)
	if errHTTP != nil {
		return nil, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventReadIncidents == 0 {
		return nil, herr.Forbidden("The requestor does not have EventReadIncidents permission on this Event", nil)
	}
	ctx := req.Context()

	incidentNumber, err := conv.ParseInt32(req.PathValue("incidentNumber"))
	if err != nil {
		return nil, herr.BadRequest("Failed to parse incident number", err).From("[ParseInt32]")
	}
	attachmentNumber, err := conv.ParseInt32(req.PathValue("attachmentNumber"))
	if err != nil {
		return nil, herr.BadRequest("Failed to parse attachment number", err).From("[ParseInt32]")
	}

	_, reportEntries, errHTTP := fetchIncident(ctx, action.imsDBQ, event.ID, incidentNumber)
	if errHTTP != nil {
		return nil, errHTTP.From("[fetchIncident]")
	}

	var filename string
	for _, reportEntry := range reportEntries {
		if reportEntry.ID == attachmentNumber {
			filename = reportEntry.AttachedFile.String
			break
		}
	}
	if filename == "" {
		return nil, herr.NotFound("No attachment for this ID", nil)
	}

	var file io.ReadSeeker
	switch action.attachmentsStore.Type {
	case conf.AttachmentsStoreLocal:
		file, err = action.attachmentsStore.Local.Dir.Open(filename)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, herr.NotFound("File does not exist", nil)
			}
			return nil, herr.InternalServerError("Failed to open file", err)
		}
	case conf.AttachmentsStoreS3:
		file, errHTTP = mustGetS3File(ctx, action.s3Client, action.attachmentsStore.S3.Bucket, action.attachmentsStore.S3.CommonKeyPrefix, filename)
		if errHTTP != nil {
			return nil, errHTTP.From("[mustGetS3File]")
		}
	default:
		return nil, herr.NotFound("Attachments are not currently supported", nil)
	}

	return file, nil
}

func mustGetS3File(ctx context.Context, s3Client *attachment.S3Client, bucket, prefix, filename string) (io.ReadSeeker, *herr.HTTPError) {
	file, errHTTP := s3Client.GetObject(ctx, bucket, prefix+filename)
	if errHTTP != nil {
		return nil, errHTTP.From("[GetObject]")
	}
	return file, nil
}

func (action GetFieldReportAttachment) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	file, errHTTP := action.getFieldReportAttachment(req)
	if errHTTP != nil {
		errHTTP.From("[getFieldReportAttachment]").WriteResponse(w)
		return
	}
	http.ServeContent(w, req, "Attached File", time.Now(), file)
}

func (action GetFieldReportAttachment) getFieldReportAttachment(req *http.Request) (io.ReadSeeker, *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.imsAdmins)
	if errHTTP != nil {
		return nil, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&(authz.EventReadAllFieldReports|authz.EventReadOwnFieldReports) == 0 {
		return nil, herr.Forbidden("The requestor does not have permission to read Field Reports on this Event", nil)
	}
	// i.e. the user has EventReadOwnFieldReports, but not EventReadAllFieldReports
	limitedAccess := eventPermissions&authz.EventReadAllFieldReports == 0

	ctx := req.Context()

	fieldReportNumber, err := conv.ParseInt32(req.PathValue("fieldReportNumber"))
	if err != nil {
		return nil, herr.BadRequest("Failed to parse Field Report number", err).From("[ParseInt32]")
	}
	attachmentNumber, err := conv.ParseInt32(req.PathValue("attachmentNumber"))
	if err != nil {
		return nil, herr.BadRequest("Failed to parse attachment number", err).From("[ParseInt32]")
	}

	_, reportEntries, errHTTP := fetchFieldReport(ctx, action.imsDBQ, event.ID, fieldReportNumber)
	if errHTTP != nil {
		return nil, errHTTP.From("[fetchFieldReport]")
	}

	if limitedAccess {
		if !containsAuthor(reportEntries, jwtCtx.Claims.RangerHandle()) {
			return nil, herr.Forbidden("The requestor does not have permission to read this particular Field Report", nil)
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
		return nil, herr.NotFound("No attachment for this ID", nil)
	}

	var file io.ReadSeeker
	switch action.attachmentsStore.Type {
	case conf.AttachmentsStoreLocal:
		file, err = action.attachmentsStore.Local.Dir.Open(filename)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, herr.NotFound("File does not exist", nil)
			}
			return nil, herr.InternalServerError("Failed to open file", err)
		}
	case conf.AttachmentsStoreS3:
		file, errHTTP = mustGetS3File(ctx, action.s3Client, action.attachmentsStore.S3.Bucket, action.attachmentsStore.S3.CommonKeyPrefix, filename)
		if errHTTP != nil {
			return nil, errHTTP.From("[mustGetS3File]")
		}
	default:
		return nil, herr.NotFound("Attachments are not currently supported", nil)
	}
	return file, nil
}

func (action AttachToIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	reID, errHTTP := action.attachToIncident(req)
	if errHTTP != nil {
		errHTTP.From("[attachToIncident]").WriteResponse(w)
		return
	}
	slog.Info("Saved Incident attachment")
	w.Header().Set("IMS-Report-Entry-Number", conv.FormatInt32(reID))
	http.Error(w, "Saved Incident attachment", http.StatusNoContent)
}

func (action AttachToIncident) attachToIncident(req *http.Request) (int32, *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.imsAdmins)
	if errHTTP != nil {
		return 0, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventWriteIncidents == 0 {
		return 0, herr.Forbidden("The requestor does not have EventWriteIncidents permission on this Event", nil)
	}
	ctx := req.Context()

	incidentNumber, err := conv.ParseInt32(req.PathValue("incidentNumber"))
	if err != nil {
		return 0, herr.BadRequest("Failed to parse incident number", err).From("[ParseInt32]")
	}

	// this must match the key sent by the client
	fi, fiHead, err := req.FormFile(IMSAttachmentFormKey)
	if err != nil {
		return 0, herr.BadRequest("Failed to parse file", err)
	}
	defer shut(fi)

	sniffedContentType, extension, errHTTP := sniffFile(fi)
	if errHTTP != nil {
		return 0, errHTTP.From("[sniffFile]")
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
			return 0, herr.InternalServerError("Failed to create file", err).From("[Create]")
		}
		defer shut(outFi)
		if _, err = io.Copy(outFi, fi); err != nil {
			return 0, herr.InternalServerError("Failed to write file", err).From("[Copy]")
		}
	case conf.AttachmentsStoreS3:
		s3Name := action.attachmentsStore.S3.CommonKeyPrefix + newFileName
		if errHTTP = action.s3Client.UploadToS3(ctx, action.attachmentsStore.S3.Bucket, s3Name, fi); errHTTP != nil {
			return 0, errHTTP.From("[UploadToS3]")
		}
	default:
		return 0, herr.NotFound("Attachments are not currently supported", nil)
	}

	reText := fmt.Sprintf("%v uploaded a file\nOriginal name:%v\nType: %v\nSize: %v",
		jwtCtx.Claims.RangerHandle(), fiHead.Filename, sniffedContentType, format.HumanByteSize(fiHead.Size))
	reID, errHTTP := addIncidentReportEntry(ctx, action.imsDBQ, action.imsDBQ, event.ID, incidentNumber,
		jwtCtx.Claims.RangerHandle(), reText, false, newFileName)
	if errHTTP != nil {
		return 0, errHTTP.From("[addIncidentReportEntry]")
	}

	action.es.notifyIncidentUpdate(event.Name, incidentNumber)
	return reID, nil
}

func (action AttachToFieldReport) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	reID, errHTTP := action.attachToFieldReport(req)
	if errHTTP != nil {
		errHTTP.From("[attachToFieldReport]").WriteResponse(w)
		return
	}
	slog.Info("Saved Field Report attachment")
	w.Header().Set("IMS-Report-Entry-Number", conv.FormatInt32(reID))
	http.Error(w, "Saved Field Report attachment", http.StatusNoContent)
}
func (action AttachToFieldReport) attachToFieldReport(req *http.Request) (int32, *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.imsAdmins)
	if errHTTP != nil {
		return 0, errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&(authz.EventWriteAllFieldReports|authz.EventWriteOwnFieldReports) == 0 {
		return 0, herr.Forbidden("The requestor does not have permission to write Field Reports on this Event", nil)
	}
	// i.e. the user has EventReadOwnFieldReports, but not EventReadAllFieldReports
	limitedAccess := eventPermissions&authz.EventReadAllFieldReports == 0
	ctx := req.Context()

	fieldReportNumber, err := conv.ParseInt32(req.PathValue("fieldReportNumber"))
	if err != nil {
		return 0, herr.BadRequest("Failed to parse Field Report number", err).From("[ParseInt32]")
	}

	fieldReport, entries, errHTTP := fetchFieldReport(ctx, action.imsDBQ, event.ID, fieldReportNumber)
	if errHTTP != nil {
		return 0, errHTTP.From("[fetchFieldReport]")
	}
	if limitedAccess {
		if !containsAuthor(entries, jwtCtx.Claims.RangerHandle()) {
			return 0, herr.Forbidden("The requestor does not have permission to read this particular Field Report", nil)
		}
	}

	// this must match the key sent by the client
	fi, fiHead, err := req.FormFile(IMSAttachmentFormKey)
	if err != nil {
		return 0, herr.BadRequest("Failed to parse file", err)
	}
	defer shut(fi)

	sniffedContentType, extension, errHTTP := sniffFile(fi)
	if errHTTP != nil {
		return 0, errHTTP.From("[sniffFile]")
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
			return 0, herr.InternalServerError("Failed to create file", err)
		}
		defer shut(outFi)
		if _, err = io.Copy(outFi, fi); err != nil {
			return 0, herr.InternalServerError("Failed to write file", err)
		}
	case conf.AttachmentsStoreS3:
		s3Name := action.attachmentsStore.S3.CommonKeyPrefix + newFileName
		if errHTTP = action.s3Client.UploadToS3(ctx, action.attachmentsStore.S3.Bucket, s3Name, fi); errHTTP != nil {
			return 0, errHTTP.From("[UploadToS3]")
		}
	default:
		return 0, herr.NotFound("Attachments are not currently supported", nil)
	}

	reText := fmt.Sprintf("%v uploaded a file\nOriginal name:%v\nType: %v\nSize: %v",
		jwtCtx.Claims.RangerHandle(), fiHead.Filename, sniffedContentType, format.HumanByteSize(fiHead.Size))
	reID, errHTTP := addFRReportEntry(ctx, action.imsDBQ, action.imsDBQ, event.ID, fieldReportNumber,
		jwtCtx.Claims.RangerHandle(), reText, false, newFileName)
	if errHTTP != nil {
		return 0, errHTTP.From("[addFRReportEntry]")
	}

	action.es.notifyFieldReportUpdate(event.Name, fieldReportNumber)
	if fieldReport.IncidentNumber.Valid {
		action.es.notifyIncidentUpdate(event.Name, fieldReport.IncidentNumber.Int32)
	}
	return reID, nil
}

func sniffFile(fi multipart.File) (contentType string, extension string, errHTTP *herr.HTTPError) {
	// This must be >= http.sniffLen (it's a private field, so we can't read it directly)
	const sniffLen = 512
	head := make([]byte, sniffLen)
	if n, err := fi.ReadAt(head, 0); err != nil {
		// It's fine if the file is less than sniffLen bytes long.
		// We just need to shorten the byte slice afterward to the actual file length.
		if errors.Is(err, io.EOF) {
			head = head[:n]
		} else {
			return "", "", herr.InternalServerError("Failed to detect content type", err).From("[ReadAt]")
		}
	}

	// We'll detect the contentType and file extension, rather than trust any value from the client.
	sniffedContentType := http.DetectContentType(head)

	return sniffedContentType, extensionByType(sniffedContentType), nil
}

func extensionByType(contentType string) string {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "" {
		return ""
	}

	// We mostly rely on mime.ExtensionsByType, but just picking the first element of that list
	// often gives a weird extension. e.g. image/jpeg --> ".jpe". Hence, we special-case some
	// common MIME types below.
	switch mediaType {
	case "image/jpeg":
		return ".jpg"
	case "text/html":
		return ".html"
	case "text/plain":
		return ".txt"
	case "video/mp4":
		return ".mp4"
	default:
		extensions, _ := mime.ExtensionsByType(contentType)
		if len(extensions) > 0 {
			return extensions[0]
		}
	}
	return ""
}
