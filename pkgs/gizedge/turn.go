package gizedge

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/pion/turn/v4"
)

type turnRuntime struct {
	packetConn net.PacketConn
	server     *turn.Server
}

func startTURN(cfg TURNConfig) (*turnRuntime, error) {
	if !cfg.enabled() {
		return nil, nil
	}
	host, _, err := netSplitHostPort("turn.public-endpoint", cfg.PublicEndpoint)
	if err != nil {
		return nil, err
	}
	relayAddress := strings.TrimSpace(cfg.RelayAddress)
	if relayAddress == "" {
		relayAddress = host
	}
	relayIP := net.ParseIP(relayAddress)
	if relayIP == nil {
		return nil, fmt.Errorf("edge: turn.relay-address must be an IP address")
	}
	packetConn, err := net.ListenPacket("udp", cfg.Listen)
	if err != nil {
		return nil, fmt.Errorf("edge: listen turn udp: %w", err)
	}
	runtime := &turnRuntime{packetConn: packetConn}
	server, err := turn.NewServer(turn.ServerConfig{
		Realm: cfg.Realm,
		AuthHandler: func(username, realm string, _ net.Addr) ([]byte, bool) {
			if username != cfg.Username || realm != cfg.Realm {
				return nil, false
			}
			return turn.GenerateAuthKey(username, realm, cfg.Credential), true
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: packetConn,
				RelayAddressGenerator: &turn.RelayAddressGeneratorPortRange{
					RelayAddress: relayIP,
					Address:      "0.0.0.0",
					MinPort:      cfg.RelayMinPort,
					MaxPort:      cfg.RelayMaxPort,
				},
			},
		},
	})
	if err != nil {
		_ = packetConn.Close()
		return nil, fmt.Errorf("edge: start turn server: %w", err)
	}
	runtime.server = server
	return runtime, nil
}

func (r *turnRuntime) Close() error {
	if r == nil {
		return nil
	}
	var errs []error
	if r.server != nil {
		errs = append(errs, r.server.Close())
		r.server = nil
	}
	if r.packetConn != nil {
		errs = append(errs, r.packetConn.Close())
		r.packetConn = nil
	}
	return errors.Join(errs...)
}
