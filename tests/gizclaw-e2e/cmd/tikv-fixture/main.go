package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"github.com/GizClaw/gizclaw-go/tests/testsupport/tikvfixture"
)

func main() {
	address := flag.String("pd", "tikv-pd:2379", "isolated E2E PD address")
	readRouting := flag.Bool("read-routing", false, "verify two distinct regional Learners")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	var topology any
	var err error
	if *readRouting {
		topology, err = tikvfixture.ConfigureReadRouting(ctx, *address)
	} else {
		topology, err = tikvfixture.Configure(ctx, *address)
	}
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(topology); err != nil {
		log.Fatal(err)
	}
}
