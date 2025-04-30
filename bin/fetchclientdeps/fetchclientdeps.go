package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var client = &http.Client{
	Timeout: 10 * time.Second,
}

func main() {
	// The GOMOD variable gives an absolute path to go.mod, which we use to find
	// the repo root directory
	cmd := exec.Command("go", "env", "GOMOD")
	goModPathBytes, err := cmd.CombinedOutput()
	must(err)
	repoRoot := filepath.Dir(strings.TrimSpace(string(goModPathBytes)))
	if !pathExists(repoRoot) {
		log.Fatalf("Repo root %v does not exist", repoRoot)
	}

	staticExtDir := filepath.Join(repoRoot, "web", "static", "ext")
	must(os.MkdirAll(staticExtDir, 0755))

	must(existOrFetch(
		path.Join(staticExtDir, "bootstrap.min.css"),
		"https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css"),
	)
	must(existOrFetch(
		path.Join(staticExtDir, "bootstrap.bundle.min.js"),
		"https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/js/bootstrap.bundle.min.js"),
	)
	must(existOrFetch(
		path.Join(staticExtDir, "jquery.min.js"),
		"https://code.jquery.com/jquery-3.1.0.min.js"),
	)
	must(existOrFetch(
		path.Join(staticExtDir, "dataTables.min.js"),
		"https://cdn.datatables.net/2.2.2/js/dataTables.min.js"),
	)
	must(existOrFetch(
		path.Join(staticExtDir, "dataTables.bootstrap5.min.js"),
		"https://cdn.datatables.net/2.2.2/js/dataTables.bootstrap5.min.js"),
	)
	must(existOrFetch(
		path.Join(staticExtDir, "dataTables.bootstrap5.min.css"),
		"https://cdn.datatables.net/2.2.2/css/dataTables.bootstrap5.min.css"),
	)
}

func existOrFetch(dest, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("[Get]: %w", err)
	}
	defer resp.Body.Close()

	if pathExists(dest) {
		// File already exists. No need to download again.
		log.Printf("No need to re-fetch: %v", dest)
		return nil
	}
	log.Printf("Fetching %v", url)

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("[Create]: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return fmt.Errorf("[Copy]: %w", err)
	}
	return nil
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
