package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRenderCommandReferenceFromPublicRoot(t *testing.T) {
	root, err := NewRootForDocs()
	if err != nil {
		t.Fatalf("NewRootForDocs() error = %v", err)
	}

	var out bytes.Buffer
	if err := RenderCommandReference(&out, root); err != nil {
		t.Fatalf("RenderCommandReference() error = %v", err)
	}

	document := out.String()
	for _, want := range []string{
		"## `agora quickstart create`",
		"`--template-only`",
		"## `agora project doctor`",
	} {
		if !strings.Contains(document, want) {
			t.Errorf("generated command reference does not contain %q", want)
		}
	}
}

func TestRenderCommandReferenceReturnsWriteError(t *testing.T) {
	root, err := NewRootForDocs()
	if err != nil {
		t.Fatalf("NewRootForDocs() error = %v", err)
	}

	wantErr := errors.New("write failed")
	err = RenderCommandReference(errorWriter{err: wantErr}, root)
	if !errors.Is(err, wantErr) {
		t.Fatalf("RenderCommandReference() error = %v, want %v", err, wantErr)
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
