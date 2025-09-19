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
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/attachment"
	"github.com/burningmantech/ranger-ims-go/lib/authz"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/format"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/gabriel-vasile/mimetype"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
)

const (
	IMSAttachmentFormKey = "imsAttachment"
	octetStream          = "application/octet-stream"
)

type GetIncidentAttachment struct {
	imsDBQ           *store.DBQ
	userStore        *directory.UserStore
	attachmentsStore conf.AttachmentsStore
	s3Client         *attachment.S3Client
	imsAdmins        []string
}

type AttachToIncident struct {
	imsDBQ           *store.DBQ
	userStore        *directory.UserStore
	es               *EventSourcerer
	attachmentsStore conf.AttachmentsStore
	s3Client         *attachment.S3Client
	imsAdmins        []string
}

type GetFieldReportAttachment struct {
	imsDBQ           *store.DBQ
	userStore        *directory.UserStore
	attachmentsStore conf.AttachmentsStore
	s3Client         *attachment.S3Client
	imsAdmins        []string
}

type AttachToFieldReport struct {
	imsDBQ           *store.DBQ
	userStore        *directory.UserStore
	es               *EventSourcerer
	attachmentsStore conf.AttachmentsStore
	s3Client         *attachment.S3Client
	imsAdmins        []string
}

func (action GetIncidentAttachment) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	file, contentType, errHTTP := action.getIncidentAttachment(req)
	if errHTTP != nil {
		errHTTP.From("[getIncidentAttachment]").WriteResponse(w)
		return
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, req, "Attached File", time.Now(), file)
}

func (action GetIncidentAttachment) getIncidentAttachment(
	req *http.Request,
) (fi io.ReadSeeker, contentType string, errHTTP *herr.HTTPError) {
	event, _, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return nil, "", errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&authz.EventReadIncidents == 0 {
		return nil, "", herr.Forbidden("The requestor does not have EventReadIncidents permission on this Event", nil)
	}
	ctx := req.Context()

	incidentNumber, err := conv.ParseInt32(req.PathValue("incidentNumber"))
	if err != nil {
		return nil, "", herr.BadRequest("Failed to parse incident number", err).From("[ParseInt32]")
	}
	attachmentNumber, err := conv.ParseInt32(req.PathValue("attachmentNumber"))
	if err != nil {
		return nil, "", herr.BadRequest("Failed to parse attachment number", err).From("[ParseInt32]")
	}

	_, reportEntries, errHTTP := fetchIncident(ctx, action.imsDBQ, event.ID, incidentNumber)
	if errHTTP != nil {
		return nil, "", errHTTP.From("[fetchIncident]")
	}

	var filename string
	for _, reportEntry := range reportEntries {
		if reportEntry.ID == attachmentNumber {
			filename = reportEntry.AttachedFile.String
			break
		}
	}

	file, errHTTP := retrieveFile(ctx, action.attachmentsStore, action.s3Client, filename)
	if errHTTP != nil {
		return nil, "", errHTTP.From("[retrieveFile]")
	}

	mtype, errHTTP := sniffFile(file)
	if errHTTP != nil {
		return nil, "", errHTTP.From("[sniffFile]")
	}
	contentType = safeToPreviewContentType(mtype.String())

	return file, contentType, nil
}

var safeToPreviewMediaTypes = []string{
	"application/pdf",
	"image/gif",
	"image/heic",
	"image/jpeg",
	"image/png",
	"image/tiff",
	"image/webp",
	"text/plain",
	"video/mp4",
	"video/x-msvideo",
}

// safeToPreviewContentType returns a safe form of contentType if possible, or octetStream otherwise.
//
// This is important for the client side. For example, if we're serving an HTML document,
// we want the client to think it's just text/plain, so that it doesn't attempt to render it.
// SVG graphics are similarly a problem, since they can include scripting. The client
// previews these attachments in the same origin as IMS, which leaves us open to XSS attacks
// for unsafe files. This function works conservatively by returning octetStream unless we
// know the content type ought to be safe.
func safeToPreviewContentType(contentType string) string {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return octetStream
	}
	if slices.Contains(safeToPreviewMediaTypes, mediaType) {
		return contentType
	}
	// Any Chromium browser won't play videos with video/quicktime media type (https://crbug.com/40714674),
	// but it will play those same videos if you use video/mp4 as the media type. This is technically incorrect,
	// but it works for Chromium browsers, and is also not a problem for Firefox or Safari.
	if mediaType == "video/quicktime" {
		return mime.FormatMediaType("video/mp4", nil)
	}
	if strings.HasPrefix(mediaType, "text/") {
		return mime.FormatMediaType("text/plain", params)
	}
	return mime.FormatMediaType(octetStream, nil)
}

func previewableContentType(contentType string) bool {
	return safeToPreviewContentType(contentType) != octetStream
}

func retrieveFile(
	ctx context.Context, attachmentsStore conf.AttachmentsStore,
	s3Client *attachment.S3Client, filename string,
) (io.ReadSeeker, *herr.HTTPError) {
	if filename == "" {
		return nil, herr.NotFound("No attachment for this ID", nil)
	}
	var file io.ReadSeeker
	var err error
	var errHTTP *herr.HTTPError
	switch attachmentsStore.Type {
	case conf.AttachmentsStoreLocal:
		file, err = attachmentsStore.Local.Dir.Open(filename)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, herr.NotFound("File does not exist", nil)
			}
			return nil, herr.InternalServerError("Failed to open file", err).From("[Open]")
		}
	case conf.AttachmentsStoreS3:
		file, errHTTP = mustGetS3File(ctx, s3Client, attachmentsStore.S3.Bucket, attachmentsStore.S3.CommonKeyPrefix, filename)
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
	file, contentType, errHTTP := action.getFieldReportAttachment(req)
	if errHTTP != nil {
		errHTTP.From("[getFieldReportAttachment]").WriteResponse(w)
		return
	}
	w.Header().Set("Content-Type", contentType)
	http.ServeContent(w, req, "Attached File", time.Now(), file)
}

func (action GetFieldReportAttachment) getFieldReportAttachment(
	req *http.Request,
) (fi io.ReadSeeker, contentType string, errHTTP *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
	if errHTTP != nil {
		return nil, "", errHTTP.From("[getEventPermissions]")
	}
	if eventPermissions&(authz.EventReadAllFieldReports|authz.EventReadOwnFieldReports) == 0 {
		return nil, "", herr.Forbidden("The requestor does not have permission to read Field Reports on this Event", nil)
	}
	// i.e. the user has EventReadOwnFieldReports, but not EventReadAllFieldReports
	limitedAccess := eventPermissions&authz.EventReadAllFieldReports == 0

	ctx := req.Context()

	fieldReportNumber, err := conv.ParseInt32(req.PathValue("fieldReportNumber"))
	if err != nil {
		return nil, "", herr.BadRequest("Failed to parse Field Report number", err).From("[ParseInt32]")
	}
	attachmentNumber, err := conv.ParseInt32(req.PathValue("attachmentNumber"))
	if err != nil {
		return nil, "", herr.BadRequest("Failed to parse attachment number", err).From("[ParseInt32]")
	}

	_, reportEntries, errHTTP := fetchFieldReport(ctx, action.imsDBQ, event.ID, fieldReportNumber)
	if errHTTP != nil {
		return nil, "", errHTTP.From("[fetchFieldReport]")
	}

	if limitedAccess {
		if !containsAuthor(reportEntries, jwtCtx.Claims.RangerHandle()) {
			return nil, "", herr.Forbidden("The requestor does not have permission to read this particular Field Report", nil)
		}
	}

	var filename string
	for _, reportEntry := range reportEntries {
		if reportEntry.ID == attachmentNumber {
			filename = reportEntry.AttachedFile.String
			break
		}
	}

	file, errHTTP := retrieveFile(ctx, action.attachmentsStore, action.s3Client, filename)
	if errHTTP != nil {
		return nil, "", errHTTP.From("[retrieveFile]")
	}

	mtype, errHTTP := sniffFile(file)
	if errHTTP != nil {
		return nil, "", errHTTP.From("[sniffFile]")
	}
	contentType = safeToPreviewContentType(mtype.String())

	return file, contentType, nil
}

func (action AttachToIncident) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	reID, errHTTP := action.attachToIncident(req)
	if errHTTP != nil {
		errHTTP.From("[attachToIncident]").WriteResponse(w)
		return
	}
	slog.Info("Saved Incident attachment")
	w.Header().Set("IMS-Report-Entry-Number", conv.FormatInt(reID))
	herr.WriteNoContentResponse(w, "Saved Incident attachment")
}

func (action AttachToIncident) attachToIncident(req *http.Request) (int32, *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
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
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return 0, herr.RequestEntityTooLarge(fmt.Sprintf("The supplied file is above the server limit of %v", format.HumanByteSize(mbe.Limit)), err)
		}
		return 0, herr.BadRequest("Failed to parse file", err)
	}
	defer shut(fi)

	mtype, errHTTP := sniffFile(fi)
	if errHTTP != nil {
		return 0, errHTTP.From("[sniffFile]")
	}

	newFileName := fmt.Sprintf("event_%05d_incident_%05d_%v%v", event.ID, incidentNumber, rand.Text(), mtype.Extension())
	slog.Info("User uploaded an incident attachment",
		"user", jwtCtx.Claims.RangerHandle(),
		"eventName", event.Name,
		"incidentNumber", incidentNumber,
		"originalName", fiHead.Filename,
		"newFileName", newFileName,
		"size", fiHead.Size,
		"contentType", mtype.String(),
		"extension", mtype.Extension(),
	)

	errHTTP = saveFile(ctx, action.attachmentsStore, action.s3Client, newFileName, fi)
	if errHTTP != nil {
		return 0, errHTTP.From("[saveFile]")
	}

	reText := fmt.Sprintf("File Name: %v, Size: %v, Type:%v",
		fiHead.Filename, format.HumanByteSize(fiHead.Size), mtype.String())
	reID, errHTTP := addIncidentReportEntry(
		ctx, action.imsDBQ, action.imsDBQ, event.ID, incidentNumber, jwtCtx.Claims.RangerHandle(),
		reText, false, newFileName, fiHead.Filename, mtype.String(),
	)
	if errHTTP != nil {
		return 0, errHTTP.From("[addIncidentReportEntry]")
	}

	action.es.notifyIncidentUpdateV2(event.ID, incidentNumber)
	return reID, nil
}

func saveFile(
	ctx context.Context, attachmentsStore conf.AttachmentsStore,
	s3Client *attachment.S3Client, newFileName string, fi multipart.File,
) *herr.HTTPError {
	switch attachmentsStore.Type {
	case conf.AttachmentsStoreLocal:
		outFi, err := attachmentsStore.Local.Dir.Create(newFileName)
		if err != nil {
			return herr.InternalServerError("Failed to create file", err).From("[Create]")
		}
		defer shut(outFi)
		_, err = io.Copy(outFi, fi)
		if err != nil {
			return herr.InternalServerError("Failed to write file", err).From("[Copy]")
		}
	case conf.AttachmentsStoreS3:
		s3Name := attachmentsStore.S3.CommonKeyPrefix + newFileName
		errHTTP := s3Client.UploadToS3(ctx, attachmentsStore.S3.Bucket, s3Name, fi)
		if errHTTP != nil {
			return errHTTP.From("[UploadToS3]")
		}
	default:
		return herr.NotFound("Attachments are not currently supported", nil)
	}
	return nil
}

func (action AttachToFieldReport) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	reID, errHTTP := action.attachToFieldReport(req)
	if errHTTP != nil {
		errHTTP.From("[attachToFieldReport]").WriteResponse(w)
		return
	}
	slog.Info("Saved Field Report attachment")
	w.Header().Set("IMS-Report-Entry-Number", conv.FormatInt(reID))
	herr.WriteNoContentResponse(w, "Saved Field Report attachment")
}
func (action AttachToFieldReport) attachToFieldReport(req *http.Request) (int32, *herr.HTTPError) {
	event, jwtCtx, eventPermissions, errHTTP := getEventPermissions(req, action.imsDBQ, action.userStore, action.imsAdmins)
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
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return 0, herr.RequestEntityTooLarge(fmt.Sprintf("The supplied file is above the server limit of %v", format.HumanByteSize(mbe.Limit)), err)
		}
		return 0, herr.BadRequest("Failed to parse file", err)
	}
	defer shut(fi)

	mtype, errHTTP := sniffFile(fi)
	if errHTTP != nil {
		return 0, errHTTP.From("[sniffFile]")
	}

	newFileName := fmt.Sprintf("event_%05d_fieldreport_%05d_%v%v", event.ID, fieldReportNumber, rand.Text(), mtype.Extension())
	slog.Info("User uploaded a Field Report attachment",
		"user", jwtCtx.Claims.RangerHandle(),
		"eventName", event.Name,
		"fieldReportNumber", fieldReportNumber,
		"originalName", fiHead.Filename,
		"newFileName", newFileName,
		"size", fiHead.Size,
		"contentType", mtype.String(),
		"extension", mtype.Extension(),
	)

	errHTTP = saveFile(ctx, action.attachmentsStore, action.s3Client, newFileName, fi)
	if errHTTP != nil {
		return 0, errHTTP.From("[saveFile]")
	}

	reText := fmt.Sprintf("File Name: %v, Size: %v, Type: %v",
		fiHead.Filename, format.HumanByteSize(fiHead.Size), mtype.String())
	reID, errHTTP := addFRReportEntry(
		ctx, action.imsDBQ, action.imsDBQ, event.ID, fieldReportNumber,
		jwtCtx.Claims.RangerHandle(), reText, false,
		newFileName, fiHead.Filename, mtype.String(),
	)
	if errHTTP != nil {
		return 0, errHTTP.From("[addFRReportEntry]")
	}

	action.es.notifyFieldReportUpdateV2(event.ID, fieldReportNumber)
	if fieldReport.IncidentNumber.Valid {
		action.es.notifyIncidentUpdateV2(event.ID, fieldReport.IncidentNumber.Int32)
	}
	return reID, nil
}

func sniffFile(fi io.ReadSeeker) (*mimetype.MIME, *herr.HTTPError) {
	mtype, err := mimetype.DetectReader(fi)
	if err != nil {
		return mtype, herr.InternalServerError("Failed to detect content type", err).From("[DetectReader]")
	}
	slog.Info("found mime type details", "mime", mtype.String(), "ext", mtype.Extension())
	_, err = fi.Seek(0, io.SeekStart)
	if err != nil {
		return mtype, herr.InternalServerError("Failed to detect content type", err).From("[Seek]")
	}
	return mtype, nil
}
