package gizwebrtc

import (
	"fmt"
	"net"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	"github.com/pion/logging"
	"github.com/pion/webrtc/v4"
)

type CipherMode string

const (
	CipherModeChaChaPoly CipherMode = "chacha_poly"
	CipherModeAES256GCM  CipherMode = "aes_256_gcm"
	CipherModePlaintext  CipherMode = "plaintext"
)

type ListenConfig struct {
	// ICEAddr is the UDP/TCP bind address used for shared WebRTC ICE muxes.
	// If empty, Pion uses its default ephemeral ICE sockets.
	ICEAddr string

	SecurityPolicy   giznet.SecurityPolicy
	PeerEventHandler giznet.PeerEventHandler
	CipherMode       CipherMode
}

func Listen(key *giznet.KeyPair) (*Listener, error) {
	return new(ListenConfig).Listen(key)
}

func (c *ListenConfig) Listen(key *giznet.KeyPair) (*Listener, error) {
	if key == nil {
		return nil, fmt.Errorf("gizwebrtc: nil key pair")
	}
	if c == nil {
		c = &ListenConfig{}
	}
	api, closers, err := newPionAPI(c)
	if err != nil {
		return nil, err
	}
	l := &Listener{
		key:        key,
		cfg:        *c,
		api:        api,
		closers:    closers,
		acceptCh:   make(chan giznet.Conn, acceptQueueSize),
		closeCh:    make(chan struct{}),
		replaySeen: make(map[string]int64),
	}
	if l.cfg.CipherMode == "" {
		l.cfg.CipherMode = CipherModeChaChaPoly
	}
	return l, nil
}

func newPionAPI(c *ListenConfig) (*webrtc.API, []func() error, error) {
	var mediaEngine webrtc.MediaEngine
	if err := mediaEngine.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, nil, fmt.Errorf("gizwebrtc: register opus codec: %w", err)
	}

	settingEngine := webrtc.SettingEngine{}
	settingEngine.DetachDataChannels()
	settingEngine.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeUDP4,
		webrtc.NetworkTypeTCP4,
	})

	var closers []func() error
	if c != nil && c.ICEAddr != "" {
		if isLoopbackICEAddr(c.ICEAddr) {
			settingEngine.SetIncludeLoopbackCandidate(true)
		}
		logger := logging.NewDefaultLoggerFactory().NewLogger("gizwebrtc")
		udpConn, err := net.ListenPacket("udp", c.ICEAddr)
		if err != nil {
			return nil, nil, fmt.Errorf("gizwebrtc: listen ICE UDP: %w", err)
		}
		closers = append(closers, udpConn.Close)
		settingEngine.SetICEUDPMux(webrtc.NewICEUDPMux(logger, udpConn))

		tcpListener, err := net.Listen("tcp", c.ICEAddr)
		if err != nil {
			_ = udpConn.Close()
			return nil, nil, fmt.Errorf("gizwebrtc: listen ICE TCP: %w", err)
		}
		closers = append(closers, tcpListener.Close)
		settingEngine.SetICETCPMux(webrtc.NewICETCPMux(logger, tcpListener, 0))
	}

	return webrtc.NewAPI(
		webrtc.WithMediaEngine(&mediaEngine),
		webrtc.WithSettingEngine(settingEngine),
	), closers, nil
}

func isLoopbackICEAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
