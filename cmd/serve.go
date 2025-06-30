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
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/actionlog"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/burningmantech/ranger-ims-go/web"
	"github.com/spf13/cobra"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
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
	baseCfg := conf.DefaultIMS()
	imsCfg := mustApplyEnvConfig(baseCfg, envFilename)
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
	userStore := directory.NewUserStore(clubhouseDBQ, imsCfg.Directory.InMemoryCacheTTL)

	var s3Client *attachment.S3Client
	if imsCfg.AttachmentsStore.Type == conf.AttachmentsStoreS3 {
		s3Client, err = attachment.NewS3Client(ctx)
		must(err)
	}

	imsDB, err := store.SqlDB(ctx, imsCfg.Store, true)
	must(err)
	imsDBQ := store.NewDBQ(imsDB, imsdb.New())
	actionLogger := actionlog.NewLogger(ctx, imsDBQ, imsCfg.Core.ActionLogEnabled, false)

	notifyCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)

	eventSource := api.NewEventSourcerer()
	mux := http.NewServeMux()
	api.AddToMux(mux, eventSource, imsCfg, imsDBQ, userStore, s3Client, actionLogger)
	web.AddToMux(mux, imsCfg)

	s := &http.Server{
		Handler:     mux,
		ReadTimeout: 1 * time.Minute,
		// This needs to be long to support long-lived EventSource calls.
		// After this duration, a client will be disconnected and forced
		// to reconnect.
		WriteTimeout:   30 * time.Minute,
		MaxHeaderBytes: 1 << 20,
	}
	s.RegisterOnShutdown(func() {
		actionLogger.Close()
		eventSource.Server.Close()
	})

	listener, err := net.Listen("tcp", net.JoinHostPort(imsCfg.Core.Host, conv.FormatInt(imsCfg.Core.Port)))
	must(err)
	addr := net.JoinHostPort(imsCfg.Core.Host, conv.FormatInt(listener.Addr().(*net.TCPAddr).Port))

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
	// The goroutine will block here until the NotifyContext is done
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
	// TODO: maybe bring back pretty logging for local use only
	// logger := slog.New(
	//	log.NewHandler(
	//		&slog.HandlerOptions{Level: logLevel},
	//	),
	//)
	// slog.SetDefault(logger)
	slog.SetLogLoggerLevel(logLevel)
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
