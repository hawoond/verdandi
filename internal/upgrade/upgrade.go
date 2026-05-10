package upgrade

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	buildversion "github.com/genie-cvc/verdandi/internal/version"
)

const defaultAPIBaseURL = "https://api.github.com"
const defaultRepository = "hawoond/verdandi"

type Options struct {
	Version    string
	InstallDir string
	Repository string
	APIBaseURL string
	GOOS       string
	GOARCH     string
	DryRun     bool
	Force      bool
	Client     *http.Client
}

type Result struct {
	Version         string `json:"version"`
	TagName         string `json:"tagName"`
	ArchiveName     string `json:"archiveName"`
	InstallDir      string `json:"installDir"`
	DryRun          bool   `json:"dryRun"`
	AlreadyUpToDate bool   `json:"alreadyUpToDate,omitempty"`
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type releaseManifest struct {
	Product   string `json:"product"`
	Version   string `json:"version"`
	Artifacts []struct {
		Name   string `json:"name"`
		OS     string `json:"os"`
		Arch   string `json:"arch"`
		Format string `json:"format"`
		SHA256 string `json:"sha256"`
	} `json:"artifacts"`
}

func Run(options Options) (Result, error) {
	options = normalizeOptions(options)
	release, err := fetchRelease(options)
	if err != nil {
		return Result{}, err
	}
	version := strings.TrimPrefix(release.TagName, "v")
	archiveName := releaseArchiveName(version, options.GOOS, options.GOARCH)
	result := Result{
		Version:     version,
		TagName:     release.TagName,
		ArchiveName: archiveName,
		InstallDir:  options.InstallDir,
		DryRun:      options.DryRun,
	}
	archiveURL, err := releaseAssetURL(release, archiveName)
	if err != nil {
		return Result{}, err
	}
	checksumsURL, err := releaseAssetURL(release, "checksums.txt")
	if err != nil {
		return Result{}, err
	}
	manifestURL, err := releaseAssetURL(release, "manifest.json")
	if err != nil {
		return Result{}, err
	}
	if isCurrentVersion(version) && !options.Force {
		result.AlreadyUpToDate = true
		return result, nil
	}
	if options.DryRun {
		return result, nil
	}

	archive, err := download(options, archiveURL)
	if err != nil {
		return Result{}, err
	}
	checksums, err := download(options, checksumsURL)
	if err != nil {
		return Result{}, err
	}
	if err := verifyChecksums(checksums, archiveName, archive); err != nil {
		return Result{}, err
	}
	manifestData, err := download(options, manifestURL)
	if err != nil {
		return Result{}, err
	}
	if err := verifyManifest(manifestData, version, archiveName, archive); err != nil {
		return Result{}, err
	}
	if err := installArchive(archiveName, archive, options.InstallDir); err != nil {
		return Result{}, err
	}
	return result, nil
}

func normalizeOptions(options Options) Options {
	if options.Repository == "" {
		options.Repository = defaultRepository
	}
	if options.APIBaseURL == "" {
		options.APIBaseURL = defaultAPIBaseURL
	}
	options.APIBaseURL = strings.TrimRight(options.APIBaseURL, "/")
	if options.GOOS == "" {
		options.GOOS = runtime.GOOS
	}
	if options.GOARCH == "" {
		options.GOARCH = runtime.GOARCH
	}
	if options.InstallDir == "" {
		options.InstallDir = defaultInstallDir()
	}
	if options.Client == nil {
		options.Client = http.DefaultClient
	}
	return options
}

func fetchRelease(options Options) (githubRelease, error) {
	url := options.APIBaseURL + "/repos/" + options.Repository + "/releases/latest"
	if options.Version != "" {
		url = options.APIBaseURL + "/repos/" + options.Repository + "/releases/tags/v" + strings.TrimPrefix(options.Version, "v")
	}
	data, err := download(options, url)
	if err != nil {
		return githubRelease{}, err
	}
	var release githubRelease
	if err := json.Unmarshal(data, &release); err != nil {
		return githubRelease{}, fmt.Errorf("decode release metadata: %w", err)
	}
	if release.TagName == "" {
		return githubRelease{}, fmt.Errorf("release metadata missing tag_name")
	}
	return release, nil
}

func download(options Options, url string) ([]byte, error) {
	if strings.HasPrefix(url, "/") {
		url = options.APIBaseURL + url
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	resp, err := options.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func releaseAssetURL(release githubRelease, name string) (string, error) {
	for _, asset := range release.Assets {
		if asset.Name == name {
			return asset.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("release asset not found: %s", name)
}

func releaseArchiveName(version string, goos string, goarch string) string {
	name := fmt.Sprintf("verdandi_%s_%s_%s", strings.TrimPrefix(version, "v"), goos, goarch)
	if goos == "windows" {
		return name + ".zip"
	}
	return name + ".tar.gz"
}

func verifyChecksums(checksums []byte, archiveName string, archive []byte) error {
	expected := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[len(fields)-1] == archiveName {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("checksums.txt missing %s", archiveName)
	}
	actual := sha256Bytes(archive)
	if actual != expected {
		return fmt.Errorf("checksum mismatch for %s", archiveName)
	}
	return nil
}

func verifyManifest(data []byte, version string, archiveName string, archive []byte) error {
	var manifest releaseManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.Product != "" && manifest.Product != "verdandi" {
		return fmt.Errorf("manifest product = %q, want verdandi", manifest.Product)
	}
	if manifest.Version != "" && manifest.Version != version {
		return fmt.Errorf("manifest version = %q, want %s", manifest.Version, version)
	}
	actual := sha256Bytes(archive)
	for _, artifact := range manifest.Artifacts {
		if artifact.Name == archiveName {
			if artifact.SHA256 != actual {
				return fmt.Errorf("manifest checksum mismatch for %s", archiveName)
			}
			return nil
		}
	}
	return fmt.Errorf("manifest missing artifact: %s", archiveName)
}

func installArchive(archiveName string, archive []byte, installDir string) error {
	tmp, err := os.MkdirTemp("", "verdandi-upgrade-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if strings.HasSuffix(archiveName, ".zip") {
		if err := extractZip(archive, tmp); err != nil {
			return err
		}
	} else {
		if err := extractTarGzip(archive, tmp); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("archive did not contain a package directory")
	}
	packageDir := filepath.Join(tmp, entries[0].Name())
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return err
	}
	for _, binary := range []string{"verdandi", "verdandi-mcp", "verdandi-spinning-wheel"} {
		source := filepath.Join(packageDir, binary)
		if _, err := os.Stat(source); err != nil && os.IsNotExist(err) {
			source = filepath.Join(packageDir, binary+".exe")
		}
		if _, err := os.Stat(source); err != nil {
			return fmt.Errorf("missing binary in archive: %s", binary)
		}
		target := filepath.Join(installDir, binary)
		if err := copyFile(source, target, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func extractTarGzip(data []byte, destination string) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeExtractPath(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(output, tarReader); err != nil {
				_ = output.Close()
				return err
			}
			if err := output.Close(); err != nil {
				return err
			}
		}
	}
}

func extractZip(data []byte, destination string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, file := range reader.File {
		target, err := safeExtractPath(destination, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, file.FileInfo().Mode()); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		source, err := file.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.FileInfo().Mode())
		if err != nil {
			_ = source.Close()
			return err
		}
		if _, err := io.Copy(output, source); err != nil {
			_ = source.Close()
			_ = output.Close()
			return err
		}
		_ = source.Close()
		if err := output.Close(); err != nil {
			return err
		}
	}
	return nil
}

func safeExtractPath(destination string, name string) (string, error) {
	target := filepath.Join(destination, filepath.Clean(name))
	if !strings.HasPrefix(target, filepath.Clean(destination)+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive entry escapes destination: %s", name)
	}
	return target, nil
}

func copyFile(source string, target string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func sha256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func defaultInstallDir() string {
	if value := os.Getenv("VERDANDI_INSTALL_DIR"); value != "" {
		return value
	}
	if executable, err := os.Executable(); err == nil {
		if dir := filepath.Dir(executable); dir != "" {
			return dir
		}
	}
	return "/usr/local/bin"
}

func isCurrentVersion(candidate string) bool {
	current := strings.TrimPrefix(buildversion.Version, "v")
	candidate = strings.TrimPrefix(candidate, "v")
	return current != "" && current != "dev" && current != "unknown" && current == candidate
}
