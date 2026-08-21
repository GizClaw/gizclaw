package main

import (
	"errors"
	"io"
	"os"

	"github.com/GizClaw/gizclaw-go/cmd/internal/commands"
)

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		os.Exit(exitCode(err))
	}
}

func run(args []string, stderr io.Writer) error {
	root := commands.New()
	root.SetArgs(args)
	root.SetErr(stderr)
	return root.Execute()
}

func exitCode(err error) int {
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return 1
}
