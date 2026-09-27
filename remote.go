package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const defaultRemoteBaseURL = "https://raw.githubusercontent.com/kbairak/flagpick/main/config/"

var remoteBaseURL = defaultRemoteBaseURL
var httpClient = &http.Client{Timeout: 30 * time.Second}

const maxDownloads = 4

// md5Hex returns the lowercase hex MD5 digest of b.
func md5Hex(b []byte) string {
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])
}

// httpGet fetches url and returns the response body, erroring on any non-200.
func httpGet(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// fetchManifest downloads and parses the flat name→md5 manifest.
func fetchManifest(base string) (map[string]string, error) {
	b, err := httpGet(base + "manifest.json")
	if err != nil {
		return nil, err
	}
	var manifest map[string]string
	if err := json.Unmarshal(b, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return manifest, nil
}

// fetchConfig downloads the yaml config for name.
func fetchConfig(base, name string) ([]byte, error) {
	return httpGet(base + name + ".yaml")
}

// validCommandName reports whether name is safe to use as a filename.
func validCommandName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		if r == '/' || r == '\\' {
			return false
		}
	}
	return true
}

// pendingDownloads returns the sorted manifest names whose local config in the
// data directory is missing or whose MD5 does not match the manifest.
func pendingDownloads(dataDir string, manifest map[string]string) ([]string, error) {
	names := make([]string, 0, len(manifest))
	for name := range manifest {
		names = append(names, name)
	}
	sort.Strings(names)

	var pending []string
	for _, name := range names {
		if !validCommandName(name) {
			return nil, fmt.Errorf("invalid command name %q in manifest", name)
		}
		local, err := os.ReadFile(filepath.Join(dataDir, name+".yaml"))
		if err != nil {
			if os.IsNotExist(err) {
				pending = append(pending, name)
				continue
			}
			return nil, err
		}
		if md5Hex(local) != manifest[name] {
			pending = append(pending, name)
		}
	}
	return pending, nil
}

// downloadAll fetches every name concurrently and returns the name→bytes map.
// On any error it returns the first error and a nil map: the caller must
// discard all results.
func downloadAll(base string, names []string) (map[string][]byte, error) {
	jobs := make(chan string)
	results := make(chan struct {
		name string
		data []byte
		err  error
	}, len(names))

	workers := maxDownloads
	if len(names) < workers {
		workers = len(names)
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for name := range jobs {
				data, err := fetchConfig(base, name)
				results <- struct {
					name string
					data []byte
					err  error
				}{name, data, err}
			}
		}()
	}

	go func() {
		for _, name := range names {
			jobs <- name
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	out := make(map[string][]byte, len(names))
	var firstErr error
	for r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		out[r.name] = r.data
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// writeFileAtomic writes data to path via a .tmp file and rename.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// updateConfigs refreshes the data directory from the remote manifest. It is
// all-or-nothing: nothing is written unless every download succeeds.
func updateConfigs(base, dataDir string) error {
	manifest, err := fetchManifest(base)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	pending, err := pendingDownloads(dataDir, manifest)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		fmt.Println("configs up to date")
		return nil
	}
	downloaded, err := downloadAll(base, pending)
	if err != nil {
		return err
	}

	// Write every file to .tmp first, then rename. Clean up on failure.
	written := make([]string, 0, len(downloaded))
	names := make([]string, 0, len(downloaded))
	for name := range downloaded {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := filepath.Join(dataDir, name+".yaml")
		if err := writeFileAtomic(p, downloaded[name]); err != nil {
			for _, w := range written {
				os.Remove(filepath.Join(dataDir, w+".yaml.tmp"))
			}
			return err
		}
		written = append(written, name)
	}
	for _, name := range names {
		fmt.Println("updated " + name)
	}
	return nil
}
