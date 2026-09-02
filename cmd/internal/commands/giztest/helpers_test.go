package giztestcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

// mustVariables builds the variable set a driver test needs, failing the test
// when a spec is invalid.
func mustVariables(t *testing.T, specs map[string]giztest.VariableSpec) *giztest.Variables {
	t.Helper()
	vars, err := giztest.NewVariables(specs)
	if err != nil {
		t.Fatalf("build variables: %v", err)
	}
	return vars
}

// validDocument is the canonical single-step document the CLI tests run.
const validDocument = `# User Story:
# As a registered device,
# I want to ping the target server,
# So that I can verify Peer RPC connectivity.
version: gizclaw.test/v1alpha1
name: ping-connectivity
clients:
  peer:
    identity: ephemeral
    connection: webrtc
    access_point: ${endpoint}
variables:
  endpoint:
    direction: input
    type: string
    value: 127.0.0.1:8080
  server_time:
    direction: output
    type: string
steps:
  - id: ping
    client: peer
    rpc:
      method: all.ping
      request: {}
    capture:
      server_time: /server_time
    expect:
      /server_time:
        present: true
`

// writeTestDocument stages one Giztest document in a temporary directory and
// returns its path.
func writeTestDocument(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "case.giztest.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// testDriverSession builds one live session over the driver's substituted
// transports, so a driver test can run a single step without dialing.
func testDriverSession(d *driver, clients *clientSet) giztest.Session {
	if clients == nil {
		clients = &clientSet{}
	}
	return &session{driver: d, clients: clients, streams: newPeerStreamSessions()}
}
