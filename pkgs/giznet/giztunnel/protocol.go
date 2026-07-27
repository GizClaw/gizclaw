// Package giztunnel multiplexes logical Giznet connections over reliable
// streams while keeping loss-tolerant packets on a separate packet lane.
package giztunnel

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const (
	Version = 1

	defaultMaxFrameSize = 64 * 1024
	maxOpenFrameSize    = 4 * 1024
	maxRejectReasonSize = 256
	frameHeaderSize     = 9
)

var (
	frameMagic = [4]byte{'G', 'Z', 'T', '1'}

	ErrInvalidFrame     = errors.New("giztunnel: invalid frame")
	ErrFrameTooLarge    = errors.New("giztunnel: frame too large")
	ErrInvalidState     = errors.New("giztunnel: invalid state")
	ErrSessionRejected  = errors.New("giztunnel: session rejected")
	ErrSessionExists    = errors.New("giztunnel: session already exists")
	ErrSessionNotFound  = errors.New("giztunnel: session not found")
	ErrBufferLimit      = errors.New("giztunnel: buffer limit exceeded")
	ErrServiceForbidden = errors.New("giztunnel: service forbidden")
)

type frameType byte

const (
	frameSessionOpen frameType = iota + 1
	frameSessionAccepted
	frameSessionRejected
	frameStreamOpen
	frameStreamData
	frameStreamClose
	frameSessionClose
)

// SessionID identifies one logical connection on the unreliable packet lane.
type SessionID [16]byte

func NewSessionID() (SessionID, error) {
	var id SessionID
	_, err := rand.Read(id[:])
	return id, err
}

func (id SessionID) IsZero() bool {
	return id == SessionID{}
}

func (id SessionID) String() string {
	return hex.EncodeToString(id[:])
}

// OpenRequest is the delegated identity envelope sent by an Edge.
type OpenRequest struct {
	SessionID       SessionID
	ClientPublicKey giznet.PublicKey
	EdgePublicKey   giznet.PublicKey
	ServerPublicKey giznet.PublicKey
	IssuedAtUnix    int64
	ExpiresAtUnix   int64
	RemoteAddr      string
}

type openRequestWire struct {
	Version         int    `json:"version"`
	SessionID       string `json:"session_id"`
	ClientPublicKey string `json:"client_public_key"`
	EdgePublicKey   string `json:"edge_public_key"`
	ServerPublicKey string `json:"server_public_key"`
	IssuedAtUnix    int64  `json:"issued_at_unix"`
	ExpiresAtUnix   int64  `json:"expires_at_unix"`
	RemoteAddr      string `json:"remote_addr,omitempty"`
}

func encodeOpenRequest(open OpenRequest) ([]byte, error) {
	if open.SessionID.IsZero() || open.ClientPublicKey.IsZero() ||
		open.EdgePublicKey.IsZero() || open.ServerPublicKey.IsZero() {
		return nil, fmt.Errorf("%w: zero identity", ErrInvalidFrame)
	}
	return json.Marshal(openRequestWire{
		Version:         Version,
		SessionID:       open.SessionID.String(),
		ClientPublicKey: open.ClientPublicKey.String(),
		EdgePublicKey:   open.EdgePublicKey.String(),
		ServerPublicKey: open.ServerPublicKey.String(),
		IssuedAtUnix:    open.IssuedAtUnix,
		ExpiresAtUnix:   open.ExpiresAtUnix,
		RemoteAddr:      open.RemoteAddr,
	})
}

func decodeOpenRequest(data []byte) (OpenRequest, error) {
	if len(data) == 0 || len(data) > maxOpenFrameSize {
		return OpenRequest{}, ErrFrameTooLarge
	}
	var wire openRequestWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return OpenRequest{}, fmt.Errorf("%w: open envelope: %v", ErrInvalidFrame, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return OpenRequest{}, fmt.Errorf("%w: trailing open envelope", ErrInvalidFrame)
	}
	if wire.Version != Version {
		return OpenRequest{}, fmt.Errorf("%w: version %d", ErrInvalidFrame, wire.Version)
	}
	sessionRaw, err := hex.DecodeString(wire.SessionID)
	if err != nil || len(sessionRaw) != len(SessionID{}) {
		return OpenRequest{}, fmt.Errorf("%w: session id", ErrInvalidFrame)
	}
	var open OpenRequest
	copy(open.SessionID[:], sessionRaw)
	if err := open.ClientPublicKey.UnmarshalText([]byte(wire.ClientPublicKey)); err != nil {
		return OpenRequest{}, fmt.Errorf("%w: client public key", ErrInvalidFrame)
	}
	if err := open.EdgePublicKey.UnmarshalText([]byte(wire.EdgePublicKey)); err != nil {
		return OpenRequest{}, fmt.Errorf("%w: edge public key", ErrInvalidFrame)
	}
	if err := open.ServerPublicKey.UnmarshalText([]byte(wire.ServerPublicKey)); err != nil {
		return OpenRequest{}, fmt.Errorf("%w: server public key", ErrInvalidFrame)
	}
	open.IssuedAtUnix = wire.IssuedAtUnix
	open.ExpiresAtUnix = wire.ExpiresAtUnix
	open.RemoteAddr = strings.TrimSpace(wire.RemoteAddr)
	if open.SessionID.IsZero() || open.ClientPublicKey.IsZero() ||
		open.EdgePublicKey.IsZero() || open.ServerPublicKey.IsZero() {
		return OpenRequest{}, fmt.Errorf("%w: zero identity", ErrInvalidFrame)
	}
	return open, nil
}

func writeFrame(w io.Writer, typ frameType, payload []byte, maxFrameSize int) error {
	if maxFrameSize <= 0 {
		maxFrameSize = defaultMaxFrameSize
	}
	if len(payload) > maxFrameSize {
		return ErrFrameTooLarge
	}
	var header [frameHeaderSize]byte
	copy(header[:4], frameMagic[:])
	header[4] = byte(typ)
	binary.BigEndian.PutUint32(header[5:], uint32(len(payload)))
	if err := writeAll(w, header[:]); err != nil {
		return err
	}
	if len(payload) == 0 {
		return nil
	}
	return writeAll(w, payload)
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func readFrame(r io.Reader, maxFrameSize int) (frameType, []byte, error) {
	if maxFrameSize <= 0 {
		maxFrameSize = defaultMaxFrameSize
	}
	var header [frameHeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	if !bytes.Equal(header[:4], frameMagic[:]) {
		return 0, nil, fmt.Errorf("%w: magic", ErrInvalidFrame)
	}
	size := int(binary.BigEndian.Uint32(header[5:]))
	if size > maxFrameSize {
		return 0, nil, ErrFrameTooLarge
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return frameType(header[4]), payload, nil
}

func encodeStreamOpen(id, service uint64) []byte {
	var payload [16]byte
	binary.BigEndian.PutUint64(payload[:8], id)
	binary.BigEndian.PutUint64(payload[8:], service)
	return payload[:]
}

func decodeStreamOpen(payload []byte) (uint64, uint64, error) {
	if len(payload) != 16 {
		return 0, 0, ErrInvalidFrame
	}
	return binary.BigEndian.Uint64(payload[:8]), binary.BigEndian.Uint64(payload[8:]), nil
}

func encodeStreamID(id uint64) []byte {
	var payload [8]byte
	binary.BigEndian.PutUint64(payload[:], id)
	return payload[:]
}

func decodeStreamID(payload []byte) (uint64, error) {
	if len(payload) != 8 {
		return 0, ErrInvalidFrame
	}
	return binary.BigEndian.Uint64(payload), nil
}

func encodeStreamData(id uint64, data []byte) []byte {
	payload := make([]byte, 8+len(data))
	binary.BigEndian.PutUint64(payload[:8], id)
	copy(payload[8:], data)
	return payload
}

func decodeStreamData(payload []byte) (uint64, []byte, error) {
	if len(payload) < 8 {
		return 0, nil, ErrInvalidFrame
	}
	return binary.BigEndian.Uint64(payload[:8]), payload[8:], nil
}
