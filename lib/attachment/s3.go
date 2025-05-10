package attachment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"io"
	"log/slog"
	"time"
)

func UploadToS3(ctx context.Context, bucketName, objectName string, file io.Reader) error {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("[LoadDefaultConfig]: %w", err)
	}
	start := time.Now()

	client := s3.NewFromConfig(cfg)

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectName),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("[PutObject]: %w", err)
	}
	slog.Debug("Uploaded attachment to S3", "objectName", objectName, "duration", time.Since(start))
	return nil
}

func GetObject(ctx context.Context, bucketName, objectName string) (file io.ReadSeeker, exists bool, err error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("[LoadDefaultConfig]: %w", err)
	}

	client := s3.NewFromConfig(cfg)

	start := time.Now()
	output, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectName),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchKey" {
			slog.Debug("No such key in S3", "bucket", bucketName, "object", objectName)

			// Just tell the caller that the object does not exist, without propagating the error.
			// This lets them easily decide whether to return a 404 or a 500.
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("[GetObject]: %w", err)
	}

	// This reads the whole object in memory, which isn't ideal, but it lets us use the
	// http.ServeContent(..) in attachments.go, which requires an io.ReadSeeker.
	// In an ideal world, we'd just stream the object to the IMS API client.
	buf := bytes.Buffer{}
	_, err = io.Copy(&buf, output.Body)
	slog.Debug("Read attachment from S3", "objectName", objectName, "duration", time.Since(start))
	if err != nil {
		return nil, true, fmt.Errorf("[io.Copy]: %w", err)
	}
	return bytes.NewReader(buf.Bytes()), true, nil
}
