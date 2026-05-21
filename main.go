package main

import (
	"fmt"
	"os"

	"github.com/AgoraIO/cli/internal/cli"
)

func main() {
	app, err := cli.NewApp()
	if err != nil {
		if cli.JSONRequested(os.Args[1:]) {
			_ = cli.EmitJSONError("agora", err, 1, "")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if err := app.Execute(); err != nil {
		if code, ok := cli.ExitCode(err); ok {
			os.Exit(code)
		}
		if cli.ErrorRendered(err) {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
