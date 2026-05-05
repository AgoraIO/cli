package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectProjectEnvCredentialLayout(t *testing.T) {
	t.Run("explicit nextjs", func(t *testing.T) {
		layout, err := detectProjectEnvCredentialLayout(t.TempDir(), "nextjs")
		if err != nil || layout != projectEnvLayoutNextjs {
			t.Fatalf("expected nextjs layout, got %v err %v", layout, err)
		}
	})
	t.Run("explicit standard", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"next":"15.0.0"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		layout, err := detectProjectEnvCredentialLayout(dir, "standard")
		if err != nil || layout != projectEnvLayoutStandard {
			t.Fatalf("expected standard layout, got %v err %v", layout, err)
		}
	})
	t.Run("unknown template", func(t *testing.T) {
		_, err := detectProjectEnvCredentialLayout(t.TempDir(), "rails")
		if err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("package.json next dependency", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"next":"15.0.0","react":"19.0.0"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		layout, err := detectProjectEnvCredentialLayout(dir, "")
		if err != nil || layout != projectEnvLayoutNextjs {
			t.Fatalf("expected nextjs, got %v err %v", layout, err)
		}
	})
	t.Run("next.config without package in same dir", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "next.config.mjs"), []byte("export default {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		layout, err := detectProjectEnvCredentialLayout(dir, "")
		if err != nil || layout != projectEnvLayoutNextjs {
			t.Fatalf("expected nextjs from next.config, got %v err %v", layout, err)
		}
	})
	t.Run("parent package.json next app", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"next":"15.0.0"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		nested := filepath.Join(root, "apps", "web")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		layout, err := detectProjectEnvCredentialLayout(nested, "")
		if err != nil || layout != projectEnvLayoutNextjs {
			t.Fatalf("expected nextjs from parent package.json, got %v err %v", layout, err)
		}
	})
	t.Run("local binding projectType", func(t *testing.T) {
		dir := t.TempDir()
		if err := writeLocalProjectBinding(dir, localProjectBinding{ProjectType: "standard"}); err != nil {
			t.Fatal(err)
		}
		layout, err := detectProjectEnvCredentialLayout(dir, "")
		if err != nil || layout != projectEnvLayoutStandard {
			t.Fatalf("expected standard from projectType, got %v err %v", layout, err)
		}
	})
	t.Run("local binding projectType nextjs without package", func(t *testing.T) {
		dir := t.TempDir()
		if err := writeLocalProjectBinding(dir, localProjectBinding{ProjectType: "nextjs"}); err != nil {
			t.Fatal(err)
		}
		layout, err := detectProjectEnvCredentialLayout(dir, "")
		if err != nil || layout != projectEnvLayoutNextjs {
			t.Fatalf("expected nextjs from projectType, got %v err %v", layout, err)
		}
	})
	t.Run("local binding template nextjs", func(t *testing.T) {
		dir := t.TempDir()
		if err := writeLocalProjectBinding(dir, localProjectBinding{Template: "nextjs"}); err != nil {
			t.Fatal(err)
		}
		layout, err := detectProjectEnvCredentialLayout(dir, "")
		if err != nil || layout != projectEnvLayoutNextjs {
			t.Fatalf("expected nextjs from binding, got %v err %v", layout, err)
		}
	})
	t.Run("env.local.example at root", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "env.local.example"), []byte("NEXT_PUBLIC_AGORA_APP_ID=\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		layout, err := detectProjectEnvCredentialLayout(dir, "")
		if err != nil || layout != projectEnvLayoutNextjs {
			t.Fatalf("expected nextjs from env.local.example, got %v err %v", layout, err)
		}
	})
}

func TestProjectCredentialEnvValuesForLayout(t *testing.T) {
	cert := "signkey"
	p := projectDetail{Name: "P", ProjectID: "prj", AppID: "app_x", SignKey: &cert}
	stdVals, err := projectCredentialEnvValuesForLayout(p, projectEnvLayoutStandard)
	if err != nil {
		t.Fatal(err)
	}
	if stdVals["AGORA_APP_ID"] != "app_x" || stdVals["AGORA_APP_CERTIFICATE"] != cert {
		t.Fatalf("unexpected standard values: %+v", stdVals)
	}
	nextVals, err := projectCredentialEnvValuesForLayout(p, projectEnvLayoutNextjs)
	if err != nil {
		t.Fatal(err)
	}
	if nextVals["NEXT_PUBLIC_AGORA_APP_ID"] != "app_x" || nextVals["NEXT_AGORA_APP_CERTIFICATE"] != cert {
		t.Fatalf("unexpected nextjs values: %+v", nextVals)
	}
}

func TestSyncLocalProjectBindingAfterEnvWrite(t *testing.T) {
	dir := t.TempDir()
	cert := "cert_val"
	p := projectDetail{ProjectID: "prj_x", Name: "Proj", AppID: "app_x", SignKey: &cert}
	target := projectTarget{project: p, region: "global"}
	if err := writeLocalProjectBinding(dir, localProjectBinding{ProjectID: "prj_x", ProjectName: "Proj", Region: "global"}); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(dir, ".env.local")
	updated, metadataPath, err := syncLocalProjectBindingAfterEnvWrite(dir, dir, envFile, target, "nextjs")
	if err != nil || !updated {
		t.Fatalf("expected metadata update, err=%v updated=%v", err, updated)
	}
	if metadataPath != ".agora/project.json" {
		t.Fatalf("expected metadata path, got %q", metadataPath)
	}
	binding, err := loadLocalProjectBinding(dir)
	if err != nil {
		t.Fatal(err)
	}
	if binding.ProjectType != "nextjs" || binding.EnvPath != ".env.local" {
		t.Fatalf("unexpected binding: %+v", binding)
	}
	again, _, err := syncLocalProjectBindingAfterEnvWrite(dir, dir, envFile, target, "nextjs")
	if err != nil || again {
		t.Fatalf("expected idempotent skip, err=%v again=%v", err, again)
	}

	otherDir := t.TempDir()
	if err := writeLocalProjectBinding(otherDir, localProjectBinding{ProjectID: "prj_x", Template: "python"}); err != nil {
		t.Fatal(err)
	}
	skip, _, err := syncLocalProjectBindingAfterEnvWrite(otherDir, otherDir, filepath.Join(otherDir, "server", ".env"), target, "standard")
	if err != nil || !skip {
		t.Fatalf("expected binding project details refresh even when template set, err=%v updated=%v", err, skip)
	}

	createDir := t.TempDir()
	created, createdPath, err := syncLocalProjectBindingAfterEnvWrite(createDir, createDir, filepath.Join(createDir, "apps", "web", ".env.local"), target, "nextjs")
	if err != nil || !created {
		t.Fatalf("expected missing binding create, err=%v updated=%v", err, created)
	}
	if createdPath != ".agora/project.json" {
		t.Fatalf("unexpected created metadata path: %q", createdPath)
	}
	createdBinding, err := loadLocalProjectBinding(createDir)
	if err != nil {
		t.Fatal(err)
	}
	if createdBinding.ProjectID != "prj_x" || createdBinding.ProjectType != "nextjs" || createdBinding.EnvPath != "apps/web/.env.local" {
		t.Fatalf("unexpected created binding: %+v", createdBinding)
	}
}
