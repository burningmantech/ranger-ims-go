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

package main

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"golang.org/x/sync/errgroup"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var client = &http.Client{
	Timeout: 10 * time.Second,
}

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	// The GOMOD variable gives an absolute path to go.mod, which we use to find
	// the repo root directory
	cmd := exec.Command("go", "env", "GOMOD")
	goModPathBytes, err := cmd.CombinedOutput()
	must(err)
	repoRoot, err := os.OpenRoot(filepath.Dir(strings.TrimSpace(string(goModPathBytes))))
	must(err)

	staticExtDir := filepath.Join("web", "static", "ext")
	err = repoRoot.MkdirAll(staticExtDir, 0750)
	must(err)

	staticExtRoot, err := repoRoot.OpenRoot(staticExtDir)
	must(err)

	g, groupCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return existOrFetch(groupCtx, staticExtRoot,
			"bootstrap.min.css",
			"https://cdn.jsdelivr.net/npm/bootstrap@5.3.7/dist/css/bootstrap.min.css",
			"sha384-LN+7fdVzj6u52u30Kp6M/trliBMCMKTyK833zpbD+pXdCLuTusPj697FH4R/5mcr",
		)
	})

	g.Go(func() error {
		return existOrFetch(groupCtx, staticExtRoot,
			"bootstrap.bundle.min.js",
			"https://cdn.jsdelivr.net/npm/bootstrap@5.3.7/dist/js/bootstrap.bundle.min.js",
			"sha384-ndDqU0Gzau9qJ1lfW4pNLlhNTkCfHzAVBReH9diLvGRem5+R9g2FzA8ZGN954O5Q",
		)
	})

	g.Go(func() error {
		return existOrFetch(groupCtx, staticExtRoot,
			"jquery.min.js",
			"https://code.jquery.com/jquery-3.7.1.min.js",
			"sha256-/JqT3SQfawRcv/BIHPThkBvs0OEvtFFmqPF/lYI/Cxo=",
		)
	})

	g.Go(func() error {
		return existOrFetch(groupCtx, staticExtRoot,
			"dataTables.min.js",
			"https://cdn.datatables.net/2.3.2/js/dataTables.min.js",
			"sha384-RZEqG156bBQSxYY9lwjUz/nKVkqYj/QNK9dEjjyJ/EVTO7ndWwk6ZWEkvaKdRm/U",
		)
	})

	g.Go(func() error {
		return existOrFetch(groupCtx, staticExtRoot,
			"dataTables.bootstrap5.min.js",
			"https://cdn.datatables.net/2.3.2/js/dataTables.bootstrap5.min.js",
			"sha384-G85lmdZCo2WkHaZ8U1ZceHekzKcg37sFrs4St2+u/r2UtfvSDQmQrkMsEx4Cgv/W",
		)
	})

	g.Go(func() error {
		return existOrFetch(groupCtx, staticExtRoot,
			"dataTables.bootstrap5.min.css",
			"https://cdn.datatables.net/2.3.2/css/dataTables.bootstrap5.min.css",
			"sha384-e4/pU/7fdyaPKtXkqAgHNgoYAb2LNmChhpSuSp8o6saYtS2sP+JZsu8Wy/7mGV7w",
		)
	})

	must(g.Wait())
}

func existOrFetch(ctx context.Context, dir *os.Root, basename, url, tegridy string) error {
	if pathExists(dir.Stat(basename)) {
		err := checkIntegrity(dir, basename, tegridy)
		if err == nil {
			log.Printf("File already exists with expected hash: %v", basename)
			return nil
		} else {
			log.Printf("Will re-fetch file because it has the wrong hash: %v", basename)
		}
	}

	log.Printf("Fetching: %v", url)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("[NewRequestWithContext]: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("[Get]: %w", err)
	}
	defer logClose(resp.Body)

	f, err := dir.Create(basename)
	if err != nil {
		return fmt.Errorf("[Create]: %w", err)
	}
	defer logClose(resp.Body)

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("[Copy]: %w", err)
	}
	return checkIntegrity(dir, basename, tegridy)
}

func checkIntegrity(root *os.Root, basename, tegridy string) error {
	f, err := root.Open(basename)
	must(err)
	b, err := io.ReadAll(f)
	must(err)

	var hasher = sha256.New()
	switch {
	case strings.HasPrefix(tegridy, "sha256-"):
		hasher = sha256.New()
		tegridy = strings.TrimPrefix(tegridy, "sha256-")
	case strings.HasPrefix(tegridy, "sha384-"):
		hasher = sha512.New384()
		tegridy = strings.TrimPrefix(tegridy, "sha384-")
	case strings.HasPrefix(tegridy, "sha512-"):
		hasher = sha512.New()
		tegridy = strings.TrimPrefix(tegridy, "sha512-")
	case tegridy == "":
		return fmt.Errorf("missing hash name prefix: %v", tegridy)
	}
	_, err = hasher.Write(b)
	must(err)
	hashStr := base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	if tegridy != hashStr {
		return fmt.Errorf("bad hash for %v\nWanted %v, got %v\n", f.Name(), tegridy, hashStr)
	}
	return nil
}

func pathExists(_ os.FileInfo, err error) bool {
	return !os.IsNotExist(err)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func logClose(closer io.Closer) {
	err := closer.Close()
	if err != nil {
		log.Printf("Failed to close connection: %v", err)
	}
}
