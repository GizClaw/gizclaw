package rpcapi

import (
	"fmt"
	"net/url"
	"unicode/utf8"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

// MaxAudioPlayerItems is the bounded device playlist capacity.
const MaxAudioPlayerItems = 32

// ValidateAudioPlayerItems validates a complete replacement or append request.
func ValidateAudioPlayerItems(items []*rpcpb.AudioPlayerItem, appendOnly bool) error {
	if len(items) > MaxAudioPlayerItems || (appendOnly && len(items) == 0) {
		return fmt.Errorf("audioplayer: invalid playlist length")
	}
	for _, item := range items {
		if item == nil || len(item.Url) == 0 || len(item.Url) > 1024 || !utf8.ValidString(item.Url) {
			return fmt.Errorf("audioplayer: invalid audio URL")
		}
		parsed, err := url.Parse(item.Url)
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
			return fmt.Errorf("audioplayer: audio URL must use HTTPS without credentials or fragment")
		}
		if !validAudioPlayerText(item.Title, 128) || !validAudioPlayerText(item.SourceRef, 128) {
			return fmt.Errorf("audioplayer: invalid item metadata")
		}
	}
	return nil
}

// ValidAudioPlayerRepeat reports whether a repeat mode is supported.
func ValidAudioPlayerRepeat(repeat string) bool {
	return repeat == "off" || repeat == "one" || repeat == "all"
}

// ValidateAudioPlayerStatus validates one complete player snapshot before it
// crosses a provider or storage boundary. Diagnostics never include input URLs.
func ValidateAudioPlayerStatus(status *rpcpb.AudioPlayerStatus) error {
	if status == nil {
		return fmt.Errorf("audioplayer: missing status")
	}
	switch status.State {
	case "stopped", "buffering", "playing", "ended", "error":
	default:
		return fmt.Errorf("audioplayer: invalid state")
	}
	if !ValidAudioPlayerRepeat(status.Repeat) || status.PlaylistLength > MaxAudioPlayerItems {
		return fmt.Errorf("audioplayer: invalid mode or playlist length")
	}
	if status.CurrentIndex != nil && *status.CurrentIndex >= status.PlaylistLength {
		return fmt.Errorf("audioplayer: index outside playlist")
	}
	if (status.State == "playing" || status.State == "buffering") && status.CurrentIndex == nil {
		return fmt.Errorf("audioplayer: active playback requires an index")
	}
	// Preserve exact millisecond integers across JavaScript and other SDKs.
	const maxExactInteger = uint64(1<<53 - 1)
	if status.ObservedAtUnixMs < 0 || uint64(status.ObservedAtUnixMs) > maxExactInteger || status.PositionMs > maxExactInteger || (status.DurationMs != nil && (*status.DurationMs > maxExactInteger || status.PositionMs > *status.DurationMs)) {
		return fmt.Errorf("audioplayer: invalid progress")
	}
	if !validAudioPlayerText(status.ErrorCode, 128) || !validAudioPlayerText(status.ErrorMessage, 512) {
		return fmt.Errorf("audioplayer: invalid error detail")
	}
	if status.State != "error" && (status.ErrorCode != nil || status.ErrorMessage != nil) {
		return fmt.Errorf("audioplayer: error details require error state")
	}
	return nil
}

func validAudioPlayerText(value *string, limit int) bool {
	return value == nil || (len(*value) <= limit && utf8.ValidString(*value))
}

// ValidateAudioPlayerRequest validates a device player command before dispatch.
func ValidateAudioPlayerRequest(message proto.Message) error {
	if message == nil || !message.ProtoReflect().IsValid() {
		return fmt.Errorf("audioplayer: missing request")
	}
	switch request := message.(type) {
	case *rpcpb.ClientDeviceAudioPlayerGetRequest, *rpcpb.ClientDeviceAudioPlayerPlaylistGetRequest, *rpcpb.ClientDeviceAudioPlayerStopRequest:
		return nil
	case *rpcpb.ClientDeviceAudioPlayerPlaylistSetRequest:
		return ValidateAudioPlayerItems(request.Items, false)
	case *rpcpb.ClientDeviceAudioPlayerPlaylistAppendRequest:
		return ValidateAudioPlayerItems(request.Items, true)
	case *rpcpb.ClientDeviceAudioPlayerPlayRequest:
		if request.Index != nil && *request.Index < MaxAudioPlayerItems {
			return nil
		}
	case *rpcpb.ClientDeviceAudioPlayerModeSetRequest:
		if ValidAudioPlayerRepeat(request.Repeat) {
			return nil
		}
	}
	return fmt.Errorf("audioplayer: invalid request")
}

// ValidateAudioPlayerResponse rejects malformed device responses before use.
func ValidateAudioPlayerResponse(message proto.Message) error {
	if message == nil || !message.ProtoReflect().IsValid() {
		return fmt.Errorf("audioplayer: missing response")
	}
	switch response := message.(type) {
	case interface {
		GetValue() *rpcpb.AudioPlayerStatus
	}:
		return ValidateAudioPlayerStatus(response.GetValue())
	case *rpcpb.ClientDeviceAudioPlayerPlaylistGetResponse:
		return ValidateAudioPlayerItems(response.Items, false)
	default:
		return fmt.Errorf("audioplayer: invalid response")
	}
}
