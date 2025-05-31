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
	chqueries "github.com/burningmantech/ranger-ims-go/directory/clubhousedb"
	"github.com/burningmantech/ranger-ims-go/lib/attachment"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"github.com/burningmantech/ranger-ims-go/lib/log"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/burningmantech/ranger-ims-go/web"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	envfileFlagName    = "envfile"
	envFileDefaultName = ".env"

	printConfigFlagName = "print-config"
)

// serveCmd represents the serve command.
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Launch the IMS server",
	Long: "Launch the IMS server\n\n" +
		"Configuration will be read from conf/imsd.toml, and can be overridden by environment variables.",
	Run: runServer,
}

func runServer(cmd *cobra.Command, args []string) {
	imsCfg := mustInitConfig(envFilename)
	os.Exit(runServerInternal(context.Background(), imsCfg, printConfig, make(chan string, 1)))
}

// runServerInternal starts the IMS server and blocks until it is terminated.
//
// The supplied channel will be provided with the address of the server at the time when
// the server is started and ready to accept connections.
func runServerInternal(
	ctx context.Context, unvalidatedCfg *conf.IMSConfig,
	printConfig bool, listeningAddr chan<- string,
) (exitCode int) {
	must(unvalidatedCfg.Validate())
	imsCfg := unvalidatedCfg

	configureLogger(imsCfg)

	if printConfig {
		cfgStr := imsCfg.PrintRedacted()
		stderrPrintf("Here's the final redacted IMSConfig:\n\n%v\n\n", cfgStr)
		stderrPrintf("With JWTSecret: %v...%v\n", imsCfg.Core.JWTSecret[:1], imsCfg.Core.JWTSecret[len(imsCfg.Core.JWTSecret)-1:])
	}

	clubhouseDB, err := directory.MariaDB(ctx, imsCfg.Directory)
	must(err)
	clubhouseDBQ := directory.NewDBQ(clubhouseDB, chqueries.New(), imsCfg.Directory.InMemoryCacheTTL)
	userStore := directory.NewUserStore(clubhouseDBQ)

	var s3Client *attachment.S3Client
	if imsCfg.AttachmentsStore.Type == conf.AttachmentsStoreS3 {
		s3Client, err = attachment.NewS3Client(ctx)
		must(err)
	}

	imsDB, err := store.SqlDB(ctx, imsCfg.Store, true)
	must(err)
	imsDBQ := store.NewDBQ(imsDB, imsdb.New())

	notifyCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)

	eventSource := api.NewEventSourcerer()
	mux := http.NewServeMux()
	api.AddToMux(mux, eventSource, imsCfg, imsDBQ, userStore, s3Client)
	web.AddToMux(mux, imsCfg)

	s := &http.Server{
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

	addr := fmt.Sprintf("%v:%v", imsCfg.Core.Host, imsCfg.Core.Port)
	listener, err := net.Listen("tcp", addr)
	must(err)
	addr = fmt.Sprintf("%v:%v", imsCfg.Core.Host, listener.Addr().(*net.TCPAddr).Port)

	go func() {
		err := s.Serve(listener)
		slog.Error("Serve", "err", err)
	}()

	slog.Info("IMS server is ready for connections", "addr", addr)
	slog.Info(fmt.Sprintf("Visit the web frontend at http://%v/ims/app", addr))

	_, _ = fmt.Fprint(os.Stderr, `
[31m  ▀█▀ █▄█ █▀▀   █▀▄ █ █ █▀█ █▀█ ▀█▀ █▀█ █▀▀ █  [0m
[32m   █  █ █ ▀▀█   █▀▄ █ █ █ █ █ █  █  █ █ █ █ ▀  [0m
[34m  ▀▀▀ ▀ ▀ ▀▀▀   ▀ ▀ ▀▀▀ ▀ ▀ ▀ ▀ ▀▀▀ ▀ ▀ ▀▀▀ ▀  [0m

`)

	listeningAddr <- addr
	close(listeningAddr)
	// The goroutine will hang here until the NotifyContext is done
	<-notifyCtx.Done()
	stop()
	slog.Error("Shutting down gracefully, press Ctrl+C again to force")

	// Tell the server to shut down, giving it this much time to do so gracefully.
	// Don't parent this ctx on the notifyCtx, because it's already done.
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = s.Shutdown(timeoutCtx)
	slog.Error("Server shut down", "err", err)
	stop()
	cancel()
	return 69
}

func configureLogger(imsCfg *conf.IMSConfig) {
	var logLevel slog.Level
	must(logLevel.UnmarshalText([]byte(imsCfg.Core.LogLevel)))
	logger := slog.New(
		log.NewHandler(
			&slog.HandlerOptions{Level: logLevel},
		),
	)
	slog.SetDefault(logger)
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
func mustInitConfig(envFileName string) *conf.IMSConfig {
	newCfg := conf.DefaultIMS()
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
		newCfg.Core.Host = v
	}
	if v, ok := lookupEnv("IMS_PORT"); ok {
		newCfg.Core.Port, err = conv.ParseInt32(v)
		must(err)
	}
	if v, ok := lookupEnv("IMS_DEPLOYMENT"); ok {
		newCfg.Core.Deployment = conf.DeploymentType(strings.ToLower(v))
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
	if v, ok := lookupEnv("IMS_DB_STORE_TYPE"); ok {
		newCfg.Store.Type = conf.DBStoreType(strings.ToLower(v))
	}
	if v, ok := lookupEnv("IMS_DB_HOST_NAME"); ok {
		newCfg.Store.MariaDB.HostName = v
	}
	if v, ok := lookupEnv("IMS_DB_HOST_PORT"); ok {
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
	// These three AWS env vars use the standard names, hence no "IMS_" prefix.
	if v, ok := lookupEnv("AWS_ACCESS_KEY_ID"); ok {
		newCfg.AttachmentsStore.S3.AWSAccessKeyID = v
	}
	if v, ok := lookupEnv("AWS_SECRET_ACCESS_KEY"); ok {
		newCfg.AttachmentsStore.S3.AWSSecretAccessKey = v
	}
	if v, ok := lookupEnv("AWS_REGION"); ok {
		newCfg.AttachmentsStore.S3.AWSRegion = v
	}

	if v, ok := lookupEnv("IMS_ATTACHMENTS_S3_BUCKET"); ok {
		newCfg.AttachmentsStore.S3.Bucket = v
	}
	if v, ok := lookupEnv("IMS_ATTACHMENTS_S3_COMMON_KEY_PREFIX"); ok {
		newCfg.AttachmentsStore.S3.CommonKeyPrefix = v
	}

	return newCfg
}

var (
	envFilename string
	printConfig bool
)

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVar(&envFilename, envfileFlagName, envFileDefaultName,
		"An env file from which to load IMS server configuration. "+
			"Defaults to '.env' in the current directory")
	serveCmd.Flags().BoolVar(&printConfig, printConfigFlagName, true,
		"Whether to print the redacted IMSConfig on server startup")
}

// must logs an error and panics. This should only be done for
// startup errors, not after the server is up and running.
func must(err error) {
	if err != nil {
		panic("got a startup error: " + err.Error())
	}
}

func stderrPrintf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format, args...)
}
