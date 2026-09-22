package gizcli

import (
	"context"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

// ClientToolHandler implements the request and response messages selected by
// ClientTool. The SDK rejects malformed payloads before calling the handler.
// Handlers must validate device-specific preconditions before applying effects.
type ClientToolHandler func(context.Context, proto.Message) (proto.Message, error)

// HandleClientTool installs a predefined tool provider. A nil handler removes
// the registration. The tool list is derived from the installed providers.
func (c *Client) HandleClientTool(tool rpcpb.ClientTool, handler ClientToolHandler) error {
	if c == nil {
		return fmt.Errorf("gizclaw: nil client")
	}
	if _, err := rpcapi.ClientToolMetadata(tool); err != nil {
		return err
	}
	c.deviceMu.Lock()
	defer c.deviceMu.Unlock()
	if handler == nil {
		delete(c.toolHandlers, tool)
		return nil
	}
	if c.toolHandlers == nil {
		c.toolHandlers = make(map[rpcpb.ClientTool]ClientToolHandler)
	}
	c.toolHandlers[tool] = handler
	return nil
}

func (c *Client) clientToolHandler(tool rpcpb.ClientTool) ClientToolHandler {
	c.deviceMu.RLock()
	defer c.deviceMu.RUnlock()
	return c.toolHandlers[tool]
}

// ObserveClientTool observes a decoded tool/v0 invocation, including a tool the
// device does not implement. The callback runs without an SDK mutex held.
func (c *Client) ObserveClientTool(observer func(rpcpb.ClientTool)) {
	c.clientRPCMu.Lock()
	defer c.clientRPCMu.Unlock()
	c.clientToolObserver = observer
}

func (c *Client) observeClientTool(tool rpcpb.ClientTool) {
	c.clientRPCMu.RLock()
	observer := c.clientToolObserver
	c.clientRPCMu.RUnlock()
	if observer != nil {
		observer(tool)
	}
}
