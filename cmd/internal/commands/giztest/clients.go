package giztestcmd

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

type clientSet struct {
	mu        sync.Mutex
	clients   map[string]*gizcli.Client
	serve     map[string]<-chan error
	inbound   map[string]*inboundCounter
	endpoints map[string]string
	// dial redials one client on its existing identity, used by reconnect.
	dial map[string]func(context.Context) (*gizcli.Client, <-chan error, error)
}

func connectClients(ctx context.Context, specs map[string]giztest.ClientSpec, steps []giztest.Step, vars *giztest.Variables) (*clientSet, error) {
	set := &clientSet{
		clients:   map[string]*gizcli.Client{},
		serve:     map[string]<-chan error{},
		inbound:   map[string]*inboundCounter{},
		endpoints: map[string]string{},
		dial:      map[string]func(context.Context) (*gizcli.Client, <-chan error, error){},
	}
	names := make([]string, 0, len(specs))
	for name := range specs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		spec := specs[name]
		endpointValue, err := vars.Resolve(spec.AccessPoint)
		if err != nil {
			return set, err
		}
		endpoint, ok := endpointValue.(string)
		if !ok {
			return set, fmt.Errorf("client %s access_point must resolve to string", name)
		}
		info, err := gizcli.FetchServerInfo(ctx, endpoint)
		if err != nil {
			return set, fmt.Errorf("client %s: %w", name, err)
		}
		set.endpoints[name] = endpoint
		key, err := giznet.GenerateKeyPair()
		if err != nil {
			return set, err
		}
		// dial brings up one connection on this identity. A reconnect calls it
		// again so the Server sees the same device on a replacement Peer.
		dial := func(ctx context.Context) (*gizcli.Client, <-chan error, error) {
			client := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
				return gizwebrtc.Dial(ctx, key, info.TransportPublicKey, gizwebrtc.DialConfig{SignalingURL: info.SignalingURL, ICEServers: info.ICEServers, SecurityPolicy: policy})
			}}
			if err := configureClientRPC(client, name, steps, vars, set.inbound); err != nil {
				return nil, nil, err
			}
			if err := client.Dial(info.PublicKey, endpoint); err != nil {
				_ = client.Close()
				return nil, nil, fmt.Errorf("client %s dial: %w", name, err)
			}
			errCh := make(chan error, 1)
			go func() { errCh <- client.Serve() }()
			return client, errCh, nil
		}
		set.dial[name] = dial
		client, errCh, err := dial(ctx)
		if err != nil {
			return set, err
		}
		set.clients[name], set.serve[name] = client, errCh
		if spec.RegistrationToken != "" {
			tokenValue, err := vars.Resolve(spec.RegistrationToken)
			if err != nil {
				return set, err
			}
			token, ok := tokenValue.(string)
			if !ok {
				return set, fmt.Errorf("client %s registration_token must resolve to string", name)
			}
			if _, err := client.Register(ctx, "giztest.register."+name, token); err != nil {
				return set, fmt.Errorf("client %s register: %w", name, err)
			}
		}
	}
	return set, nil
}

func (s *clientSet) fingerprints() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]string, len(s.clients))
	for name, client := range s.clients {
		if client != nil && client.KeyPair != nil {
			result[name] = client.KeyPair.Public.ShortString()
		}
	}
	return result
}

func (s *clientSet) endpoint(name string) (string, error) {
	endpoint := s.endpoints[name]
	if endpoint == "" {
		return "", fmt.Errorf("unknown client %q", name)
	}
	return endpoint, nil
}

func (s *clientSet) get(name string) (*gizcli.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.clients[name]
	if c == nil {
		return nil, fmt.Errorf("unknown client %q", name)
	}
	return c, nil
}

// reconnect closes one client's Peer connection and dials a replacement on the
// same identity, which is how a device that switched network or rebooted
// reaches the Server again. The client RPC providers are reinstalled on the
// new connection and keep their existing inbound counters.
//
// awaitMs bounds the replacement dial. That context governs signaling only,
// not the lifetime of the connection the dial returns, so bounding it here
// does not shorten the reconnected client's life.
func (s *clientSet) reconnect(ctx context.Context, name string, awaitMs int) error {
	if awaitMs > 0 {
		bounded, cancel := context.WithTimeout(ctx, time.Duration(awaitMs)*time.Millisecond)
		defer cancel()
		ctx = bounded
	}
	s.mu.Lock()
	previous, dial := s.clients[name], s.dial[name]
	s.mu.Unlock()
	if previous == nil || dial == nil {
		return fmt.Errorf("unknown client %q", name)
	}
	_ = previous.Close()
	s.mu.Lock()
	served := s.serve[name]
	s.mu.Unlock()
	if served != nil {
		select {
		case <-served:
		case <-time.After(2 * time.Second):
		}
	}
	client, errCh, err := dial(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.clients[name], s.serve[name] = client, errCh
	s.mu.Unlock()
	return nil
}

func (s *clientSet) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.clients {
		_ = c.Close()
	}
	for _, ch := range s.serve {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
		}
	}
}
