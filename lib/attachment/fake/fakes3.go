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

package fake

import (
	"bytes"
	"context"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/burningmantech/ranger-ims-go/lib/attachment"
	"io"
)

type S3Funcs struct {
	objects map[BucketAndKey][]byte
}

type BucketAndKey struct {
	Bucket string
	Key    string
}

func NewS3Funcs() *S3Funcs {
	return &S3Funcs{
		objects: make(map[BucketAndKey][]byte),
	}
}

func (s S3Funcs) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	b, _ := io.ReadAll(params.Body)
	s.objects[BucketAndKey{*params.Bucket, *params.Key}] = b
	return &s3.PutObjectOutput{}, nil
}

func (s S3Funcs) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	obj, ok := s.objects[BucketAndKey{*params.Bucket, *params.Key}]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{
		Body: io.NopCloser(bytes.NewReader(obj)),
	}, nil
}

// force the fake to implement the interface.
var _ attachment.S3Funcs = (*S3Funcs)(nil)
