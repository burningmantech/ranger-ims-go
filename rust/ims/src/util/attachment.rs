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

//! Attachment storage in S3, ported from the Go server's `lib/attachment`.

use aws_sdk_s3::primitives::ByteStream;
use axum::http::StatusCode;

use crate::conf::{AttachmentsStoreType, IMSConfig};
use crate::util::herr::HttpError;

#[derive(Clone)]
pub struct S3Client {
    client: aws_sdk_s3::Client,
}

impl S3Client {
    /// An S3 client configured from the standard AWS environment, or None when
    /// attachments aren't stored in S3.
    pub async fn from_config(cfg: &IMSConfig) -> Result<Option<S3Client>, String> {
        if !cfg
            .attachments_store
            .store_type
            .is(AttachmentsStoreType::S3)
        {
            return Ok(None);
        }
        let aws_cfg = aws_config::load_from_env().await;
        Ok(Some(S3Client {
            client: aws_sdk_s3::Client::new(&aws_cfg),
        }))
    }

    pub async fn upload(&self, bucket: &str, key: &str, data: Vec<u8>) -> Result<(), HttpError> {
        self.client
            .put_object()
            .bucket(bucket)
            .key(key)
            .body(ByteStream::from(data))
            .send()
            .await
            .map_err(|e| {
                HttpError::internal_server_error(
                    "IMS failed to upload the file to S3. There may be an internet connectivity issue.",
                    anyhow::anyhow!("{e:?}"),
                )
                .from_src("[PutObject]")
            })?;
        Ok(())
    }

    pub async fn get(&self, bucket: &str, key: &str) -> Result<Vec<u8>, HttpError> {
        let out = self
            .client
            .get_object()
            .bucket(bucket)
            .key(key)
            .send()
            .await
            .map_err(|e| {
                if e.as_service_error().is_some_and(|se| se.is_no_such_key()) {
                    return HttpError::new(
                        StatusCode::NOT_FOUND,
                        "File does not exist",
                        Some(anyhow::anyhow!("{e:?}")),
                    )
                    .from_src("[GetObject]");
                }
                HttpError::internal_server_error(
                "IMS failed to pull the file from S3. There may be an internet connectivity issue.",
                anyhow::anyhow!("{e:?}"),
            )
            .from_src("[GetObject]")
            })?;
        let bytes = out.body.collect().await.map_err(|e| {
            HttpError::internal_server_error("Failed to read attachment", e).from_src("[io.Copy]")
        })?;
        Ok(bytes.into_bytes().to_vec())
    }
}
