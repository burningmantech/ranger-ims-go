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

package conf_test

import (
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestPrintRedacted(t *testing.T) {
	t.Parallel()
	cfg := conf.IMSConfig{
		Core: conf.ConfigCore{
			Admins: []string{"admin user"},
		},
		Store: conf.DBStore{
			Type: conf.DBStoreTypeMaria,
			MariaDB: conf.DBStoreMaria{
				Username: "db username",
				Password: "db password",
			},
		},
		Directory: conf.Directory{
			ClubhouseDB: conf.ClubhouseDB{
				Username: "clubhouse username",
				Password: "clubhouse password",
			},
		},
	}

	redacted := cfg.PrintRedacted()
	assert.Contains(t, redacted, "admin user")
	assert.Contains(t, redacted, "db username")
	assert.NotContains(t, redacted, "db password")
	assert.NotContains(t, redacted, "user password")
	assert.Contains(t, redacted, "clubhouse username")
	assert.NotContains(t, redacted, "clubhouse password")
}

func TestValidateBase(t *testing.T) {
	t.Parallel()

	cfg := conf.DefaultIMS()
	require.NoError(t, cfg.Validate())

	// must have AccessTokenLifetime <= RefreshTokenLifetime
	cfg.Core.AccessTokenLifetime = cfg.Core.RefreshTokenLifetime + 1
	require.Error(t, cfg.Validate())
}

func TestValidateDBStore(t *testing.T) {
	t.Parallel()

	cfg := conf.DefaultIMS()
	cfg.Store.Type = "invalid type"
	require.Error(t, cfg.Validate())
}

func TestValidateDirectory(t *testing.T) {
	t.Parallel()

	cfg := conf.DefaultIMS()
	cfg.Directory.Directory = "invalid type"
	require.Error(t, cfg.Validate())
}

func TestValidateNonDevDeployment(t *testing.T) {
	t.Parallel()

	cfg := conf.DefaultIMS()
	cfg.Core.Deployment = "not a valid deployment"
	require.Error(t, cfg.Validate())

	// non-dev deployment requires ClubhouseDB
	cfg = conf.DefaultIMS()
	cfg.Core.Deployment = conf.DeploymentTypeProduction
	cfg.Directory.Directory = conf.DirectoryTypeFake
	require.Error(t, cfg.Validate())

	// non-dev deployment requires MariaDB store
	cfg = conf.DefaultIMS()
	cfg.Core.Deployment = conf.DeploymentTypeProduction
	cfg.Store.Type = conf.DBStoreTypeNoOp
	require.Error(t, cfg.Validate())
}
func TestValidateAttachmentsStore(t *testing.T) {
	t.Parallel()
	temp, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)

	cfg := conf.DefaultIMS()
	cfg.AttachmentsStore.Type = conf.AttachmentsStoreS3
	// this will ultimately be ignored
	cfg.AttachmentsStore.Local.Dir = temp
	cfg.AttachmentsStore.S3 = conf.S3Attachments{
		AWSAccessKeyID:     "abc",
		AWSSecretAccessKey: "def",
		AWSRegion:          "there",
		Bucket:             "buck",
		CommonKeyPrefix:    "a/b",
	}
	require.NoError(t, cfg.Validate())

	//// This field is required for an S3 attachments store
	// cfg.AttachmentsStore.S3.AWSSecretAccessKey = ""
	// require.Error(t, cfg.Validate())

	// local attachments store requires a local dir to be set
	cfg = conf.DefaultIMS()
	cfg.AttachmentsStore.Type = conf.AttachmentsStoreLocal
	require.Error(t, cfg.Validate())

	cfg = conf.DefaultIMS()
	cfg.AttachmentsStore.Type = "invalid type"
	require.Error(t, cfg.Validate())
}
