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
	"fmt"
	"github.com/burningmantech/ranger-ims-go/lib/redact"
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
			CacheControlShort:    20 * time.Minute,
			CacheControlLong:     2 * time.Hour,
		},
		Store: Store{
			MariaDB: StoreMariaDB{
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
			InMemoryCacheTTL: 10 * time.Minute,
		},
	}
}

func (c *IMSConfig) PrintRedacted() (string, error) {
	b, err := redact.ToBytes(c)
	return string(b), err
}

func (c *IMSConfig) String() string {
	b, err := redact.ToBytes(c)
	if err != nil {
		panic(err)
	}
	return string(b)
}

type IMSConfig struct {
	Core ConfigCore
	// TODO: finish attachments feature
	AttachmentsStore struct {
		S3 struct {
			S3AccessKeyId     string
			S3SecretAccessKey string `redact:"true"`
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
	MasterKey            string `redact:"true"`
	JWTSecret            string `redact:"true"`
	AttachmentsStore     string
	Deployment           string

	// CacheControlShort is the duration we set in various responses' Cache-Control headers
	// for resources that aren't expected to change often, but still do change (e.g. the list of
	// Events, Personnel, and Incident Types). Set this to 0 to disable that client-side caching.
	CacheControlShort time.Duration

	// CacheControlLong is the duration we set in various responses' Cache-Control headers
	// for resources that won't change unless IMS is recompiled or its IMSConfig altered.
	// For example, this is used for all the template html, JS, and CSS
	CacheControlLong time.Duration

	// LogLevel should be one of DEBUG, INFO, WARN, or ERROR
	LogLevel string
}

type Store struct {
	MariaDB StoreMariaDB
}

type StoreMariaDB struct {
	HostName string
	HostPort int32
	Database string
	Username string
	Password string `redact:"true"`
}

type TestUser struct {
	Handle      string
	Email       string
	Status      string
	DirectoryID int64
	Password    string `redact:"true"`
	Onsite      bool
	Positions   []string
	Teams       []string
}

type Directory struct {
	Directory        DirectoryType
	TestUsers        []TestUser
	ClubhouseDB      ClubhouseDB
	InMemoryCacheTTL time.Duration
}

type ClubhouseDB struct {
	Hostname string
	Database string
	Username string
	Password string `redact:"true"`
}
