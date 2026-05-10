package upgrade

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunInstallsCurrentTargetRelease(t *testing.T) {
	archiveName := releaseArchiveName("1.2.3", runtime.GOOS, runtime.GOARCH)
	files := map[string][]byte{}
	archive := createReleaseArchive(t, archiveName)
	files[archiveName] = archive
	files["checksums.txt"] = []byte(sha256Hex(archive) + "  " + archiveName + "\n")
	files["manifest.json"] = []byte(releaseManifestJSON(t, "1.2.3", archiveName, sha256Hex(archive)))

	server := fakeReleaseServer(t, "v1.2.3", files)
	installDir := t.TempDir()

	result, err := Run(Options{
		Version:    "1.2.3",
		InstallDir: installDir,
		APIBaseURL: server.URL,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Force:      true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Version != "1.2.3" || result.ArchiveName != archiveName || result.InstallDir != installDir {
		t.Fatalf("unexpected result: %#v", result)
	}
	for _, binary := range []string{"verdandi", "verdandi-mcp", "verdandi-spinning-wheel"} {
		path := filepath.Join(installDir, binary)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected installed %s: %v", binary, err)
		}
		if !strings.Contains(string(data), "fake "+binary) {
			t.Fatalf("unexpected installed %s content: %s", binary, data)
		}
	}
}

func TestRunDryRunDoesNotInstall(t *testing.T) {
	archiveName := releaseArchiveName("1.2.3", runtime.GOOS, runtime.GOARCH)
	files := map[string][]byte{
		archiveName:     createReleaseArchive(t, archiveName),
		"checksums.txt": []byte("unused  " + archiveName + "\n"),
		"manifest.json": []byte(`{"product":"verdandi","version":"1.2.3","artifacts":[]}` + "\n"),
	}
	server := fakeReleaseServer(t, "v1.2.3", files)
	installDir := t.TempDir()

	result, err := Run(Options{
		Version:    "1.2.3",
		InstallDir: installDir,
		APIBaseURL: server.URL,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		DryRun:     true,
		Force:      true,
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.DryRun {
		t.Fatalf("expected dry-run result: %#v", result)
	}
	if entries, err := os.ReadDir(installDir); err != nil || len(entries) != 0 {
		t.Fatalf("dry-run should not install files, entries=%#v err=%v", entries, err)
	}
}

func TestRunRejectsChecksumMismatch(t *testing.T) {
	archiveName := releaseArchiveName("1.2.3", runtime.GOOS, runtime.GOARCH)
	archive := createReleaseArchive(t, archiveName)
	files := map[string][]byte{
		archiveName:     archive,
		"checksums.txt": []byte("bad  " + archiveName + "\n"),
		"manifest.json": []byte(releaseManifestJSON(t, "1.2.3", archiveName, sha256Hex(archive))),
	}
	server := fakeReleaseServer(t, "v1.2.3", files)

	_, err := Run(Options{
		Version:    "1.2.3",
		InstallDir: t.TempDir(),
		APIBaseURL: server.URL,
		GOOS:       runtime.GOOS,
		GOARCH:     runtime.GOARCH,
		Force:      true,
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestInstallArchiveIgnoresAppleDoubleEntries(t *testing.T) {
	archiveName := "verdandi_1.2.3_darwin_arm64.tar.gz"
	installDir := t.TempDir()

	if err := installArchive(archiveName, createAppleDoubleArchive(t, archiveName), installDir); err != nil {
		t.Fatalf("installArchive returned error: %v", err)
	}
	for _, binary := range []string{"verdandi", "verdandi-mcp", "verdandi-spinning-wheel"} {
		path := filepath.Join(installDir, binary)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected installed %s: %v", binary, err)
		}
		if !strings.Contains(string(data), "fake "+binary) {
			t.Fatalf("unexpected installed %s content: %s", binary, data)
		}
	}
}

func fakeReleaseServer(t *testing.T, tag string, files map[string][]byte) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/hawoond/verdandi/releases/latest":
			writeReleaseResponse(t, w, tag, files)
		case r.URL.Path == "/repos/hawoond/verdandi/releases/tags/"+tag:
			writeReleaseResponse(t, w, tag, files)
		case strings.HasPrefix(r.URL.Path, "/download/"):
			name := strings.TrimPrefix(r.URL.Path, "/download/")
			data, ok := files[name]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeReleaseResponse(t *testing.T, w http.ResponseWriter, tag string, files map[string][]byte) {
	t.Helper()

	type asset struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}
	assets := []asset{}
	for name := range files {
		assets = append(assets, asset{Name: name, BrowserDownloadURL: "/download/" + name})
	}
	response := struct {
		TagName string  `json:"tag_name"`
		Assets  []asset `json:"assets"`
	}{TagName: tag, Assets: assets}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		t.Fatalf("encode release response: %v", err)
	}
}

func createReleaseArchive(t *testing.T, archiveName string) []byte {
	t.Helper()

	tmp := t.TempDir()
	packageName := strings.TrimSuffix(strings.TrimSuffix(archiveName, ".gz"), ".tar")
	packageName = strings.TrimSuffix(packageName, ".zip")
	stage := filepath.Join(tmp, packageName)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatalf("create stage: %v", err)
	}
	for _, binary := range []string{"verdandi", "verdandi-mcp", "verdandi-spinning-wheel"} {
		if err := os.WriteFile(filepath.Join(stage, binary), []byte("fake "+binary+"\n"), 0o755); err != nil {
			t.Fatalf("write fake binary: %v", err)
		}
	}

	archivePath := filepath.Join(tmp, archiveName)
	if strings.HasSuffix(archiveName, ".zip") {
		createZipFromDir(t, stage, archivePath, packageName)
		data, err := os.ReadFile(archivePath)
		if err != nil {
			t.Fatalf("read archive: %v", err)
		}
		return data
	}
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, binary := range []string{"verdandi", "verdandi-mcp", "verdandi-spinning-wheel"} {
		path := filepath.Join(stage, binary)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat fake binary: %v", err)
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			t.Fatalf("create tar header: %v", err)
		}
		header.Name = filepath.ToSlash(filepath.Join(packageName, binary))
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		source, err := os.Open(path)
		if err != nil {
			t.Fatalf("open fake binary: %v", err)
		}
		if _, err := io.Copy(tarWriter, source); err != nil {
			_ = source.Close()
			t.Fatalf("write tar body: %v", err)
		}
		_ = source.Close()
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	return data
}

func createAppleDoubleArchive(t *testing.T, archiveName string) []byte {
	t.Helper()

	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, archiveName)
	packageName := strings.TrimSuffix(archiveName, ".tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)

	writeTarFile(t, tarWriter, "._"+packageName, []byte("mac metadata"))
	writeTarDir(t, tarWriter, packageName)
	for _, binary := range []string{"verdandi", "verdandi-mcp", "verdandi-spinning-wheel"} {
		writeTarFile(t, tarWriter, filepath.ToSlash(filepath.Join(packageName, "._"+binary)), []byte("mac metadata"))
		writeTarFile(t, tarWriter, filepath.ToSlash(filepath.Join(packageName, binary)), []byte("fake "+binary+"\n"))
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	return data
}

func writeTarDir(t *testing.T, tarWriter *tar.Writer, name string) {
	t.Helper()

	header := &tar.Header{
		Name:     filepath.ToSlash(name),
		Mode:     0o755,
		Typeflag: tar.TypeDir,
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("write tar dir: %v", err)
	}
}

func writeTarFile(t *testing.T, tarWriter *tar.Writer, name string, data []byte) {
	t.Helper()

	header := &tar.Header{
		Name:     filepath.ToSlash(name),
		Mode:     0o755,
		Size:     int64(len(data)),
		Typeflag: tar.TypeReg,
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tarWriter.Write(data); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
}

func createZipFromDir(t *testing.T, stage string, archivePath string, packageName string) {
	t.Helper()

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zipWriter := zip.NewWriter(file)
	for _, binary := range []string{"verdandi", "verdandi-mcp", "verdandi-spinning-wheel"} {
		path := filepath.Join(stage, binary)
		writer, err := zipWriter.Create(filepath.ToSlash(filepath.Join(packageName, binary+".exe")))
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read fake binary: %v", err)
		}
		if _, err := writer.Write(data); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close zip file: %v", err)
	}
}

func releaseManifestJSON(t *testing.T, version string, archiveName string, checksum string) string {
	t.Helper()

	format := "tar.gz"
	if strings.HasSuffix(archiveName, ".zip") {
		format = "zip"
	}
	manifest := map[string]any{
		"product": "verdandi",
		"version": version,
		"artifacts": []map[string]any{{
			"name":   archiveName,
			"os":     runtime.GOOS,
			"arch":   runtime.GOARCH,
			"format": format,
			"sha256": checksum,
		}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	return string(data) + "\n"
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
