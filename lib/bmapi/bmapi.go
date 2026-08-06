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

// Package bmapi is a small client for the public Burning Man API
// (https://api.burningman.org), which is where IMS gets the camp, art, and
// mutant vehicle data behind an Event's Places.
package bmapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Kind is a Burning Man API resource that maps onto an IMS place type. The
// values are the API's own path segments, e.g. "/api/camp".
type Kind string

const (
	KindArt  Kind = "art"
	KindCamp Kind = "camp"
	KindMV   Kind = "mv"
)

// maxResponseBytes caps how much we'll read from the API. A year of camps is
// on the order of a megabyte, so this leaves a lot of headroom while still
// keeping a misbehaving upstream from filling our memory.
const maxResponseBytes = 64 << 20

// requestTimeout bounds a single call to the API. These are large,
// uncached-looking responses, so it's generous.
const requestTimeout = 2 * time.Minute

// Record is one camp, art piece, or mutant vehicle. The fields IMS needs are
// pulled out, and Raw keeps the object exactly as the API sent it, since that
// whole object is what IMS stores as a Place's external data.
type Record struct {
	Name           string
	LocationString string
	Raw            json.RawMessage
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

// Fetch returns every record of the given kind for the given year.
func (c *Client) Fetch(ctx context.Context, kind Kind, year int32) ([]Record, error) {
	u := fmt.Sprintf("%v/api/%v?%v", c.baseURL, kind,
		url.Values{"year": {strconv.FormatInt(int64(year), 10)}}.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("[NewRequestWithContext]: %w", err)
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("[Do]: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("[ReadAll]: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("burning man API returned %v: %v",
			resp.Status, summarize(body))
	}

	var raws []json.RawMessage
	err = json.Unmarshal(body, &raws)
	if err != nil {
		return nil, fmt.Errorf("[Unmarshal]: burning man API returned "+
			"something other than a JSON array (%v): %w", summarize(body), err)
	}

	records := make([]Record, 0, len(raws))
	for _, raw := range raws {
		var fields struct {
			Name           string `json:"name"`
			LocationString string `json:"location_string"`
		}
		err = json.Unmarshal(raw, &fields)
		if err != nil {
			return nil, fmt.Errorf("[Unmarshal]: %w", err)
		}
		records = append(records, Record{
			Name:           fields.Name,
			LocationString: fields.LocationString,
			Raw:            raw,
		})
	}
	return records, nil
}

// ParseKind maps an IMS place type onto the API resource that provides it.
func ParseKind(placeType string) (Kind, error) {
	switch Kind(placeType) {
	case KindArt, KindCamp, KindMV:
		return Kind(placeType), nil
	default:
		return "", errors.New("no Burning Man API data for place type " + placeType)
	}
}

// summarize trims a response body down to something safe to put in an error.
func summarize(body []byte) string {
	const limit = 200
	s := strings.TrimSpace(string(body))
	if len(s) > limit {
		s = s[:limit] + "…"
	}
	return s
}
