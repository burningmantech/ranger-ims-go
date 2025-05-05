package cmd

import (
	"fmt"
	imspassword "github.com/burningmantech/ranger-ims-go/lib/authn"
	"github.com/spf13/cobra"
	"log"
	"os"
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
	_, err := fmt.Fprintln(os.Stdout, imspassword.NewSalted(password))
	if err != nil {
		log.Fatal(err)
	}
}
