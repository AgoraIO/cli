package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteCredentialEnvSharedBehavior(t *testing.T) {
	certificate := "cert_new"
	project := projectDetail{Name: "Demo", AppID: "app_new", SignKey: &certificate}

	t.Run("standard layout preserves user values and normalizes legacy keys", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".env")
		existing := "USER_VALUE=keep\nAPP_ID=old\nAPP_CERTIFICATE=old\nNEXT_PUBLIC_AGORA_APP_ID=old\n"
		if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := writeCredentialEnv(credentialEnvTarget{
			Path:              path,
			AppIDKey:          "AGORA_APP_ID",
			AppCertificateKey: "AGORA_APP_CERTIFICATE",
		}, project, credentialEnvWriteOptions{AllowAppend: true})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != "updated" {
			t.Fatalf("status = %q, want updated", result.Status)
		}
		content := readCredentialEnvTestFile(t, path)
		for _, want := range []string{
			"USER_VALUE=keep",
			"AGORA_APP_ID=app_new",
			"AGORA_APP_CERTIFICATE=cert_new",
			"# Replaced by Agora CLI: APP_ID=old",
			"# Replaced by Agora CLI: APP_CERTIFICATE=old",
			"# Replaced by Agora CLI: NEXT_PUBLIC_AGORA_APP_ID=old",
		} {
			if !strings.Contains(content, want) {
				t.Errorf("env does not contain %q: %s", want, content)
			}
		}
	})

	t.Run("managed target seeds its example and reports created", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".env.local")
		example := filepath.Join(dir, ".env.local.example")
		if err := os.WriteFile(example, []byte("NEXT_PUBLIC_AGORA_APP_ID=\nNEXT_AGORA_APP_CERTIFICATE=\nUSER_VALUE=keep\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := writeCredentialEnv(credentialEnvTarget{
			Path:              path,
			ExamplePath:       example,
			AppIDKey:          "NEXT_PUBLIC_AGORA_APP_ID",
			AppCertificateKey: "NEXT_AGORA_APP_CERTIFICATE",
		}, project, credentialEnvWriteOptions{AllowAppend: true})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != "created" {
			t.Fatalf("status = %q, want created", result.Status)
		}
		content := readCredentialEnvTestFile(t, path)
		if !strings.Contains(content, "USER_VALUE=keep") || !strings.Contains(content, "NEXT_PUBLIC_AGORA_APP_ID=app_new") || !strings.Contains(content, "NEXT_AGORA_APP_CERTIFICATE=cert_new") {
			t.Fatalf("unexpected seeded env: %s", content)
		}
	})

	t.Run("explicit non-default file still requires append", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "custom.credentials")
		if err := os.WriteFile(path, []byte("USER_VALUE=keep\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := writeCredentialEnv(credentialEnvTarget{
			Path:              path,
			AppIDKey:          "AGORA_APP_ID",
			AppCertificateKey: "AGORA_APP_CERTIFICATE",
		}, project, credentialEnvWriteOptions{})
		if err == nil || !strings.Contains(err.Error(), "Use --append") {
			t.Fatalf("error = %v, want append guidance", err)
		}
	})

	t.Run("overwrite removes unrelated values", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".env")
		if err := os.WriteFile(path, []byte("USER_VALUE=remove\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := writeCredentialEnv(credentialEnvTarget{
			Path:              path,
			AppIDKey:          "AGORA_APP_ID",
			AppCertificateKey: "AGORA_APP_CERTIFICATE",
		}, project, credentialEnvWriteOptions{Overwrite: true})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != "overwritten" || strings.Contains(readCredentialEnvTestFile(t, path), "USER_VALUE") {
			t.Fatalf("unexpected overwrite result: %+v", result)
		}
	})

	t.Run("missing certificate is rejected before writing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".env")
		_, err := writeCredentialEnv(credentialEnvTarget{
			Path:              path,
			AppIDKey:          "AGORA_APP_ID",
			AppCertificateKey: "AGORA_APP_CERTIFICATE",
		}, projectDetail{Name: "No Certificate", AppID: "app"}, credentialEnvWriteOptions{AllowAppend: true})
		if err == nil || !strings.Contains(err.Error(), "does not have an app certificate") {
			t.Fatalf("error = %v, want missing certificate error", err)
		}
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("credential file was written despite missing certificate: %v", statErr)
		}
	})
}

func readCredentialEnvTestFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
