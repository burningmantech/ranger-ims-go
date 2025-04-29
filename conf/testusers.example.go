package conf

import (
	"github.com/burningmantech/ranger-ims-go/auth/password"
	"runtime"
	"strings"
)

func init() {
	_, filename, _, _ := runtime.Caller(0)
	if strings.Contains(filename, ".example") {
		return
	}
	testUsers = append(testUsers,
		TestUser{
			Handle:      "Hardware",
			Email:       "hardware@rangers.brc",
			Status:      "active",
			DirectoryID: 10101,
			Password:    password.NewSalted("Hardware"),
			Onsite:      true,
			Positions:   []string{"Driver", "Dancer"},
			Teams:       []string{"Driving Team"},
		},
		TestUser{
			Handle:      "Parenthetical",
			Email:       "parenthetical@rangers.brc",
			Status:      "active",
			DirectoryID: 90909,
			Password:    password.NewSalted("Parenthetical"),
			Onsite:      true,
			Positions:   nil,
			Teams:       nil,
		},
	)
}
