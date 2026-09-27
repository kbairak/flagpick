package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const testConfigYAML = `pool:
  options:
    - name: x
      kind: flag
      type: bool
      long: --x
options: [x]
`

func TestMD5Hex(t *testing.T) {
	if got := md5Hex([]byte("abc")); got != "900150983cd24fb0d6963f7d28e17f72" {
		t.Fatalf("md5Hex(abc) = %q", got)
	}
}

func TestValidCommandName(t *testing.T) {
	valid := []string{"rg", "my-tool"}
	for _, n := range valid {
		if !validCommandName(n) {
			t.Errorf("validCommandName(%q) = false, want true", n)
		}
	}
	invalid := []string{"", ".", "..", "a/b", `a\b`}
	for _, n := range invalid {
		if validCommandName(n) {
			t.Errorf("validCommandName(%q) = true, want false", n)
		}
	}
}

func TestPendingDownloads(t *testing.T) {
	dir := t.TempDir()
	manifest := map[string]string{
		"rg": "900150983cd24fb0d6963f7d28e17f72", // md5("abc")
	}

	// (a) empty dir → pending.
	pending, err := pendingDownloads(dir, manifest)
	if err != nil {
		t.Fatalf("pendingDownloads: %v", err)
	}
	if len(pending) != 1 || pending[0] != "rg" {
		t.Fatalf("pending = %v, want [rg]", pending)
	}

	// (b) matching md5 → not pending.
	if err := os.WriteFile(filepath.Join(dir, "rg.yaml"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	pending, err = pendingDownloads(dir, manifest)
	if err != nil {
		t.Fatalf("pendingDownloads: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %v, want none", pending)
	}

	// (c) mismatched md5 → pending.
	if err := os.WriteFile(filepath.Join(dir, "rg.yaml"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	pending, err = pendingDownloads(dir, manifest)
	if err != nil {
		t.Fatalf("pendingDownloads: %v", err)
	}
	if len(pending) != 1 || pending[0] != "rg" {
		t.Fatalf("pending = %v, want [rg]", pending)
	}

	// (d) invalid manifest name → error.
	bad := map[string]string{"../evil": "x"}
	if _, err := pendingDownloads(dir, bad); err == nil {
		t.Fatal("expected error for invalid manifest name")
	}
}

func newUpdateServer(t *testing.T, body string) (*httptest.Server, *int) {
	t.Helper()
	sum := md5Hex([]byte(body))
	manifest := fmt.Sprintf("{\"rg\": %q}", sum)
	yamlHits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			fmt.Fprint(w, manifest)
		case "/rg.yaml":
			yamlHits++
			fmt.Fprint(w, body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &yamlHits
}

func TestUpdateConfigs(t *testing.T) {
	srv, _ := newUpdateServer(t, testConfigYAML)
	dir := t.TempDir()

	if err := updateConfigs(srv.URL+"/", dir); err != nil {
		t.Fatalf("updateConfigs: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "rg.yaml"))
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != testConfigYAML {
		t.Fatalf("downloaded = %q, want %q", got, testConfigYAML)
	}
	if _, err := parseConfigBytes(t, got); err != nil {
		t.Fatalf("downloaded config invalid: %v", err)
	}
}

func TestUpdateConfigsSkipsUpToDate(t *testing.T) {
	srv, hits := newUpdateServer(t, testConfigYAML)
	dir := t.TempDir()

	if err := updateConfigs(srv.URL+"/", dir); err != nil {
		t.Fatalf("first updateConfigs: %v", err)
	}
	if *hits != 1 {
		t.Fatalf("first call yaml hits = %d, want 1", *hits)
	}
	if err := updateConfigs(srv.URL+"/", dir); err != nil {
		t.Fatalf("second updateConfigs: %v", err)
	}
	if *hits != 1 {
		t.Fatalf("second call fetched yaml again: hits = %d, want 1", *hits)
	}
}

func TestUpdateConfigsDownloadFailureWritesNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			fmt.Fprint(w, `{"rg": "900150983cd24fb0d6963f7d28e17f72"}`)
		case "/rg.yaml":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := updateConfigs(srv.URL+"/", dir); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(filepath.Join(dir, "rg.yaml")); !os.IsNotExist(err) {
		t.Fatalf("rg.yaml exists after failure (err = %v)", err)
	}
}
