// Command log-sink runs the test-only Volc TLS protocol fixture. No production
// binary imports this command or the fixture.
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/GizClaw/gizclaw-go/tests/testsupport/volctlsfixture"
)

func main() {
	server := &http.Server{Addr: ":9091", Handler: volctlsfixture.NewHandler(), ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}
