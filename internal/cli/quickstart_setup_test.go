package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveQuickstartSetupUsesMatchingPNPM(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"packageManager":"pnpm@9.15.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	template, err := selectQuickstartDefinition("nextjs", "video-call")
	if err != nil {
		t.Fatal(err)
	}

	setup := resolveQuickstartSetup(template, root, func(_ string, command string) (string, bool) {
		if command == "pnpm" {
			return "9.15.9", true
		}
		return "", false
	})

	wantSteps := []string{"cd " + filepath.Base(root), "pnpm install --frozen-lockfile", "pnpm dev"}
	if !reflect.DeepEqual(setup.NextSteps, wantSteps) {
		t.Fatalf("next steps:\n got: %#v\nwant: %#v", setup.NextSteps, wantSteps)
	}
	if setup.PackageManager == nil || setup.PackageManager.Strategy != "native" || !setup.PackageManager.Ready || setup.PackageManager.RequiredVersion != "9.15.9" || setup.PackageManager.DetectedVersion != "9.15.9" || setup.PackageManager.SelectedName != "pnpm" || setup.PackageManager.SelectedVersion != "9.15.9" {
		t.Fatalf("unexpected package manager result: %+v", setup.PackageManager)
	}
}

func TestResolveQuickstartSetupFallsBackToNPMWhenPNPMIsMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"packageManager":"pnpm@9.15.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	template, err := selectQuickstartDefinition("nextjs", "video-call")
	if err != nil {
		t.Fatal(err)
	}

	setup := resolveQuickstartSetup(template, root, func(_ string, command string) (string, bool) {
		if command == "npm" {
			return "10.9.4", true
		}
		return "", false
	})

	wantSteps := []string{
		"cd " + filepath.Base(root),
		"npm install --package-lock=false",
		"npm run dev",
	}
	if !reflect.DeepEqual(setup.NextSteps, wantSteps) {
		t.Fatalf("next steps:\n got: %#v\nwant: %#v", setup.NextSteps, wantSteps)
	}
	if setup.PackageManager == nil || setup.PackageManager.Strategy != "npm" || !setup.PackageManager.Ready || setup.PackageManager.DetectedVersion != "" || setup.PackageManager.SelectedName != "npm" || setup.PackageManager.SelectedVersion != "10.9.4" {
		t.Fatalf("unexpected package manager result: %+v", setup.PackageManager)
	}
}

func TestResolveQuickstartSetupFallsBackToNPMWhenPNPMVersionDiffers(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"packageManager":"pnpm@9.15.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	template, err := selectQuickstartDefinition("nextjs", "video-call")
	if err != nil {
		t.Fatal(err)
	}

	setup := resolveQuickstartSetup(template, root, func(_ string, command string) (string, bool) {
		if command == "pnpm" {
			return "8.15.9", true
		}
		if command == "npm" {
			return "10.9.4", true
		}
		return "", false
	})

	if setup.PackageManager == nil || setup.PackageManager.Strategy != "npm" || setup.PackageManager.DetectedVersion != "8.15.9" || setup.PackageManager.SelectedName != "npm" || setup.PackageManager.SelectedVersion != "10.9.4" {
		t.Fatalf("unexpected package manager result: %+v", setup.PackageManager)
	}
	if got := setup.NextSteps[1]; got != "npm install --package-lock=false" {
		t.Fatalf("install step = %q", got)
	}
}

func TestResolveQuickstartSetupReportsUnavailableWithoutPNPMOrNPM(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"packageManager":"pnpm@9.15.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	template, err := selectQuickstartDefinition("nextjs", "video-call")
	if err != nil {
		t.Fatal(err)
	}

	setup := resolveQuickstartSetup(template, root, func(_ string, _ string) (string, bool) {
		return "", false
	})

	wantSteps := []string{"cd " + filepath.Base(root)}
	if !reflect.DeepEqual(setup.NextSteps, wantSteps) {
		t.Fatalf("next steps:\n got: %#v\nwant: %#v", setup.NextSteps, wantSteps)
	}
	if setup.PackageManager == nil || setup.PackageManager.Strategy != "unavailable" || setup.PackageManager.Ready || setup.PackageManager.Message == "" {
		t.Fatalf("unexpected package manager result: %+v", setup.PackageManager)
	}
}

func TestResolveQuickstartSetupDoesNotInterpolateMalformedPackageManager(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"packageManager":"pnpm@9.15.9 && echo unsafe"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	template, err := selectQuickstartDefinition("nextjs", "video-call")
	if err != nil {
		t.Fatal(err)
	}

	setup := resolveQuickstartSetup(template, root, func(_ string, command string) (string, bool) {
		return "", command == "npm"
	})

	wantSteps := []string{"cd " + filepath.Base(root), "pnpm install", "pnpm dev"}
	if !reflect.DeepEqual(setup.NextSteps, wantSteps) || setup.PackageManager != nil {
		t.Fatalf("malformed declaration must use safe catalog steps, got %+v", setup)
	}
}

func TestResolveQuickstartSetupLeavesOtherQuickstartsUnchanged(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"packageManager":"pnpm@10.23.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	template, err := selectQuickstartDefinition("go", "voice-agent")
	if err != nil {
		t.Fatal(err)
	}

	setup := resolveQuickstartSetup(template, root, func(_ string, _ string) (string, bool) {
		return "10.23.0", true
	})

	wantSteps := []string{"cd " + filepath.Base(root), "make setup", "make dev"}
	if !reflect.DeepEqual(setup.NextSteps, wantSteps) || setup.PackageManager != nil {
		t.Fatalf("non-RTC quickstart setup changed: %+v", setup)
	}
}
