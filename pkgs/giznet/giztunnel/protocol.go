// Package giztunnel binds logical giznet connections to native WebRTC
// DataChannels while retaining an unreliable session-tagged Opus lane.
package giztunnel

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const (
	Version                 = 2
	LabelPrefix             = "giznet/v2/tunnel/"
	maxLabelSize            = 512
	maxRemoteAddrSize       = 256
	maxRejectReasonSize     = 256
	sessionResultHeaderSize = 7
)

var (
	ErrInvalidFrame     = errors.New("giztunnel: invalid frame")
	ErrFrameTooLarge    = errors.New("giztunnel: frame too large")
	ErrInvalidState     = errors.New("giztunnel: invalid state")
	ErrSessionRejected  = errors.New("giztunnel: session rejected")
	ErrSessionExists    = errors.New("giztunnel: session already exists")
	ErrSessionNotFound  = errors.New("giztunnel: session not found")
	ErrBufferLimit      = errors.New("giztunnel: buffer limit exceeded")
	ErrServiceForbidden = errors.New("giztunnel: service forbidden")
)

var sessionResultMagic = [4]byte{'G', 'Z', 'T', '2'}

type labelKind uint8

const (
	labelControl labelKind = iota + 1
	labelPacket
	labelService
)

type sessionResultStatus byte

const (
	sessionAccepted sessionResultStatus = iota
	sessionRejected
)

// SessionID identifies one logical connection within one physical Edge
// PeerConnection.
type SessionID [16]byte

// NewSessionID returns a cryptographically random logical-session identifier.
func NewSessionID() (SessionID, error) {
	var id SessionID
	_, err := io.ReadFull(rand.Reader, id[:])
	return id, err
}

func (id SessionID) IsZero() bool { return id == SessionID{} }

func (id SessionID) String() string { return hex.EncodeToString(id[:]) }

// SessionDeclaration is the trusted Edge declaration encoded in a control
// channel label.
type SessionDeclaration struct {
	SessionID       SessionID
	ClientPublicKey giznet.PublicKey
	RemoteAddr      string
}

type parsedLabel struct {
	kind      labelKind
	session   SessionID
	client    giznet.PublicKey
	remote    string
	service   uint64
	channelID uint64
}

func controlLabel(declaration SessionDeclaration) (string, error) {
	if declaration.SessionID.IsZero() || declaration.ClientPublicKey.IsZero() ||
		len(declaration.RemoteAddr) > maxRemoteAddrSize {
		return "", ErrInvalidFrame
	}
	remote := "-"
	if declaration.RemoteAddr != "" {
		remote = base64.RawURLEncoding.EncodeToString([]byte(declaration.RemoteAddr))
	}
	label := LabelPrefix + declaration.SessionID.String() + "/control/" +
		declaration.ClientPublicKey.String() + "/" + remote
	if len(label) > maxLabelSize {
		return "", ErrFrameTooLarge
	}
	return label, nil
}

func packetLabel(id SessionID) (string, error) {
	if id.IsZero() {
		return "", ErrInvalidFrame
	}
	return LabelPrefix + id.String() + "/packet", nil
}

func serviceLabel(id SessionID, service, channelID uint64) (string, error) {
	if id.IsZero() || channelID == 0 {
		return "", ErrInvalidFrame
	}
	return LabelPrefix + id.String() + "/service/" +
		strconv.FormatUint(service, 10) + "/" + strconv.FormatUint(channelID, 10), nil
}

func parseLabel(label string) (parsedLabel, error) {
	if len(label) == 0 || len(label) > maxLabelSize || !strings.HasPrefix(label, LabelPrefix) {
		return parsedLabel{}, ErrInvalidFrame
	}
	parts := strings.Split(strings.TrimPrefix(label, LabelPrefix), "/")
	if len(parts) < 2 {
		return parsedLabel{}, ErrInvalidFrame
	}
	id, err := parseSessionID(parts[0])
	if err != nil {
		return parsedLabel{}, err
	}
	switch parts[1] {
	case "control":
		if len(parts) != 4 {
			return parsedLabel{}, ErrInvalidFrame
		}
		var client giznet.PublicKey
		if err := client.UnmarshalText([]byte(parts[2])); err != nil || client.IsZero() || client.String() != parts[2] {
			return parsedLabel{}, ErrInvalidFrame
		}
		remote, err := decodeRemoteAddr(parts[3])
		if err != nil {
			return parsedLabel{}, err
		}
		return parsedLabel{kind: labelControl, session: id, client: client, remote: remote}, nil
	case "packet":
		if len(parts) != 2 {
			return parsedLabel{}, ErrInvalidFrame
		}
		return parsedLabel{kind: labelPacket, session: id}, nil
	case "service":
		if len(parts) != 4 {
			return parsedLabel{}, ErrInvalidFrame
		}
		service, err := parseCanonicalUint(parts[2], true)
		if err != nil {
			return parsedLabel{}, err
		}
		channelID, err := parseCanonicalUint(parts[3], false)
		if err != nil {
			return parsedLabel{}, err
		}
		return parsedLabel{kind: labelService, session: id, service: service, channelID: channelID}, nil
	default:
		return parsedLabel{}, ErrInvalidFrame
	}
}

func parseSessionID(value string) (SessionID, error) {
	if len(value) != hex.EncodedLen(len(SessionID{})) || strings.ToLower(value) != value {
		return SessionID{}, ErrInvalidFrame
	}
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != len(SessionID{}) {
		return SessionID{}, ErrInvalidFrame
	}
	var id SessionID
	copy(id[:], raw)
	if id.IsZero() {
		return SessionID{}, ErrInvalidFrame
	}
	return id, nil
}

func parseCanonicalUint(value string, allowZero bool) (uint64, error) {
	if value == "" || len(value) > 20 || len(value) > 1 && value[0] == '0' {
		return 0, ErrInvalidFrame
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || !allowZero && parsed == 0 || strconv.FormatUint(parsed, 10) != value {
		return 0, ErrInvalidFrame
	}
	return parsed, nil
}

func decodeRemoteAddr(value string) (string, error) {
	if value == "-" {
		return "", nil
	}
	if value == "" || len(value) > base64.RawURLEncoding.EncodedLen(maxRemoteAddrSize) {
		return "", ErrInvalidFrame
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(raw) > maxRemoteAddrSize || base64.RawURLEncoding.EncodeToString(raw) != value {
		return "", ErrInvalidFrame
	}
	return string(raw), nil
}

func encodeSessionResult(status sessionResultStatus, reason string) ([]byte, error) {
	if status == sessionAccepted && reason != "" || status != sessionAccepted && status != sessionRejected ||
		len(reason) > maxRejectReasonSize || !utf8.ValidString(reason) {
		return nil, ErrInvalidFrame
	}
	payload := make([]byte, sessionResultHeaderSize+len(reason))
	copy(payload[:4], sessionResultMagic[:])
	payload[4] = byte(status)
	binary.BigEndian.PutUint16(payload[5:7], uint16(len(reason)))
	copy(payload[7:], reason)
	return payload, nil
}

func decodeSessionResult(payload []byte) (sessionResultStatus, string, error) {
	if len(payload) < sessionResultHeaderSize || len(payload) > sessionResultHeaderSize+maxRejectReasonSize ||
		string(payload[:4]) != string(sessionResultMagic[:]) {
		return 0, "", ErrInvalidFrame
	}
	status := sessionResultStatus(payload[4])
	reasonSize := int(binary.BigEndian.Uint16(payload[5:7]))
	if reasonSize != len(payload)-sessionResultHeaderSize || !utf8.Valid(payload[7:]) {
		return 0, "", ErrInvalidFrame
	}
	reason := string(payload[7:])
	if status == sessionAccepted && reason != "" || status != sessionAccepted && status != sessionRejected {
		return 0, "", ErrInvalidFrame
	}
	return status, reason, nil
}

func rejectionError(reason string) error {
	if reason == "" {
		return ErrSessionRejected
	}
	return fmt.Errorf("%w: %s", ErrSessionRejected, reason)
}
