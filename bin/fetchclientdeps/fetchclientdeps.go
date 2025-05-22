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
	"errors"
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
	repoRoot := filepath.Dir(strings.TrimSpace(string(goModPathBytes)))
	if !pathExists(os.Stat(repoRoot)) {
		must(fmt.Errorf("repo root %v does not exist", repoRoot))
	}

	staticExtDir := filepath.Join(repoRoot, "web", "static", "ext")
	must(os.MkdirAll(staticExtDir, 0750))
	root, err := os.OpenRoot(staticExtDir)
	must(err)

	g, groupCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return existOrFetch(groupCtx, root,
			"bootstrap.min.css",
			"https://cdn.jsdelivr.net/npm/bootstrap@5.3.6/dist/css/bootstrap.min.css",
			"sha384-4Q6Gf2aSP4eDXB8Miphtr37CMZZQ5oXLH2yaXMJ2w8e2ZtHTl7GptT4jmndRuHDT",
		)
	})

	g.Go(func() error {
		return existOrFetch(groupCtx, root,
			"bootstrap.min.css",
			"https://cdn.jsdelivr.net/npm/bootstrap@5.3.6/dist/css/bootstrap.min.css",
			"sha384-4Q6Gf2aSP4eDXB8Miphtr37CMZZQ5oXLH2yaXMJ2w8e2ZtHTl7GptT4jmndRuHDT",
		)
	})

	g.Go(func() error {
		return existOrFetch(groupCtx, root,
			"bootstrap.bundle.min.js",
			"https://cdn.jsdelivr.net/npm/bootstrap@5.3.6/dist/js/bootstrap.bundle.min.js",
			"sha384-j1CDi7MgGQ12Z7Qab0qlWQ/Qqz24Gc6BM0thvEMVjHnfYGF0rmFCozFSxQBxwHKO",
		)
	})

	g.Go(func() error {
		return existOrFetch(groupCtx, root,
			"jquery.min.js",
			"https://code.jquery.com/jquery-3.7.1.min.js",
			"sha256-/JqT3SQfawRcv/BIHPThkBvs0OEvtFFmqPF/lYI/Cxo=",
		)
	})

	g.Go(func() error {
		return existOrFetch(groupCtx, root,
			"dataTables.min.js",
			"https://cdn.datatables.net/2.3.1/js/dataTables.min.js",
			"sha384-LiV1KhVIIiAY/+IrQtQib29gCaonfR5MgtWzPCTBVtEVJ7uYd0u8jFmf4xka4WVy",
		)
	})

	g.Go(func() error {
		return existOrFetch(groupCtx, root,
			"dataTables.bootstrap5.min.js",
			"https://cdn.datatables.net/2.3.1/js/dataTables.bootstrap5.min.js",
			"sha384-G85lmdZCo2WkHaZ8U1ZceHekzKcg37sFrs4St2+u/r2UtfvSDQmQrkMsEx4Cgv/W",
		)
	})

	g.Go(func() error {
		return existOrFetch(groupCtx, root,
			"dataTables.bootstrap5.min.css",
			"https://cdn.datatables.net/2.3.1/css/dataTables.bootstrap5.min.css",
			"sha384-5hBbs6yhVjtqKk08rsxdk9xO80wJES15HnXHglWBQoj3cus3WT+qDJRpvs5rRP2c",
		)
	})

	must(g.Wait())
}

func existOrFetch(ctx context.Context, dir *os.Root, basename, url, tegridy string) error {
	if pathExists(dir.Stat(basename)) {
		// File already exists. No need to download again.
		log.Printf("No need to re-fetch: %v", basename)
	} else {
		log.Printf("Fetching %v", url)
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
		return errors.New("missing tegridy")
	}
	_, err = hasher.Write(b)
	must(err)
	hashStr := base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	if tegridy != hashStr {
		return fmt.Errorf("bad tegridy for %v. Wanted %v, got %v\n"+
			"If you just upgraded that dep, you'll need to delete the local version now "+
			"to pick up the new one", basename, tegridy, hashStr)
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
	if err := closer.Close(); err != nil {
		log.Printf("Failed to close connection: %v", err)
	}
}
