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

//! Attachments on Incidents, Field Reports, and Visits. Ported from the Go
//! server's `api/attachment.go`.

use std::collections::HashMap;
use std::path::Path as FsPath;

use axum::Extension;
use axum::body::Body;
use axum::extract::{Path, Query, State};
use axum::http::header::{self, CONTENT_TYPE};
use axum::http::{HeaderMap, HeaderValue, StatusCode};
use axum::response::Response;
use serde::Deserialize;

use super::fieldreport::{contains_author, fetch_field_report};
use super::helpers::*;
use super::incident::fetch_incident;
use super::middleware::JwtCtx;
use super::reportentry::{NewReportEntry, Parent, add_report_entry, path_i32};
use super::state::{AppState, S};
use super::visit::fetch_visit;
use crate::authz;
use crate::conf::AttachmentsStoreType;
use crate::store::models::ReportEntry;
use crate::util::format::human_byte_size;
use crate::util::herr::{self, HttpError};
use crate::util::mimesniff;

pub const IMS_ATTACHMENT_FORM_KEY: &str = "imsAttachment";
const OCTET_STREAM: &str = "application/octet-stream";

const SAFE_TO_PREVIEW_MEDIA_TYPES: [&str; 10] = [
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
];

/// A tchar, per RFC 7230, as Go's mime package judges token characters.
fn is_token_char(c: char) -> bool {
    c.is_ascii() && !c.is_ascii_control() && !" ()<>@,;:\\\"/[]?=".contains(c)
}

/// Parses "type/subtype; k=v" as Go's `mime.ParseMediaType` does, well
/// enough for the media types sniffing produces.
fn parse_media_type(v: &str) -> Option<(String, Vec<(String, String)>)> {
    let mut parts = v.split(';');
    let media_type = parts.next()?.trim().to_ascii_lowercase();
    let (major, minor) = media_type.split_once('/')?;
    if major.is_empty()
        || minor.is_empty()
        || !media_type.chars().all(|c| is_token_char(c) || c == '/')
    {
        return None;
    }
    let mut params = Vec::new();
    for p in parts {
        let p = p.trim();
        if p.is_empty() {
            continue;
        }
        let (k, v) = p.split_once('=')?;
        params.push((
            k.trim().to_ascii_lowercase(),
            v.trim().trim_matches('"').to_string(),
        ));
    }
    Some((media_type, params))
}

/// Go's `mime.FormatMediaType`, for the parameter values used here.
pub fn format_media_type(t: &str, params: &[(&str, &str)]) -> String {
    let mut out = t.to_string();
    let mut sorted = params.to_vec();
    sorted.sort_by_key(|(k, _)| *k);
    for (k, v) in sorted {
        out.push_str("; ");
        if !v.is_ascii() {
            out.push_str(&format!("{k}*=utf-8''"));
            for b in v.bytes() {
                if b.is_ascii_alphanumeric() || b"!#$&+-.^_`|~".contains(&b) {
                    out.push(b as char);
                } else {
                    out.push_str(&format!("%{b:02X}"));
                }
            }
        } else if !v.is_empty() && v.chars().all(is_token_char) {
            out.push_str(&format!("{k}={v}"));
        } else {
            let escaped = v.replace('\\', "\\\\").replace('"', "\\\"");
            out.push_str(&format!("{k}=\"{escaped}\""));
        }
    }
    out
}

/// A safe form of the content type, or octet-stream. Attachments are
/// previewed in IMS's own origin, so anything that could run script (HTML,
/// SVG) mustn't go out as itself.
pub fn safe_to_preview_content_type(content_type: &str) -> String {
    let Some((media_type, params)) = parse_media_type(content_type) else {
        return OCTET_STREAM.to_string();
    };
    if SAFE_TO_PREVIEW_MEDIA_TYPES.contains(&media_type.as_str()) {
        return content_type.to_string();
    }
    // Chromium won't play video/quicktime, but plays the same files as video/mp4.
    if media_type == "video/quicktime" {
        return "video/mp4".to_string();
    }
    if media_type.starts_with("text/") {
        let params: Vec<(&str, &str)> = params
            .iter()
            .map(|(k, v)| (k.as_str(), v.as_str()))
            .collect();
        return format_media_type("text/plain", &params);
    }
    OCTET_STREAM.to_string()
}

pub fn previewable_content_type(content_type: &str) -> bool {
    safe_to_preview_content_type(content_type) != OCTET_STREAM
}

async fn retrieve_file(st: &AppState, filename: &str) -> HResult<Vec<u8>> {
    if filename.is_empty() {
        return Err(HttpError::not_found_msg("No attachment for this ID"));
    }
    let store = &st.cfg.attachments_store;
    match store.store_type.get() {
        Some(AttachmentsStoreType::Local) => {
            let dir = store.local.dir.as_deref().unwrap_or(FsPath::new("."));
            // Like Go's os.Root, refuse anything that could leave the directory.
            if filename.contains('/') || filename.contains('\\') || filename == ".." {
                return Err(HttpError::internal_server_error(
                    "Failed to open file",
                    anyhow::anyhow!("path escapes from parent"),
                )
                .from_src("[Open]"));
            }
            match tokio::fs::read(dir.join(filename)).await {
                Ok(b) => Ok(b),
                Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
                    Err(HttpError::not_found_msg("File does not exist"))
                }
                Err(e) => {
                    Err(HttpError::internal_server_error("Failed to open file", e)
                        .from_src("[Open]"))
                }
            }
        }
        Some(AttachmentsStoreType::S3) => {
            let s3 = st.s3.as_ref().ok_or_else(|| {
                HttpError::internal_server_error("No S3 client", anyhow::anyhow!("no S3 client"))
            })?;
            s3.get(
                &store.s3.bucket,
                &format!("{}{filename}", store.s3.common_key_prefix),
            )
            .await
            .map_err(|e| e.from_src("[mustGetS3File]"))
        }
        _ => Err(HttpError::not_found_msg(
            "Attachments are not currently supported",
        )),
    }
}

async fn save_file(st: &AppState, new_file_name: &str, data: Vec<u8>) -> HResult<()> {
    let store = &st.cfg.attachments_store;
    match store.store_type.get() {
        Some(AttachmentsStoreType::Local) => {
            let dir = store.local.dir.as_deref().unwrap_or(FsPath::new("."));
            tokio::fs::write(dir.join(new_file_name), data)
                .await
                .map_err(|e| {
                    HttpError::internal_server_error("Failed to write file", e).from_src("[Copy]")
                })
        }
        Some(AttachmentsStoreType::S3) => {
            let s3 = st.s3.as_ref().ok_or_else(|| {
                HttpError::internal_server_error("No S3 client", anyhow::anyhow!("no S3 client"))
            })?;
            s3.upload(
                &store.s3.bucket,
                &format!("{}{new_file_name}", store.s3.common_key_prefix),
                data,
            )
            .await
            .map_err(|e| e.from_src("[UploadToS3]"))
        }
        _ => Err(HttpError::not_found_msg(
            "Attachments are not currently supported",
        )),
    }
}

#[derive(Deserialize)]
pub struct AttachmentQuery {
    #[serde(default)]
    download: String,
}

/// An attachment, opened in the browser's tab by default, or saved when
/// `download=true`.
fn serve_attachment(
    file: Vec<u8>,
    content_type: &str,
    name: &str,
    download: bool,
    req_headers: &HeaderMap,
) -> Response {
    let mut disposition = if download { "attachment" } else { "inline" }.to_string();
    if !name.is_empty() {
        disposition = format_media_type(&disposition, &[("filename", name)]);
    }
    let total = file.len();
    let (status, body, range) = match parse_range(req_headers, total) {
        Some((start, end)) => (
            StatusCode::PARTIAL_CONTENT,
            file[start..=end].to_vec(),
            Some((start, end)),
        ),
        None => (StatusCode::OK, file, None),
    };
    let len = body.len();
    let mut resp = Response::new(Body::from(body));
    *resp.status_mut() = status;
    let h = resp.headers_mut();
    let set = |h: &mut HeaderMap, k: header::HeaderName, v: &str| {
        if let Ok(v) = HeaderValue::from_str(v) {
            h.insert(k, v);
        }
    };
    set(h, CONTENT_TYPE, content_type);
    set(h, header::CONTENT_DISPOSITION, &disposition);
    // A previewed attachment is a page in IMS's own origin, so it mustn't be
    // sniffed into another type, nor run scripts.
    set(h, header::X_CONTENT_TYPE_OPTIONS, "nosniff");
    set(h, header::CONTENT_SECURITY_POLICY, "sandbox");
    set(h, header::ACCEPT_RANGES, "bytes");
    set(h, header::LAST_MODIFIED, &httpdate_now());
    set(h, header::CONTENT_LENGTH, &len.to_string());
    if let Some((start, end)) = range {
        set(
            h,
            header::CONTENT_RANGE,
            &format!("bytes {start}-{end}/{total}"),
        );
    }
    resp
}

fn httpdate_now() -> String {
    chrono::Utc::now()
        .format("%a, %d %b %Y %H:%M:%S GMT")
        .to_string()
}

/// A single "bytes=a-b" range, as http.ServeContent would honor it.
fn parse_range(headers: &HeaderMap, total: usize) -> Option<(usize, usize)> {
    let v = headers.get(header::RANGE)?.to_str().ok()?;
    let spec = v.strip_prefix("bytes=")?;
    if spec.contains(',') || total == 0 {
        return None;
    }
    let (start, end) = spec.split_once('-')?;
    let (start, end) = if start.is_empty() {
        let suffix: usize = end.parse().ok()?;
        (total.saturating_sub(suffix), total - 1)
    } else {
        let start: usize = start.parse().ok()?;
        let end = if end.is_empty() {
            total - 1
        } else {
            end.parse::<usize>().ok()?.min(total - 1)
        };
        (start, end)
    };
    (start <= end && start < total).then_some((start, end))
}

fn find_attachment(entries: &[ReportEntry], id: i32) -> (String, String) {
    entries
        .iter()
        .find(|re| re.id == id)
        .map(|re| {
            (
                re.attached_file.clone().unwrap_or_default(),
                re.attached_file_original_name.clone().unwrap_or_default(),
            )
        })
        .unwrap_or_default()
}

async fn serve_stored(
    st: &AppState,
    entries: &[ReportEntry],
    id: i32,
    download: bool,
    headers: &HeaderMap,
) -> HResult<Response> {
    let (filename, name) = find_attachment(entries, id);
    let file = retrieve_file(st, &filename)
        .await
        .map_err(|e| e.from_src("[retrieveFile]"))?;
    let mtype = mimesniff::detect(&file);
    let content_type = safe_to_preview_content_type(&mtype.mime);
    Ok(serve_attachment(
        file,
        &content_type,
        &name,
        download,
        headers,
    ))
}

pub async fn get_incident_attachment(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    Query(q): Query<AttachmentQuery>,
    headers: HeaderMap,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & authz::EVENT_READ_INCIDENTS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventReadIncidents permission on this Event",
        ));
    }
    let number = path_i32(&p, "incidentNumber", "Failed to parse incident number")?;
    let attachment = path_i32(&p, "attachmentNumber", "Failed to parse attachment number")?;
    let (_, entries) = fetch_incident(&st, event.id, number)
        .await
        .map_err(|e| e.from_src("[fetchIncident]"))?;
    serve_stored(&st, &entries, attachment, q.download == "true", &headers).await
}

pub async fn get_field_report_attachment(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    Query(q): Query<AttachmentQuery>,
    headers: HeaderMap,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & (authz::EVENT_READ_ALL_FIELD_REPORTS | authz::EVENT_READ_OWN_FIELD_REPORTS) == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have permission to read Field Reports on this Event",
        ));
    }
    let limited_access = perms & authz::EVENT_READ_ALL_FIELD_REPORTS == 0;
    let number = path_i32(
        &p,
        "fieldReportNumber",
        "Failed to parse Field Report number",
    )?;
    let attachment = path_i32(&p, "attachmentNumber", "Failed to parse attachment number")?;
    let (_, entries) = fetch_field_report(&st, event.id, number)
        .await
        .map_err(|e| e.from_src("[fetchFieldReport]"))?;
    if limited_access && !contains_author(&entries, claims_of(&jwt)?.ranger_handle()) {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have permission to read this particular Field Report",
        ));
    }
    serve_stored(&st, &entries, attachment, q.download == "true", &headers).await
}

pub async fn get_visit_attachment(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    Query(q): Query<AttachmentQuery>,
    headers: HeaderMap,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & authz::EVENT_READ_VISITS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventReadVisits permission on this Event",
        ));
    }
    let number = path_i32(&p, "visitNumber", "Failed to parse visit number")?;
    let attachment = path_i32(&p, "attachmentNumber", "Failed to parse attachment number")?;
    let (_, entries) = fetch_visit(&st, event.id, number)
        .await
        .map_err(|e| e.from_src("[fetchVisit]"))?;
    serve_stored(&st, &entries, attachment, q.download == "true", &headers).await
}

struct UploadedFile {
    filename: String,
    data: Vec<u8>,
}

/// Reads the attachment out of a multipart form, as Go's `FormFile` does.
async fn read_form_file(st: &AppState, headers: &HeaderMap, body: Body) -> HResult<UploadedFile> {
    let limit = st.cfg.core.max_request_bytes;
    let too_large = |e: anyhow::Error| {
        HttpError::request_entity_too_large(
            format!(
                "The supplied file is above the server limit of {}",
                human_byte_size(limit)
            ),
            e,
        )
    };
    let content_type = headers
        .get(CONTENT_TYPE)
        .and_then(|v| v.to_str().ok())
        .unwrap_or_default();
    let boundary = multer::parse_boundary(content_type).map_err(|e| {
        HttpError::bad_request(
            "Failed to parse file",
            anyhow::anyhow!("request Content-Type isn't multipart/form-data: {e}"),
        )
    })?;
    let mut multipart = multer::Multipart::new(body.into_data_stream(), boundary);
    loop {
        let field = match multipart.next_field().await {
            Ok(Some(f)) => f,
            Ok(None) => {
                return Err(HttpError::bad_request(
                    "Failed to parse file",
                    anyhow::anyhow!("http: no such file"),
                ));
            }
            Err(e) => return Err(classify_multipart_error(e, &too_large)),
        };
        if field.name() != Some(IMS_ATTACHMENT_FORM_KEY) || field.file_name().is_none() {
            continue;
        }
        let filename = field.file_name().unwrap_or_default().to_string();
        let data = field
            .bytes()
            .await
            .map_err(|e| classify_multipart_error(e, &too_large))?;
        return Ok(UploadedFile {
            filename,
            data: data.to_vec(),
        });
    }
}

fn classify_multipart_error(
    e: multer::Error,
    too_large: &dyn Fn(anyhow::Error) -> HttpError,
) -> HttpError {
    let mut source: Option<&(dyn std::error::Error + 'static)> = Some(&e);
    while let Some(s) = source {
        if s.downcast_ref::<http_body_util::LengthLimitError>()
            .is_some()
        {
            return too_large(anyhow::anyhow!("http: request body too large"));
        }
        if let Some(ae) = s.downcast_ref::<axum::Error>()
            && is_length_limit_error(ae)
        {
            return too_large(anyhow::anyhow!("http: request body too large"));
        }
        source = s.source();
    }
    HttpError::bad_request("Failed to parse file", e)
}

/// Like Go's `crypto/rand.Text`.
fn rand_text() -> String {
    use rand::Rng;
    const ALPHABET: &[u8] = b"ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
    let mut rng = rand::rng();
    (0..26)
        .map(|_| ALPHABET[rng.random_range(0..ALPHABET.len())] as char)
        .collect()
}

#[allow(clippy::too_many_arguments)]
async fn store_upload(
    st: &AppState,
    parent: Parent,
    kind: &str,
    event_id: i32,
    number: i32,
    author: &str,
    file: UploadedFile,
    text_sep: &str,
) -> HResult<i32> {
    let mtype = mimesniff::detect(&file.data);
    let new_file_name = format!(
        "event_{event_id:05}_{kind}_{number:05}_{}{}",
        rand_text(),
        mtype.extension
    );
    tracing::info!(user = author, original_name = %file.filename, new_file_name = %new_file_name, size = file.data.len(), content_type = %mtype.mime, "User uploaded an attachment");
    let size = file.data.len() as i64;
    save_file(st, &new_file_name, file.data)
        .await
        .map_err(|e| e.from_src("[saveFile]"))?;
    let text = format!(
        "File Name: {}, Size: {}, Type:{text_sep}{}",
        file.filename,
        human_byte_size(size),
        mtype.mime
    );
    let mut conn = st
        .db
        .acquire()
        .await
        .map_err(db_err("Failed to create report entry", "[Acquire]"))?;
    add_report_entry(
        &mut conn,
        parent,
        event_id,
        number,
        &NewReportEntry {
            author: author.to_string(),
            text,
            generated: false,
            attached_file: new_file_name,
            attached_file_original_name: file.filename,
            attached_file_media_type: mtype.mime,
        },
    )
    .await
    .map_err(|e| e.from_src("[addReportEntry]"))
}

fn saved_response(message: &str, re_id: i32) -> Response {
    let mut resp = herr::no_content_response(message);
    resp.headers_mut()
        .insert("IMS-Report-Entry-Number", HeaderValue::from(re_id));
    resp
}

pub async fn attach_to_incident(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & authz::EVENT_WRITE_INCIDENTS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventWriteIncidents permission on this Event",
        ));
    }
    let number = path_i32(&p, "incidentNumber", "Failed to parse incident number")?;
    let file = read_form_file(&st, &headers, body).await?;
    let author = claims_of(&jwt)?.ranger_handle().to_string();
    let re_id = store_upload(
        &st,
        Parent::Incident,
        "incident",
        event.id,
        number,
        &author,
        file,
        "",
    )
    .await?;
    st.es.notify_incident_update(event.id, number);
    Ok(saved_response("Saved Incident attachment", re_id))
}

pub async fn attach_to_field_report(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & (authz::EVENT_WRITE_ALL_FIELD_REPORTS | authz::EVENT_WRITE_OWN_FIELD_REPORTS) == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have permission to write Field Reports on this Event",
        ));
    }
    // Attaching adds a Report Entry, so it's an edit, held to what editing allows.
    let limited_access = perms & authz::EVENT_WRITE_ALL_FIELD_REPORTS == 0;
    let number = path_i32(
        &p,
        "fieldReportNumber",
        "Failed to parse Field Report number",
    )?;
    let (fr, entries) = fetch_field_report(&st, event.id, number)
        .await
        .map_err(|e| e.from_src("[fetchFieldReport]"))?;
    let author = claims_of(&jwt)?.ranger_handle().to_string();
    if limited_access && !contains_author(&entries, &author) {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have permission to edit this particular Field Report",
        ));
    }
    let file = read_form_file(&st, &headers, body).await?;
    let re_id = store_upload(
        &st,
        Parent::FieldReport,
        "fieldreport",
        event.id,
        number,
        &author,
        file,
        " ",
    )
    .await?;
    st.es.notify_field_report_update(event.id, number);
    if let Some(incident) = fr.incident_number {
        st.es.notify_incident_update(event.id, incident);
    }
    Ok(saved_response("Saved Field Report attachment", re_id))
}

pub async fn attach_to_visit(
    State(st): State<S>,
    Extension(jwt): Extension<JwtCtx>,
    Path(p): Path<HashMap<String, String>>,
    headers: HeaderMap,
    body: Body,
) -> HResult<Response> {
    let (event, perms) = get_event_permissions(&st, &jwt, &p["eventName"])
        .await
        .map_err(|e| e.from_src("[getEventPermissions]"))?;
    if perms & authz::EVENT_WRITE_VISITS == 0 {
        return Err(HttpError::forbidden_msg(
            "The requestor does not have EventWriteVisits permission on this Event",
        ));
    }
    let number = path_i32(&p, "visitNumber", "Failed to parse visit number")?;
    let file = read_form_file(&st, &headers, body).await?;
    let author = claims_of(&jwt)?.ranger_handle().to_string();
    let re_id = store_upload(
        &st,
        Parent::Visit,
        "visit",
        event.id,
        number,
        &author,
        file,
        "",
    )
    .await?;
    st.es.notify_visit_update(event.id, number);
    Ok(saved_response("Saved Visit attachment", re_id))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn safe_to_preview() {
        assert_eq!(
            safe_to_preview_content_type("application/pdf"),
            "application/pdf"
        );
        assert_eq!(safe_to_preview_content_type("image/png"), "image/png");
        assert_eq!(
            safe_to_preview_content_type("text/plain; charset=utf-8"),
            "text/plain; charset=utf-8"
        );
        assert_eq!(safe_to_preview_content_type("video/quicktime"), "video/mp4");
        assert_eq!(safe_to_preview_content_type("text/html"), "text/plain");
        assert_eq!(
            safe_to_preview_content_type("text/html; charset=utf-8"),
            "text/plain; charset=utf-8"
        );
        assert_eq!(safe_to_preview_content_type("image/svg+xml"), OCTET_STREAM);
        assert_eq!(
            safe_to_preview_content_type("application/json"),
            OCTET_STREAM
        );
        assert_eq!(safe_to_preview_content_type(""), OCTET_STREAM);
        assert_eq!(
            safe_to_preview_content_type("not a media type"),
            OCTET_STREAM
        );
    }

    #[test]
    fn media_type_formatting() {
        assert_eq!(
            format_media_type("inline", &[("filename", "a.txt")]),
            "inline; filename=a.txt"
        );
        assert_eq!(
            format_media_type("inline", &[("filename", "my file.txt")]),
            "inline; filename=\"my file.txt\""
        );
        assert_eq!(
            format_media_type("attachment", &[("filename", "é.txt")]),
            "attachment; filename*=utf-8''%C3%A9.txt"
        );
    }

    #[test]
    fn ranges() {
        let mut h = HeaderMap::new();
        assert_eq!(parse_range(&h, 10), None);
        h.insert(header::RANGE, "bytes=2-4".parse().unwrap());
        assert_eq!(parse_range(&h, 10), Some((2, 4)));
        h.insert(header::RANGE, "bytes=-3".parse().unwrap());
        assert_eq!(parse_range(&h, 10), Some((7, 9)));
        h.insert(header::RANGE, "bytes=8-".parse().unwrap());
        assert_eq!(parse_range(&h, 10), Some((8, 9)));
    }
}
