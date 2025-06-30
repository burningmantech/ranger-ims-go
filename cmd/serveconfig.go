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

package cmd

import (
	"fmt"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/joho/godotenv"
	"log/slog"
	"os"
	"strings"
	"time"
)

// mustApplyEnvConfig reads in the .env file and ENV variables and applies those to baseCfg.
func mustApplyEnvConfig(baseCfg *conf.IMSConfig, envFileName string) *conf.IMSConfig {
	err := godotenv.Load(envFileName)

	if err != nil && !os.IsNotExist(err) {
		must(err)
	}
	if os.IsNotExist(err) {
		// if it's not the default
		if envFileName != ".env" {
			must(fmt.Errorf("envfile '%v' was set by the caller, but the file was not found", envFileName))
		}
		slog.Info("No .env file found. Carrying on with IMSConfig defaults and environment variable overrides")
	}

	if v, ok := lookupEnv("IMS_HOSTNAME"); ok {
		baseCfg.Core.Host = v
	}
	if v, ok := lookupEnv("IMS_PORT"); ok {
		baseCfg.Core.Port, err = conv.ParseInt32(v)
		must(err)
	}
	if v, ok := lookupEnv("IMS_DEPLOYMENT"); ok {
		baseCfg.Core.Deployment = conf.DeploymentType(strings.ToLower(v))
	}
	// This should really be called "IMS_REFRESH_TOKEN_LIFETIME". This name of
	// "IMS_TOKEN_LIFETIME" predates our use of refresh tokens, and what it tried
	// to convey, i.e. the maximum duration for a session, is now what we mean
	// when we talk about a refresh token's lifetime.
	if v, ok := lookupEnv("IMS_TOKEN_LIFETIME"); ok {
		seconds, err := conv.ParseInt64(v)
		must(err)
		baseCfg.Core.RefreshTokenLifetime = time.Duration(seconds) * time.Second
	}
	if v, ok := lookupEnv("IMS_ACCESS_TOKEN_LIFETIME"); ok {
		seconds, err := conv.ParseInt64(v)
		must(err)
		baseCfg.Core.AccessTokenLifetime = time.Duration(seconds) * time.Second
	}
	if v, ok := lookupEnv("IMS_CACHE_CONTROL_SHORT"); ok {
		dur, err := time.ParseDuration(v)
		must(err)
		baseCfg.Core.CacheControlShort = dur
	}
	if v, ok := lookupEnv("IMS_DIRECTORY_CACHE_TTL"); ok {
		dur, err := time.ParseDuration(v)
		must(err)
		baseCfg.Directory.InMemoryCacheTTL = dur
	}
	if v, ok := lookupEnv("IMS_CACHE_CONTROL_LONG"); ok {
		// These values must be given with a time unit in the env variable,
		// e.g. "20s" or "5m10s". ParseDuration will fail here if the value
		// is just a nonzero number.
		dur, err := time.ParseDuration(v)
		must(err)
		baseCfg.Core.CacheControlLong = dur
	}
	if v, ok := lookupEnv("IMS_LOG_LEVEL"); ok {
		baseCfg.Core.LogLevel = v
	}
	if v, ok := lookupEnv("IMS_ACTION_LOG_ENABLED"); ok {
		baseCfg.Core.ActionLogEnabled = strings.EqualFold(v, "true")
	}
	if v, ok := lookupEnv("IMS_DIRECTORY"); ok {
		baseCfg.Directory.Directory = conf.DirectoryType(strings.ToLower(v))
	}
	if v, ok := lookupEnv("IMS_ADMINS"); ok {
		baseCfg.Core.Admins = strings.Split(v, ",")
	}
	if v, ok := lookupEnv("IMS_JWT_SECRET"); ok {
		baseCfg.Core.JWTSecret = v
	}
	if v, ok := lookupEnv("IMS_DB_STORE_TYPE"); ok {
		baseCfg.Store.Type = conf.DBStoreType(strings.ToLower(v))
	}
	if v, ok := lookupEnv("IMS_DB_HOST_NAME"); ok {
		baseCfg.Store.MariaDB.HostName = v
	}
	if v, ok := lookupEnv("IMS_DB_HOST_PORT"); ok {
		baseCfg.Store.MariaDB.HostPort, err = conv.ParseInt32(v)
		must(err)
	}
	if v, ok := lookupEnv("IMS_DB_DATABASE"); ok {
		baseCfg.Store.MariaDB.Database = v
	}
	if v, ok := lookupEnv("IMS_DB_USER_NAME"); ok {
		baseCfg.Store.MariaDB.Username = v
	}
	if v, ok := lookupEnv("IMS_DB_PASSWORD"); ok {
		baseCfg.Store.MariaDB.Password = v
	}
	if v, ok := lookupEnv("IMS_DMS_HOSTNAME"); ok {
		baseCfg.Directory.ClubhouseDB.Hostname = v
	}
	if v, ok := lookupEnv("IMS_DMS_DATABASE"); ok {
		baseCfg.Directory.ClubhouseDB.Database = v
	}
	if v, ok := lookupEnv("IMS_DMS_USERNAME"); ok {
		baseCfg.Directory.ClubhouseDB.Username = v
	}
	if v, ok := lookupEnv("IMS_DMS_PASSWORD"); ok {
		baseCfg.Directory.ClubhouseDB.Password = v
	}
	if v, ok := lookupEnv("IMS_ATTACHMENTS_STORE"); ok {
		baseCfg.AttachmentsStore.Type = conf.AttachmentsStoreType(v)
	}
	if v, ok := lookupEnv("IMS_ATTACHMENTS_LOCAL_DIR"); ok {
		err = os.MkdirAll(v, 0750)
		must(err)
		root, err := os.OpenRoot(v)
		must(err)
		baseCfg.AttachmentsStore.Local.Dir = root
	}
	// These three AWS env vars use the standard names, hence no "IMS_" prefix.
	if v, ok := lookupEnv("AWS_ACCESS_KEY_ID"); ok {
		baseCfg.AttachmentsStore.S3.AWSAccessKeyID = v
	}
	if v, ok := lookupEnv("AWS_SECRET_ACCESS_KEY"); ok {
		baseCfg.AttachmentsStore.S3.AWSSecretAccessKey = v
	}
	if v, ok := lookupEnv("AWS_REGION"); ok {
		baseCfg.AttachmentsStore.S3.AWSRegion = v
	}

	if v, ok := lookupEnv("IMS_ATTACHMENTS_S3_BUCKET"); ok {
		baseCfg.AttachmentsStore.S3.Bucket = v
	}
	if v, ok := lookupEnv("IMS_ATTACHMENTS_S3_COMMON_KEY_PREFIX"); ok {
		baseCfg.AttachmentsStore.S3.CommonKeyPrefix = v
	}

	return baseCfg
}

func lookupEnv(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	// When doing `docker run --env-file .env`, Docker passes in vars without removing
	// the double-quotes, e.g. IMS_HOSTNAME="localhost" would actually get passed into
	// the program with the double-quotes in place.
	// https://github.com/docker/cli/issues/3630
	if strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"") {
		v = v[1 : len(v)-1]
	}
	return v, true
}
