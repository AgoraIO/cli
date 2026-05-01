package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// maxExtractedBinaryBytes caps the size of any binary we extract from a
// downloaded release archive (gosec G110: decompression bomb defense).
// 256 MiB is roughly 25x the size of the current Go-built `agora` binary,
// providing significant headroom for future feature growth while still
// preventing a hostile archive from filling the user's disk.
const maxExtractedBinaryBytes int64 = 256 << 20

const installReceiptFileName = "agora.install.json"

type installReceipt struct {
	SchemaVersion int    `json:"schemaVersion"`
	Tool          string `json:"tool"`
	InstallMethod string `json:"installMethod"`
	InstallPath   string `json:"installPath"`
	Version       string `json:"version"`
	InstalledAt   string `json:"installedAt"`
	Source        string `json:"source"`
}

type installProvenance struct {
	Method         string
	Source         string
	InstalledPath  string
	ReceiptPath    string
	UpgradeCommand string
}

// performSelfUpdate downloads and atomically replaces the running binary with
// the latest release from GitHub when the install was managed by install.sh /
// install.ps1. For Homebrew, npm, or any other managed channel, it returns
// nil with a status of "manual" so the caller can print the manager-specific
// upgrade command. dryRun resolves the target version and prints what would
// happen without making any filesystem changes.
//
// Returns a result map for the upgrade envelope and any error.
func (a *App) performSelfUpdate(dryRun bool) (map[string]any, error) {
	provenance := detectInstallProvenance(a.env)

	current := strings.TrimSpace(version)
	if current == "" {
		current = "dev"
	}
	result := map[string]any{
		"action":         "upgrade",
		"command":        provenance.UpgradeCommand,
		"currentVersion": current,
		"installMethod":  provenance.Method,
		"installSource":  provenance.Source,
		"installedPath":  provenance.InstalledPath,
		"upgradeCommand": provenance.UpgradeCommand,
	}
	if provenance.ReceiptPath != "" {
		result["receiptPath"] = provenance.ReceiptPath
	}

	// Channels we don't manage in-process. Defer to the package manager.
	if provenance.Method != "installer" {
		result["status"] = "manual"
		return result, nil
	}

	// Resolve the latest release tag from GitHub.
	latest, err := resolveLatestVersion(a.env)
	if err != nil {
		// Network errors here are not fatal at the envelope level; agents can
		// still see the manual command and try again later. Surface as a
		// warning by returning a structured error code.
		return nil, &cliError{Message: fmt.Sprintf("could not resolve the latest release: %v", err), Code: "UPGRADE_NETWORK_FAILED"}
	}
	result["latestVersion"] = latest

	if isSameOrNewer(current, latest) {
		result["status"] = "up-to-date"
		return result, nil
	}

	if dryRun {
		result["status"] = "dry-run"
		return result, nil
	}

	// Locate the running binary so we can atomically replace it.
	exePath, err := os.Executable()
	if err != nil {
		return nil, &cliError{Message: fmt.Sprintf("could not locate the running binary: %v", err), Code: "UPGRADE_BINARY_RESOLVE_FAILED"}
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return nil, &cliError{Message: fmt.Sprintf("could not resolve symlinks for the running binary: %v", err), Code: "UPGRADE_BINARY_RESOLVE_FAILED"}
	}

	// Download archive + checksums into a scratch dir.
	tmpDir, err := os.MkdirTemp("", "agora-upgrade-*")
	if err != nil {
		return nil, &cliError{Message: fmt.Sprintf("could not create temp dir: %v", err), Code: "UPGRADE_DOWNLOAD_FAILED"}
	}
	defer os.RemoveAll(tmpDir)

	goos := runtime.GOOS
	goarch := runtime.GOARCH
	archiveName, ext, err := upgradeArchiveName(latest, goos, goarch)
	if err != nil {
		return nil, &cliError{Message: err.Error(), Code: "UPGRADE_UNSUPPORTED_PLATFORM"}
	}

	baseURL := strings.TrimRight(strings.TrimSpace(a.env["RELEASES_DOWNLOAD_BASE_URL"]), "/")
	if baseURL == "" {
		repo := strings.TrimSpace(a.env["GITHUB_REPO"])
		if repo == "" {
			repo = "AgoraIO/cli"
		}
		baseURL = fmt.Sprintf("https://github.com/%s/releases/download", repo)
	}

	archiveURL := fmt.Sprintf("%s/v%s/%s", baseURL, latest, archiveName)
	checksumsURL := fmt.Sprintf("%s/v%s/checksums.txt", baseURL, latest)

	archivePath := filepath.Join(tmpDir, archiveName)
	if err := downloadFile(archiveURL, archivePath, a.env); err != nil {
		return nil, &cliError{Message: fmt.Sprintf("could not download %s: %v", archiveURL, err), Code: "UPGRADE_DOWNLOAD_FAILED"}
	}
	checksumsPath := filepath.Join(tmpDir, "checksums.txt")
	if err := downloadFile(checksumsURL, checksumsPath, a.env); err != nil {
		return nil, &cliError{Message: fmt.Sprintf("could not download %s: %v", checksumsURL, err), Code: "UPGRADE_DOWNLOAD_FAILED"}
	}

	// Verify SHA-256.
	expected, err := expectedChecksumFor(checksumsPath, archiveName)
	if err != nil {
		return nil, &cliError{Message: fmt.Sprintf("could not parse checksums: %v", err), Code: "UPGRADE_CHECKSUM_FAILED"}
	}
	if expected == "" {
		return nil, &cliError{Message: fmt.Sprintf("checksum for %s not found in checksums.txt", archiveName), Code: "UPGRADE_CHECKSUM_FAILED"}
	}
	actual, err := sha256OfFile(archivePath)
	if err != nil {
		return nil, &cliError{Message: fmt.Sprintf("could not hash downloaded archive: %v", err), Code: "UPGRADE_CHECKSUM_FAILED"}
	}
	if !strings.EqualFold(expected, actual) {
		return nil, &cliError{
			Message: fmt.Sprintf("checksum mismatch for %s; expected %s, got %s", archiveName, expected, actual),
			Code:    "UPGRADE_CHECKSUM_FAILED",
		}
	}

	// Extract the new binary to a temp file next to the running one.
	binaryName := "agora"
	if goos == "windows" {
		binaryName = "agora.exe"
	}
	extractedPath := filepath.Join(tmpDir, binaryName)
	if ext == "tar.gz" {
		if err := extractFromTarGz(archivePath, binaryName, extractedPath); err != nil {
			return nil, &cliError{Message: fmt.Sprintf("could not extract %s: %v", binaryName, err), Code: "UPGRADE_EXTRACT_FAILED"}
		}
	} else {
		if err := extractFromZip(archivePath, binaryName, extractedPath); err != nil {
			return nil, &cliError{Message: fmt.Sprintf("could not extract %s: %v", binaryName, err), Code: "UPGRADE_EXTRACT_FAILED"}
		}
	}
	if err := os.Chmod(extractedPath, 0o755); err != nil {
		// chmod only applies meaningfully on Unix; ignore failures on Windows.
		if goos != "windows" {
			return nil, &cliError{Message: fmt.Sprintf("could not chmod new binary: %v", err), Code: "UPGRADE_INSTALL_FAILED"}
		}
	}

	// Atomic replace: copy extracted binary into a temp file in the same
	// directory as the current binary, then rename over the current binary.
	exeDir := filepath.Dir(exePath)
	tempDest := filepath.Join(exeDir, fmt.Sprintf(".agora.upgrade.%d.tmp", time.Now().UnixNano()))
	if err := copyFile(extractedPath, tempDest); err != nil {
		return nil, &cliError{Message: fmt.Sprintf("could not stage new binary: %v", err), Code: "UPGRADE_INSTALL_FAILED"}
	}
	// On Windows, os.Rename over a running .exe usually fails. Provide a clearer error.
	if err := os.Rename(tempDest, exePath); err != nil {
		_ = os.Remove(tempDest)
		if goos == "windows" {
			return nil, &cliError{
				Message: fmt.Sprintf("could not replace running binary at %s (Windows holds .exe files open). Close all agora processes and retry, or run `agora upgrade --check` to verify the new version is available.", exePath),
				Code:    "UPGRADE_INSTALL_FAILED",
			}
		}
		return nil, &cliError{Message: fmt.Sprintf("could not replace running binary at %s: %v", exePath, err), Code: "UPGRADE_INSTALL_FAILED"}
	}

	result["status"] = "upgraded"
	result["installedPath"] = exePath
	if receiptPath, err := writeInstallReceipt(exePath, latest, "agora upgrade"); err == nil {
		result["receiptPath"] = receiptPath
	} else {
		result["receiptWarning"] = fmt.Sprintf("could not write install receipt: %v", err)
	}
	return result, nil
}

func detectInstallProvenance(env map[string]string) installProvenance {
	exePath := ""
	if path, err := os.Executable(); err == nil {
		exePath = path
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			exePath = resolved
		}
	}
	return detectInstallProvenanceForPath(env, exePath)
}

func detectInstallProvenanceForPath(_ map[string]string, exePath string) installProvenance {
	exePath = filepath.Clean(strings.TrimSpace(exePath))
	if exePath != "" {
		receiptPath := installReceiptPath(exePath)
		if receipt, err := readInstallReceipt(receiptPath); err == nil && receipt.validForPath(exePath) {
			return installProvenance{
				Method:         receipt.InstallMethod,
				Source:         receipt.Source,
				InstalledPath:  exePath,
				ReceiptPath:    receiptPath,
				UpgradeCommand: upgradeCommandForInstallMethod(receipt.InstallMethod),
			}
		}
	}

	method, source := inferInstallMethodFromPath(exePath)
	return installProvenance{
		Method:         method,
		Source:         source,
		InstalledPath:  exePath,
		UpgradeCommand: upgradeCommandForInstallMethod(method),
	}
}

func installReceiptPath(exePath string) string {
	if strings.TrimSpace(exePath) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(exePath), installReceiptFileName)
}

func readInstallReceipt(path string) (installReceipt, error) {
	var receipt installReceipt
	if strings.TrimSpace(path) == "" {
		return receipt, os.ErrNotExist
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return receipt, err
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func writeInstallReceipt(exePath, installedVersion, source string) (string, error) {
	receiptPath := installReceiptPath(exePath)
	if receiptPath == "" {
		return "", errors.New("install receipt path is empty")
	}
	receipt := installReceipt{
		SchemaVersion: 1,
		Tool:          "agora",
		InstallMethod: "installer",
		InstallPath:   exePath,
		Version:       strings.TrimPrefix(strings.TrimSpace(installedVersion), "v"),
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
		Source:        source,
	}
	raw, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return "", err
	}
	raw = append(raw, '\n')
	tmpPath := filepath.Join(filepath.Dir(receiptPath), fmt.Sprintf(".%s.%d.tmp", installReceiptFileName, time.Now().UnixNano()))
	if err := os.WriteFile(tmpPath, raw, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmpPath, receiptPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return receiptPath, nil
}

func (r installReceipt) validForPath(exePath string) bool {
	if r.SchemaVersion != 1 || r.Tool != "agora" || strings.TrimSpace(r.InstallMethod) == "" {
		return false
	}
	if !knownInstallMethod(r.InstallMethod) {
		return false
	}
	if strings.TrimSpace(r.InstallPath) == "" || strings.TrimSpace(exePath) == "" {
		return false
	}
	return sameCleanPath(r.InstallPath, exePath)
}

func knownInstallMethod(method string) bool {
	switch method {
	case "installer", "npm", "homebrew", "scoop", "chocolatey", "winget", "unknown":
		return true
	default:
		return false
	}
}

func sameCleanPath(a, b string) bool {
	left := filepath.Clean(strings.TrimSpace(a))
	right := filepath.Clean(strings.TrimSpace(b))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func inferInstallMethodFromPath(exePath string) (string, string) {
	normalizedPath := strings.ToLower(filepath.ToSlash(filepath.Clean(strings.TrimSpace(exePath))))
	baseName := strings.ToLower(filepath.Base(strings.TrimSpace(exePath)))
	switch {
	case strings.Contains(normalizedPath, "/node_modules/"):
		return "npm", "path"
	case strings.Contains(normalizedPath, "/cellar/"):
		return "homebrew", "path"
	case strings.Contains(normalizedPath, "/scoop/shims/") || strings.Contains(normalizedPath, "/scoop/apps/"):
		return "scoop", "path"
	case strings.Contains(normalizedPath, "/chocolatey/bin/") || strings.Contains(normalizedPath, "/chocolatey/lib/"):
		return "chocolatey", "path"
	case strings.Contains(normalizedPath, "/winget/packages/"):
		return "winget", "path"
	case baseName != "agora" && baseName != "agora.exe":
		return "unknown", "fallback"
	default:
		return "installer", "fallback"
	}
}

func upgradeCommandForInstallMethod(method string) string {
	switch method {
	case "npm":
		return "npm update -g agoraio-cli"
	case "homebrew":
		return "brew upgrade agoraio/tap/agora-cli"
	case "scoop":
		return "scoop update agora"
	case "chocolatey":
		return "choco upgrade agora"
	case "winget":
		return "winget upgrade Agora.Cli"
	case "installer":
		if runtime.GOOS == "windows" {
			return "irm https://raw.githubusercontent.com/AgoraIO/cli/main/install.ps1 | iex"
		}
		return "curl -fsSL https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh | sh"
	default:
		if runtime.GOOS == "windows" {
			return "irm https://raw.githubusercontent.com/AgoraIO/cli/main/install.ps1 | iex"
		}
		return "curl -fsSL https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh | sh"
	}
}

// upgradeArchiveName returns the GoReleaser archive filename for the given
// version + platform and its extension ("tar.gz" or "zip"). It mirrors the
// naming in .goreleaser.yaml.
func upgradeArchiveName(version, goos, goarch string) (string, string, error) {
	switch goos {
	case "windows":
		return fmt.Sprintf("agora-cli-go_v%s_%s_%s.zip", version, goos, goarch), "zip", nil
	case "darwin", "linux":
		return fmt.Sprintf("agora-cli-go_v%s_%s_%s.tar.gz", version, goos, goarch), "tar.gz", nil
	default:
		return "", "", fmt.Errorf("self-update is not supported on %s/%s; use the platform installer", goos, goarch)
	}
}

// resolveLatestVersion queries the GitHub API for the latest release tag and
// returns the bare version (no leading "v"). Honors GITHUB_API_URL and
// GITHUB_TOKEN/GH_TOKEN for testing and rate-limit relief.
func resolveLatestVersion(env map[string]string) (string, error) {
	apiBase := strings.TrimRight(strings.TrimSpace(env["GITHUB_API_URL"]), "/")
	if apiBase == "" {
		apiBase = "https://api.github.com"
	}
	repo := strings.TrimSpace(env["GITHUB_REPO"])
	if repo == "" {
		repo = "AgoraIO/cli"
	}
	url := fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repo)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := strings.TrimSpace(env["GITHUB_TOKEN"]); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if token := strings.TrimSpace(env["GH_TOKEN"]); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	tag := strings.TrimPrefix(strings.TrimSpace(payload.TagName), "v")
	if tag == "" {
		return "", errors.New("GitHub API returned an empty tag name")
	}
	return tag, nil
}

// downloadFile fetches url to dest. Honors GITHUB_TOKEN/GH_TOKEN.
func downloadFile(url, dest string, env map[string]string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	if token := strings.TrimSpace(env["GITHUB_TOKEN"]); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if token := strings.TrimSpace(env["GH_TOKEN"]); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func expectedChecksumFor(checksumsPath, archiveName string) (string, error) {
	f, err := os.Open(checksumsPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == archiveName {
			return strings.ToLower(fields[0]), nil
		}
	}
	return "", nil
}

func sha256OfFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func extractFromTarGz(archivePath, member, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("%s not found in archive", member)
		}
		if err != nil {
			return err
		}
		if filepath.Base(hdr.Name) == member && hdr.Typeflag == tar.TypeReg {
			out, err := os.Create(dest)
			if err != nil {
				return err
			}
			defer out.Close()
			// Cap extraction at maxExtractedBinaryBytes (gosec G110: bound
			// the decompressed size so a malicious archive cannot exhaust
			// disk). The CLI binary is well under the cap.
			if _, err = io.Copy(out, io.LimitReader(tr, maxExtractedBinaryBytes+1)); err != nil {
				return err
			}
			info, statErr := out.Stat()
			if statErr != nil {
				return statErr
			}
			if info.Size() > maxExtractedBinaryBytes {
				return fmt.Errorf("extracted binary %s exceeds %d byte safety cap (refusing to install)", member, maxExtractedBinaryBytes)
			}
			return nil
		}
	}
}

func extractFromZip(archivePath, member, dest string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, file := range zr.File {
		if filepath.Base(file.Name) != member {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		out, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer out.Close()
		// Cap extraction at maxExtractedBinaryBytes (gosec G110).
		if _, err = io.Copy(out, io.LimitReader(rc, maxExtractedBinaryBytes+1)); err != nil {
			return err
		}
		info, statErr := out.Stat()
		if statErr != nil {
			return statErr
		}
		if info.Size() > maxExtractedBinaryBytes {
			return fmt.Errorf("extracted binary %s exceeds %d byte safety cap (refusing to install)", member, maxExtractedBinaryBytes)
		}
		return nil
	}
	return fmt.Errorf("%s not found in archive", member)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// isSameOrNewer compares two semver-ish strings naively. Returns true if
// current >= latest. Both inputs may be prefixed with "v" or be "dev".
// Pre-release tags ("0.1.7-rc.1") are treated as older than their base version.
//
// This intentionally does not use a full semver library to keep the binary
// small; the comparison is good enough for the upgrade short-circuit.
func isSameOrNewer(current, latest string) bool {
	current = strings.TrimPrefix(strings.TrimSpace(current), "v")
	latest = strings.TrimPrefix(strings.TrimSpace(latest), "v")
	if current == "" || current == "dev" {
		return false
	}
	if current == latest {
		return true
	}
	// Strip prerelease suffixes for comparison; an exact match was handled above.
	currentBase := strings.SplitN(current, "-", 2)[0]
	latestBase := strings.SplitN(latest, "-", 2)[0]
	cParts := strings.Split(currentBase, ".")
	lParts := strings.Split(latestBase, ".")
	for i := 0; i < len(cParts) || i < len(lParts); i++ {
		var c, l int
		if i < len(cParts) {
			c, _ = atoiSafe(cParts[i])
		}
		if i < len(lParts) {
			l, _ = atoiSafe(lParts[i])
		}
		if c > l {
			return true
		}
		if c < l {
			return false
		}
	}
	return true
}

func atoiSafe(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n, fmt.Errorf("non-numeric")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}
