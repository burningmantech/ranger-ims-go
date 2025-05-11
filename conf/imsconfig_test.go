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
