package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseUsesLegacyArchiveNaming(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "0.1.6", want: false},
		{version: "0.1.7", want: true},
		{version: "0.2.0", want: true},
		{version: "0.2.1", want: false},
		{version: "0.3.0", want: false},
		{version: "v0.2.0", want: true},
	}
	for _, tt := range tests {
		if got := releaseUsesLegacyArchiveNaming(tt.version); got != tt.want {
			t.Fatalf("releaseUsesLegacyArchiveNaming(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
}

func TestUpgradeArchiveCandidatesUsesNewNamingFrom021(t *testing.T) {
	candidates, err := upgradeArchiveCandidates("0.2.1", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].name != "agora-cli_v0.2.1_linux_amd64.tar.gz" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestUpgradeArchiveCandidatesUsesLegacyNamingThrough020(t *testing.T) {
	candidates, err := upgradeArchiveCandidates("0.2.0", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].name != "agora-cli-go_v0.2.0_linux_amd64.tar.gz" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestInstallerUpgradeCommandUsesDirectGitHubScript(t *testing.T) {
	command := upgradeCommandForInstallMethod("installer")
	if runtime.GOOS == "windows" {
		if !strings.Contains(command, "https://raw.githubusercontent.com/AgoraIO/cli/main/install.ps1") {
			t.Fatalf("installer command = %q, want raw GitHub PowerShell installer", command)
		}
		return
	}
	if !strings.Contains(command, "https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh") {
		t.Fatalf("installer command = %q, want raw GitHub shell installer", command)
	}
}

func TestResolveLatestVersionFromGitHubFixture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/AgoraIO/cli/releases/latest" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization header = %q", got)
		}
		_, _ = io.WriteString(w, `{"tag_name":"v1.2.3"}`)
	}))
	t.Cleanup(server.Close)

	version, err := resolveLatestVersion(map[string]string{
		"GITHUB_API_URL": server.URL,
		"GITHUB_REPO":    "AgoraIO/cli",
		"GITHUB_TOKEN":   "test-token",
	})
	if err != nil {
		t.Fatalf("resolveLatestVersion() error = %v", err)
	}
	if version != "1.2.3" {
		t.Fatalf("resolveLatestVersion() = %q, want 1.2.3", version)
	}
}

func TestResolveLatestVersionFailures(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "HTTP error", status: http.StatusServiceUnavailable, body: "unavailable"},
		{name: "invalid JSON", status: http.StatusOK, body: "{"},
		{name: "missing tag", status: http.StatusOK, body: `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			t.Cleanup(server.Close)

			if _, err := resolveLatestVersion(map[string]string{"GITHUB_API_URL": server.URL}); err == nil {
				t.Fatal("resolveLatestVersion() error = nil, want failure")
			}
		})
	}
}

func TestDownloadFileAndChecksumHelpers(t *testing.T) {
	payload := []byte("release archive")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization header = %q", got)
		}
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)

	destination := filepath.Join(t.TempDir(), "archive.tar.gz")
	if err := downloadFile(server.URL, destination, map[string]string{"GH_TOKEN": "test-token"}); err != nil {
		t.Fatalf("downloadFile() error = %v", err)
	}
	downloaded, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(downloaded, payload) {
		t.Fatalf("downloaded data = %q, want %q", downloaded, payload)
	}

	digest := sha256.Sum256(payload)
	wantSHA := hex.EncodeToString(digest[:])
	gotSHA, err := sha256OfFile(destination)
	if err != nil {
		t.Fatalf("sha256OfFile() error = %v", err)
	}
	if gotSHA != wantSHA {
		t.Fatalf("sha256OfFile() = %q, want %q", gotSHA, wantSHA)
	}

	checksums := filepath.Join(t.TempDir(), "checksums.txt")
	contents := fmt.Sprintf("ignored  other.zip\n%s *archive.tar.gz\n", wantSHA)
	if err := os.WriteFile(checksums, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	gotExpected, err := expectedChecksumFor(checksums, "archive.tar.gz")
	if err != nil {
		t.Fatalf("expectedChecksumFor() error = %v", err)
	}
	if gotExpected != wantSHA {
		t.Fatalf("expectedChecksumFor() = %q, want %q", gotExpected, wantSHA)
	}
	missing, err := expectedChecksumFor(checksums, "missing.zip")
	if err != nil || missing != "" {
		t.Fatalf("expectedChecksumFor() missing archive = %q, %v; want empty result", missing, err)
	}
}

func TestDownloadFileRejectsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no release", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	if err := downloadFile(server.URL, filepath.Join(t.TempDir(), "release"), nil); err == nil {
		t.Fatal("downloadFile() error = nil, want failure")
	}
}

func TestExtractReleaseArchivesAndCopy(t *testing.T) {
	const binaryName = "agora"
	payload := []byte("native binary")
	directory := t.TempDir()

	tarPath := filepath.Join(directory, "release.tar.gz")
	writeTarGzFixture(t, tarPath, "nested/"+binaryName, payload)
	tarOutput := filepath.Join(directory, "from-tar")
	if err := extractFromTarGz(tarPath, binaryName, tarOutput); err != nil {
		t.Fatalf("extractFromTarGz() error = %v", err)
	}
	assertFileContents(t, tarOutput, payload)

	zipPath := filepath.Join(directory, "release.zip")
	writeZipFixture(t, zipPath, "nested/"+binaryName, payload)
	zipOutput := filepath.Join(directory, "from-zip")
	if err := extractFromZip(zipPath, binaryName, zipOutput); err != nil {
		t.Fatalf("extractFromZip() error = %v", err)
	}
	assertFileContents(t, zipOutput, payload)

	copyOutput := filepath.Join(directory, "copy")
	if err := copyFile(zipOutput, copyOutput); err != nil {
		t.Fatalf("copyFile() error = %v", err)
	}
	assertFileContents(t, copyOutput, payload)

	if err := extractFromTarGz(tarPath, "missing", filepath.Join(directory, "missing-tar")); err == nil {
		t.Fatal("extractFromTarGz() missing binary error = nil")
	}
	if err := extractFromZip(zipPath, "missing", filepath.Join(directory, "missing-zip")); err == nil {
		t.Fatal("extractFromZip() missing binary error = nil")
	}
}

func writeTarGzFixture(t *testing.T, path, name string, payload []byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(payload))}); err != nil {
		t.Fatalf("WriteHeader() error = %v", err)
	}
	if _, err := tarWriter.Write(payload); err != nil {
		t.Fatalf("tar Write() error = %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("tar Close() error = %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file Close() error = %v", err)
	}
}

func writeZipFixture(t *testing.T, path, name string, payload []byte) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	zipWriter := zip.NewWriter(file)
	entry, err := zipWriter.Create(name)
	if err != nil {
		t.Fatalf("zip Create() error = %v", err)
	}
	if _, err := entry.Write(payload); err != nil {
		t.Fatalf("zip Write() error = %v", err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("zip Close() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file Close() error = %v", err)
	}
}

func assertFileContents(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("ReadFile(%q) = %q, want %q", path, got, want)
	}
}
