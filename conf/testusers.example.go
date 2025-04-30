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
		TestUser{
			Handle:      "Defect",
			Email:       "defect@rangers.brc",
			Status:      "active",
			DirectoryID: 20202,
			Password:    password.NewSalted("Defect"),
			Onsite:      true,
			Positions:   []string{},
			Teams:       []string{},
		},
		TestUser{
			Handle:      "Irate",
			Email:       "irate@rangers.brc",
			Status:      "active",
			DirectoryID: 50505,
			Password:    password.NewSalted("Irate"),
			Onsite:      false,
			Positions:   []string{},
			Teams:       []string{},
		},
		TestUser{
			Handle:      "Loosy",
			Email:       "loosy@rangers.brc",
			Status:      "active",
			DirectoryID: 70707,
			Password:    password.NewSalted("Loosy"),
			Onsite:      true,
			Positions:   []string{},
			Teams:       []string{},
		},
	)
}
