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
	"github.com/burningmantech/ranger-ims-go/api"
	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/directory"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/web"
	"github.com/spf13/cobra"
	"log"
	"log/slog"
	"net/http"
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

	mux := http.NewServeMux()
	api.AddToMux(mux, imsCfg, &store.DB{DB: imsDB}, userStore)
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
	slog.Info("IMS server up-and-running", "address", addr)
	log.Fatal(s.ListenAndServe())
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// serveCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// serveCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
