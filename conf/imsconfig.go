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
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"reflect"
	"strings"
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

func printRedacted(w io.Writer, v reflect.Value, indent string) error {
	const nestIndent = "    "
	s := v
	typeOfT := s.Type()
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)

		redact := strings.EqualFold(typeOfT.Field(i).Tag.Get("redact"), "true")

		switch f.Kind() {
		case reflect.Struct:
			x1 := reflect.ValueOf(f.Interface())
			_, err := fmt.Fprintf(w, "%v%v\n", indent, typeOfT.Field(i).Name)
			if err != nil {
				return err
			}
			if redact {
				_, err = fmt.Fprintf(w, "%v🤐🤐🤐🤐🤐\n", indent+nestIndent)
				if err != nil {
					return err
				}
			} else {
				err = printRedacted(w, x1, indent+nestIndent)
				if err != nil {
					return err
				}
			}
		case reflect.Slice:
			x1 := reflect.ValueOf(f.Interface())
			sliceElemType := f.Type().Elem()
			if sliceElemType.Kind() != reflect.Struct {
				printVal := "[🤐🤐🤐🤐]"
				if !redact {
					printVal = fmt.Sprint(f.Interface())
				}
				_, err := fmt.Fprintf(w, "%v%v = %v\n", indent, typeOfT.Field(i).Name, printVal)
				if err != nil {
					return err
				}
			} else {
				for j := 0; j < x1.Len(); j++ {
					_, err := fmt.Fprintf(w, "%v%v[%d]\n", indent, typeOfT.Field(i).Name, j)
					if err != nil {
						return err
					}
					if redact {
						_, err = fmt.Fprintf(w, "%v🤐🤐\n", indent+nestIndent)
						if err != nil {
							return err
						}
					} else {
						err = printRedacted(w, x1.Index(j), indent+nestIndent)
						if err != nil {
							return err
						}
					}
				}
			}
		case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
			reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
			printVal := "🤐🤐🤐"
			if !redact {
				printVal = fmt.Sprint(f.Interface())
			}
			_, err := fmt.Fprintf(w, "%v%v = %v\n", indent, typeOfT.Field(i).Name, printVal)
			if err != nil {
				return err
			}
		default:
			// e.g. we haven't bothered adding map support, because it hasn't been needed yet
			panic("unsupported field kind: " + f.Kind().String())
		}
	}
	return nil
}

func (c *IMSConfig) PrintRedacted() (string, error) {
	output := &bytes.Buffer{}
	err := printRedacted(output, reflect.ValueOf(c).Elem(), "")
	if err != nil {
		return "", fmt.Errorf("[printRedacted]: %w", err)
	}
	return output.String(), nil
}

func (c *IMSConfig) String() string {
	s, err := c.PrintRedacted()
	if err != nil {
		panic(err)
	}
	return s
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
	Directory   DirectoryType
	TestUsers   []TestUser
	ClubhouseDB ClubhouseDB
}

type ClubhouseDB struct {
	Hostname string
	Database string
	Username string
	Password string `redact:"true"`
}
