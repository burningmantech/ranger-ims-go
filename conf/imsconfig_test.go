package conf_test

import (
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/stretchr/testify/require"
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

	s, err := cfg.PrintRedacted()
	require.NoError(t, err)
	require.Contains(t, s, "admin user")
	require.Contains(t, s, "db username")
	require.NotContains(t, s, "db password")
	require.NotContains(t, s, "user password")
	require.Contains(t, s, "clubhouse username")
	require.NotContains(t, s, "clubhouse password")
}
