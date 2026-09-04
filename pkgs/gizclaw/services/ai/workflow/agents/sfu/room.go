package sfu

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// disconnectReason is the provider-neutral subset of SFU disconnect causes
// the session reacts to.
type disconnectReason int

const (
	// disconnectOther covers network failures, SFU restarts and every reason
	// that warrants a bounded reconnect.
	disconnectOther disconnectReason = iota
	// disconnectLeave reports a disconnect the session itself requested.
	disconnectLeave
	// disconnectDuplicateIdentity reports that the same Peer joined from
	// another Server; the session terminates without reconnecting.
	disconnectDuplicateIdentity
)

// connectParams identifies one participant attachment.
type connectParams struct {
	URL      string
	Room     string
	Identity string
}

// rtpReader is the remote track surface the downlink consumes.
type rtpReader interface {
	ReadRTP() (*rtp.Packet, interceptor.Attributes, error)
}

// roomEvents receives connector callbacks. Implementations must not block:
// the SDK invokes them from its own goroutines.
type roomEvents interface {
	onDisconnected(reason disconnectReason)
	onReconnecting()
	onReconnected()
	onTrackSubscribed(identity, trackID string, reader rtpReader)
	onTrackUnsubscribed(trackID string)
	onTrackMuted(trackID string)
	onParticipantDisconnected(identity string)
	// onDataPacket delivers one user data packet. identity is the sender as
	// authenticated by the SFU, never taken from the payload.
	onDataPacket(identity, topic string, payload []byte)
}

// roomClient is one live participant connection. Disconnect is idempotent.
type roomClient interface {
	WriteAudio(sample media.Sample) error
	// PublishData sends one payload on the reliable data channel to every
	// participant of the Room under the given topic.
	PublishData(topic string, payload []byte) error
	Disconnect()
}

// roomConnector joins an SFU Room. The LiveKit implementation is the only
// place the LiveKit SDK is touched; tests substitute a fake.
type roomConnector interface {
	connect(ctx context.Context, params connectParams, events roomEvents) (roomClient, error)
}

const (
	livekitTokenTTL         = 2 * time.Minute
	livekitDefaultConnectTO = 15 * time.Second
)

// livekitConnector joins LiveKit Rooms with Server-held API credentials.
type livekitConnector struct {
	apiKey    string
	apiSecret string
}

func (c livekitConnector) connect(ctx context.Context, params connectParams, events roomEvents) (roomClient, error) {
	token, err := c.mintToken(params)
	if err != nil {
		return nil, err
	}
	callback := &lksdk.RoomCallback{
		OnDisconnectedWithReason: func(reason lksdk.DisconnectionReason) {
			events.onDisconnected(mapDisconnectReason(reason))
		},
		OnReconnecting: events.onReconnecting,
		OnReconnected:  events.onReconnected,
		OnParticipantDisconnected: func(participant *lksdk.RemoteParticipant) {
			if participant != nil {
				events.onParticipantDisconnected(participant.Identity())
			}
		},
		ParticipantCallback: lksdk.ParticipantCallback{
			OnDataPacket: func(data lksdk.DataPacket, params lksdk.DataReceiveParams) {
				user, ok := data.(*lksdk.UserDataPacket)
				if !ok || user == nil {
					return
				}
				events.onDataPacket(params.SenderIdentity, user.Topic, user.Payload)
			},
			OnTrackSubscribed: func(track *webrtc.TrackRemote, publication *lksdk.RemoteTrackPublication, participant *lksdk.RemoteParticipant) {
				if track == nil || publication == nil || participant == nil || track.Kind() != webrtc.RTPCodecTypeAudio {
					return
				}
				events.onTrackSubscribed(participant.Identity(), publication.SID(), track)
			},
			OnTrackUnsubscribed: func(_ *webrtc.TrackRemote, publication *lksdk.RemoteTrackPublication, _ *lksdk.RemoteParticipant) {
				if publication != nil {
					events.onTrackUnsubscribed(publication.SID())
				}
			},
			OnTrackMuted: func(publication lksdk.TrackPublication, _ lksdk.Participant) {
				if remote, ok := publication.(*lksdk.RemoteTrackPublication); ok && remote != nil {
					events.onTrackMuted(remote.SID())
				}
			},
		},
	}
	timeout := livekitDefaultConnectTO
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	room, err := lksdk.ConnectToRoomWithToken(params.URL, token, callback, lksdk.WithAutoSubscribe(true), lksdk.WithConnectTimeout(timeout))
	if err != nil {
		return nil, fmt.Errorf("sfu: connect to room: %w", err)
	}
	track, err := webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{
		MimeType:  webrtc.MimeTypeOpus,
		ClockRate: 48000,
		Channels:  2,
	}, "audio", "gizclaw")
	if err != nil {
		room.Disconnect()
		return nil, fmt.Errorf("sfu: create local audio track: %w", err)
	}
	publication, err := room.LocalParticipant.PublishTrack(track, &lksdk.TrackPublicationOptions{
		Name:   "audio",
		Source: livekit.TrackSource_MICROPHONE,
	})
	if err != nil {
		room.Disconnect()
		return nil, fmt.Errorf("sfu: publish local audio track: %w", err)
	}
	return &livekitRoom{room: room, track: track, publication: publication}, nil
}

func (c livekitConnector) mintToken(params connectParams) (string, error) {
	if c.apiKey == "" || c.apiSecret == "" {
		return "", errors.New("sfu: LiveKit API credentials are not configured")
	}
	grant := &auth.VideoGrant{RoomJoin: true, Room: params.Room}
	grant.SetCanPublish(true)
	grant.SetCanPublishData(true)
	grant.SetCanSubscribe(true)
	token, err := auth.NewAccessToken(c.apiKey, c.apiSecret).
		SetIdentity(params.Identity).
		SetValidFor(livekitTokenTTL).
		SetVideoGrant(grant).
		ToJWT()
	if err != nil {
		return "", fmt.Errorf("sfu: mint participant token: %w", err)
	}
	return token, nil
}

func mapDisconnectReason(reason lksdk.DisconnectionReason) disconnectReason {
	switch reason {
	case lksdk.DuplicateIdentity:
		return disconnectDuplicateIdentity
	case lksdk.LeaveRequested:
		return disconnectLeave
	default:
		return disconnectOther
	}
}

type livekitRoom struct {
	room        *lksdk.Room
	track       *webrtc.TrackLocalStaticSample
	publication *lksdk.LocalTrackPublication
}

func (r *livekitRoom) WriteAudio(sample media.Sample) error {
	return r.track.WriteSample(sample)
}

func (r *livekitRoom) PublishData(topic string, payload []byte) error {
	return r.room.LocalParticipant.PublishData(payload, lksdk.WithDataPublishReliable(true), lksdk.WithDataPublishTopic(topic))
}

func (r *livekitRoom) Disconnect() {
	if r.publication != nil {
		_ = r.room.LocalParticipant.UnpublishTrack(r.publication.SID())
		r.publication = nil
	}
	r.room.Disconnect()
}
