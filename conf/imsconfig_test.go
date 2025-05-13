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
			TestUsers: []conf.TestUser{
				{
					Password: "user password",
				},
			},
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

func TestValidate(t *testing.T) {
	t.Parallel()
	cfg := conf.DefaultIMS()
	require.NoError(t, cfg.Validate())

	cfg.Directory.TestUsers = []conf.TestUser{{}}
	cfg.AttachmentsStore.Type = conf.AttachmentsStoreS3
	cfg.AttachmentsStore.S3 = conf.S3Attachments{
		AWSAccessKeyID:     "abc",
		AWSSecretAccessKey: "def",
		AWSRegion:          "there",
		Bucket:             "buck",
		CommonKeyPrefix:    "a/b",
	}
	require.NoError(t, cfg.Validate())
}
