package cmd

import (
	"errors"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/srabraham/ranger-ims-go/conf"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "ranger-ims-go",
	Short: "The Ranger IMS server",
	Long:  "The Black Rock Ranger IMS server",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "conf/imsd.toml", "config file")
}

// initConfig reads in the .env file and ENV variables if set.
func initConfig() {
	newCfg := conf.DefaultIMS()
	err := godotenv.Load()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Info("No .env file found. Carrying on with IMSConfig defaults and environment variable overrides")
		} else {
			log.Fatal("Error loading .env file: " + err.Error())
		}
	}
	if v, ok := os.LookupEnv("IMS_HOSTNAME"); ok {
		newCfg.Core.Host = v
	}
	if v, ok := os.LookupEnv("IMS_PORT"); ok {
		num, err := strconv.ParseInt(v, 10, 32)
		must(err)
		newCfg.Core.Port = int32(num)
	}
	if v, ok := os.LookupEnv("IMS_DEPLOYMENT"); ok {
		newCfg.Core.Deployment = strings.ToLower(v)
	}
	if v, ok := os.LookupEnv("IMS_TOKEN_LIFETIME"); ok {
		seconds, err := strconv.ParseInt(v, 10, 64)
		must(err)
		newCfg.Core.TokenLifetime = time.Duration(seconds) * time.Second
	}
	if v, ok := os.LookupEnv("IMS_LOG_LEVEL"); ok {
		newCfg.Core.LogLevel = v
	}
	if v, ok := os.LookupEnv("IMS_DIRECTORY"); ok {
		newCfg.Directory.Directory = conf.DirectoryType(strings.ToLower(v))
	}
	if v, ok := os.LookupEnv("IMS_ADMINS"); ok {
		newCfg.Core.Admins = strings.Split(v, ",")
	}
	if v, ok := os.LookupEnv("IMS_JWT_SECRET"); ok {
		newCfg.Core.JWTSecret = v
	}
	if v, ok := os.LookupEnv("IMS_DB_HOST_NAME"); ok {
		newCfg.Store.MySQL.HostName = v
	}
	if v, ok := os.LookupEnv("IMS_DB_HOST_POST"); ok {
		num, err := strconv.ParseInt(v, 10, 32)
		must(err)
		newCfg.Store.MySQL.HostPort = int32(num)
	}
	if v, ok := os.LookupEnv("IMS_DB_DATABASE"); ok {
		newCfg.Store.MySQL.Database = v
	}
	if v, ok := os.LookupEnv("IMS_DB_USER_NAME"); ok {
		newCfg.Store.MySQL.Username = v
	}
	if v, ok := os.LookupEnv("IMS_DB_PASSWORD"); ok {
		newCfg.Store.MySQL.Password = v
	}
	if v, ok := os.LookupEnv("IMS_DMS_HOSTNAME"); ok {
		newCfg.Directory.ClubhouseDB.Hostname = v
	}
	if v, ok := os.LookupEnv("IMS_DMS_DATABASE"); ok {
		newCfg.Directory.ClubhouseDB.Database = v
	}
	if v, ok := os.LookupEnv("IMS_DMS_USERNAME"); ok {
		newCfg.Directory.ClubhouseDB.Username = v
	}
	if v, ok := os.LookupEnv("IMS_DMS_PASSWORD"); ok {
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

func must(err error) {
	if err != nil {
		log.Panic(err)
	}
}
