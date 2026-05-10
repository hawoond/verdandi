package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseBuildScriptPackagesCurrentTarget(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	script := filepath.Join(root, "scripts", "release_build.sh")
	dist := t.TempDir()

	cmd := exec.Command("bash", script)
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"VERDANDI_RELEASE_TARGETS=current",
		"VERDANDI_VERSION=test",
		"VERDANDI_COMMIT=testcommit",
		"VERDANDI_BUILD_DATE=2026-05-05T00:00:00Z",
		"VERDANDI_DIST_DIR="+dist,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release build script failed: %v\n%s", err, output)
	}

	archives, err := filepath.Glob(filepath.Join(dist, "verdandi_test_*"))
	if err != nil {
		t.Fatalf("glob release archives: %v", err)
	}
	if len(archives) == 0 {
		t.Fatalf("expected release archives in %s; output:\n%s", dist, output)
	}
	if _, err := os.Stat(filepath.Join(dist, "checksums.txt")); err != nil {
		t.Fatalf("missing checksums.txt: %v", err)
	}
	manifest := readReleaseManifest(t, filepath.Join(dist, "manifest.json"))
	if manifest.Product != "verdandi" || manifest.Version != "test" || manifest.Commit != "testcommit" || manifest.BuildDate != "2026-05-05T00:00:00Z" {
		t.Fatalf("unexpected release manifest metadata: %#v", manifest)
	}
	sbom := readReleaseSBOM(t, filepath.Join(dist, "sbom.spdx.json"))
	if sbom.SPDXVersion != "SPDX-2.3" || sbom.DataLicense != "CC0-1.0" || sbom.Name != "verdandi-test" {
		t.Fatalf("unexpected SBOM metadata: %#v", sbom)
	}
	if !releaseSBOMHasPackage(sbom, "github.com/genie-cvc/verdandi", "test") {
		t.Fatalf("SBOM missing root module package: %#v", sbom.Packages)
	}

	archive := releaseArchiveForCurrentTarget(t, dist)
	manifestArtifact := releaseManifestArtifactFor(t, manifest, filepath.Base(archive))
	if manifestArtifact.OS != runtime.GOOS || manifestArtifact.Arch != runtime.GOARCH {
		t.Fatalf("unexpected manifest target metadata: %#v", manifestArtifact)
	}
	expectedFormat := "tar.gz"
	if runtime.GOOS == "windows" {
		expectedFormat = "zip"
	}
	if manifestArtifact.Format != expectedFormat {
		t.Fatalf("expected manifest format %q, got %#v", expectedFormat, manifestArtifact)
	}
	if manifestArtifact.SHA256 != sha256File(t, archive) {
		t.Fatalf("manifest checksum mismatch for %s", archive)
	}
	extracted := filepath.Join(t.TempDir(), "release")
	extractReleaseArchive(t, archive, extracted)
	binary := filepath.Join(extracted, "verdandi_test_"+runtime.GOOS+"_"+runtime.GOARCH, "verdandi")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	versionOutput, err := exec.Command(binary, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("run packaged verdandi --version: %v\n%s", err, versionOutput)
	}
	if !strings.Contains(string(versionOutput), "version=test") {
		t.Fatalf("expected packaged binary version metadata, got %q", versionOutput)
	}
	for _, path := range []string{
		filepath.Join(extracted, "verdandi_test_"+runtime.GOOS+"_"+runtime.GOARCH, "install_release.sh"),
		filepath.Join(extracted, "verdandi_test_"+runtime.GOOS+"_"+runtime.GOARCH, "docs", "INSTALL.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected install helper in archive at %s: %v", path, err)
		}
	}
}

type releaseManifest struct {
	Product   string                    `json:"product"`
	Version   string                    `json:"version"`
	Commit    string                    `json:"commit"`
	BuildDate string                    `json:"buildDate"`
	Artifacts []releaseManifestArtifact `json:"artifacts"`
}

type releaseManifestArtifact struct {
	Name   string `json:"name"`
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Format string `json:"format"`
	SHA256 string `json:"sha256"`
}

type releaseSBOM struct {
	SPDXVersion string               `json:"spdxVersion"`
	DataLicense string               `json:"dataLicense"`
	Name        string               `json:"name"`
	Packages    []releaseSBOMPackage `json:"packages"`
}

type releaseSBOMPackage struct {
	Name        string `json:"name"`
	VersionInfo string `json:"versionInfo"`
	SPDXID      string `json:"SPDXID"`
}

func readReleaseManifest(t *testing.T, path string) releaseManifest {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read release manifest: %v", err)
	}
	var manifest releaseManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse release manifest: %v\n%s", err, data)
	}
	return manifest
}

func readReleaseSBOM(t *testing.T, path string) releaseSBOM {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read release SBOM: %v", err)
	}
	var sbom releaseSBOM
	if err := json.Unmarshal(data, &sbom); err != nil {
		t.Fatalf("parse release SBOM: %v\n%s", err, data)
	}
	return sbom
}

func releaseSBOMHasPackage(sbom releaseSBOM, name string, version string) bool {
	for _, pkg := range sbom.Packages {
		if pkg.Name == name && pkg.VersionInfo == version && strings.HasPrefix(pkg.SPDXID, "SPDXRef-Package-") {
			return true
		}
	}
	return false
}

func releaseManifestArtifactFor(t *testing.T, manifest releaseManifest, name string) releaseManifestArtifact {
	t.Helper()

	for _, artifact := range manifest.Artifacts {
		if artifact.Name == name {
			return artifact
		}
	}
	t.Fatalf("manifest missing artifact %s: %#v", name, manifest.Artifacts)
	return releaseManifestArtifact{}
}

func sha256File(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file for sha256: %v", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestReleaseNotesScriptIncludesInstallAndChecksums(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "verdandi_1.2.3_linux_amd64.tar.gz"), []byte("archive"), 0o644); err != nil {
		t.Fatalf("write fake archive: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "checksums.txt"), []byte("abc123  verdandi_1.2.3_linux_amd64.tar.gz\n"), 0o644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "manifest.json"), []byte(`{"product":"verdandi","version":"1.2.3","artifacts":[]}`+"\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "sbom.spdx.json"), []byte(`{"spdxVersion":"SPDX-2.3","packages":[]}`+"\n"), 0o644); err != nil {
		t.Fatalf("write sbom: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "release_notes.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"VERDANDI_VERSION=1.2.3",
		"VERDANDI_DIST_DIR="+dist,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("release notes script failed: %v\n%s", err, output)
	}

	notes, err := os.ReadFile(filepath.Join(dist, "release-notes.md"))
	if err != nil {
		t.Fatalf("read release notes: %v", err)
	}
	text := string(notes)
	for _, expected := range []string{
		"# Verdandi 1.2.3",
		"verdandi_1.2.3_linux_amd64.tar.gz",
		"tar -xzf",
		"install_release.sh",
		"verdandi --version",
		"verdandi upgrade",
		"checksums.txt",
		"manifest.json",
		"sbom.spdx.json",
		"Build Manifest",
		"Software Bill of Materials",
		"abc123",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("release notes missing %q:\n%s", expected, text)
		}
	}
}

func TestPublishReleaseScriptCreatesVersionedGitHubRelease(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	dist := t.TempDir()
	for name, content := range map[string]string{
		"verdandi_1.2.3_linux_amd64.tar.gz": "archive",
		"verdandi_1.2.3_windows_amd64.zip":  "archive",
		"checksums.txt":                     "abc123  verdandi_1.2.3_linux_amd64.tar.gz\n",
		"manifest.json":                     `{"product":"verdandi","version":"1.2.3","artifacts":[]}` + "\n",
		"sbom.spdx.json":                    `{"spdxVersion":"SPDX-2.3","packages":[]}` + "\n",
		"release-notes.md":                  "# Verdandi 1.2.3\n",
	} {
		if err := os.WriteFile(filepath.Join(dist, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gh.log")
	fakeGH := filepath.Join(binDir, "gh")
	if err := os.WriteFile(fakeGH, []byte(`#!/usr/bin/env bash
echo "$*" >> "$GH_LOG"
if [[ "$1 $2" == "release view" ]]; then
  exit 1
fi
exit 0
`), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "publish_release.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GH_LOG="+logPath,
		"VERDANDI_VERSION=1.2.3",
		"VERDANDI_DIST_DIR="+dist,
		"GITHUB_SHA=abc123",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("publish release script failed: %v\n%s", err, output)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake gh log: %v", err)
	}
	if !strings.Contains(string(log), "release create v1.2.3") {
		t.Fatalf("expected release create for v1.2.3, got:\n%s", log)
	}
	if !strings.Contains(string(log), "--notes-file") {
		t.Fatalf("expected notes file to be passed, got:\n%s", log)
	}
	if !strings.Contains(string(log), "manifest.json") {
		t.Fatalf("expected manifest asset to be uploaded, got:\n%s", log)
	}
	if !strings.Contains(string(log), "sbom.spdx.json") {
		t.Fatalf("expected SBOM asset to be uploaded, got:\n%s", log)
	}
}

func TestPublishReleaseScriptWorksWithoutGitHubSHA(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	dist := t.TempDir()
	for name, content := range map[string]string{
		"verdandi_1.2.3_linux_amd64.tar.gz": "archive",
		"checksums.txt":                     "abc123  verdandi_1.2.3_linux_amd64.tar.gz\n",
		"manifest.json":                     `{"product":"verdandi","version":"1.2.3","artifacts":[]}` + "\n",
		"sbom.spdx.json":                    `{"spdxVersion":"SPDX-2.3","packages":[]}` + "\n",
		"release-notes.md":                  "# Verdandi 1.2.3\n",
	} {
		if err := os.WriteFile(filepath.Join(dist, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gh.log")
	fakeGH := filepath.Join(binDir, "gh")
	if err := os.WriteFile(fakeGH, []byte(`#!/usr/bin/env bash
echo "$*" >> "$GH_LOG"
if [[ "$1 $2" == "release view" ]]; then
  exit 1
fi
exit 0
`), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "publish_release.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GH_LOG="+logPath,
		"VERDANDI_VERSION=1.2.3",
		"VERDANDI_DIST_DIR="+dist,
		"GITHUB_SHA=",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("publish release script failed without GITHUB_SHA: %v\n%s", err, output)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake gh log: %v", err)
	}
	if !strings.Contains(string(log), "release create v1.2.3") {
		t.Fatalf("expected release create for v1.2.3, got:\n%s", log)
	}
	if strings.Contains(string(log), "--target") {
		t.Fatalf("did not expect --target without GITHUB_SHA, got:\n%s", log)
	}
}

func TestInstallReleaseScriptInstallsArchiveBinaries(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	dist := t.TempDir()
	packageName := "verdandi_1.2.3_" + runtime.GOOS + "_" + runtime.GOARCH
	stage := filepath.Join(t.TempDir(), packageName)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatalf("create package stage: %v", err)
	}
	for _, name := range []string{"verdandi", "verdandi-mcp", "verdandi-spinning-wheel"} {
		if err := os.WriteFile(filepath.Join(stage, name), []byte("#!/usr/bin/env sh\necho "+name+"\n"), 0o755); err != nil {
			t.Fatalf("write fake binary: %v", err)
		}
	}
	archive := filepath.Join(dist, packageName+".tar.gz")
	if err := createTarGzipFromDir(stage, archive, packageName); err != nil {
		t.Fatalf("create fake archive: %v", err)
	}
	checksumCmd := exec.Command("shasum", "-a", "256", filepath.Base(archive))
	checksumCmd.Dir = dist
	checksum, err := checksumCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compute checksum: %v\n%s", err, checksum)
	}
	if err := os.WriteFile(filepath.Join(dist, "checksums.txt"), checksum, 0o644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}

	binDir := t.TempDir()
	cmd := exec.Command("bash", filepath.Join(root, "scripts", "install_release.sh"), archive)
	cmd.Dir = dist
	cmd.Env = append(os.Environ(), "VERDANDI_INSTALL_DIR="+binDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install release script failed: %v\n%s", err, output)
	}
	for _, name := range []string{"verdandi", "verdandi-mcp", "verdandi-spinning-wheel"} {
		installed := filepath.Join(binDir, name)
		if _, err := os.Stat(installed); err != nil {
			t.Fatalf("expected installed %s: %v", name, err)
		}
	}
}

func TestInstallReleaseScriptRejectsManifestChecksumMismatch(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	dist := t.TempDir()
	packageName := "verdandi_1.2.3_" + runtime.GOOS + "_" + runtime.GOARCH
	stage := filepath.Join(t.TempDir(), packageName)
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatalf("create package stage: %v", err)
	}
	for _, name := range []string{"verdandi", "verdandi-mcp", "verdandi-spinning-wheel"} {
		if err := os.WriteFile(filepath.Join(stage, name), []byte("#!/usr/bin/env sh\necho "+name+"\n"), 0o755); err != nil {
			t.Fatalf("write fake binary: %v", err)
		}
	}
	archive := filepath.Join(dist, packageName+".tar.gz")
	if err := createTarGzipFromDir(stage, archive, packageName); err != nil {
		t.Fatalf("create fake archive: %v", err)
	}
	checksumCmd := exec.Command("shasum", "-a", "256", filepath.Base(archive))
	checksumCmd.Dir = dist
	checksum, err := checksumCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compute checksum: %v\n%s", err, checksum)
	}
	if err := os.WriteFile(filepath.Join(dist, "checksums.txt"), checksum, 0o644); err != nil {
		t.Fatalf("write checksums: %v", err)
	}
	manifest := `{"product":"verdandi","version":"1.2.3","artifacts":[{"name":"` + filepath.Base(archive) + `","sha256":"bad"}]}` + "\n"
	if err := os.WriteFile(filepath.Join(dist, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := exec.Command("bash", filepath.Join(root, "scripts", "install_release.sh"), archive)
	cmd.Dir = dist
	cmd.Env = append(os.Environ(), "VERDANDI_INSTALL_DIR="+t.TempDir())
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected install release script to reject manifest mismatch, got success:\n%s", output)
	}
	if !strings.Contains(string(output), "manifest checksum mismatch") {
		t.Fatalf("expected manifest mismatch error, got:\n%s", output)
	}
}

func releaseArchiveForCurrentTarget(t *testing.T, dist string) string {
	t.Helper()

	pattern := filepath.Join(dist, "verdandi_test_"+runtime.GOOS+"_"+runtime.GOARCH+".*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob release archive: %v", err)
	}
	for _, match := range matches {
		if strings.HasSuffix(match, ".tar.gz") || strings.HasSuffix(match, ".zip") {
			return match
		}
	}
	t.Fatalf("missing release archive matching %s; got %#v", pattern, matches)
	return ""
}

func extractReleaseArchive(t *testing.T, archive string, destination string) {
	t.Helper()

	if strings.HasSuffix(archive, ".zip") {
		extractZip(t, archive, destination)
		return
	}
	extractTarGzip(t, archive, destination)
}

func extractTarGzip(t *testing.T, archive string, destination string) {
	t.Helper()

	file, err := os.Open(archive)
	if err != nil {
		t.Fatalf("open tar.gz: %v", err)
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("read gzip: %v", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("read tar entry: %v", err)
		}
		target := filepath.Join(destination, filepath.Clean(header.Name))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatalf("create tar dir: %v", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("create tar parent: %v", err)
			}
			output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				t.Fatalf("create tar file: %v", err)
			}
			if _, err := io.Copy(output, tarReader); err != nil {
				_ = output.Close()
				t.Fatalf("extract tar file: %v", err)
			}
			if err := output.Close(); err != nil {
				t.Fatalf("close tar file: %v", err)
			}
		}
	}
}

func extractZip(t *testing.T, archive string, destination string) {
	t.Helper()

	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		target := filepath.Join(destination, filepath.Clean(file.Name))
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatalf("create zip dir: %v", err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("create zip parent: %v", err)
		}
		input, err := file.Open()
		if err != nil {
			t.Fatalf("open zip entry: %v", err)
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			_ = input.Close()
			t.Fatalf("create zip file: %v", err)
		}
		if _, err := io.Copy(output, input); err != nil {
			_ = input.Close()
			_ = output.Close()
			t.Fatalf("extract zip file: %v", err)
		}
		if err := input.Close(); err != nil {
			_ = output.Close()
			t.Fatalf("close zip input: %v", err)
		}
		if err := output.Close(); err != nil {
			t.Fatalf("close zip output: %v", err)
		}
	}
}

func createTarGzipFromDir(source string, archive string, packageName string) error {
	output, err := os.Create(archive)
	if err != nil {
		return err
	}
	defer output.Close()
	gzipWriter := gzip.NewWriter(output)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(filepath.Dir(source), path)
		if err != nil {
			return err
		}
		if rel == "." {
			rel = packageName
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		_, err = io.Copy(tarWriter, input)
		return err
	})
}
