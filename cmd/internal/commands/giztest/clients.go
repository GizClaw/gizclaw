package giztest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

const maxServerInfoBytes = 1 << 20

type clientSet struct {
	mu      sync.Mutex
	clients map[string]*gizcli.Client
	serve   map[string]<-chan error
	inbound map[string]*inboundCounter
}
type serverInfo struct {
	publicKey, transportKey giznet.PublicKey
	signalingURL            string
	ice                     []gizwebrtc.ICEServer
}

func connectClients(ctx context.Context, specs map[string]ClientSpec, steps []Step, vars *variables) (*clientSet, error) {
	set := &clientSet{clients: map[string]*gizcli.Client{}, serve: map[string]<-chan error{}, inbound: map[string]*inboundCounter{}}
	names := make([]string, 0, len(specs))
	for name := range specs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		spec := specs[name]
		endpointValue, err := vars.resolve(spec.AccessPoint)
		if err != nil {
			set.Close()
			return nil, err
		}
		endpoint, ok := endpointValue.(string)
		if !ok {
			set.Close()
			return nil, fmt.Errorf("client %s access_point must resolve to string", name)
		}
		info, err := fetchServerInfo(ctx, endpoint)
		if err != nil {
			set.Close()
			return nil, fmt.Errorf("client %s: %w", name, err)
		}
		key, err := giznet.GenerateKeyPair()
		if err != nil {
			set.Close()
			return nil, err
		}
		client := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			return gizwebrtc.Dial(ctx, key, info.transportKey, gizwebrtc.DialConfig{SignalingURL: info.signalingURL, ICEServers: info.ice, SecurityPolicy: policy})
		}}
		if err := configureClientRPC(client, name, steps, vars, set.inbound); err != nil {
			set.Close()
			return nil, err
		}
		if err := client.Dial(info.publicKey, endpoint); err != nil {
			set.Close()
			return nil, fmt.Errorf("client %s dial: %w", name, err)
		}
		errCh := make(chan error, 1)
		go func() { errCh <- client.Serve() }()
		set.clients[name], set.serve[name] = client, errCh
		if spec.RegistrationToken != "" {
			tokenValue, err := vars.resolve(spec.RegistrationToken)
			if err != nil {
				set.Close()
				return nil, err
			}
			token, ok := tokenValue.(string)
			if !ok {
				set.Close()
				return nil, fmt.Errorf("client %s registration_token must resolve to string", name)
			}
			if _, err := client.Register(ctx, "giztest.register."+name, token); err != nil {
				set.Close()
				return nil, fmt.Errorf("client %s register: %w", name, err)
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

func fetchServerInfo(ctx context.Context, endpoint string) (serverInfo, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" || strings.Contains(endpoint, "://") {
		return serverInfo{}, fmt.Errorf("access_point must be host[:port]")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+endpoint+"/server-info", nil)
	if err != nil {
		return serverInfo{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return serverInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return serverInfo{}, fmt.Errorf("server-info status %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxServerInfoBytes+1))
	if err != nil {
		return serverInfo{}, err
	}
	if len(data) > maxServerInfoBytes {
		return serverInfo{}, fmt.Errorf("server-info exceeds %d bytes", maxServerInfoBytes)
	}
	var body struct {
		PublicKey     string                                                     `json:"public_key"`
		Protocol      string                                                     `json:"protocol"`
		SignalingPath string                                                     `json:"signaling_path"`
		ICEServers    []gizwebrtc.ICEServer                                      `json:"ice_servers"`
		Transport     *struct{ Mode, Endpoint, PublicKey, SignalingPath string } `json:"transport"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return serverInfo{}, err
	}
	if body.Protocol != "" && body.Protocol != "gizclaw-webrtc" {
		return serverInfo{}, fmt.Errorf("unsupported protocol %q", body.Protocol)
	}
	var serverPK giznet.PublicKey
	if err := serverPK.UnmarshalText([]byte(strings.TrimSpace(body.PublicKey))); err != nil || serverPK.IsZero() {
		return serverInfo{}, fmt.Errorf("invalid server public key")
	}
	transportPK, host, path, ice := serverPK, endpoint, strings.TrimSpace(body.SignalingPath), body.ICEServers
	if body.Transport != nil {
		if body.Transport.Mode != "edge-gateway" {
			return serverInfo{}, fmt.Errorf("unsupported transport mode %q", body.Transport.Mode)
		}
		host = strings.TrimSpace(body.Transport.Endpoint)
		if strings.Contains(host, "://") || host == "" {
			return serverInfo{}, fmt.Errorf("invalid transport endpoint")
		}
		if err := transportPK.UnmarshalText([]byte(strings.TrimSpace(body.Transport.PublicKey))); err != nil || transportPK.IsZero() {
			return serverInfo{}, fmt.Errorf("invalid transport key")
		}
		path, ice = strings.TrimSpace(body.Transport.SignalingPath), nil
	}
	if path == "" {
		path = gizwebrtc.SignalingPath
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return serverInfo{}, fmt.Errorf("invalid signaling path")
	}
	return serverInfo{publicKey: serverPK, transportKey: transportPK, signalingURL: (&url.URL{Scheme: "http", Host: host, Path: path}).String(), ice: ice}, nil
}
