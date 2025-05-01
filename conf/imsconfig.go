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
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"
)

var (
	Cfg       *IMSConfig
	testUsers []TestUser
)

// DefaultIMS is the base configuration used for the IMS server.
// It gets overridden by values in conf/imsd.toml, then the result
// of that gets overridden by environment variables.
func DefaultIMS() *IMSConfig {
	return &IMSConfig{
		Core: ConfigCore{
			Host:                 "localhost",
			Port:                 80,
			JWTSecret:            rand.Text(),
			Deployment:           "dev",
			LogLevel:             "INFO",
			AccessTokenLifetime:  15 * time.Minute,
			RefreshTokenLifetime: 8 * time.Hour,
		},
		Store: Store{
			MySQL: StoreMySQL{
				HostName: "localhost",
				HostPort: 3306,
				Database: "ims",
			},
		},
		Directory: Directory{
			Directory: DirectoryTypeClubhouseDB,
			TestUsers: testUsers,
			ClubhouseDB: ClubhouseDB{
				Hostname: "localhost:3306",
				Database: "rangers",
			},
		},
	}
}

func (c *IMSConfig) String() string {
	if c == nil {
		return "nil"
	}
	marshalled, err := json.MarshalIndent(*c, "", "  ")
	if err != nil {
		return "failed to marshal IMSConfig"
	}
	return string(marshalled)
}

type IMSConfig struct {
	Core ConfigCore
	// TODO: finish attachments feature
	AttachmentsStore struct {
		S3 struct {
			S3AccessKeyId     string
			S3SecretAccessKey string
			S3DefaultRegion   string
			S3Bucket          string
		}
	}
	Store     Store
	Directory Directory
}

type DirectoryType string
type DeploymentType string

const (
	DirectoryTypeClubhouseDB DirectoryType = "clubhousedb"
	DirectoryTypeTestUsers   DirectoryType = "testusers"
	DeploymentTypeDev                      = "dev"
	DeploymentTypeStaging                  = "staging"
	DeploymentTypeProduction               = "production"
)

func (d DirectoryType) Validate() error {
	switch d {
	case DirectoryTypeClubhouseDB, DirectoryTypeTestUsers:
		return nil
	default:
		return fmt.Errorf("unknown directory type %v", d)
	}
}

func (d DeploymentType) Validate() error {
	switch d {
	case DeploymentTypeDev, DeploymentTypeStaging, DeploymentTypeProduction:
		return nil
	default:
		return fmt.Errorf("unknown deployment type %v", d)
	}
}

type ConfigCore struct {
	Host                 string
	Port                 int32
	AccessTokenLifetime  time.Duration
	RefreshTokenLifetime time.Duration
	Admins               []string
	MasterKey            string
	// JWTSecret won't get marshalled as part of String() due to the json "-" tag.
	JWTSecret        string `json:"-"`
	AttachmentsStore string
	Deployment       string

	// LogLevel should be one of DEBUG, INFO, WARN, or ERROR
	LogLevel string
}

type Store struct {
	MySQL StoreMySQL
}

type StoreMySQL struct {
	HostName string
	HostPort int32
	Database string
	Username string
	// Password won't get marshalled as part of String() due to the json "-" tag.
	Password string `json:"-"`
}

type TestUser struct {
	Handle      string
	Email       string
	Status      string
	DirectoryID int64
	Password    string
	Onsite      bool
	Positions   []string
	Teams       []string
}

type Directory struct {
	Directory   DirectoryType
	TestUsers   []TestUser
	ClubhouseDB ClubhouseDB
}

type ClubhouseDB struct {
	Hostname string
	Database string
	Username string
	// Password won't get marshalled as part of String() due to the json "-" tag.
	Password string `json:"-"`
}
