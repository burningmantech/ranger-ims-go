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
	"errors"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/lib/redact"
	"os"
	"time"
)

var defaultTestUsers []TestUser

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
			TestUsers: defaultTestUsers,
			ClubhouseDB: ClubhouseDB{
				Hostname: "localhost:3306",
				Database: "rangers",
			},
			InMemoryCacheTTL: 10 * time.Minute,
		},
		AttachmentsStore: AttachmentsStore{
			Type: AttachmentsStoreNone,
		},
	}
}

// Validate should be called after an IMSConfig has been fully configured.
func (c *IMSConfig) Validate() error {
	var errs []error
	errs = append(errs, c.Directory.Directory.Validate())
	errs = append(errs, c.AttachmentsStore.Type.Validate())
	if c.AttachmentsStore.Type == AttachmentsStoreLocal {
		if c.AttachmentsStore.Local.Dir == nil {
			errs = append(errs, errors.New("local attachments store has been requested, "+
				"so a local directory must be provided"))
		}
	}
	if c.Core.Deployment != "dev" {
		if c.Directory.Directory == DirectoryTypeTestUsers {
			errs = append(errs, errors.New("do not use TestUsers outside dev! A ClubhouseDB must be provided"))
		}
	}
	if c.Core.AccessTokenLifetime > c.Core.RefreshTokenLifetime {
		errs = append(errs, errors.New("access token lifetime should not be greater than refresh token lifetime"))
	}
	return errors.Join(errs...)
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
	Core             ConfigCore
	AttachmentsStore AttachmentsStore
	Store            Store
	Directory        Directory
}

type DirectoryType string

type AttachmentsStoreType string
type DeploymentType string

const (
	DirectoryTypeClubhouseDB DirectoryType        = "clubhousedb"
	DirectoryTypeTestUsers   DirectoryType        = "testusers"
	AttachmentsStoreLocal    AttachmentsStoreType = "local"
	AttachmentsStoreS3       AttachmentsStoreType = "s3"
	AttachmentsStoreNone     AttachmentsStoreType = "none"
	DeploymentTypeDev                             = "dev"
	DeploymentTypeStaging                         = "staging"
	DeploymentTypeProduction                      = "production"
)

func (d DirectoryType) Validate() error {
	switch d {
	case DirectoryTypeClubhouseDB, DirectoryTypeTestUsers:
		return nil
	default:
		return fmt.Errorf("unknown directory type %v", d)
	}
}

func (a AttachmentsStoreType) Validate() error {
	switch a {
	case AttachmentsStoreLocal, AttachmentsStoreS3, AttachmentsStoreNone:
		return nil
	default:
		return fmt.Errorf("unknown attachments store type %v", a)
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

type AttachmentsStore struct {
	Type  AttachmentsStoreType
	Local LocalAttachments
	S3    S3Attachments
}

type ClubhouseDB struct {
	Hostname string
	Database string
	Username string
	Password string `redact:"true"`
}

type LocalAttachments struct {
	Dir *os.Root
}

type S3Attachments struct {
	S3AccessKeyID     string
	S3SecretAccessKey string `redact:"true"`
	S3DefaultRegion   string
	S3Bucket          string
	S3BucketSubPath   string
}
