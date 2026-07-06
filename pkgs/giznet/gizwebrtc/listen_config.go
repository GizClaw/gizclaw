package gizwebrtc

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/pion/ice/v4"
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
	// ICEUDPAddr is the UDP bind address used for the shared WebRTC ICE mux.
	// If set, it takes precedence over ICEAddr for UDP.
	ICEUDPAddr string
	// ICETCPAddr is the TCP bind address used for the shared WebRTC ICE mux.
	// If set, it takes precedence over ICEAddr for TCP.
	ICETCPAddr string

	SecurityPolicy   giznet.SecurityPolicy
	PeerEventHandler giznet.PeerEventHandler
	CipherMode       CipherMode
	NAT1To1IPs       []string
	ICELite          bool
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
	settingEngine.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	if iceLite(c) {
		settingEngine.SetLite(true)
	}
	if ips := nat1To1IPs(c); len(ips) > 0 {
		settingEngine.SetNAT1To1IPs(ips, webrtc.ICECandidateTypeHost)
	}

	var closers []func() error
	udpAddr, tcpAddr := iceMuxAddrs(c)
	var networkTypes []webrtc.NetworkType
	if udpAddr != "" {
		if isLoopbackICEAddr(udpAddr) {
			settingEngine.SetIncludeLoopbackCandidate(true)
		}
		logger := logging.NewDefaultLoggerFactory().NewLogger("gizwebrtc")
		udpConn, err := net.ListenPacket("udp", udpAddr)
		if err != nil {
			return nil, nil, fmt.Errorf("gizwebrtc: listen ICE UDP: %w", err)
		}
		closers = append(closers, udpConn.Close)
		settingEngine.SetICEUDPMux(webrtc.NewICEUDPMux(logger, udpConn))
		networkTypes = append(networkTypes, webrtc.NetworkTypeUDP4)
	}

	if tcpAddr != "" {
		if isLoopbackICEAddr(tcpAddr) {
			settingEngine.SetIncludeLoopbackCandidate(true)
		}
		logger := logging.NewDefaultLoggerFactory().NewLogger("gizwebrtc")
		tcpListener, err := net.Listen("tcp", tcpAddr)
		if err != nil {
			for _, closeFn := range closers {
				_ = closeFn()
			}
			return nil, nil, fmt.Errorf("gizwebrtc: listen ICE TCP: %w", err)
		}
		closers = append(closers, tcpListener.Close)
		settingEngine.SetICETCPMux(webrtc.NewICETCPMux(logger, tcpListener, 0))
		networkTypes = append(networkTypes, webrtc.NetworkTypeTCP4)
	}
	if len(networkTypes) > 0 {
		settingEngine.SetNetworkTypes(networkTypes)
	} else {
		settingEngine.SetNetworkTypes([]webrtc.NetworkType{
			webrtc.NetworkTypeUDP4,
		})
	}

	return webrtc.NewAPI(
		webrtc.WithMediaEngine(&mediaEngine),
		webrtc.WithSettingEngine(settingEngine),
	), closers, nil
}

func iceLite(c *ListenConfig) bool {
	if c != nil && c.ICELite {
		return true
	}
	value := strings.TrimSpace(os.Getenv("GIZCLAW_WEBRTC_ICE_LITE"))
	if value == "" {
		value = strings.TrimSpace(os.Getenv("GIZCLAW_E2E_WEBRTC_ICE_LITE"))
	}
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func nat1To1IPs(c *ListenConfig) []string {
	if c != nil && len(c.NAT1To1IPs) > 0 {
		return c.NAT1To1IPs
	}
	value := strings.TrimSpace(os.Getenv("GIZCLAW_WEBRTC_NAT1TO1_IPS"))
	if value == "" {
		value = strings.TrimSpace(os.Getenv("GIZCLAW_E2E_WEBRTC_NAT1TO1_IPS"))
	}
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := parts[:0]
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func iceMuxAddrs(c *ListenConfig) (udpAddr string, tcpAddr string) {
	if c == nil {
		udpAddr = ""
		tcpAddr = ""
	} else {
		udpAddr = c.ICEUDPAddr
		tcpAddr = c.ICETCPAddr
		if c.ICEAddr != "" {
			if udpAddr == "" {
				udpAddr = c.ICEAddr
			}
			if tcpAddr == "" {
				tcpAddr = c.ICEAddr
			}
		}
	}
	if udpAddr == "" {
		udpAddr = strings.TrimSpace(os.Getenv("GIZCLAW_WEBRTC_ICE_UDP_ADDR"))
		if udpAddr == "" {
			udpAddr = strings.TrimSpace(os.Getenv("GIZCLAW_E2E_WEBRTC_ICE_UDP_ADDR"))
		}
	}
	if tcpAddr == "" {
		tcpAddr = strings.TrimSpace(os.Getenv("GIZCLAW_WEBRTC_ICE_TCP_ADDR"))
		if tcpAddr == "" {
			tcpAddr = strings.TrimSpace(os.Getenv("GIZCLAW_E2E_WEBRTC_ICE_TCP_ADDR"))
		}
	}
	return udpAddr, tcpAddr
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
