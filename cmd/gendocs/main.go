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
	"os"

	"github.com/AgoraIO/cli/internal/cli"
)

func main() {
	out := flag.String("o", "docs/commands.md", "destination markdown file")
	check := flag.Bool("check", false, "exit non-zero if the destination file would change (used in CI to detect drift)")
	flag.Parse()

	root, err := cli.NewRootForDocs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gendocs: failed to build root command: %v\n", err)
		os.Exit(1)
	}

	var buffer bytes.Buffer
	if err := cli.RenderCommandReference(&buffer, root); err != nil {
		fmt.Fprintf(os.Stderr, "gendocs: render failed: %v\n", err)
		os.Exit(1)
	}

	if *check {
		existing, err := os.ReadFile(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gendocs: cannot read %s for drift check: %v\n", *out, err)
			fmt.Fprintln(os.Stderr, "Hint: run `make docs-commands` to generate it.")
			os.Exit(2)
		}
		if !bytes.Equal(existing, buffer.Bytes()) {
			fmt.Fprintf(os.Stderr, "gendocs: %s is out of date.\n", *out)
			fmt.Fprintln(os.Stderr, "Run `make docs-commands` and commit the result.")
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "gendocs: %s is up to date.\n", *out)
		return
	}

	if err := os.WriteFile(*out, buffer.Bytes(), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gendocs: failed to write %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "gendocs: wrote %s (%d bytes)\n", *out, buffer.Len())
}
