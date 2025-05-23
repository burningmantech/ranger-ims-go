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
	"github.com/burningmantech/ranger-ims-go/lib/authn"
	"runtime"
	"strings"
)

func init() {
	// Configure TestUsers here. Note that these won't be loaded by the server if
	// this filename contains ".example", so you'll want to copy this file as
	// "testusers.go" so that the server knows to load it. That'll also let you make
	// local customizations more easily, because testusers.go is gitignored.
	addTestUsers := []TestUser{
		{
			Handle:      "Hardware",
			Email:       "hardware@rangers.brc",
			Status:      "active",
			DirectoryID: 10101,
			Password:    authn.NewSalted("Hardware"),
			Onsite:      true,
			Positions:   []string{"Driver", "Dancer"},
			Teams:       []string{"Driving Team"},
		},
		{
			Handle:      "Parenthetical",
			Email:       "parenthetical@rangers.brc",
			Status:      "active",
			DirectoryID: 90909,
			Password:    authn.NewSalted("Parenthetical"),
			Onsite:      true,
			Positions:   nil,
			Teams:       nil,
		},
		{
			Handle:      "Defect",
			Email:       "defect@rangers.brc",
			Status:      "active",
			DirectoryID: 20202,
			Password:    authn.NewSalted("Defect"),
			Onsite:      true,
			Positions:   []string{},
			Teams:       []string{},
		},
		{
			Handle:      "Irate",
			Email:       "irate@rangers.brc",
			Status:      "active",
			DirectoryID: 50505,
			Password:    authn.NewSalted("Irate"),
			Onsite:      false,
			Positions:   []string{},
			Teams:       []string{},
		},
		{
			Handle:      "Loosy",
			Email:       "loosy@rangers.brc",
			Status:      "active",
			DirectoryID: 70707,
			Password:    authn.NewSalted("Loosy"),
			Onsite:      true,
			Positions:   []string{},
			Teams:       []string{},
		},
	}
	_, filename, _, _ := runtime.Caller(0)
	if strings.Contains(filename, ".example") {
		return
	}
	defaultTestUsers = append(defaultTestUsers, addTestUsers...)
}
