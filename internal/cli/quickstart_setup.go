package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var pinnedPackageManagerPattern = regexp.MustCompile(`^(pnpm)@([0-9]+\.[0-9]+\.[0-9]+)$`)

type quickstartPackageManagerResult struct {
	Name            string `json:"name"`
	RequiredVersion string `json:"requiredVersion"`
	DetectedVersion string `json:"detectedVersion,omitempty"`
	SelectedName    string `json:"selectedName,omitempty"`
	SelectedVersion string `json:"selectedVersion,omitempty"`
	Strategy        string `json:"strategy"`
	Ready           bool   `json:"ready"`
	Message         string `json:"message,omitempty"`
}

type quickstartSetup struct {
	NextSteps      []string
	PackageManager *quickstartPackageManagerResult
}

type quickstartToolProbe func(root, command string) (version string, available bool)

func quickstartPackageManagerSummary(packageManager *quickstartPackageManagerResult) string {
	if packageManager == nil {
		return ""
	}
	base := packageManager.Name + " " + packageManager.RequiredVersion
	if packageManager.Strategy == "unavailable" {
		return base + " unavailable"
	}
	if packageManager.SelectedName == packageManager.Name {
		return base
	}
	return base + " via " + packageManager.SelectedName + " " + packageManager.SelectedVersion
}

func probeQuickstartTool(root, command string) (string, bool) {
	path, err := exec.LookPath(command)
	if err != nil {
		return "", false
	}
	cmd := exec.Command(path, "--version")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(output)), true
}

func resolveQuickstartSetup(template quickstartTemplate, targetDir string, probe quickstartToolProbe) quickstartSetup {
	setup := quickstartSetup{NextSteps: initNextSteps(template, targetDir)}
	if template.ID != "nextjs-video-call" {
		return setup
	}
	name, version, ok := readQuickstartPackageManager(targetDir)
	if !ok || name != "pnpm" {
		return setup
	}
	detectedVersion, available := probe(targetDir, name)
	packageManager := &quickstartPackageManagerResult{
		Name:            name,
		RequiredVersion: version,
		DetectedVersion: detectedVersion,
	}
	setup.PackageManager = packageManager
	if available && detectedVersion == version {
		setup.NextSteps = []string{
			"cd " + filepath.Base(targetDir),
			"pnpm install --frozen-lockfile",
			template.RunCommand,
		}
		packageManager.Strategy = "native"
		packageManager.SelectedName = name
		packageManager.SelectedVersion = detectedVersion
		packageManager.Ready = true
		return setup
	}
	if npmVersion, npmAvailable := probe(targetDir, "npm"); npmAvailable {
		setup.NextSteps = []string{
			"cd " + filepath.Base(targetDir),
			"npm install --package-lock=false",
			"npm run dev",
		}
		packageManager.Strategy = "npm"
		packageManager.SelectedName = "npm"
		packageManager.SelectedVersion = npmVersion
		packageManager.Ready = true
		return setup
	}
	setup.NextSteps = []string{"cd " + filepath.Base(targetDir)}
	packageManager.Strategy = "unavailable"
	packageManager.Message = "pnpm " + version + " is unavailable and npm was not found; install Node.js with npm or install pnpm, then rerun the setup commands."
	return setup
}

func readQuickstartPackageManager(root string) (name, version string, ok bool) {
	raw, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return "", "", false
	}
	var manifest struct {
		PackageManager string `json:"packageManager"`
	}
	if json.Unmarshal(raw, &manifest) != nil {
		return "", "", false
	}
	parts := pinnedPackageManagerPattern.FindStringSubmatch(strings.TrimSpace(manifest.PackageManager))
	if len(parts) != 3 {
		return "", "", false
	}
	return parts[1], parts[2], true
}
