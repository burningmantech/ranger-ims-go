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
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/lib/attachment"
	"github.com/burningmantech/ranger-ims-go/lib/attachment/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestExtensionByType(t *testing.T) {
	t.Parallel()
	assert.Equal(t, ".bmp", extensionByType("image/bmp"))
	assert.Equal(t, ".csv", extensionByType("text/csv"))
	assert.Equal(t, ".gif", extensionByType("image/gif"))
	assert.Equal(t, ".html", extensionByType("text/html"))
	assert.Equal(t, ".jpg", extensionByType("image/jpeg"))
	assert.Equal(t, ".mp4", extensionByType("video/mp4"))
	assert.Equal(t, ".pdf", extensionByType("application/pdf"))
	assert.Equal(t, ".png", extensionByType("image/png"))
	assert.Equal(t, ".txt", extensionByType("text/plain"))
	assert.Equal(t, ".zip", extensionByType("application/zip"))

	assert.Empty(t, extensionByType("notta/mime"))
}

func TestSaveAndRetrieveS3File(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	config := conf.AttachmentsStore{
		Type: "s3",
		S3:   conf.S3Attachments{},
	}
	filename := "myfile.txt"
	fileContents := []byte("hello world")

	// put the fake S3 client in place
	client, err := attachment.NewS3Client(ctx)
	require.NoError(t, err)
	client.S3Funcs = fake.NewS3Funcs()

	// make a test file, then upload it to the fake S3
	tempFilePath := filepath.Join(t.TempDir(), filename)
	err = os.WriteFile(tempFilePath, fileContents, 0600)
	require.NoError(t, err)
	file, err := os.Open(tempFilePath) // #nosec G304
	require.NoError(t, err)
	errHTTP := saveFile(ctx,
		config,
		client,
		filename,
		file,
	)
	require.Nil(t, errHTTP)

	// now retrieve the file from the fake S3
	fileResp, httpError := retrieveFile(ctx, config, client, filename)
	assert.Nil(t, httpError)
	all, err := io.ReadAll(fileResp)
	require.NoError(t, err)
	assert.Equal(t, fileContents, all)
}

func TestSaveAndRetrieveLocalFile(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tempRoot, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)
	config := conf.AttachmentsStore{
		Type:  "local",
		Local: conf.LocalAttachments{Dir: tempRoot},
	}
	filename := "myfile.txt"
	fileContents := []byte("hello world")

	// make a test file, then save it via the attachments code
	tempFilePath := filepath.Join(t.TempDir(), filename)
	err = os.WriteFile(tempFilePath, fileContents, 0600)
	require.NoError(t, err)
	file, err := os.Open(tempFilePath) // #nosec G304
	require.NoError(t, err)
	errHTTP := saveFile(ctx,
		config,
		nil,
		filename,
		file,
	)
	require.Nil(t, errHTTP)

	// now retrieve the file from the fake S3
	fileResp, httpError := retrieveFile(ctx, config, nil, filename)
	assert.Nil(t, httpError)
	all, err := io.ReadAll(fileResp)
	require.NoError(t, err)
	assert.Equal(t, fileContents, all)
}

func TestSaveAndRetrieveLocalFile_Errors(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tempRoot, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)
	config := conf.AttachmentsStore{
		Type:  "local",
		Local: conf.LocalAttachments{Dir: tempRoot},
	}

	// Try to retrieve a file that doesn't exist
	_, httpError := retrieveFile(ctx, config, nil, "this-file-doesnt-exist")
	require.Error(t, httpError)
	assert.Equal(t, http.StatusNotFound, httpError.Code)

	// Request with empty filename
	_, httpError = retrieveFile(ctx, config, nil, "")
	require.Error(t, httpError)
	assert.Equal(t, http.StatusNotFound, httpError.Code)

	// Try to retrieve a file from outside the local attachments root.
	// i.e. this call to TempDir() creates another temp directory, separate from the one
	// at the top of this test. os.Root won't let us escape from the preconfigured Root.
	_, httpError = retrieveFile(ctx, config, nil, t.TempDir())
	require.Error(t, httpError)
	assert.Equal(t, http.StatusInternalServerError, httpError.Code)
}
