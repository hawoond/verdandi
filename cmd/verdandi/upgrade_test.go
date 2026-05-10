package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestRunUpgradeCommandPrintsDryRunJSON(t *testing.T) {
	archiveName := "verdandi_1.2.3_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	if runtime.GOOS == "windows" {
		archiveName = "verdandi_1.2.3_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip"
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/hawoond/verdandi/releases/tags/v1.2.3" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{
			"tag_name": "v1.2.3",
			"assets": [
				{"name": "` + archiveName + `", "browser_download_url": "/download/archive"},
				{"name": "checksums.txt", "browser_download_url": "/download/checksums.txt"},
				{"name": "manifest.json", "browser_download_url": "/download/manifest.json"}
			]
		}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runUpgrade([]string{
		"--version", "1.2.3",
		"--api-url", server.URL,
		"--install-dir", t.TempDir(),
		"--dry-run",
		"--json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runUpgrade exit code = %d, stderr=%s", code, stderr.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("decode upgrade JSON: %v\n%s", err, stdout.String())
	}
	if payload["action"] != "upgrade" || payload["version"] != "1.2.3" || payload["dryRun"] != true {
		t.Fatalf("unexpected upgrade output: %#v", payload)
	}
	if payload["archiveName"] == "" || payload["installDir"] == "" {
		t.Fatalf("upgrade output missing archive/install fields: %#v", payload)
	}
}
