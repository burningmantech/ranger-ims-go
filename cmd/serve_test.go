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

package cmd

import (
	"context"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

// TestMustInitConfig should be the only test in the whole repo that
// so freely plays around with environment variables, since parallel
// testing means other tests will notice the result of "Setenvs" that
// occur at the same time.
//
// All other tests should use a conf.IMSConfig struct instead, as that
// is unaffected by environment variables changing later.
func TestMustInitConfig(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("IMS_HOSTNAME", "host")
	t.Setenv("IMS_PORT", "1234")
	t.Setenv("IMS_PASSWORD", "password")
	t.Setenv("IMS_DEPLOYMENT", "dev")
	t.Setenv("IMS_TOKEN_LIFETIME", "1000")
	t.Setenv("IMS_ACCESS_TOKEN_LIFETIME", "100")
	t.Setenv("IMS_CACHE_CONTROL_SHORT", "3m")
	t.Setenv("IMS_CACHE_CONTROL_LONG", "7m")
	t.Setenv("IMS_DIRECTORY_CACHE_TTL", "15m")
	t.Setenv("IMS_LOG_LEVEL", "WARN")
	t.Setenv("IMS_DIRECTORY", "clubhousedb")
	t.Setenv("IMS_ADMINS", "alice,bob")
	t.Setenv("IMS_JWT_SECRET", "shhh")
	t.Setenv("IMS_DB_HOST_NAME", "db")
	t.Setenv("IMS_DB_HOST_PORT", "555")
	t.Setenv("IMS_DB_DATABASE", "ims")
	t.Setenv("IMS_DB_USER_NAME", "me")
	t.Setenv("IMS_DB_PASSWORD", "boo")
	t.Setenv("IMS_DMS_HOSTNAME", "db2")
	t.Setenv("IMS_DMS_DATABASE", "rangerz")
	t.Setenv("IMS_DMS_USERNAME", "me2")
	t.Setenv("IMS_DMS_PASSWORD", "woo")
	t.Setenv("IMS_ATTACHMENTS_STORE", "local")
	t.Setenv("IMS_ATTACHMENTS_LOCAL_DIR", tempDir)
	t.Setenv("AWS_ACCESS_KEY_ID", "my name")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "my key")
	t.Setenv("AWS_REGION", "mars")
	t.Setenv("IMS_ATTACHMENTS_S3_BUCKET", "big-bucket")
	t.Setenv("IMS_ATTACHMENTS_S3_COMMON_KEY_PREFIX", "safe/dir")

	cfg := mustInitConfig(".env")

	assert.Equal(t, "host", cfg.Core.Host)
	assert.Equal(t, int32(1234), cfg.Core.Port)
	assert.Equal(t, "dev", cfg.Core.Deployment)
	assert.Equal(t, 1000*time.Second, cfg.Core.RefreshTokenLifetime)
	assert.Equal(t, 100*time.Second, cfg.Core.AccessTokenLifetime)
	assert.Equal(t, 3*time.Minute, cfg.Core.CacheControlShort)
	assert.Equal(t, 7*time.Minute, cfg.Core.CacheControlLong)
	assert.Equal(t, 15*time.Minute, cfg.Directory.InMemoryCacheTTL)
	assert.Equal(t, "WARN", cfg.Core.LogLevel)
	assert.Equal(t, conf.DirectoryTypeClubhouseDB, cfg.Directory.Directory)
	assert.Equal(t, []string{"alice", "bob"}, cfg.Core.Admins)
	assert.Equal(t, "shhh", cfg.Core.JWTSecret)
	assert.Equal(t, "db", cfg.Store.MariaDB.HostName)
	assert.Equal(t, int32(555), cfg.Store.MariaDB.HostPort)
	assert.Equal(t, "ims", cfg.Store.MariaDB.Database)
	assert.Equal(t, "me", cfg.Store.MariaDB.Username)
	assert.Equal(t, "boo", cfg.Store.MariaDB.Password)
	assert.Equal(t, "db2", cfg.Directory.ClubhouseDB.Hostname)
	assert.Equal(t, "rangerz", cfg.Directory.ClubhouseDB.Database)
	assert.Equal(t, "me2", cfg.Directory.ClubhouseDB.Username)
	assert.Equal(t, "woo", cfg.Directory.ClubhouseDB.Password)
	assert.Equal(t, conf.AttachmentsStoreLocal, cfg.AttachmentsStore.Type)
	assert.Equal(t, tempDir, cfg.AttachmentsStore.Local.Dir.Name())
	assert.Equal(t, "my name", cfg.AttachmentsStore.S3.AWSAccessKeyID)
	assert.Equal(t, "my key", cfg.AttachmentsStore.S3.AWSSecretAccessKey)
	assert.Equal(t, "mars", cfg.AttachmentsStore.S3.AWSRegion)
	assert.Equal(t, "big-bucket", cfg.AttachmentsStore.S3.Bucket)
	assert.Equal(t, "safe/dir", cfg.AttachmentsStore.S3.CommonKeyPrefix)
}

func TestRunServer(t *testing.T) {
	t.Parallel()
	imsCfg := conf.DefaultIMS()

	// this will have the server pick a random port
	imsCfg.Core.Port = 0
	imsCfg.Directory.Directory = conf.DirectoryTypeTestUsers
	imsCfg.Store.Type = conf.DBStoreTypeNoOp

	// Start the server, then cancel it.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	exitCode := runServerInternal(ctx, imsCfg, true)
	assert.Equal(t, 69, exitCode)
}
