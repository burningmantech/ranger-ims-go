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

package attachment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/burningmantech/ranger-ims-go/lib/herr"
)

// S3Funcs is an interface for the S3 AWS APIs that IMS actually uses.
//
// This is made to be easy to implement with a fake for testing.
type S3Funcs interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type S3Client struct {
	S3Funcs S3Funcs
}

func NewS3Client(ctx context.Context) (*S3Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("[LoadDefaultConfig]: %w", err)
	}
	return &S3Client{S3Funcs: s3.NewFromConfig(cfg)}, nil
}

func (c *S3Client) UploadToS3(ctx context.Context, bucketName, objectName string, file io.Reader) *herr.HTTPError {
	start := time.Now()
	_, err := c.S3Funcs.PutObject(
		ctx,
		&s3.PutObjectInput{
			Bucket: new(bucketName),
			Key:    new(objectName),
			Body:   file,
		},
	)
	if err != nil {
		return herr.InternalServerError("IMS failed to upload the file to S3. There may be an internet connectivity issue.", err).From("[PutObject]")
	}
	slog.Debug("Uploaded attachment to S3", "objectName", objectName, "duration", time.Since(start))
	return nil
}

func (c *S3Client) GetObject(ctx context.Context, bucketName, objectName string) (file io.ReadSeeker, httpError *herr.HTTPError) {
	start := time.Now()
	output, err := c.S3Funcs.GetObject(
		ctx,
		&s3.GetObjectInput{
			Bucket: new(bucketName),
			Key:    new(objectName),
		},
	)
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchKey" {
			slog.Debug("No such key in S3", "bucket", bucketName, "object", objectName)

			return nil, herr.NotFound("File does not exist", err).From("[GetObject]")
		}
		return nil, herr.InternalServerError("IMS failed to pull the file from S3. There may be an internet connectivity issue.", err).From("[GetObject]")
	}
	defer shut(output.Body)
	// This reads the whole object in memory, which isn't ideal, but it lets us use the
	// http.ServeContent(..) in attachments.go, which requires an io.ReadSeeker.
	// In an ideal world, we'd just stream the object to the IMS API client.
	buf := bytes.Buffer{}
	_, err = io.Copy(&buf, output.Body)
	slog.Debug("Read attachment from S3", "objectName", objectName, "duration", time.Since(start))
	if err != nil {
		return nil, herr.InternalServerError("Failed to read attachment", err).From("[io.Copy]")
	}
	return bytes.NewReader(buf.Bytes()), nil
}

func shut(c io.Closer) {
	_ = c.Close()
}
