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

// mib is the number of bytes in 1 MiB.
const mib = 1 << 20

// DefaultIMS is the base configuration used for the IMS server.
// It gets overridden by values in .env, if present, then the result
// of that gets overridden by environment variables. See mustApplyEnvConfig
// for all the environment variable names.
func DefaultIMS() *IMSConfig {
	return &IMSConfig{
		Core: ConfigCore{
			Host:                 "localhost",
			Port:                 8080,
			JWTSecret:            rand.Text(),
			Deployment:           "dev",
			LogLevel:             "INFO",
			AccessTokenLifetime:  15 * time.Minute,
			RefreshTokenLifetime: 8 * time.Hour,
			CacheControlShort:    20 * time.Minute,
			CacheControlLong:     2 * time.Hour,
			MaxRequestBytes:      100 * mib,
			ActionLogEnabled:     true,
		},
		Store: DBStore{
			Type: DBStoreTypeNoOp,
			MariaDB: DBStoreMaria{
				HostName: "localhost",
				HostPort: 3306,
				Database: "ims",
				// Some arbitrary value. We'll get errors from MariaDB if the server
				// hits the DB with too many parallel requests.
				MaxOpenConns: 20,
			},
		},
		Directory: Directory{
			Directory: DirectoryTypeNoOp,
			ClubhouseDB: ClubhouseDB{
				Hostname: "localhost:3306",
				Database: "rangers",
			},
			InMemoryCacheTTL: 5 * time.Minute,
		},
		AttachmentsStore: AttachmentsStore{
			Type: AttachmentsStoreNone,
		},
	}
}

// Validate should be called after an IMSConfig has been fully configured.
func (c *IMSConfig) Validate() error {
	var errs []error

	// IMS database
	errs = append(errs, c.Store.Type.Validate())
	if c.Store.Type != DBStoreTypeMaria {
		c.Store.MariaDB = DBStoreMaria{}
	}

	// User directory
	errs = append(errs, c.Directory.Directory.Validate())
	if c.Directory.Directory != DirectoryTypeClubhouseDB {
		c.Directory.ClubhouseDB = ClubhouseDB{}
	}

	// Deployment
	errs = append(errs, c.Core.Deployment.Validate())
	if c.Core.Deployment != DeploymentTypeDev {
		if c.Directory.Directory != DirectoryTypeClubhouseDB {
			errs = append(errs, errors.New("non-dev environments must use a ClubhouseDB directory"))
		}
		if c.Store.Type != DBStoreTypeMaria {
			errs = append(errs, errors.New("non-dev environments must use a MariaDB datastore"))
		}
	}

	// Attachments store
	errs = append(errs, c.AttachmentsStore.Type.Validate())
	if c.AttachmentsStore.Type == AttachmentsStoreLocal {
		if c.AttachmentsStore.Local.Dir == nil {
			errs = append(errs, errors.New("local attachments store requires a local directory"))
		}
		c.AttachmentsStore.S3 = S3Attachments{}
	}
	if c.AttachmentsStore.Type == AttachmentsStoreS3 {
		s3 := c.AttachmentsStore.S3
		if s3.AWSRegion == "" || s3.Bucket == "" {
			errs = append(errs, errors.New("s3 attachments store requires Default AWSRegion and Bucket"))
		}
		// if s3.AWSAccessKeyID == "" || s3.AWSSecretAccessKey == "" || s3.AWSRegion == "" || s3.Bucket == "" {
		//	errs = append(errs, errors.New("s3 attachments store requires Key ID, Secret Key, Default AWSRegion, and Bucket"))
		//}
		if c.AttachmentsStore.Local.Dir != nil {
			errs = append(errs, c.AttachmentsStore.Local.Dir.Close())
		}
		c.AttachmentsStore.Local = LocalAttachments{}
	}

	// Assorted other validations
	if c.Core.AccessTokenLifetime > c.Core.RefreshTokenLifetime {
		errs = append(errs, errors.New("access token lifetime should not be greater than refresh token lifetime"))
	}
	return errors.Join(errs...)
}

func (c *IMSConfig) PrintRedacted() string {
	return c.String()
}

func (c *IMSConfig) String() string {
	return string(redact.ToBytes(c))
}

type IMSConfig struct {
	Core             ConfigCore
	AttachmentsStore AttachmentsStore
	Store            DBStore
	Directory        Directory
}

type DirectoryType string

type AttachmentsStoreType string
type DeploymentType string

type DBStoreType string

// All these consts should have lowercase values to allow case-insensitive matching.
const (
	DirectoryTypeClubhouseDB DirectoryType        = "clubhousedb"
	DirectoryTypeNoOp        DirectoryType        = "noop"
	AttachmentsStoreLocal    AttachmentsStoreType = "local"
	AttachmentsStoreS3       AttachmentsStoreType = "s3"
	AttachmentsStoreNone     AttachmentsStoreType = "none"
	DeploymentTypeDev        DeploymentType       = "dev"
	DeploymentTypeTraining   DeploymentType       = "training"
	DeploymentTypeStaging    DeploymentType       = "staging"
	DeploymentTypeProduction DeploymentType       = "production"
	DBStoreTypeMaria         DBStoreType          = "mariadb"
	DBStoreTypeNoOp          DBStoreType          = "noop"
)

func (d DBStoreType) Validate() error {
	switch d {
	case DBStoreTypeMaria, DBStoreTypeNoOp:
		return nil
	default:
		return fmt.Errorf("unknown DB store type %v", d)
	}
}

func (d DirectoryType) Validate() error {
	switch d {
	case DirectoryTypeClubhouseDB, DirectoryTypeNoOp:
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
	case DeploymentTypeDev, DeploymentTypeStaging, DeploymentTypeProduction, DeploymentTypeTraining:
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
	Deployment           DeploymentType

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

	// MaxRequestBytes is a hard limit on request sizes that will be permitted by the API server.
	// This serve as a backstop against accidentally or maliciously large requests.
	MaxRequestBytes int64

	// ActionLogEnabled is a global toggle switch for enabling writing to the ACTION_LOG table.
	ActionLogEnabled bool
}

type DBStore struct {
	Type    DBStoreType
	MariaDB DBStoreMaria
}

type DBStoreMaria struct {
	HostName     string
	HostPort     int32
	Database     string
	Username     string
	Password     string `redact:"true"`
	MaxOpenConns int32
}

type Directory struct {
	Directory        DirectoryType
	ClubhouseDB      ClubhouseDB
	InMemoryCacheTTL time.Duration
}

type AttachmentsStore struct {
	Type  AttachmentsStoreType
	Local LocalAttachments
	S3    S3Attachments
}

type ClubhouseDB struct {
	Hostname     string
	Database     string
	Username     string
	Password     string `redact:"true"`
	MaxOpenConns int32
}

type LocalAttachments struct {
	Dir *os.Root
}

type S3Attachments struct {
	AWSAccessKeyID     string
	AWSSecretAccessKey string `redact:"true"`
	AWSRegion          string
	Bucket             string
	CommonKeyPrefix    string
}
