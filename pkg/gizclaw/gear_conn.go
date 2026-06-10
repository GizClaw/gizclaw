package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkg/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkg/audio/stampedopus"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"golang.org/x/sync/errgroup"
)

var (
	ErrNilGearConn          = errors.New("gizclaw: nil gear conn")
	ErrNilGearConnTransport = errors.New("gizclaw: nil gear conn transport")
	ErrNilGearConnService   = errors.New("gizclaw: nil gear conn service")
	ErrNilGearConnMixer     = errors.New("gizclaw: nil gear conn mixer")
)

const gearConnMixerFormat = pcm.L16Mono16K

const gearConnOpusFrameDuration = 20 * time.Millisecond

// GearConn is the in-memory runtime for one active gear connection.
// It wraps the existing PeerService bundle and serves one live conn at a time.
type GearConn struct {
	Conn    *giznet.Conn
	Service *PeerService

	closeOnce              sync.Once
	mixer                  *pcm.Mixer
	rpc                    *rpcServer
	lastOpusFrameTimestamp atomic.Uint64
	closed                 atomic.Bool
}

// CreateAudioTrack creates a writable audio track on the peer mixer.
// The mixer itself is intentionally kept private to GearConn.
func (h *GearConn) CreateAudioTrack(opts ...pcm.TrackOption) (pcm.Track, *pcm.TrackCtrl, error) {
	mx, err := h.audioMixer()
	if err != nil {
		return nil, nil, err
	}
	return mx.CreateTrack(opts...)
}

// serve proxies to the existing PeerService implementation for one live conn.
func (h *GearConn) serve() error {
	if h == nil {
		return ErrNilGearConn
	}
	if h.Conn == nil {
		return ErrNilGearConnTransport
	}
	if h.Service == nil {
		return ErrNilGearConnService
	}
	if err := h.Service.validateServices(); err != nil {
		return err
	}
	h.init()

	var g errgroup.Group
	g.Go(h.serveService)
	g.Go(h.servePackets)
	g.Go(h.serveRPC)
	err := g.Wait()
	if err != nil {
		_ = h.close()
	}
	return err
}

func (h *GearConn) serveService() error {
	defer func() {
		_ = h.close()
	}()
	return h.Service.ServeConn(h.Conn)
}

func (h *GearConn) servePackets() error {
	if _, err := h.audioMixer(); err != nil {
		return err
	}
	h.streamMixedAudioLoop()
	return nil
}

func (h *GearConn) serveRPC() error {
	listener := h.Conn.ListenService(ServiceRPC)
	defer func() {
		_ = listener.Close()
	}()
	server := h.rpcServer()
	for {
		stream, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go func(stream net.Conn) {
			if err := server.Handle(stream); err != nil {
				_ = stream.Close()
			}
		}(stream)
	}
}

// Ping opens a fresh RPC stream, sends one ping, and closes it.
//
// Our current RPC transport uses one KCP stream per round trip so multiple RPC
// requests can run concurrently on separate streams. This is closer to
// HTTP/1.0-style request lifecycles; HTTP/1.1-style stream reuse is not
// supported yet.
func (h *GearConn) Ping(ctx context.Context, id string) (*rpcapi.PingResponse, error) {
	stream, err := h.rpcConn()
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()
	return h.rpcServer().Ping(ctx, stream, id)
}

func (h *GearConn) rpcConn() (net.Conn, error) {
	conn := h.Conn
	stream, err := conn.Dial(ServiceRPC)
	if err != nil {
		return nil, fmt.Errorf("gizclaw: dial rpc stream: %w", err)
	}
	return stream, nil
}

func (h *GearConn) init() {
	h.initMixer()
	h.initRPC()
}

func (h *GearConn) initRPC() {
	if h == nil || h.rpc != nil {
		return
	}
	h.rpc = &rpcServer{}
	if h.Service != nil {
		if h.Service.manager != nil {
			h.rpc.peer = h.Service.manager.Peers
			h.rpc.peerRun = h.Service.manager.PeerRun
		}
		h.rpc.serverInfo = h.Service.public
	}
	if h.Conn != nil {
		h.rpc.callerPublicKey = h.Conn.PublicKey()
	}
}

func (h *GearConn) rpcServer() *rpcServer {
	h.initRPC()
	return h.rpc
}

func (h *GearConn) initMixer() {
	if h == nil {
		return
	}
	if h.mixer == nil {
		h.mixer = pcm.NewMixer(gearConnMixerFormat)
	}
}

func (h *GearConn) audioMixer() (*pcm.Mixer, error) {
	if h == nil {
		return nil, ErrNilGearConn
	}
	if h.mixer == nil {
		return nil, ErrNilGearConnMixer
	}
	return h.mixer, nil
}

func (h *GearConn) close() error {
	if h == nil {
		return nil
	}
	var closeErr error
	h.closeOnce.Do(func() {
		h.closed.Store(true)
		if h.Conn != nil {
			if err := h.Conn.Close(); err != nil && !errors.Is(err, giznet.ErrConnClosed) {
				closeErr = errors.Join(closeErr, err)
			}
		}
		mx := h.mixer
		if mx != nil {
			closeErr = errors.Join(closeErr, mx.Close())
		}
	})
	return closeErr
}

func (h *GearConn) streamMixedAudioLoop() {
	hasWrittenBefore := false
	for !h.isClosed() {
		wrote, err := h.streamMixedAudio(hasWrittenBefore)
		hasWrittenBefore = hasWrittenBefore || wrote
		if err != nil {
			slog.Error("gizclaw: mixed audio stream failed; retrying", "error", err)
		}
	}
}

func (h *GearConn) streamMixedAudio(hasWrittenBefore bool) (wrote bool, err error) {
	mx := h.mixer
	enc, err := opus.NewEncoder(gearConnMixerFormat.SampleRate(), gearConnMixerFormat.Channels(), opus.ApplicationAudio)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = enc.Close()
	}()

	frameSize := int(gearConnMixerFormat.SamplesInDuration(gearConnOpusFrameDuration))
	for {
		chunk, err := gearConnMixerFormat.ReadChunk(mx, gearConnOpusFrameDuration)
		if err != nil {
			if h.isClosed() && errors.Is(err, io.ErrClosedPipe) {
				return wrote, nil
			}
			return wrote, err
		}

		packet, err := enc.Encode(gearConnPCMChunkToInt16(chunk), frameSize)
		if err != nil {
			return wrote, err
		}
		if !hasWrittenBefore {
			h.lastOpusFrameTimestamp.Store(uint64(time.Now().UnixMilli()))
			hasWrittenBefore = true
			wrote = true
		}
		payload := stampedopus.Pack(h.lastOpusFrameTimestamp.Load(), packet)
		if _, err := h.Conn.Write(ProtocolStampedOpus, payload); err != nil {
			return wrote, err
		}
		h.lastOpusFrameTimestamp.Add(uint64(gearConnOpusFrameDuration / time.Millisecond))
	}
}

func (h *GearConn) isClosed() bool {
	if h == nil {
		return true
	}
	return h.closed.Load()
}

func gearConnPCMChunkToInt16(chunk pcm.Chunk) []int16 {
	dataChunk, ok := chunk.(*pcm.DataChunk)
	if !ok || len(dataChunk.Data) == 0 {
		return nil
	}
	data := dataChunk.Data
	out := make([]int16, len(data)/2)
	for i := range out {
		lo := uint16(data[i*2])
		hi := uint16(data[i*2+1]) << 8
		out[i] = int16(lo | hi)
	}
	return out
}
