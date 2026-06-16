package genx

import (
	"context"
	"fmt"
	"slices"
)

// Role constants define the producer of a message.
const (
	RoleUser  Role = "user"
	RoleModel Role = "model"
	RoleTool  Role = "tool"
)

var (
	_ Payload = (Contents)(nil)
	_ Payload = (*ToolCall)(nil)
	_ Payload = (*ToolResult)(nil)

	_ Part = (*Blob)(nil)
	_ Part = (Text)("")
)

// MessageChunk represents a chunk in a genx Stream.
type MessageChunk struct {
	Role     Role
	Name     string
	Part     Part
	ToolCall *ToolCall
	Ctrl     *StreamCtrl
}

// StreamCtrl controls Stream routing and state.
type StreamCtrl struct {
	StreamID string `json:"stream_id,omitempty"`
	Label    string `json:"label,omitempty"`
	Error    string `json:"error,omitempty"`

	BeginOfStream bool  `json:"begin_of_stream,omitempty"`
	EndOfStream   bool  `json:"end_of_stream,omitempty"`
	Timestamp     int64 `json:"timestamp,omitempty"`
}

// IsBeginOfStream returns true if this chunk is a begin-of-stream marker.
func (c *MessageChunk) IsBeginOfStream() bool {
	return c != nil && c.Ctrl != nil && c.Ctrl.BeginOfStream
}

// IsEndOfStream returns true if this chunk is an end-of-stream boundary marker.
func (c *MessageChunk) IsEndOfStream() bool {
	return c != nil && c.Ctrl != nil && c.Ctrl.EndOfStream
}

// NewBeginOfStream creates a BOS marker with the given StreamID.
func NewBeginOfStream(streamID string) *MessageChunk {
	return &MessageChunk{Ctrl: &StreamCtrl{StreamID: streamID, BeginOfStream: true}}
}

// NewEndOfStream creates an EOS marker with the given MIME type.
func NewEndOfStream(mimeType string) *MessageChunk {
	return &MessageChunk{
		Part: &Blob{MIMEType: mimeType, Data: nil},
		Ctrl: &StreamCtrl{EndOfStream: true},
	}
}

// NewTextEndOfStream creates a text EoS marker.
func NewTextEndOfStream() *MessageChunk {
	return &MessageChunk{
		Part: Text(""),
		Ctrl: &StreamCtrl{EndOfStream: true},
	}
}

// Clone returns a deep copy of the MessageChunk.
func (c *MessageChunk) Clone() *MessageChunk {
	chk := &MessageChunk{
		Role: c.Role,
		Name: c.Name,
	}
	if c.Part != nil {
		chk.Part = c.Part.clone()
	}
	if c.ToolCall != nil {
		t := *c.ToolCall
		chk.ToolCall = &t
	}
	if c.Ctrl != nil {
		ctrl := *c.Ctrl
		chk.Ctrl = &ctrl
	}
	return chk
}

type Message struct {
	Role    Role
	Name    string
	Payload Payload
}

// Role identifies the producer of a message.
type Role string

func (r Role) String() string {
	return string(r)
}

type Payload interface {
	isPayload()
}

type FuncCall struct {
	Name      string
	Arguments string

	tool *FuncTool
}

func (f *FuncCall) Invoke(ctx context.Context) (any, error) {
	if f.tool == nil {
		return nil, fmt.Errorf("tool not found: name=%s", f.Name)
	}
	if f.tool.Invoke == nil {
		return nil, fmt.Errorf("invoke function not set: name=%s", f.Name)
	}
	return f.tool.Invoke(ctx, f, f.Arguments)
}

type ToolCall struct {
	ID       string
	FuncCall *FuncCall
}

func (*ToolCall) isPayload() {}

func (tool *ToolCall) Invoke(ctx context.Context) (any, error) {
	if tool.FuncCall == nil {
		return nil, fmt.Errorf("invoke can only be called on function call: id=%s", tool.ID)
	}
	return tool.FuncCall.Invoke(ctx)
}

type ToolResult struct {
	ID     string
	Result string
}

func (*ToolResult) isPayload() {}

type Contents []Part

func (Contents) isPayload() {}

// Part is the content payload of a MessageChunk.
type Part interface {
	isPart()
	clone() Part
}

// Blob represents binary data with a MIME type.
type Blob struct {
	MIMEType string
	Data     []byte
}

func (b *Blob) clone() Part {
	return &Blob{MIMEType: b.MIMEType, Data: slices.Clone(b.Data)}
}

func (*Blob) isPart() {}

// Text represents string content in a MessageChunk.
type Text string

func (t Text) clone() Part {
	return t
}

func (Text) isPart() {}
