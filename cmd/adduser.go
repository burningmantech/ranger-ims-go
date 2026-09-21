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
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"syscall"

	"github.com/burningmantech/ranger-ims-go/conf"
	"github.com/burningmantech/ranger-ims-go/lib/argon2id"
	"github.com/burningmantech/ranger-ims-go/store"
	"github.com/burningmantech/ranger-ims-go/store/imsdb"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var addUserCmd = &cobra.Command{
	Use:   "add-user",
	Short: "Add or update a user in the IMS-native directory",
	Long: "Add or update a user in the IMS-native directory\n\n" +
		"This is for deployments with IMS_DIRECTORY=ims, and it's mainly useful for\n" +
		"bootstrapping: a fresh deployment has no users, and the admin web UI requires\n" +
		"logging in as a user listed in IMS_ADMINS. Use this command to create that\n" +
		"first user, then add their handle to IMS_ADMINS.\n\n" +
		"If a user with the given handle already exists, their password (and email,\n" +
		"if provided) is updated, and the user is marked active.\n\n" +
		"The password is read from an interactive prompt, or from stdin\n" +
		"with --password-stdin.",
	RunE: runAddUser,
}

var (
	addUserEnvFilename   string
	addUserHandle        string
	addUserEmail         string
	addUserOnsite        bool
	addUserPasswordStdin bool
)

func init() {
	rootCmd.AddCommand(addUserCmd)

	addUserCmd.Flags().StringVar(&addUserEnvFilename, envfileFlagName, envFileDefaultName,
		"An env file from which to load IMS server configuration. "+
			"Defaults to '.env' in the current directory")
	addUserCmd.Flags().StringVar(&addUserHandle, "handle", "",
		"The user's handle, i.e. the name they use to log in")
	addUserCmd.Flags().StringVar(&addUserEmail, "email", "",
		"The user's email address (optional; may also be used to log in)")
	addUserCmd.Flags().BoolVar(&addUserOnsite, "onsite", false,
		"Mark the user as onsite (relevant only to 'onsite' validity access rules)")
	addUserCmd.Flags().BoolVar(&addUserPasswordStdin, "password-stdin", false,
		"Read the password from stdin rather than prompting for it")
	_ = addUserCmd.MarkFlagRequired("handle")
}

func runAddUser(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	imsCfg := mustApplyEnvConfig(conf.DefaultIMS(), addUserEnvFilename)
	err := checkAddUserConfig(imsCfg)
	if err != nil {
		return err
	}

	password, err := readPassword(addUserPasswordStdin, cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("[readPassword]: %w", err)
	}
	hashed := argon2id.CreateHash(password, argon2id.SecondRecommendedParams)

	imsDB, err := store.SqlDB(ctx, imsCfg.Store, true)
	if err != nil {
		return fmt.Errorf("[store.SqlDB]: %w", err)
	}
	defer func() { _ = imsDB.Close() }()
	imsDBQ := store.NewDBQ(imsDB, imsdb.New())

	var onsite *bool
	if cmd.Flags().Changed("onsite") {
		onsite = &addUserOnsite
	}
	created, err := upsertDirectoryUser(ctx, imsDBQ, addUserHandle, addUserEmail, onsite, hashed)
	if err != nil {
		return err
	}
	if created {
		cmd.Printf("Created user %v\n", addUserHandle)
	} else {
		cmd.Printf("Updated existing user %v\n", addUserHandle)
	}
	cmd.Printf("To make this user an IMS administrator, add %q to IMS_ADMINS\n", addUserHandle)
	return nil
}

func checkAddUserConfig(imsCfg *conf.IMSConfig) error {
	if imsCfg.Directory.Directory != conf.DirectoryTypeIMS {
		return fmt.Errorf("add-user manages the IMS-native directory, but this deployment's "+
			"IMS_DIRECTORY is %q. Set IMS_DIRECTORY=ims to use the IMS-native directory",
			imsCfg.Directory.Directory)
	}
	if imsCfg.Store.Type != conf.DBStoreTypeMaria {
		return fmt.Errorf("add-user requires a MariaDB IMS datastore, but this deployment's "+
			"store type is %q", imsCfg.Store.Type)
	}
	return nil
}

// upsertDirectoryUser creates the user with the given handle, or, if one already
// exists, reactivates them and resets their password. An empty email or a nil
// onsite leaves an existing user's value alone.
func upsertDirectoryUser(
	ctx context.Context, imsDBQ *store.DBQ,
	handle, email string, onsite *bool, hashedPassword string,
) (created bool, err error) {
	existing, err := imsDBQ.DirectoryPersonByHandle(ctx, imsDBQ, handle)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		_, err = imsDBQ.DirectoryCreatePerson(ctx, imsDBQ, imsdb.DirectoryCreatePersonParams{
			Handle:   handle,
			Email:    sql.NullString{String: email, Valid: email != ""},
			Password: hashedPassword,
			Active:   true,
			Onsite:   onsite != nil && *onsite,
		})
		if err != nil {
			return false, fmt.Errorf("[DirectoryCreatePerson]: %w", err)
		}
		return true, nil
	case err != nil:
		return false, fmt.Errorf("[DirectoryPersonByHandle]: %w", err)
	}

	newEmail := existing.Email
	if email != "" {
		newEmail = sql.NullString{String: email, Valid: true}
	}
	newOnsite := existing.Onsite
	if onsite != nil {
		newOnsite = *onsite
	}
	err = imsDBQ.DirectoryUpdatePerson(ctx, imsDBQ, imsdb.DirectoryUpdatePersonParams{
		Handle: existing.Handle,
		Email:  newEmail,
		Active: true,
		Onsite: newOnsite,
		ID:     existing.ID,
	})
	if err != nil {
		return false, fmt.Errorf("[DirectoryUpdatePerson]: %w", err)
	}
	err = imsDBQ.DirectorySetPersonPassword(ctx, imsDBQ, imsdb.DirectorySetPersonPasswordParams{
		Password: hashedPassword,
		ID:       existing.ID,
	})
	if err != nil {
		return false, fmt.Errorf("[DirectorySetPersonPassword]: %w", err)
	}
	return false, nil
}

func readPassword(fromStdin bool, stdin io.Reader) (string, error) {
	if fromStdin {
		password, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && password == "" {
			return "", fmt.Errorf("failed to read password from stdin: %w", err)
		}
		password = strings.TrimRight(password, "\r\n")
		if password == "" {
			return "", errors.New("empty password provided on stdin")
		}
		return password, nil
	}
	stderrPrintf("Password: ")
	first, err := term.ReadPassword(syscall.Stdin)
	stderrPrintf("\n")
	if err != nil {
		return "", fmt.Errorf("failed to read password (use --password-stdin if not on a terminal): %w", err)
	}
	stderrPrintf("Confirm password: ")
	second, err := term.ReadPassword(syscall.Stdin)
	stderrPrintf("\n")
	if err != nil {
		return "", fmt.Errorf("failed to read password confirmation: %w", err)
	}
	if string(first) != string(second) {
		return "", errors.New("passwords did not match")
	}
	if len(first) == 0 {
		return "", errors.New("empty password provided")
	}
	return string(first), nil
}
