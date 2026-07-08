package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	rpcgen "github.com/GizClaw/gizclaw-go/tools/gzc-rpcgen/internal"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	cfg := rpcgen.Config{}
	flags := flag.NewFlagSet("gzc-rpcgen", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.ProtoPath, "proto", "", "Peer RPC protobuf schema with numeric method ids")
	flags.StringVar(&cfg.PayloadProtoPath, "payload-proto", "", "Peer RPC method payload protobuf schema with field numbers")
	flags.StringVar(&cfg.OutDir, "out", "sdk/c/gizclaw/generated", "Generated C output directory")
	flags.StringVar(&cfg.Package, "package", "gzc", "C symbol prefix")
	flags.BoolVar(&cfg.Check, "check", false, "Verify generated files are up to date")
	flags.BoolVar(&cfg.Format, "format", true, "Format generated output")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if err := rpcgen.Run(cfg); err != nil {
		fmt.Fprintf(stderr, "gzc-rpcgen: %v\n", err)
		return 1
	}
	return 0
}
