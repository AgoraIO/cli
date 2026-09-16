// gendocs regenerates docs/commands.md from the live cobra command tree.
//
// Usage (from repo root):
//
//	go run ./cmd/gendocs                  # write docs/commands.md
//	go run ./cmd/gendocs -o /tmp/cmd.md   # custom path
//	go run ./cmd/gendocs -check           # exit non-zero if the file would change
//
// CI uses -check on every PR to fail when somebody adds a command without
// regenerating the reference. The release workflow uses the default mode
// to ship a fresh page alongside the binary.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/AgoraIO/cli/internal/cli"
)

var (
	newRootForDocs         = cli.NewRootForDocs
	renderCommandReference = cli.RenderCommandReference
)

func main() {
	if exitCode := run(os.Args[1:], os.Stderr); exitCode != 0 {
		os.Exit(exitCode)
	}
}

func run(args []string, stderr io.Writer) int {
	flags := flag.NewFlagSet("gendocs", flag.ContinueOnError)
	flags.SetOutput(stderr)
	out := flags.String("o", "docs/commands.md", "destination markdown file")
	check := flags.Bool("check", false, "exit non-zero if the destination file would change (used in CI to detect drift)")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	root, err := newRootForDocs()
	if err != nil {
		fmt.Fprintf(stderr, "gendocs: failed to build root command: %v\n", err)
		return 1
	}

	var buffer bytes.Buffer
	if err := renderCommandReference(&buffer, root); err != nil {
		fmt.Fprintf(stderr, "gendocs: render failed: %v\n", err)
		return 1
	}

	if *check {
		existing, err := os.ReadFile(*out)
		if err != nil {
			fmt.Fprintf(stderr, "gendocs: cannot read %s for drift check: %v\n", *out, err)
			fmt.Fprintln(stderr, "Hint: run `make docs-commands` to generate it.")
			return 2
		}
		if !bytes.Equal(existing, buffer.Bytes()) {
			fmt.Fprintf(stderr, "gendocs: %s is out of date.\n", *out)
			fmt.Fprintln(stderr, "Run `make docs-commands` and commit the result.")
			return 1
		}
		fmt.Fprintf(stderr, "gendocs: %s is up to date.\n", *out)
		return 0
	}

	if err := os.WriteFile(*out, buffer.Bytes(), 0o644); err != nil {
		fmt.Fprintf(stderr, "gendocs: failed to write %s: %v\n", *out, err)
		return 1
	}
	fmt.Fprintf(stderr, "gendocs: wrote %s (%d bytes)\n", *out, buffer.Len())
	return 0
}
