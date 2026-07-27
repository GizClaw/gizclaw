package gizclaw

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giztools"
)

// peerClientToolInvoker is deliberately bound to one accepted connection. A
// model cannot select or spoof the Peer that executes a client_rpc Tool.
type peerClientToolInvoker struct {
	conn giznet.Conn
}

func (i peerClientToolInvoker) InvokeClientTool(
	ctx context.Context,
	name string,
	args []byte,
) ([]byte, error) {
	return giztools.ClientRPCExecutor{}.Invoke(ctx, i.conn, name, args)
}
