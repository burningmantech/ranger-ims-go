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
	"context"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/api"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/web"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const envfileFlagName = "envfile"

// serveCmd represents the serve command.
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Launch the IMS server",
	Long: "Launch the IMS server\n\n" +
		"Configuration will be read from conf/imsd.toml, and can be overridden by environment variables.",
	Run: runServer,
}

func runServer(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	imsCfg := mustInitConfig(cmd.Flags().Lookup(envfileFlagName))

	var logLevel slog.Level
	must(logLevel.UnmarshalText([]byte(imsCfg.Core.LogLevel)))
	slog.SetLogLoggerLevel(logLevel)

	cfgStr := imsCfg.PrintRedacted()
	log.Printf("Here's the final redacted IMSConfig:\n\n%v\n\n", cfgStr)

	log.Printf("With JWTSecret: %v...%v", imsCfg.Core.JWTSecret[:1], imsCfg.Core.JWTSecret[len(imsCfg.Core.JWTSecret)-1:])

	var err error
	var userStore *directory.UserStore
	switch imsCfg.Directory.Directory {
	case conf.DirectoryTypeClubhouseDB:
		db, err := directory.MariaDB(ctx, imsCfg)
		must(err)
		userStore, err = directory.NewUserStore(nil, db, imsCfg.Directory.InMemoryCacheTTL)
		must(err)
	case conf.DirectoryTypeTestUsers:
		userStore, err = directory.NewUserStore(imsCfg.Directory.TestUsers, nil, imsCfg.Directory.InMemoryCacheTTL)
	default:
		err = fmt.Errorf("unknown directory %v", imsCfg.Directory.Directory)
	}
	must(err)
	imsDB, err := store.MariaDB(ctx, imsCfg.Store.MariaDB, true)
	must(err)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)

	eventSource := api.NewEventSourcerer()
	mux := http.NewServeMux()
	api.AddToMux(mux, eventSource, imsCfg, &store.DB{DB: imsDB}, userStore)
	web.AddToMux(mux, imsCfg)

	addr := fmt.Sprintf("%v:%v", imsCfg.Core.Host, imsCfg.Core.Port)
	s := &http.Server{
		Addr:        addr,
		Handler:     mux,
		ReadTimeout: 30 * time.Second,
		// This needs to be long to support long-lived EventSource calls.
		// After this duration, a client will be disconnected and forced
		// to reconnect.
		WriteTimeout:   30 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}
	s.RegisterOnShutdown(func() {
		eventSource.Server.Close()
	})
	go func() {
		err := s.ListenAndServe()
		slog.Error("ListenAndServe", "err", err)
	}()

	slog.Info("IMS server is ready for connections", "address", addr)
	slog.Info(fmt.Sprintf("Visit the web frontend at http://%v/ims/app", addr))

	// The goroutine will hang here until the NotifyContext is done
	<-ctx.Done()
	stop()
	slog.Error("Shutting down gracefully, press Ctrl+C again to force")

	// Tell the server to shut down, giving it this much time to do so gracefully.
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = s.Shutdown(timeoutCtx)
	slog.Error("Server shut down", "err", err)
	stop()
	cancel()
	os.Exit(1)
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

// mustInitConfig reads in the .env file and ENV variables if set.
func mustInitConfig(envfileFlag *pflag.Flag) *conf.IMSConfig {
	newCfg := conf.DefaultIMS()
	err := godotenv.Load(envFilename)

	if err != nil && !os.IsNotExist(err) {
		slog.Error("Exiting due to error loading .env file", "err", err)
		os.Exit(1)
	}
	if os.IsNotExist(err) {
		if envfileFlag.Changed {
			slog.Error("envfile was set by the caller, but the file was not found. Exiting...", "envFilename", envFilename)
			os.Exit(1)
		}
		slog.Info("No .env file found. Carrying on with IMSConfig defaults and environment variable overrides")
	}

	if v, ok := lookupEnv("IMS_HOSTNAME"); ok {
		newCfg.Core.Host = v
	}
	if v, ok := lookupEnv("IMS_PORT"); ok {
		newCfg.Core.Port, err = conv.ParseInt32(v)
		must(err)
	}
	if v, ok := lookupEnv("IMS_DEPLOYMENT"); ok {
		newCfg.Core.Deployment = strings.ToLower(v)
	}
	// This should really be called "IMS_REFRESH_TOKEN_LIFETIME". This name of
	// "IMS_TOKEN_LIFETIME" predates our use of refresh tokens, and what it tried
	// to convey, i.e. the maximum duration for a session, is now what we mean
	// when we talk about a refresh token's lifetime.
	if v, ok := lookupEnv("IMS_TOKEN_LIFETIME"); ok {
		seconds, err := conv.ParseInt64(v)
		must(err)
		newCfg.Core.RefreshTokenLifetime = time.Duration(seconds) * time.Second
	}
	if v, ok := lookupEnv("IMS_ACCESS_TOKEN_LIFETIME"); ok {
		seconds, err := conv.ParseInt64(v)
		must(err)
		newCfg.Core.AccessTokenLifetime = time.Duration(seconds) * time.Second
	}
	if v, ok := lookupEnv("IMS_CACHE_CONTROL_SHORT"); ok {
		dur, err := time.ParseDuration(v)
		must(err)
		newCfg.Core.CacheControlShort = dur
	}
	if v, ok := lookupEnv("IMS_DIRECTORY_CACHE_TTL"); ok {
		dur, err := time.ParseDuration(v)
		must(err)
		newCfg.Directory.InMemoryCacheTTL = dur
	}
	if v, ok := lookupEnv("IMS_CACHE_CONTROL_LONG"); ok {
		// These values must be given with a time unit in the env variable,
		// e.g. "20s" or "5m10s". ParseDuration will fail here if the value
		// is just a nonzero number.
		dur, err := time.ParseDuration(v)
		must(err)
		newCfg.Core.CacheControlLong = dur
	}
	if v, ok := lookupEnv("IMS_LOG_LEVEL"); ok {
		newCfg.Core.LogLevel = v
	}
	if v, ok := lookupEnv("IMS_DIRECTORY"); ok {
		newCfg.Directory.Directory = conf.DirectoryType(strings.ToLower(v))
	}
	if v, ok := lookupEnv("IMS_ADMINS"); ok {
		newCfg.Core.Admins = strings.Split(v, ",")
	}
	if v, ok := lookupEnv("IMS_JWT_SECRET"); ok {
		newCfg.Core.JWTSecret = v
	}
	if v, ok := lookupEnv("IMS_DB_HOST_NAME"); ok {
		newCfg.Store.MariaDB.HostName = v
	}
	if v, ok := lookupEnv("IMS_DB_HOST_POST"); ok {
		newCfg.Store.MariaDB.HostPort, err = conv.ParseInt32(v)
		must(err)
	}
	if v, ok := lookupEnv("IMS_DB_DATABASE"); ok {
		newCfg.Store.MariaDB.Database = v
	}
	if v, ok := lookupEnv("IMS_DB_USER_NAME"); ok {
		newCfg.Store.MariaDB.Username = v
	}
	if v, ok := lookupEnv("IMS_DB_PASSWORD"); ok {
		newCfg.Store.MariaDB.Password = v
	}
	if v, ok := lookupEnv("IMS_DMS_HOSTNAME"); ok {
		newCfg.Directory.ClubhouseDB.Hostname = v
	}
	if v, ok := lookupEnv("IMS_DMS_DATABASE"); ok {
		newCfg.Directory.ClubhouseDB.Database = v
	}
	if v, ok := lookupEnv("IMS_DMS_USERNAME"); ok {
		newCfg.Directory.ClubhouseDB.Username = v
	}
	if v, ok := lookupEnv("IMS_DMS_PASSWORD"); ok {
		newCfg.Directory.ClubhouseDB.Password = v
	}
	if v, ok := lookupEnv("IMS_ATTACHMENTS_STORE"); ok {
		newCfg.AttachmentsStore.Type = conf.AttachmentsStoreType(v)
	}
	if v, ok := lookupEnv("IMS_ATTACHMENTS_LOCAL_DIR"); ok {
		err = os.MkdirAll(v, 0750)
		must(err)
		root, err := os.OpenRoot(v)
		must(err)
		newCfg.AttachmentsStore.Local.Dir = root
	}

	must(newCfg.Validate())
	return newCfg
}

var envFilename string

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVar(&envFilename, envfileFlagName, ".env",
		"An env file from which to load IMS server configuration. "+
			"Defaults to '.env' in the current directory")
}

// must logs an error and kills the program. This should only be done for
// startup errors, not after the server is up and running.
func must(err error) {
	if err != nil {
		slog.Error("Exiting due to startup error", "err", err)
		os.Exit(1)
	}
}
