// Command tikv-tls-recovery owns disposable mTLS fixtures and verifies recovery
// through the production Storage constructor without reconstructing clients.
package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/tikv/client-go/v2/config"
	"github.com/tikv/client-go/v2/metrics"
)

func main() {
	if len(os.Args) != 3 || !filepath.IsAbs(os.Args[2]) {
		fmt.Fprintln(os.Stderr, "usage: tikv-tls-recovery certs|probe /absolute/fixture-directory")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "certs":
		err = generateFixtures(os.Args[2])
	case "probe":
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		err = probe(ctx, os.Args[2])
	default:
		err = errors.New("unknown fixture command")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generateFixtures(root string) error {
	for _, name := range []string{"a", "b"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		ca := &x509.Certificate{
			SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fixture-" + name + "-ca"},
			NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
			IsCA: true, BasicConstraintsValid: true,
			KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		}
		caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
		if err != nil {
			return err
		}
		if err := writePEM(filepath.Join(dir, "ca.pem"), "CERTIFICATE", caDER); err != nil {
			return err
		}
		for i, role := range []string{"server", "client"} {
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			if err != nil {
				return err
			}
			leaf := &x509.Certificate{
				SerialNumber: big.NewInt(int64(i + 2)), Subject: pkix.Name{CommonName: "fixture-" + name + "-" + role},
				NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature,
				ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			}
			if role == "server" {
				leaf.ExtKeyUsage = append(leaf.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
				leaf.DNSNames = []string{"pd-" + name, "tikv-" + name, "probe", "localhost"}
				leaf.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
			}
			der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
			if err != nil {
				return err
			}
			keyDER, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				return err
			}
			if err := writePEM(filepath.Join(dir, role+".pem"), "CERTIFICATE", der); err != nil {
				return err
			}
			if err := writePEM(filepath.Join(dir, role+"-key.pem"), "PRIVATE KEY", keyDER); err != nil {
				return err
			}
		}
		base := "/fixtures/" + name + "/"
		pdConfig := fmt.Sprintf("[security]\ncacert-path = %q\ncert-path = %q\nkey-path = %q\ncert-allowed-cn = [%q, %q]\n[replication]\nmax-replicas = 1\n", base+"ca.pem", base+"server.pem", base+"server-key.pem", "fixture-"+name+"-server", "fixture-"+name+"-client")
		tikvConfig := fmt.Sprintf("memory-usage-limit = \"1GB\"\n[security]\nca-path = %q\ncert-path = %q\nkey-path = %q\ncert-allowed-cn = [%q, %q]\n[storage.block-cache]\ncapacity = \"128MB\"\n", base+"ca.pem", base+"server.pem", base+"server-key.pem", "fixture-"+name+"-server", "fixture-"+name+"-client")
		for file, contents := range map[string]string{"pd.toml": pdConfig, "tikv.toml": tikvConfig} {
			if err := os.WriteFile(filepath.Join(dir, file), []byte(contents), 0600); err != nil {
				return err
			}
		}
	}
	return nil
}

func writePEM(path, kind string, der []byte) error {
	return os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), 0600)
}

type faultProxy struct {
	listener             net.Listener
	target               string
	ctx                  context.Context
	cancel               context.CancelFunc
	blocked              atomic.Bool
	closed               atomic.Bool
	connections          sync.Map
	workers              sync.WaitGroup
	accepted             chan struct{}
	tlsConnections       atomic.Int64
	plaintextConnections atomic.Int64
}

func newFaultProxy(ctx context.Context, address, target string) (*faultProxy, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	p := &faultProxy{listener: listener, target: target, ctx: ctx, cancel: cancel, accepted: make(chan struct{})}
	go p.accept()
	return p, nil
}

func (p *faultProxy) accept() {
	defer close(p.accepted)
	for {
		conn, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.workers.Go(func() { p.forward(conn) })
	}
}

func (p *faultProxy) forward(client net.Conn) {
	p.connections.Store(client, struct{}{})
	defer p.connections.Delete(client)
	defer client.Close()
	if p.blocked.Load() || p.closed.Load() {
		return
	}
	var first [1]byte
	if _, err := io.ReadFull(client, first[:]); err != nil {
		return
	}
	if first[0] == 0x16 {
		p.tlsConnections.Add(1)
	} else {
		p.plaintextConnections.Add(1)
	}
	backend, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(p.ctx, "tcp", p.target)
	if err != nil {
		return
	}
	p.connections.Store(backend, struct{}{})
	defer p.connections.Delete(backend)
	defer backend.Close()
	if p.blocked.Load() || p.closed.Load() {
		return
	}
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(backend, io.MultiReader(bytes.NewReader(first[:]), client)); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, backend); done <- struct{}{} }()
	<-done
	// A transport failure closes both directions; both copy workers are joined.
	_ = client.Close()
	_ = backend.Close()
	<-done
}

func (p *faultProxy) breakTransport() {
	p.blocked.Store(true)
	p.connections.Range(func(key, _ any) bool { _ = key.(net.Conn).Close(); return true })
}

func (p *faultProxy) close() {
	p.closed.Store(true)
	p.cancel()
	_ = p.listener.Close()
	p.breakTransport()
	<-p.accepted
	p.workers.Wait()
}

func healthProbeCount(address string) (uint64, error) {
	metric, ok := metrics.TiKVStatusDuration.WithLabelValues(address).(prometheus.Metric)
	if !ok {
		return 0, errors.New("SDK health histogram is unavailable")
	}
	var value dto.Metric
	if err := metric.Write(&value); err != nil {
		return 0, err
	}
	return value.GetHistogram().GetSampleCount(), nil
}

func waitFor(ctx context.Context, timeout time.Duration, check func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		if last = check(ctx); last == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("fixture deadline: %w", last)
		case <-ticker.C:
		}
	}
}

func connectorConfig(root, name string) storage.TiKVConfig {
	base := filepath.Join(root, name)
	return storage.TiKVConfig{PDAddresses: []string{"pd-" + name + ":2379"}, TLSCAFile: filepath.Join(base, "ca.pem"), TLSCertFile: filepath.Join(base, "client.pem"), TLSKeyFile: filepath.Join(base, "client-key.pem")}
}

func waitCluster(ctx context.Context, cfg storage.TiKVConfig) error {
	security := config.NewSecurity(cfg.TLSCAFile, cfg.TLSCertFile, cfg.TLSKeyFile, nil)
	tlsConfig, err := security.ToTLSConfig()
	if err != nil {
		return err
	}
	tlsConfig.MinVersion = tls.VersionTLS12
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	return waitFor(ctx, 40*time.Second, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+cfg.PDAddresses[0]+"/pd/api/v1/stores", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		var stores struct {
			Count  int `json:"count"`
			Stores []struct {
				Store struct {
					State string `json:"state_name"`
				} `json:"store"`
			} `json:"stores"`
		}
		if resp.StatusCode != http.StatusOK {
			detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			return fmt.Errorf("PD returned HTTP %d: %s", resp.StatusCode, detail)
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&stores); err != nil {
			return err
		}
		if stores.Count != 1 || len(stores.Stores) != 1 || stores.Stores[0].Store.State != "Up" {
			return errors.New("fixture store is not up")
		}
		return nil
	})
}

func writeAndRead(ctx context.Context, store kv.Store, key kv.Key) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	value := []byte("recovery-fixture")
	if err := store.Set(ctx, key, value); err != nil {
		return err
	}
	got, err := store.Get(ctx, key)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, value) {
		return errors.New("transactional value mismatch")
	}
	return nil
}

func probe(ctx context.Context, root string) error {
	a, err := newFaultProxy(ctx, ":20160", "tikv-a:20160")
	if err != nil {
		return err
	}
	defer a.close()
	b, err := newFaultProxy(ctx, ":20161", "tikv-b:20160")
	if err != nil {
		return err
	}
	defer b.close()
	configs := map[string]storage.Config{"a": connectorConfig(root, "a"), "b": connectorConfig(root, "b")}
	for _, name := range []string{"a", "b"} {
		if err := waitCluster(ctx, configs[name].(storage.TiKVConfig)); err != nil {
			return fmt.Errorf("cluster %s: %w", name, err)
		}
	}
	initialSecurity := config.GetGlobalConfig().Security
	physical, err := storage.New(configs)
	if err != nil {
		return err
	}
	defer physical.Close()
	stores := map[string]*kv.TiKV{}
	for _, name := range []string{"a", "b"} {
		client, _, err := physical.TiKV(name)
		if err != nil {
			return err
		}
		store, err := kv.NewTiKVWithClient(client, kv.TiKVReadOptions{}, nil)
		if err != nil {
			return err
		}
		stores[name] = store
		if err := writeAndRead(ctx, store, kv.Key{"tls-recovery", name}); err != nil {
			return fmt.Errorf("initial %s: %w", name, err)
		}
	}
	before, err := healthProbeCount("probe:20160")
	if err != nil {
		return err
	}
	a.breakTransport()
	broken := make(chan error, 1)
	go func() {
		attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_, err := stores["a"].Get(attempt, kv.Key{"tls-recovery", "a"})
		broken <- err
	}()
	if err := waitFor(ctx, 7*time.Second, func(context.Context) error {
		count, err := healthProbeCount("probe:20160")
		if err != nil {
			return err
		}
		if count <= before {
			return errors.New("fault did not exercise SDK liveness probing")
		}
		return nil
	}); err != nil {
		return err
	}
	if err := writeAndRead(ctx, stores["b"], kv.Key{"tls-recovery", "b"}); err != nil {
		return fmt.Errorf("independent CA connector during outage: %w", err)
	}
	a.blocked.Store(false)
	if err := waitFor(ctx, 20*time.Second, func(ctx context.Context) error { return writeAndRead(ctx, stores["a"], kv.Key{"tls-recovery", "a"}) }); err != nil {
		return fmt.Errorf("same-client mTLS recovery: %w", err)
	}
	select {
	case <-broken:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := writeAndRead(ctx, stores["b"], kv.Key{"tls-recovery", "b"}); err != nil {
		return err
	}
	if a.plaintextConnections.Load() != 0 || b.plaintextConnections.Load() != 0 {
		return errors.New("a connector attempted plaintext transport")
	}
	if !reflect.DeepEqual(initialSecurity, config.GetGlobalConfig().Security) {
		return errors.New("connector changed global TLS configuration")
	}
	count, err := healthProbeCount("probe:20160")
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"same_client_recovered": true, "independent_ca_remained_available": true, "global_security_unchanged": true, "health_probes_after_fault": count - before, "tls_connections": a.tlsConnections.Load() + b.tlsConnections.Load(), "plaintext_connections": 0})
}
