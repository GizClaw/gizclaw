package gizedge

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

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
	relayBindAddress, err := turnRelayBindAddress(cfg.Listen)
	if err != nil {
		_ = packetConn.Close()
		return nil, err
	}
	runtime := &turnRuntime{packetConn: packetConn}
	server, err := turn.NewServer(turn.ServerConfig{
		Realm: cfg.Realm,
		AuthHandler: func(username, realm string, _ net.Addr) ([]byte, bool) {
			return turnAuthKey(cfg, username, realm, time.Now())
		},
		PacketConnConfigs: []turn.PacketConnConfig{
			{
				PacketConn: packetConn,
				RelayAddressGenerator: &turn.RelayAddressGeneratorPortRange{
					RelayAddress: relayIP,
					Address:      relayBindAddress,
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

func turnAuthKey(cfg TURNConfig, username, realm string, now time.Time) ([]byte, bool) {
	if realm != cfg.Realm {
		return nil, false
	}
	if username == cfg.Username {
		return turn.GenerateAuthKey(username, realm, cfg.Credential), true
	}
	if credential, ok := turnRESTCredential(cfg, username, now); ok {
		return turn.GenerateAuthKey(username, realm, credential), true
	}
	return nil, false
}

func turnRESTCredential(cfg TURNConfig, username string, now time.Time) (string, bool) {
	username = strings.TrimSpace(username)
	if username == "" || strings.TrimSpace(cfg.Credential) == "" {
		return "", false
	}
	parts := strings.SplitN(username, ":", 2)
	if strings.TrimSpace(cfg.Username) != "" {
		if len(parts) != 2 || parts[1] != cfg.Username {
			return "", false
		}
	} else if len(parts) != 1 {
		return "", false
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || expires < now.Unix() {
		return "", false
	}
	mac := hmac.New(sha1.New, []byte(cfg.Credential))
	_, _ = mac.Write([]byte(username))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), true
}

func turnRelayBindAddress(listen string) (string, error) {
	host, _, err := netSplitHostPort("turn.listen", listen)
	if err != nil {
		return "", err
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "0.0.0.0", nil
	}
	return host, nil
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
