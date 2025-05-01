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
	"errors"
	"fmt"
	"github.com/burningmantech/ranger-ims-go/api"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/web"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Launch the IMS server",
	Long: "Launch the IMS server\n\n" +
		"Configuration will be read from conf/imsd.toml, and can be overridden by environment variables.",
	Run: runServer,
}

func runServer(cmd *cobra.Command, args []string) {
	imsCfg := conf.Cfg

	var logLevel slog.Level
	must(logLevel.UnmarshalText([]byte(imsCfg.Core.LogLevel)))
	slog.SetLogLoggerLevel(logLevel)

	log.Printf("Have config\n%v", imsCfg)
	log.Printf("With JWTSecret: %v...%v", imsCfg.Core.JWTSecret[:1], imsCfg.Core.JWTSecret[len(imsCfg.Core.JWTSecret)-1:])

	var userStore *directory.UserStore
	var err error
	switch imsCfg.Directory.Directory {
	case conf.DirectoryTypeClubhouseDB:
		userStore, err = directory.NewUserStore(nil, directory.MariaDB(imsCfg))
	case conf.DirectoryTypeTestUsers:
		userStore, err = directory.NewUserStore(imsCfg.Directory.TestUsers, nil)
	default:
		err = fmt.Errorf("unknown directory %v", imsCfg.Directory.Directory)
	}
	must(err)
	imsDB := store.MariaDB(imsCfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	mux := http.NewServeMux()
	api.AddToMux(ctx, mux, imsCfg, &store.DB{DB: imsDB}, userStore)
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

	slog.Info("IMS server ready for connections", "address", addr)
	go s.ListenAndServe()

	<-ctx.Done()
	stop()
	slog.Error("Shutting down gracefully, press Ctrl+C again to force")

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Error("Server shut down", "err", s.Shutdown(timeoutCtx))
	os.Exit(0)
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

// initConfig reads in the .env file and ENV variables if set.
func initConfig() {
	newCfg := conf.DefaultIMS()
	err := godotenv.Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Info("No .env file found. Carrying on with IMSConfig defaults and environment variable overrides")
		} else {
			slog.Error("Exiting due to error loading .env file", "err", err)
			os.Exit(1)
		}
	}
	if v, ok := lookupEnv("IMS_HOSTNAME"); ok {
		newCfg.Core.Host = v
	}
	if v, ok := lookupEnv("IMS_PORT"); ok {
		num, err := strconv.ParseInt(v, 10, 32)
		must(err)
		newCfg.Core.Port = int32(num)
	}
	if v, ok := lookupEnv("IMS_DEPLOYMENT"); ok {
		newCfg.Core.Deployment = strings.ToLower(v)
	}
	if v, ok := lookupEnv("IMS_TOKEN_LIFETIME"); ok {
		seconds, err := strconv.ParseInt(v, 10, 64)
		must(err)
		newCfg.Core.TokenLifetime = time.Duration(seconds) * time.Second
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
		newCfg.Store.MySQL.HostName = v
	}
	if v, ok := lookupEnv("IMS_DB_HOST_POST"); ok {
		num, err := strconv.ParseInt(v, 10, 32)
		must(err)
		newCfg.Store.MySQL.HostPort = int32(num)
	}
	if v, ok := lookupEnv("IMS_DB_DATABASE"); ok {
		newCfg.Store.MySQL.Database = v
	}
	if v, ok := lookupEnv("IMS_DB_USER_NAME"); ok {
		newCfg.Store.MySQL.Username = v
	}
	if v, ok := lookupEnv("IMS_DB_PASSWORD"); ok {
		newCfg.Store.MySQL.Password = v
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

	// Validations on the config created above
	must(newCfg.Directory.Directory.Validate())
	if newCfg.Core.Deployment != "dev" {
		if newCfg.Directory.Directory == conf.DirectoryTypeTestUsers {
			must(fmt.Errorf("do not use TestUsers outside dev! A ClubhouseDB must be provided"))
		}
	}

	conf.Cfg = newCfg
}

func init() {
	rootCmd.AddCommand(serveCmd)

	cobra.OnInitialize(initConfig)
}

// must logs an error and kills the program. This should only be done for
// startup errors, not after the server is up and running.
func must(err error) {
	if err != nil {
		slog.Error("Exiting due to startup error", "err", err)
		os.Exit(1)
	}
}
