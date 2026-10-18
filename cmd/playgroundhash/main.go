// Command playgroundhash computes a stable hash over the frontend/ source
// inputs. `-write` stamps it into webassets/SOURCE_HASH; `-check` compares.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	write := flag.Bool("write", false, "write SOURCE_HASH")
	check := flag.Bool("check", false, "check SOURCE_HASH")
	flag.Parse()

	sum, err := hashFrontend("frontend")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	target := filepath.Join("internal", "cli", "playground", "webassets", "SOURCE_HASH")
	switch {
	case *write:
		if err := os.WriteFile(target, []byte(sum+"\n"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
	case *check:
		got, err := os.ReadFile(target)
		if err != nil || strings.TrimSpace(string(got)) != sum {
			fmt.Fprintln(os.Stderr, "frontend/ changed but webassets not rebuilt. Run `make playground-build`.")
			os.Exit(1)
		}
	default:
		fmt.Println(sum)
	}
}

func hashFrontend(root string) (string, error) {
	var files []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "node_modules" || info.Name() == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\n", f)
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
