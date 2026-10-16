package storage

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"maps"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/tikv/client-go/v2/config"
	"github.com/tikv/client-go/v2/tikv"
	pd "github.com/tikv/pd/client"
	"github.com/tikv/pd/client/opt"
	"github.com/tikv/pd/client/pkg/caller"
)

// TiKV returns a borrowed transactional client and its read configuration.
// Storage owns the client's lifetime. Returned configuration maps are copies.
func (s *Storage) TiKV(name string) (*tikv.KVStore, TiKVConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, TiKVConfig{}, errors.New("storage: registry is closed")
	}
	client, ok := s.tikvs[name]
	if !ok {
		return nil, TiKVConfig{}, fmt.Errorf("storage: tikv %q not found", name)
	}
	cfg := s.configs[name].(TiKVConfig)
	cfg.PDAddresses = append([]string(nil), cfg.PDAddresses...)
	cfg.ReadLabels = maps.Clone(cfg.ReadLabels)
	return client, cfg, nil
}

// ValidateTiKVConfig checks addresses and read/TLS settings without network I/O.
func ValidateTiKVConfig(cfg TiKVConfig) error {
	if len(cfg.PDAddresses) == 0 {
		return errors.New("tikv requires pd_addresses")
	}
	seen := make(map[string]bool)
	for _, address := range cfg.PDAddresses {
		host, port, err := net.SplitHostPort(address)
		number, portErr := strconv.Atoi(port)
		if err != nil || portErr != nil || number < 1 || number > 65535 || strings.TrimSpace(host) == "" || port == "" || strings.ContainsAny(address, "/ \t\n") || seen[address] {
			return errors.New("tikv pd_addresses must be unique host:port addresses")
		}
		seen[address] = true
	}
	if cfg.StaleReadDelay < 0 || cfg.StaleReadDelay > 24*time.Hour || cfg.StaleReadDelay%time.Millisecond != 0 {
		return errors.New("tikv stale_read_delay must be between 1ms and 24h, or zero for strong reads")
	}
	if (cfg.StaleReadDelay > 0) != (len(cfg.ReadLabels) > 0) {
		return errors.New("tikv stale_read_delay and read_labels must be configured together")
	}
	for key, value := range cfg.ReadLabels {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" || key != strings.TrimSpace(key) || value != strings.TrimSpace(value) {
			return errors.New("tikv read_labels requires non-empty clean keys and values")
		}
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") || cfg.TLSCAFile == "" && cfg.TLSCertFile != "" {
		return errors.New("tikv TLS requires tls_ca_file and a complete optional client certificate/key pair")
	}
	return nil
}

func newTiKV(name string, cfg TiKVConfig) (*tikv.KVStore, error) {
	if err := ValidateTiKVConfig(cfg); err != nil {
		return nil, fmt.Errorf("storage: tikv %q: %w", name, err)
	}
	security := config.NewSecurity(cfg.TLSCAFile, cfg.TLSCertFile, cfg.TLSKeyFile, nil)
	tlsConfig, err := security.ToTLSConfig()
	if err != nil {
		return nil, tiKVOpenError(name, "configure TLS", err)
	}
	if tlsConfig != nil {
		tlsConfig.MinVersion = tls.VersionTLS12
		if cfg.TLSCertFile != "" {
			if _, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil {
				return nil, tiKVOpenError(name, "load client certificate", err)
			}
		}
	}
	// Build per-connector clients so TLS settings never mutate client-go globals.
	// The classic PD region API also works with the TiKV 8.5 LTS cluster;
	// the newer router stream requires PD 9.0 and is deliberately disabled.
	client, err := pd.NewClient(caller.Component("gizclaw"), cfg.PDAddresses, pd.SecurityOption{
		CAPath: cfg.TLSCAFile, CertPath: cfg.TLSCertFile, KeyPath: cfg.TLSKeyFile,
	}, opt.WithCustomTimeoutOption(5*time.Second), opt.WithEnableRouterClient(false))
	if err != nil {
		return nil, tiKVOpenError(name, "connect PD", err)
	}
	codec := tikv.NewCodecPDClient(tikv.ModeTxn, client)
	safePoint, err := tikv.NewEtcdSafePointKV(cfg.PDAddresses, tlsConfig)
	if err != nil {
		client.Close()
		return nil, tiKVOpenError(name, "connect safe point", err)
	}
	rpc := tikv.NewRPCClient(tikv.WithSecurity(security), tikv.WithCodec(codec.GetCodec()))
	store, err := tikv.NewKVStore(fmt.Sprintf("tikv-%d", client.GetClusterID(context.Background())), codec, safePoint, rpc,
		tikv.WithStoreLivenessSecurity(security))
	if err != nil {
		_ = rpc.Close()
		_ = safePoint.Close()
		client.Close()
		return nil, tiKVOpenError(name, "open transactional client", err)
	}
	return store, nil
}

func tiKVOpenError(name, operation string, err error) error {
	return &externalOperationError{operation: fmt.Sprintf("storage: tikv %q %s", name, operation), err: err}
}
