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

// Package ecsmetadata reads the ECS task metadata endpoint (version 4), which
// AWS provides to containers running on ECS, including on Fargate.
package ecsmetadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// EnvVar is set by the ECS agent to the container's metadata endpoint.
const EnvVar = "ECS_CONTAINER_METADATA_URI_V4"

// URI returns the metadata endpoint, or "" when not running on ECS.
func URI() string {
	return os.Getenv(EnvVar)
}

const (
	fetchTimeout = 5 * time.Second
	maxBodyBytes = 4 << 20
)

// All fetches every document the v4 endpoint offers and returns them as one
// human-readable string. A failed fetch is reported inline rather than
// aborting the rest.
func All(ctx context.Context, client *http.Client, uri string) string {
	sections := []struct{ title, path string }{
		{"Container metadata", ""},
		{"Task metadata", "/task"},
		{"Container stats", "/stats"},
		{"Task stats", "/task/stats"},
	}
	var buf bytes.Buffer
	for i, s := range sections {
		if i > 0 {
			buf.WriteString("\n\n")
		}
		url := uri + s.path
		heading := fmt.Sprintf("%v (%v)", s.title, url)
		_, _ = fmt.Fprintf(&buf, "%v\n%v\n\n", heading, strings.Repeat("=", len(heading)))
		buf.WriteString(fetch(ctx, client, url))
		buf.WriteString("\n")
	}
	return buf.String()
}

func fetch(ctx context.Context, client *http.Client, url string) string {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Sprintf("Failed to build request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("Failed to fetch: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Sprintf("Failed to read response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("Failed with status %v: %s", resp.Status, body)
	}
	var pretty bytes.Buffer
	err = json.Indent(&pretty, body, "", "  ")
	if err != nil {
		return string(body)
	}
	return pretty.String()
}
