package main

import (
	"context"
	"fmt"
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

	must(existOrFetch(ctx, root,
		"bootstrap.min.css",
		"https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css"),
	)
	must(existOrFetch(ctx, root,
		"bootstrap.bundle.min.js",
		"https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/js/bootstrap.bundle.min.js"),
	)
	must(existOrFetch(ctx, root,
		"jquery.min.js",
		"https://code.jquery.com/jquery-3.1.0.min.js"),
	)
	must(existOrFetch(ctx, root,
		"dataTables.min.js",
		"https://cdn.datatables.net/2.2.2/js/dataTables.min.js"),
	)
	must(existOrFetch(ctx, root,
		"dataTables.bootstrap5.min.js",
		"https://cdn.datatables.net/2.2.2/js/dataTables.bootstrap5.min.js"),
	)
	must(existOrFetch(ctx, root,
		"dataTables.bootstrap5.min.css",
		"https://cdn.datatables.net/2.2.2/css/dataTables.bootstrap5.min.css"),
	)
}

func existOrFetch(ctx context.Context, dir *os.Root, basename string, url string) error {
	if pathExists(dir.Stat(basename)) {
		// File already exists. No need to download again.
		log.Printf("No need to re-fetch: %v", basename)
		return nil
	}

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
