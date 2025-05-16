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

package attachment_test

import (
	"bytes"
	"github.com/burningmantech/ranger-ims-go/lib/attachment"
	"github.com/burningmantech/ranger-ims-go/lib/attachment/fake"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"testing"
)

func TestS3ClientGetObject(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	client, err := attachment.NewS3Client(t.Context())
	require.NoError(t, err)
	require.NotNil(t, client)
	client.S3Funcs = fake.NewS3Funcs()

	file := []byte("hello world")
	errHTTP := client.UploadToS3(ctx, "some-bucket", "myobject", bytes.NewReader(file))
	require.Nil(t, errHTTP)
	reader, errHTTP := client.GetObject(ctx, "some-bucket", "myobject")
	require.Nil(t, errHTTP)
	retrieved, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, file, retrieved)

	// doesn't exist
	_, errHTTP = client.GetObject(ctx, "some-bucket", "not a key!")
	require.NotNil(t, errHTTP)
	require.Equal(t, http.StatusNotFound, errHTTP.Code)
}
