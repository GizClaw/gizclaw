package endpointhealth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type State string

const (
	Checking        State = "checking"
	Reachable       State = "reachable"
	Unreachable     State = "unreachable"
	InvalidResponse State = "invalid-response"
)

type Result struct {
	Endpoint  string `json:"endpoint"`
	State     State  `json:"state"`
	PublicKey string `json:"public_key,omitempty"`
	CheckedAt string `json:"checked_at,omitempty"`
	Message   string `json:"message,omitempty"`
}

type Prober struct {
	Client      *http.Client
	Concurrency int

	mu    sync.RWMutex
	cache map[string]Result
}

func New() *Prober {
	return &Prober{Client: &http.Client{Timeout: 4 * time.Second}, Concurrency: 6, cache: map[string]Result{}}
}

func (p *Prober) Get(endpoint string) Result {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if result, ok := p.cache[endpoint]; ok {
		return result
	}
	return Result{Endpoint: endpoint, State: Checking}
}

func (p *Prober) ProbeAll(ctx context.Context, endpoints []string) []Result {
	concurrency := p.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	results := make([]Result, len(endpoints))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, endpoint := range endpoints {
		i, endpoint := i, endpoint
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[i] = p.remember(Result{Endpoint: endpoint, State: Unreachable, CheckedAt: time.Now().UTC().Format(time.RFC3339), Message: ctx.Err().Error()})
				return
			}
			results[i] = p.Probe(ctx, endpoint)
		}()
	}
	wg.Wait()
	return results
}

func (p *Prober) Probe(ctx context.Context, endpoint string) Result {
	now := time.Now().UTC().Format(time.RFC3339)
	result := Result{Endpoint: endpoint, State: Unreachable, CheckedAt: now}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+endpoint+"/server-info", nil)
	if err != nil {
		result.State = InvalidResponse
		result.Message = err.Error()
		return p.remember(result)
	}
	response, err := p.Client.Do(request)
	if err != nil {
		result.Message = err.Error()
		if ctx.Err() != nil {
			return p.remember(result)
		}
		return p.remember(result)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		result.State = InvalidResponse
		result.Message = fmt.Sprintf("server-info returned HTTP %d", response.StatusCode)
		return p.remember(result)
	}
	var info apitypes.ServerInfo
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64<<10))
	if err := decoder.Decode(&info); err != nil {
		result.State = InvalidResponse
		result.Message = "server-info is not valid JSON"
		return p.remember(result)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		result.State = InvalidResponse
		result.Message = "server-info contains trailing data"
		return p.remember(result)
	}
	var publicKey giznet.PublicKey
	if info.Protocol != "gizclaw-webrtc" || publicKey.UnmarshalText([]byte(info.PublicKey)) != nil || !strings.HasPrefix(info.SignalingPath, "/") || info.ServerTime == 0 {
		result.State = InvalidResponse
		result.Message = "server-info is not a GizClaw server response"
		return p.remember(result)
	}
	if host, port, splitErr := net.SplitHostPort(info.Endpoint); splitErr != nil || host == "" || port == "" {
		result.State = InvalidResponse
		result.Message = "server-info is not a GizClaw server response"
		return p.remember(result)
	}
	result.State = Reachable
	result.PublicKey = publicKey.String()
	return p.remember(result)
}

func (p *Prober) remember(result Result) Result {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cache == nil {
		p.cache = map[string]Result{}
	}
	p.cache[result.Endpoint] = result
	return result
}
