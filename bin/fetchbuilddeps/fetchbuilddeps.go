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
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
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
	defer logClose(repoRoot)

	staticExtRoot := mustMakeSubRoot(repoRoot, filepath.Join("web", "static", "ext"))
	defer logClose(staticExtRoot)

	g, groupCtx := errgroup.WithContext(ctx)

	jqueryVersion := "3.7.1"
	{
		g.Go(existOrFetch(groupCtx, staticExtRoot,
			"jquery.min.js",
			"https://code.jquery.com/jquery-"+jqueryVersion+".min.js",
			"sha256-/JqT3SQfawRcv/BIHPThkBvs0OEvtFFmqPF/lYI/Cxo=",
		))
	}
	bootstrapVersion := "5.3.8"
	{
		bootstrapDir := mustMakeSubRoot(staticExtRoot, "bootstrap")
		defer logClose(bootstrapDir)
		g.Go(existOrFetch(groupCtx, bootstrapDir,
			"bootstrap.min.css",
			"https://cdn.jsdelivr.net/npm/bootstrap@"+bootstrapVersion+"/dist/css/bootstrap.min.css",
			"sha384-sRIl4kxILFvY47J16cr9ZwB07vP4J8+LH7qKQnuqkuIAvNWLzeN8tE5YBujZqJLB",
		))
		g.Go(existOrFetch(groupCtx, bootstrapDir,
			"bootstrap.bundle.min.js",
			"https://cdn.jsdelivr.net/npm/bootstrap@"+bootstrapVersion+"/dist/js/bootstrap.bundle.min.js",
			"sha384-FKyoEForCGlyvwx9Hj09JcYn3nv7wiPVlz7YYwJrWVcXK/BmnVDxM+D2scQbITxI",
		))
	}
	datatablesVersion := "2.3.5"
	{
		datatablesDir := mustMakeSubRoot(staticExtRoot, "datatables")
		defer logClose(datatablesDir)
		g.Go(existOrFetch(groupCtx, datatablesDir,
			"dataTables.min.js",
			"https://cdn.datatables.net/"+datatablesVersion+"/js/dataTables.min.js",
			"sha384-VQb2IR8f6y3bNbMe6kK6H+edzCXdt7Z/3GtWA7zYzXcvfwYRR5rHGl46q28FbtsY",
		))
		g.Go(existOrFetch(groupCtx, datatablesDir,
			"dataTables.bootstrap5.min.js",
			"https://cdn.datatables.net/"+datatablesVersion+"/js/dataTables.bootstrap5.min.js",
			"sha384-3BApNGXgbm9rg2kjIbaEVprAGb2B0n9QyLjBrH090WdkzZ3IiUv8RZoTh5uP8oWH",
		))
		g.Go(existOrFetch(groupCtx, datatablesDir,
			"dataTables.bootstrap5.min.css",
			"https://cdn.datatables.net/"+datatablesVersion+"/css/dataTables.bootstrap5.min.css",
			"sha384-zmMNeKbOwzvUmxN8Z/VoYM+i+cwyC14+U9lq4+ZL0Ro7p1GMoh8uq8/HvIBgnh9+",
		))
	}
	flatpickrVersion := "4.6.13"
	{
		flatpickrDir := mustMakeSubRoot(staticExtRoot, "flatpickr")
		defer logClose(flatpickrDir)
		g.Go(existOrFetch(groupCtx, flatpickrDir,
			"flatpickr.min.css",
			"https://cdn.jsdelivr.net/npm/flatpickr@"+flatpickrVersion+"/dist/flatpickr.min.css",
			"sha384-RkASv+6KfBMW9eknReJIJ6b3UnjKOKC5bOUaNgIY778NFbQ8MtWq9Lr/khUgqtTt",
		))
		g.Go(existOrFetch(groupCtx, flatpickrDir,
			"flatpickr.min.js",
			"https://cdn.jsdelivr.net/npm/flatpickr@"+flatpickrVersion+"/dist/flatpickr.min.js",
			"sha384-5JqMv4L/Xa0hfvtF06qboNdhvuYXUku9ZrhZh3bSk8VXF0A/RuSLHpLsSV9Zqhl6",
		))
		// g.Go(existOrFetch(groupCtx, flatpickrDir,
		//	"globals.d.ts",
		//	"https://cdn.jsdelivr.net/npm/flatpickr@"+flatpickrVersion+"/dist/types/globals.d.ts",
		//	"sha384-TA24HjVnPU8dxXoGvs7n1PyoQFFF/ppuH52u2iMLnWbfGeoOSxGZRAOF3uEr6cOE",
		// ))
		// g.Go(existOrFetch(groupCtx, flatpickrDir,
		//	"instance.d.ts",
		//	"https://cdn.jsdelivr.net/npm/flatpickr@"+flatpickrVersion+"/dist/types/instance.d.ts",
		//	"sha384-dLko9/zDaIIaqzd0pjE6fh1kcR1N5+5H2otCGvOJYgLP6g0hdNwjnaZJSWzMFLVT",
		// ))
		// g.Go(existOrFetch(groupCtx, flatpickrDir,
		//	"locale.d.ts",
		//	"https://cdn.jsdelivr.net/npm/flatpickr@"+flatpickrVersion+"/dist/types/locale.d.ts",
		//	"sha384-jhXhYDt7UlyY46rKLm9W7Lua+EZAIX/p1aGWTFrxiUATcsK3y9kr+jaZAdWeVrTJ",
		// ))
		// g.Go(existOrFetch(groupCtx, flatpickrDir,
		//	"options.d.ts",
		//	"https://cdn.jsdelivr.net/npm/flatpickr@"+flatpickrVersion+"/dist/types/options.d.ts",
		//	"sha384-OTMArpvPXF1868803gfjh2/UTzeTM6LkCPybyzqE9rgL570U8MjHEn/cGPcUKxO9",
		// ))
	}

	must(g.Wait())
}

func mustMakeSubRoot(root *os.Root, subdir string) *os.Root {
	must(root.MkdirAll(subdir, 0750))
	subroot, err := root.OpenRoot(subdir)
	must(err)
	return subroot
}

func existOrFetch(ctx context.Context, dir *os.Root, basename, url, tegridy string) func() error {
	return func() error {
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
		log.Printf("Failed to close: %v", err)
	}
}
