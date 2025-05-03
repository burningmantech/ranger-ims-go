package conf

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPrintRedacted(t *testing.T) {
	cfg := IMSConfig{
		Core: ConfigCore{
			Admins: []string{"admin user"},
		},
		Store: Store{
			MySQL: StoreMySQL{
				Username: "db username",
				Password: "db password",
			},
		},
		Directory: Directory{
			TestUsers: []TestUser{
				{
					Password: "user password",
				},
			},
			ClubhouseDB: ClubhouseDB{
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
