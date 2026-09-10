package connection

import (
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/contextconn"
)

// DialFromContext prepares a client for the named CLI context, or the current
// context when name is empty. The returned client is not connected yet.
func DialFromContext(name string) (*gizcli.Client, giznet.PublicKey, string, error) {
	return contextconn.Dial(contextconn.Options{Context: name})
}

// ConnectFromContext returns a ready client for the named CLI context, or the
// current context when name is empty.
func ConnectFromContext(name string) (*gizcli.Client, error) {
	return contextconn.Connect(contextconn.Options{Context: name})
}
