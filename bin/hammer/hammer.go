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

// Command hammer creates Incidents, Field Reports, or Visits concurrently
// against a running IMS server, then checks that the server handed out record
// numbers correctly under contention.
//
// This exercises the number allocation race. A creation picks its number with a
// plain SELECT of MAX+1 and only then INSERTs, so concurrent creators in the
// same event can pick the same number; the (EVENT, NUMBER) primary key turns
// that into a duplicate-key error, which the server is expected to absorb by
// retrying with a fresh number.
//
// The run fails if any creation didn't return 201, if two creations came back
// with the same number, or if a created record can't be read back afterward. A
// 409 means the server exhausted its retry budget, which is a real (if
// graceful) user-facing failure.
//
// Every run writes real records to whatever server it's pointed at, and there's
// no cleanup, so point it at a scratch event. It refuses to run against a
// non-loopback host unless -allow-remote is given.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/burningmantech/ranger-ims-go/api"
	imsjson "github.com/burningmantech/ranger-ims-go/json"
	"github.com/burningmantech/ranger-ims-go/lib/conv"
	"golang.org/x/term"
)

var (
	hammerServerURL      string
	hammerEvent          string
	hammerIdentification string
	hammerPasswordStdin  bool
	hammerEntity         string
	hammerConcurrency    int
	hammerCount          int
	hammerTimeout        time.Duration
	hammerAllowRemote    bool
)

func main() {
	flag.StringVar(&hammerServerURL, "server_url", "http://localhost:8080",
		"The server URL and port of a running IMS server")
	flag.StringVar(&hammerEvent, "event", "",
		"The name of the event to create records in. Use a scratch event: this writes real records")
	flag.StringVar(&hammerIdentification, "identification", "",
		"The handle or email to log in as. Needs write permission on the event")
	flag.BoolVar(&hammerPasswordStdin, "password-stdin", false,
		"Read the password from stdin rather than prompting for it")
	flag.StringVar(&hammerEntity, "entity", "incidents",
		"What to create: incidents, field_reports, or visits")
	flag.IntVar(&hammerConcurrency, "concurrency", 16,
		"How many creations to have in flight at once")
	flag.IntVar(&hammerCount, "count", 200,
		"How many records to create in total")
	flag.DurationVar(&hammerTimeout, "timeout", 30*time.Second,
		"Per-request timeout")
	flag.BoolVar(&hammerAllowRemote, "allow-remote", false,
		"Permit hammering a host other than localhost. This writes real records there")

	flag.Usage = func() {
		out := flag.CommandLine.Output()
		_, _ = fmt.Fprintf(out,
			"Create Incidents, Field Reports, or Visits concurrently against a running IMS server.\n\n"+
				"This exercises the number allocation race. A creation picks its number with a plain\n"+
				"SELECT of MAX+1 and only then INSERTs, so concurrent creators in the same event can\n"+
				"pick the same number; the (EVENT, NUMBER) primary key turns that into a duplicate-key\n"+
				"error, which the server is expected to absorb by retrying with a fresh number.\n\n"+
				"The run fails if any creation didn't return 201, if two creations came back with the\n"+
				"same number, or if a created record can't be read back afterward. A 409 means the\n"+
				"server exhausted its retry budget, which is a real (if graceful) user-facing failure.\n\n"+
				"Every run writes real records to whatever server it's pointed at, and there's no\n"+
				"cleanup, so point it at a scratch event. It refuses to run against a non-loopback\n"+
				"host unless -allow-remote is given.\n\n"+
				"Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	err := run(context.Background())
	if err != nil {
		stderrPrintf("%v\n", err)
		os.Exit(1)
	}
}

// hammerEntityKind describes one creatable entity: the last path segment of
// its event-scoped collection, the response header carrying the new record's
// number, and a minimal creation body.
//
// The bodies are deliberately minimal. Anything richer (incident types,
// attached Rangers) would make the tool depend on what's seeded in the target
// deployment, and none of it affects the number allocation being tested.
type hammerEntityKind struct {
	name         string
	numberHeader string
	body         func(event string, i int) any
}

func hammerKind(name string) (hammerEntityKind, error) {
	switch name {
	case "incidents":
		return hammerEntityKind{
			name:         "incidents",
			numberHeader: "IMS-Incident-Number",
			body: func(event string, i int) any {
				return imsjson.Incident{
					Event:    event,
					State:    "new",
					Priority: imsjson.IncidentPriorityNormal,
					Summary:  hammerLabel(i),
				}
			},
		}, nil
	case "field_reports":
		return hammerEntityKind{
			name:         "field_reports",
			numberHeader: "IMS-Field-Report-Number",
			body: func(event string, i int) any {
				return imsjson.FieldReport{
					Event:   event,
					Summary: hammerLabel(i),
				}
			},
		}, nil
	case "visits":
		return hammerEntityKind{
			name:         "visits",
			numberHeader: "IMS-Visit-Number",
			body: func(event string, i int) any {
				return imsjson.Visit{
					Event:              event,
					GuestPreferredName: hammerLabel(i),
				}
			},
		}, nil
	}
	return hammerEntityKind{}, fmt.Errorf(
		"unknown -entity %q: want incidents, field_reports, or visits", name)
}

// hammerLabel names a created record, so that the debris a run leaves behind is
// recognizable in the UI.
func hammerLabel(i int) *string {
	label := fmt.Sprintf("[hammer] test record %d of %d", i+1, hammerCount)
	return &label
}

// hammerResult is one creation attempt's outcome.
type hammerResult struct {
	number   int32
	status   int
	duration time.Duration
	err      error
}

func run(ctx context.Context) error {
	kind, err := hammerKind(hammerEntity)
	if err != nil {
		return err
	}
	if hammerEvent == "" {
		return errors.New("-event is required")
	}
	if hammerIdentification == "" {
		return errors.New("-identification is required")
	}
	if hammerConcurrency < 1 {
		return errors.New("-concurrency must be at least 1")
	}
	if hammerCount < 1 {
		return errors.New("-count must be at least 1")
	}
	base, err := url.Parse(hammerServerURL)
	if err != nil {
		return fmt.Errorf("invalid -server_url: %w", err)
	}
	err = hammerCheckTarget(base)
	if err != nil {
		return err
	}

	password, err := readLoginPassword(hammerPasswordStdin)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: hammerTimeout,
		// Without this, the pool's default of 2 idle connections per host means
		// most workers dial a fresh connection for every request, which adds
		// latency noise and hides the contention we're trying to observe.
		Transport: &http.Transport{
			MaxIdleConnsPerHost: hammerConcurrency,
			MaxConnsPerHost:     hammerConcurrency,
		},
	}
	token, err := hammerAuthenticate(ctx, client, base, password)
	if err != nil {
		return err
	}

	stdoutPrintf("Creating %d %s in event %q, %d at a time, against %v\n",
		hammerCount, kind.name, hammerEvent, hammerConcurrency, base)

	results, elapsed := hammerRun(ctx, client, base, token, kind)
	return hammerReport(ctx, client, base, token, kind, results, elapsed)
}

// hammerCheckTarget refuses to point the hammer at a remote host by accident,
// since every run writes real records to whatever it hits.
func hammerCheckTarget(target *url.URL) error {
	if hammerAllowRemote {
		return nil
	}
	host := target.Hostname()
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("refusing to hammer non-loopback host %q, since the run would write "+
		"real records there. Pass -allow-remote if that's a test deployment", host)
}

// hammerAPIURL builds an API URL under the target server, escaping each path
// element (the event name is user-supplied).
func hammerAPIURL(base *url.URL, elems ...string) string {
	u := *base
	u.Path = path.Join(append([]string{base.Path}, elems...)...)
	u.RawPath = ""
	return u.String()
}

func hammerAuthenticate(ctx context.Context, client *http.Client, base *url.URL, password string) (string, error) {
	// #nosec G117 // The password has to be sent to the login endpoint.
	body, err := json.Marshal(api.PostAuthRequest{
		Identification: hammerIdentification,
		Password:       password,
	})
	if err != nil {
		return "", fmt.Errorf("[json.Marshal]: %w", err)
	}
	authURL := hammerAPIURL(base, "ims", "api", "auth")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("[http.NewRequestWithContext]: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// #nosec G704 // SSRF via taint analysis. The operator names the target.
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to reach %v: %w", authURL, err)
	}
	respBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("[io.ReadAll]: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed with %v: %v", resp.Status, strings.TrimSpace(string(respBody)))
	}
	var authResp api.PostAuthResponse
	err = json.Unmarshal(respBody, &authResp)
	if err != nil {
		return "", fmt.Errorf("[json.Unmarshal]: %w", err)
	}
	if authResp.Token == "" {
		return "", errors.New("login succeeded but returned no token")
	}
	return authResp.Token, nil
}

// hammerRun performs the creations, releasing every worker at once so that the
// first burst of requests races for the same number.
func hammerRun(
	ctx context.Context, client *http.Client, base *url.URL, token string, kind hammerEntityKind,
) ([]hammerResult, time.Duration) {
	results := make([]hammerResult, hammerCount)
	createURL := hammerAPIURL(base, "ims", "api", "events", hammerEvent, kind.name)

	var next atomic.Int64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range hammerConcurrency {
		wg.Go(func() {
			<-start
			for {
				i := int(next.Add(1)) - 1
				if i >= hammerCount {
					return
				}
				results[i] = hammerCreateOne(ctx, client, createURL, token, kind, i)
			}
		})
	}

	began := time.Now()
	close(start)
	wg.Wait()
	return results, time.Since(began)
}

func hammerCreateOne(
	ctx context.Context, client *http.Client, createURL, token string, kind hammerEntityKind, i int,
) hammerResult {
	body, err := json.Marshal(kind.body(hammerEvent, i))
	if err != nil {
		return hammerResult{err: fmt.Errorf("[json.Marshal]: %w", err)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewReader(body))
	if err != nil {
		return hammerResult{err: fmt.Errorf("[http.NewRequestWithContext]: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	began := time.Now()
	// #nosec G704 // SSRF via taint analysis. The operator names the target.
	resp, err := client.Do(req)
	took := time.Since(began)
	if err != nil {
		return hammerResult{duration: took, err: err}
	}
	// Drain the body, so the connection can be reused by the next creation.
	respBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	result := hammerResult{status: resp.StatusCode, duration: took}
	if resp.StatusCode != http.StatusCreated {
		result.err = fmt.Errorf("%v: %v", resp.Status, strings.TrimSpace(string(respBody)))
		return result
	}
	number, err := conv.ParseInt32(resp.Header.Get(kind.numberHeader))
	if err != nil {
		result.err = fmt.Errorf("created, but %v header was %q: %w",
			kind.numberHeader, resp.Header.Get(kind.numberHeader), err)
		return result
	}
	result.number = number
	return result
}

// hammerReport prints the run's outcome and returns an error if the server
// mishandled the contention.
func hammerReport(
	ctx context.Context, client *http.Client, base *url.URL,
	token string, kind hammerEntityKind, results []hammerResult, elapsed time.Duration,
) error {
	created := make(map[int32]int, len(results))
	byStatus := make(map[int]int)
	durations := make([]time.Duration, 0, len(results))
	var failures []string
	for i, result := range results {
		byStatus[result.status]++
		if result.duration > 0 {
			durations = append(durations, result.duration)
		}
		if result.err != nil {
			if len(failures) < 10 {
				failures = append(failures, fmt.Sprintf("  request %d: %v", i+1, result.err))
			}
			continue
		}
		created[result.number]++
	}
	slices.Sort(durations)

	succeeded := 0
	for _, count := range created {
		succeeded += count
	}
	perSecond := float64(len(results)) / max(elapsed.Seconds(), 0.000001)
	stdoutPrintf("\n  created:    %d of %d\n", succeeded, len(results))
	stdoutPrintf("  elapsed:    %v (%.1f creations/sec)\n", elapsed.Round(time.Millisecond), perSecond)
	if len(durations) > 0 {
		stdoutPrintf("  latency:    p50 %v, p95 %v, max %v\n",
			hammerPercentile(durations, 0.50).Round(time.Millisecond),
			hammerPercentile(durations, 0.95).Round(time.Millisecond),
			durations[len(durations)-1].Round(time.Millisecond))
	}
	for _, status := range slices.Sorted(maps.Keys(byStatus)) {
		if status == http.StatusCreated {
			continue
		}
		label := fmt.Sprintf("HTTP %d", status)
		if status == 0 {
			label = "no response"
		}
		stdoutPrintf("  %-11v %d\n", label+":", byStatus[status])
	}

	var duplicates []int32
	numbers := make([]int32, 0, len(created))
	for number, count := range created {
		numbers = append(numbers, number)
		if count > 1 {
			duplicates = append(duplicates, number)
		}
	}
	slices.Sort(numbers)
	slices.Sort(duplicates)
	if len(numbers) > 0 {
		stdoutPrintf("  numbers:    %d distinct, %d through %d\n",
			len(numbers), numbers[0], numbers[len(numbers)-1])
	}

	missing, err := hammerReadBack(ctx, client, base, token, kind, numbers)
	if err != nil {
		return err
	}
	stdoutPrintf("  read back:  %d of %d found on the server\n", len(numbers)-len(missing), len(numbers))

	if len(failures) > 0 {
		stdoutPrintf("\nFirst failures:\n%v\n", strings.Join(failures, "\n"))
	}

	var problems []string
	if succeeded != len(results) {
		problems = append(problems, fmt.Sprintf(
			"%d of %d creations failed", len(results)-succeeded, len(results)))
	}
	if conflicts := byStatus[http.StatusConflict]; conflicts > 0 {
		problems = append(problems, fmt.Sprintf(
			"%d creations got HTTP 409, meaning the server ran out of number-allocation retries",
			conflicts))
	}
	if len(duplicates) > 0 {
		problems = append(problems, fmt.Sprintf(
			"the server handed out these numbers more than once: %v", duplicates))
	}
	if len(missing) > 0 {
		problems = append(problems, fmt.Sprintf(
			"these numbers were returned by a creation but aren't on the server: %v", missing))
	}
	if len(problems) > 0 {
		return fmt.Errorf("hammer found problems:\n  - %v", strings.Join(problems, "\n  - "))
	}
	stdoutPrintf("\nOK: every creation got a distinct number, and all of them read back.\n")
	return nil
}

// hammerReadBack lists the event's records and reports which of the created
// numbers aren't actually there. A number that a creation returned but that no
// longer exists would mean a later creation overwrote it.
func hammerReadBack(
	ctx context.Context, client *http.Client, base *url.URL,
	token string, kind hammerEntityKind, numbers []int32,
) ([]int32, error) {
	listURL := hammerAPIURL(base, "ims", "api", "events", hammerEvent, kind.name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("[http.NewRequestWithContext]: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	// #nosec G704 // SSRF via taint analysis. The operator names the target.
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list %v: %w", kind.name, err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("[io.ReadAll]: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list %v with %v: %v",
			kind.name, resp.Status, strings.TrimSpace(string(body)))
	}
	// Only the numbers matter here, and all three entity types spell that field
	// the same way, so this avoids decoding three different response types.
	var listed []struct {
		Number int32 `json:"number"`
	}
	err = json.Unmarshal(body, &listed)
	if err != nil {
		return nil, fmt.Errorf("[json.Unmarshal]: %w", err)
	}
	onServer := make(map[int32]bool, len(listed))
	for _, record := range listed {
		onServer[record.Number] = true
	}
	var missing []int32
	for _, number := range numbers {
		if !onServer[number] {
			missing = append(missing, number)
		}
	}
	return missing, nil
}

// hammerPercentile returns the pth percentile of sorted, which must be sorted
// and non-empty.
func hammerPercentile(sorted []time.Duration, p float64) time.Duration {
	i := int(p * float64(len(sorted)))
	return sorted[min(i, len(sorted)-1)]
}

// readLoginPassword reads the password to log in with, either from stdin or
// from an interactive prompt. Unlike add-user's reader, this doesn't ask for
// confirmation, since a wrong password just fails the login.
func readLoginPassword(fromStdin bool) (string, error) {
	if fromStdin {
		password, err := bufio.NewReader(os.Stdin).ReadString('\n')
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
	password, err := term.ReadPassword(syscall.Stdin)
	stderrPrintf("\n")
	if err != nil {
		return "", fmt.Errorf("failed to read password (use -password-stdin if not on a terminal): %w", err)
	}
	if len(password) == 0 {
		return "", errors.New("empty password provided")
	}
	return string(password), nil
}

func stdoutPrintf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stdout, format, args...)
}

func stderrPrintf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format, args...)
}
