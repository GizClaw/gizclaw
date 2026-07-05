package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	rpcgen "github.com/GizClaw/gizclaw-go/tools/gzc-rpcgen/internal"
)

type includeFlags []string

func (f *includeFlags) String() string {
	return fmt.Sprint([]string(*f))
}

func (f *includeFlags) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	var includes includeFlags
	cfg := rpcgen.Config{}
	flags := flag.NewFlagSet("gzc-rpcgen", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.SchemaPath, "schema", "api/rpc.json", "Source RPC OpenAPI schema")
	flags.Var(&includes, "include", "Additional schema include root")
	flags.StringVar(&cfg.OutDir, "out", "c/gizclaw/generated", "Generated C output directory")
	flags.StringVar(&cfg.Package, "package", "gzc", "C symbol prefix")
	flags.BoolVar(&cfg.Check, "check", false, "Verify generated files are up to date")
	flags.BoolVar(&cfg.Format, "format", true, "Format generated output")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	cfg.IncludeDirs = includes

	if err := rpcgen.Run(cfg); err != nil {
		fmt.Fprintf(stderr, "gzc-rpcgen: %v\n", err)
		return 1
	}
	return 0
}
