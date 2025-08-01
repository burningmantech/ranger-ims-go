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
	"github.com/burningmantech/ranger-ims-go/lib/argon2id"
	"github.com/spf13/cobra"
)

var hashPasswordCmd = &cobra.Command{
	Use:   "hash_password",
	Short: "Get a salted hash of a password",
	Long: "Get a salted hash of a password\n\n" +
		"The result will be of the form ${salt}:${hashedPassword}",
	Run: runHashPassword,
}

// password gets passed in as a flag.
var password string

func init() {
	rootCmd.AddCommand(hashPasswordCmd)

	hashPasswordCmd.Flags().StringVar(&password, "password", "", "The password to hash")
	_ = hashPasswordCmd.MarkFlagRequired("password")
}

func runHashPassword(cmd *cobra.Command, args []string) {
	fmt.Println(argon2id.CreateHash(password, argon2id.ClubhouseParams)) //nolint:forbidigo
}
