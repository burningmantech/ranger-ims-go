package cmd

import (
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

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
	t.Setenv("IMS_DB_HOST_POST", "555")
	t.Setenv("IMS_DB_DATABASE", "ims")
	t.Setenv("IMS_DB_USER_NAME", "me")
	t.Setenv("IMS_DB_PASSWORD", "boo")
	t.Setenv("IMS_DMS_HOSTNAME", "db2")
	t.Setenv("IMS_DMS_DATABASE", "rangerz")
	t.Setenv("IMS_DMS_USERNAME", "me2")
	t.Setenv("IMS_DMS_PASSWORD", "woo")
	t.Setenv("IMS_ATTACHMENTS_STORE", "local")
	t.Setenv("IMS_ATTACHMENTS_LOCAL_DIR", tempDir)

	cfg := mustInitConfig(serveCmd.Flags().Lookup(envfileFlagName))

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
}
