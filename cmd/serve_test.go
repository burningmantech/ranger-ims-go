package cmd

import (
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
	"time"
)

func TestMustInitConfig(t *testing.T) {
	tempDir := t.TempDir()
	setenv(t, "IMS_HOSTNAME", "host")
	setenv(t, "IMS_PORT", "1234")
	setenv(t, "IMS_PASSWORD", "password")
	setenv(t, "IMS_DEPLOYMENT", "dev")
	setenv(t, "IMS_TOKEN_LIFETIME", "1000")
	setenv(t, "IMS_ACCESS_TOKEN_LIFETIME", "100")
	setenv(t, "IMS_CACHE_CONTROL_SHORT", "3m")
	setenv(t, "IMS_CACHE_CONTROL_LONG", "7m")
	setenv(t, "IMS_DIRECTORY_CACHE_TTL", "15m")
	setenv(t, "IMS_LOG_LEVEL", "WARN")
	setenv(t, "IMS_DIRECTORY", "clubhousedb")
	setenv(t, "IMS_ADMINS", "alice,bob")
	setenv(t, "IMS_JWT_SECRET", "shhh")
	setenv(t, "IMS_DB_HOST_NAME", "db")
	setenv(t, "IMS_DB_HOST_POST", "555")
	setenv(t, "IMS_DB_DATABASE", "ims")
	setenv(t, "IMS_DB_USER_NAME", "me")
	setenv(t, "IMS_DB_PASSWORD", "boo")
	setenv(t, "IMS_DMS_HOSTNAME", "db2")
	setenv(t, "IMS_DMS_DATABASE", "rangerz")
	setenv(t, "IMS_DMS_USERNAME", "me2")
	setenv(t, "IMS_DMS_PASSWORD", "woo")
	setenv(t, "IMS_ATTACHMENTS_STORE", "local")
	setenv(t, "IMS_ATTACHMENTS_LOCAL_DIR", tempDir)

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

func setenv(t *testing.T, name, value string) {
	t.Helper()
	assert.NoError(t, os.Setenv(name, value))
}
