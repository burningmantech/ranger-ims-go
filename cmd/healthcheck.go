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
	"github.com/spf13/cobra"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var healthCheckCmd = &cobra.Command{
	Use:   "healthcheck",
	Short: "Perform a health check against an IMS server",
	Long:  "Perform a health check against an IMS server",
	Run:   runHealthCheck,
}

var serverURL string

// exit is overridable for tests.
var exit = os.Exit

func init() {
	rootCmd.AddCommand(healthCheckCmd)

	healthCheckCmd.Flags().StringVar(&serverURL, "server_url", "", "The server URL and port of an IMS server")
	_ = healthCheckCmd.MarkFlagRequired("server_url")
}

func runHealthCheck(cmd *cobra.Command, args []string) {
	client := http.Client{Timeout: time.Second * 5}

	pingURL, err := url.JoinPath(serverURL, "ims/api/ping")
	must(err)

	req, err := http.NewRequestWithContext(cmd.Context(), http.MethodGet, pingURL, nil)
	must(err)

	resp, err := client.Do(req)
	must(err)

	body, err := io.ReadAll(resp.Body)
	must(err)
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Println("wanted status code 200, got", resp.StatusCode) //nolint:forbidigo
		exit(5)
		return
	}
	if strings.TrimSpace(string(body)) != "ack" {
		fmt.Printf("wanted response of 'ack', got '%v'\n", string(body)) //nolint:forbidigo
		exit(6)
		return
	}
	fmt.Println("OK") //nolint:forbidigo
	exit(0)
	return
}
