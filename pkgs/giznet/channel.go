package giznet

import (
	"context"
	"net"
)

// ChannelReliability is the transport-independent delivery class of one
// labeled native channel.
type ChannelReliability uint8

const (
	// ChannelReliabilityUnknown reports a remote channel whose negotiated
	// delivery semantics match neither supported class.
	ChannelReliabilityUnknown ChannelReliability = iota
	// ChannelReliable is a reliable, ordered byte stream used through Read and
	// Write. Callers must not rely on Read preserving write boundaries.
	ChannelReliable
	// ChannelUnreliable is unordered, never retransmitted, and preserves message
	// boundaries through ReadMessage and WriteMessage.
	ChannelUnreliable
)

func (r ChannelReliability) String() string {
	switch r {
	case ChannelReliable:
		return "reliable"
	case ChannelUnreliable:
		return "unreliable"
	default:
		return "unknown"
	}
}

// Channel is one labeled native channel opened on a physical connection.
type Channel interface {
	net.Conn
	// Label returns the immutable label that declared this channel.
	Label() string
	// Reliability reports the negotiated delivery class.
	Reliability() ChannelReliability
	// ReadMessage reads exactly one message from an unreliable channel.
	ReadMessage([]byte) (int, error)
	// WriteMessage writes exactly one message to an unreliable channel.
	WriteMessage([]byte) (int, error)
	// SetWriteBudgets applies shared outstanding-byte budgets to reliable
	// writes. It must be called before the first write.
	SetWriteBudgets(...*WriteBudget) error
}

// ChannelConn is the optional labeled native channel surface. Transports
// implement it when every logical stream can be one transport-native stream.
type ChannelConn interface {
	Conn
	// OpenChannel opens one labeled channel. A successful return means only
	// that the local transport opened it, not that the remote application
	// accepted the label. Canceling ctx closes only the pending channel.
	OpenChannel(ctx context.Context, label string, reliability ChannelReliability) (Channel, error)
	// HandleChannels claims one incoming label prefix for handler. The returned
	// function stops admission and closes channels delivered to that handler.
	HandleChannels(prefix string, handler func(Channel)) (unregister func(), err error)
}
