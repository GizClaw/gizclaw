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
}

func connectClients(ctx context.Context, specs map[string]giztest.ClientSpec, steps []giztest.Step, vars *giztest.Variables) (*clientSet, error) {
	set := &clientSet{clients: map[string]*gizcli.Client{}, serve: map[string]<-chan error{}, inbound: map[string]*inboundCounter{}, endpoints: map[string]string{}}
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
		client := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			return gizwebrtc.Dial(ctx, key, info.TransportPublicKey, gizwebrtc.DialConfig{SignalingURL: info.SignalingURL, ICEServers: info.ICEServers, SecurityPolicy: policy})
		}}
		if err := configureClientRPC(client, name, steps, vars, set.inbound); err != nil {
			return set, err
		}
		if err := client.Dial(info.PublicKey, endpoint); err != nil {
			_ = client.Close()
			return set, fmt.Errorf("client %s dial: %w", name, err)
		}
		errCh := make(chan error, 1)
		go func() { errCh <- client.Serve() }()
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
	c := s.clients[name]
	if c == nil {
		return nil, fmt.Errorf("unknown client %q", name)
	}
	return c, nil
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
