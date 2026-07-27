package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pion/turn/v4"
)

const readyFile = "/tmp/gizclaw-e2e-turn-ready"

type config struct {
	listen       string
	relayAddress net.IP
	realm        string
	username     string
	credential   string
	minPort      uint16
	maxPort      uint16
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "gizclaw e2e turn: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	packetConn, err := net.ListenPacket("udp", cfg.listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer packetConn.Close()
	server, err := turn.NewServer(turn.ServerConfig{
		Realm: cfg.realm,
		AuthHandler: func(username, realm string, _ net.Addr) ([]byte, bool) {
			return authKey(cfg, username, realm, time.Now())
		},
		PacketConnConfigs: []turn.PacketConnConfig{{
			PacketConn: packetConn,
			RelayAddressGenerator: &turn.RelayAddressGeneratorPortRange{
				RelayAddress: cfg.relayAddress,
				Address:      "0.0.0.0",
				MinPort:      cfg.minPort,
				MaxPort:      cfg.maxPort,
			},
		}},
	})
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	defer server.Close()
	if err := os.WriteFile(readyFile, []byte("ready\n"), 0o644); err != nil {
		return fmt.Errorf("write ready file: %w", err)
	}
	defer os.Remove(readyFile)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	return nil
}

func loadConfig() (config, error) {
	minPort, err := envUint16("GIZCLAW_E2E_TURN_RELAY_MIN_PORT")
	if err != nil {
		return config{}, err
	}
	maxPort, err := envUint16("GIZCLAW_E2E_TURN_RELAY_MAX_PORT")
	if err != nil {
		return config{}, err
	}
	cfg := config{
		listen:       strings.TrimSpace(os.Getenv("GIZCLAW_E2E_TURN_LISTEN")),
		relayAddress: net.ParseIP(strings.TrimSpace(os.Getenv("GIZCLAW_E2E_TURN_RELAY_ADDRESS"))),
		realm:        strings.TrimSpace(os.Getenv("GIZCLAW_E2E_TURN_REALM")),
		username:     strings.TrimSpace(os.Getenv("GIZCLAW_E2E_TURN_USERNAME")),
		credential:   strings.TrimSpace(os.Getenv("GIZCLAW_E2E_TURN_CREDENTIAL")),
		minPort:      minPort,
		maxPort:      maxPort,
	}
	switch {
	case cfg.listen == "":
		return config{}, errors.New("GIZCLAW_E2E_TURN_LISTEN is required")
	case cfg.relayAddress == nil:
		return config{}, errors.New("GIZCLAW_E2E_TURN_RELAY_ADDRESS must be an IP address")
	case cfg.realm == "":
		return config{}, errors.New("GIZCLAW_E2E_TURN_REALM is required")
	case cfg.credential == "":
		return config{}, errors.New("GIZCLAW_E2E_TURN_CREDENTIAL is required")
	case cfg.minPort > cfg.maxPort:
		return config{}, errors.New("TURN relay port range is reversed")
	default:
		return cfg, nil
	}
}

func envUint16(name string) (uint16, error) {
	value, err := strconv.ParseUint(strings.TrimSpace(os.Getenv(name)), 10, 16)
	if err != nil {
		return 0, fmt.Errorf("%s must be a uint16: %w", name, err)
	}
	return uint16(value), nil
}

func authKey(cfg config, username, realm string, now time.Time) ([]byte, bool) {
	if realm != cfg.realm {
		return nil, false
	}
	if username == cfg.username {
		return turn.GenerateAuthKey(username, realm, cfg.credential), true
	}
	parts := strings.SplitN(strings.TrimSpace(username), ":", 2)
	if len(parts) != 2 || parts[1] != cfg.username {
		return nil, false
	}
	expires, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || expires < now.Unix() {
		return nil, false
	}
	mac := hmac.New(sha1.New, []byte(cfg.credential))
	_, _ = mac.Write([]byte(username))
	credential := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return turn.GenerateAuthKey(username, realm, credential), true
}
