// Command log-sink runs the test-only Volc TLS protocol fixture. No production
// binary imports this command or the fixture.
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/GizClaw/gizclaw-go/tests/testsupport/volctlsfixture"
)

func main() {
	missingIndex := flag.String("missing-index", "", "reject searches referencing this unindexed field")
	flag.Parse()
	handler := volctlsfixture.NewHandler()
	if *missingIndex != "" {
		handler = volctlsfixture.NewHandlerWithMissingIndexes(*missingIndex)
	}
	server := &http.Server{Addr: ":9091", Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}
