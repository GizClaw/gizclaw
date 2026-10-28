// Package peerquota owns external quota decisions and provider-call lifetimes.
package peerquota

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizlog"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/sdk/go/quota"
)

var (
	ErrDenied      = &quotaError{message: "peerquota: usable time has expired", exhausted: true}
	ErrUnavailable = &quotaError{message: "peerquota: no valid quota result"}
	ErrClosed      = &quotaError{message: "peerquota: service is closed"}
)

const requestTimeout = 5 * time.Second
const retryDelay = time.Second
const idleLifetime = 5 * time.Minute
const warningInterval = time.Minute

// Reporter returns available identifiers and flushed retained hourly usage.
type Reporter interface {
	QuotaReport(context.Context, giznet.PublicKey) (quota.QuotaRequest, error)
}

type entryKey struct {
	peer   giznet.PublicKey
	policy apitypes.RuntimeProfileQuotaCustom
}

// Service borrows its report source and owns lazy per-identity workers.
// New does not start goroutines or perform I/O. Close cancels active calls,
// joins the workers and must precede closing the report source's SQL pools.
type Service struct {
	reporter Reporter
	client   *http.Client
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	entries  map[entryKey]*entry
	closed   bool
	wg       sync.WaitGroup
	done     chan struct{}
}

// New constructs an idle quota controller. All Peers and policies share one
// HTTP client and keep-alive connection pool, owned until Close joins workers.
func New(reporter Reporter) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: requestTimeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       time.Minute,
		TLSHandshakeTimeout:   requestTimeout,
		ExpectContinueTimeout: time.Second,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   requestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Service{reporter: reporter, client: client, ctx: ctx, cancel: cancel, entries: make(map[entryKey]*entry), done: make(chan struct{})}
}

type acquisition struct {
	ctx   context.Context
	reply chan acquired
}
type acquired struct {
	ctx     context.Context
	release func()
	err     error
}
type fetched struct {
	decision *quota.QuotaResponse
	err      error
}
type entry struct {
	service *Service
	key     entryKey
	policy  apitypes.RuntimeProfileQuotaCustom
	client  *quota.ClientWithResponses
	ctx     context.Context
	cancel  context.CancelFunc
	acquire chan acquisition
	release chan struct{}
	done    chan struct{}
}

// Authorize uses the last known decision without waiting for the quota hook.
// Unknown or stale decisions allow use until an explicit usable deadline expires.
// Call release when the invocation/stream ends. Caller cancellation also
// releases its lease. A denied result remains refreshable without active calls.
func (s *Service) Authorize(ctx context.Context, peer giznet.PublicKey, policy apitypes.RuntimeProfileQuotaCustom) (context.Context, func(), error) {
	if policy.Type != apitypes.RuntimeProfileQuotaCustomTypeCustom || peer.IsZero() || policy.Endpoint == "" || policy.ApiKey == "" {
		return nil, nil, ErrUnavailable
	}
	// The policy is only an internal cache identity, never a stored verifier.
	// Compare it exactly so endpoint and credential changes get distinct actors.
	key := entryKey{peer: peer, policy: policy}
	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return nil, nil, ErrClosed
		}
		e := s.entries[key]
		if e == nil {
			entryCtx, cancel := context.WithCancel(s.ctx)
			e = &entry{service: s, key: key, policy: policy, ctx: entryCtx, cancel: cancel, acquire: make(chan acquisition), release: make(chan struct{}, 128), done: make(chan struct{})}
			s.entries[key] = e
			s.wg.Add(1)
			go e.run()
		}
		s.mu.Unlock()
		request := acquisition{ctx: ctx, reply: make(chan acquired, 1)}
		select {
		case e.acquire <- request:
		case <-e.done:
			continue
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
		select {
		case result := <-request.reply:
			return result.ctx, result.release, result.err
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-e.done:
			return nil, nil, ErrClosed
		}
	}
}

// Close cancels and joins every owned worker with the caller's bound.
func (s *Service) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.cancel()
		go func() { s.wg.Wait(); close(s.done) }()
	}
	s.mu.Unlock()
	select {
	case <-s.done:
		s.client.CloseIdleConnections()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Each entry is an actor: state below is private to this goroutine. Network
// work runs in one joined child, so expiry/release remain responsive during I/O.
func (e *entry) run() {
	defer e.service.wg.Done()
	defer close(e.done)
	defer e.cancel()
	defer func() {
		e.service.mu.Lock()
		if e.service.entries[e.key] == e {
			delete(e.service.entries, e.key)
		}
		e.service.mu.Unlock()
	}()
	results := make(chan fetched, 1)
	var fetchDone <-chan struct{}
	var decision *quota.QuotaResponse
	grant := newGrant(e.ctx)
	var active int
	lastUsed := time.Now()
	refreshAt := lastUsed
	var lastWarning time.Time
	logCtx := gizlog.WithPeerPublicKey(e.ctx, e.key.peer.String())
	startFetch := func() {
		if fetchDone != nil {
			return
		}
		done := make(chan struct{})
		fetchDone = done
		go func() {
			defer close(done)
			ctx, cancel := context.WithTimeout(e.ctx, requestTimeout)
			defer cancel()
			value, err := e.fetch(ctx)
			if err == nil {
				lastWarning = time.Time{}
			} else if e.ctx.Err() == nil && time.Since(lastWarning) >= warningInterval {
				lastWarning = time.Now()
				// fetch returns only locally owned, sanitized errors. Keep log
				// sink I/O in this child so authorization and expiry stay local.
				slog.LogAttrs(logCtx, slog.LevelWarn, "peerquota: quota refresh failed; retaining last decision",
					slog.String("reason", err.Error()))
			}
			results <- fetched{decision: value, err: err}
		}()
	}
	stopGrant := func(reason error) {
		if grant != nil {
			grant.cancel(reason)
			grant = nil
		}
	}
	defer func() {
		stopGrant(ErrClosed)
		e.cancel()
		if fetchDone != nil {
			<-fetchDone
		}
	}()
	respond := func(request acquisition) {
		if decision != nil && decision.ExpiresAt != nil && !decision.ExpiresAt.After(time.Now()) {
			stopGrant(ErrDenied)
		}
		if err := request.ctx.Err(); err != nil {
			request.reply <- acquired{err: err}
			return
		}
		if grant == nil {
			request.reply <- acquired{err: ErrDenied}
			return
		}
		callCtx, cancel := context.WithCancelCause(request.ctx)
		parent := grant.ctx
		stop := context.AfterFunc(parent, func() { cancel(context.Cause(parent)) })
		active++
		var once sync.Once
		release := func() {
			once.Do(func() {
				stop()
				cancel(context.Canceled)
				select {
				case e.release <- struct{}{}:
				case <-e.ctx.Done():
				}
			})
		}
		context.AfterFunc(callCtx, release)
		request.reply <- acquired{ctx: callCtx, release: release}
	}
	timer := time.NewTimer(idleLifetime)
	defer timer.Stop()
	for {
		now := time.Now()
		if decision != nil {
			if decision.ExpiresAt != nil && !decision.ExpiresAt.After(now) {
				stopGrant(ErrDenied)
			}
		}
		if active == 0 && !lastUsed.Add(idleLifetime).After(now) {
			return
		}
		wake := now.Add(idleLifetime)
		if active == 0 {
			wake = lastUsed.Add(idleLifetime)
		}
		if fetchDone == nil {
			if !refreshAt.After(now) {
				startFetch()
			} else if refreshAt.Before(wake) {
				wake = refreshAt
			}
		}
		if decision != nil {
			if grant != nil && decision.ExpiresAt != nil && decision.ExpiresAt.Before(wake) {
				wake = *decision.ExpiresAt
			}
		}
		timer.Reset(max(time.Millisecond, time.Until(wake)))
		select {
		case <-e.ctx.Done():
			return
		case <-timer.C:
		case <-e.release:
			active--
			lastUsed = time.Now()
		case request := <-e.acquire:
			lastUsed = time.Now()
			respond(request)
		case result := <-results:
			<-fetchDone
			fetchDone = nil
			if result.err != nil {
				refreshAt = time.Now().Add(retryDelay)
			} else {
				decision = result.decision
				now := time.Now()
				refreshAt = now.Add(decision.ValidUntil.Sub(now) / 2)
				if decision.ExpiresAt == nil || decision.ExpiresAt.After(now) {
					if grant == nil {
						grant = newGrant(e.ctx)
					}
				} else {
					stopGrant(ErrDenied)
				}
			}
		}
	}
}

func (e *entry) fetch(ctx context.Context) (*quota.QuotaResponse, error) {
	if e.service.reporter == nil {
		return nil, errors.New("usage reporter is not configured")
	}
	report, err := e.service.reporter.QuotaReport(ctx, e.key.peer)
	if err != nil {
		return nil, errors.New("cannot read quota usage report")
	}
	// The entry has one joined fetch worker at a time. Retain its immutable
	// SDK client while every entry borrows the service's shared HTTP pool.
	if e.client == nil {
		endpoint, err := url.Parse(e.policy.Endpoint)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.User != nil || endpoint.Fragment != "" {
			return nil, errors.New("invalid quota endpoint")
		}
		client, err := quota.NewClientWithResponses(endpoint.Scheme+"://"+endpoint.Host, quota.WithHTTPClient(boundedClient{e.service.client}), quota.WithRequestEditorFn(func(_ context.Context, request *http.Request) error {
			target := *endpoint
			request.URL = &target
			request.Header.Set("Authorization", "Bearer "+e.policy.ApiKey)
			return nil
		}))
		if err != nil {
			return nil, errors.New("cannot construct quota client")
		}
		e.client = client
	}
	response, err := e.client.CheckQuotaWithResponse(ctx, report)
	if err != nil {
		return nil, errors.New("quota request failed")
	}
	if response.StatusCode() != http.StatusOK || response.JSON200 == nil {
		return nil, fmt.Errorf("quota HTTP status %d", response.StatusCode())
	}
	if !response.JSON200.ValidUntil.After(time.Now()) {
		return nil, errors.New("quota valid_until must be a future timestamp")
	}
	return response.JSON200, nil
}

// boundedResponseBody keeps the generated JSON client's allocation bounded.
type boundedResponseBody struct {
	io.Reader
	io.Closer
}

type boundedClient struct{ client *http.Client }

func (c boundedClient) Do(request *http.Request) (*http.Response, error) {
	response, err := c.client.Do(request)
	if err == nil {
		response.Body = boundedResponseBody{Reader: io.LimitReader(response.Body, (4<<20)+1), Closer: response.Body}
	}
	return response, err
}

// grantState owns one renewable authorization context. The entry cancels it
// on explicit usable expiry, revocation, or worker exit. Result freshness does
// not revoke a grant.
type grantState struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
}

func newGrant(parent context.Context) *grantState {
	ctx, cancel := context.WithCancelCause(parent)
	return &grantState{ctx: ctx, cancel: cancel}
}
