package cli

import (
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
