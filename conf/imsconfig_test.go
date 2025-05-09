package conf_test

import (
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPrintRedacted(t *testing.T) {
	t.Parallel()
	cfg := conf.IMSConfig{
		Core: conf.ConfigCore{
			Admins: []string{"admin user"},
		},
		Store: conf.Store{
			MariaDB: conf.StoreMariaDB{
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
