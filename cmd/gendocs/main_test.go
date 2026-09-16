package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunWritesAndChecksCommandReference(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "commands.md")
	var stderr bytes.Buffer
	if exitCode := run([]string{"-o", outputPath}, &stderr); exitCode != 0 {
		t.Fatalf("run(write) exit = %d, stderr = %s", exitCode, stderr.String())
	}
	generated, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Contains(generated, []byte("agora quickstart create")) {
		t.Fatalf("generated reference does not contain quickstart create")
	}
	if !strings.Contains(stderr.String(), "gendocs: wrote") {
		t.Fatalf("write stderr = %q", stderr.String())
	}

	stderr.Reset()
	if exitCode := run([]string{"-check", "-o", outputPath}, &stderr); exitCode != 0 {
		t.Fatalf("run(check) exit = %d, stderr = %s", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "is up to date") {
		t.Fatalf("check stderr = %q", stderr.String())
	}
}

func TestRunReportsGeneratorFailures(t *testing.T) {
	originalRoot := newRootForDocs
	originalRender := renderCommandReference
	t.Cleanup(func() {
		newRootForDocs = originalRoot
		renderCommandReference = originalRender
	})

	newRootForDocs = func() (*cobra.Command, error) {
		return nil, errors.New("root failed")
	}
	var stderr bytes.Buffer
	if exitCode := run(nil, &stderr); exitCode != 1 || !strings.Contains(stderr.String(), "failed to build root command") {
		t.Fatalf("root failure = exit %d, stderr %s", exitCode, stderr.String())
	}

	newRootForDocs = originalRoot
	renderCommandReference = func(io.Writer, *cobra.Command) error {
		return errors.New("render failed")
	}
	stderr.Reset()
	if exitCode := run(nil, &stderr); exitCode != 1 || !strings.Contains(stderr.String(), "render failed") {
		t.Fatalf("render failure = exit %d, stderr %s", exitCode, stderr.String())
	}
}

func TestRunReportsDriftAndFileFailures(t *testing.T) {
	directory := t.TempDir()
	stalePath := filepath.Join(directory, "stale.md")
	if err := os.WriteFile(stalePath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{name: "drift", args: []string{"-check", "-o", stalePath}, code: 1, want: "out of date"},
		{name: "missing check file", args: []string{"-check", "-o", filepath.Join(directory, "missing.md")}, code: 2, want: "cannot read"},
		{name: "write failure", args: []string{"-o", directory}, code: 1, want: "failed to write"},
		{name: "invalid flag", args: []string{"-unknown"}, code: 2, want: "flag provided but not defined"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if exitCode := run(tt.args, &stderr); exitCode != tt.code {
				t.Fatalf("run() exit = %d, want %d; stderr = %s", exitCode, tt.code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr does not contain %q: %s", tt.want, stderr.String())
			}
		})
	}
}
