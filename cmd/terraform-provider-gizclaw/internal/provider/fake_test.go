package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/contextconn"
)

// fakeServer is an in-memory Admin resource API. It is exposed to the provider
// through adminresource.Client so error mapping matches production.
type fakeServer struct {
	mu        sync.Mutex
	resources map[string]json.RawMessage
	// stored rewrites the resource returned by reads, keyed by kind/id.
	stored   map[string]json.RawMessage
	applied  []json.RawMessage
	gets     atomic.Int32
	deletes  atomic.Int32
	failNext atomic.Int32
}

func newFakeServer() *fakeServer {
	return &fakeServer{resources: map[string]json.RawMessage{}, stored: map[string]json.RawMessage{}}
}

var errTransport = errors.New("gizwebrtc: connection closed")

func (s *fakeServer) transportFailure() bool {
	for {
		n := s.failNext.Load()
		if n <= 0 {
			return false
		}
		if s.failNext.CompareAndSwap(n, n-1) {
			return true
		}
	}
}

func (s *fakeServer) ApplyResourceWithResponse(_ context.Context, body adminhttp.ApplyResourceJSONRequestBody, _ ...adminhttp.RequestEditorFn) (*adminhttp.ApplyResourceResponse, error) {
	if s.transportFailure() {
		return nil, errTransport
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var header resourceEnvelope
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.resources[header.Kind+"/"+header.Metadata.ID] = data
	s.applied = append(s.applied, data)
	s.mu.Unlock()
	return &adminhttp.ApplyResourceResponse{JSON200: &apitypes.ApplyResult{
		Action:     apitypes.ApplyActionCreated,
		ApiVersion: apitypes.ResourceAPIVersionGizclawAdminv1alpha1,
		Kind:       apitypes.ResourceKind(header.Kind),
		Id:         &header.Metadata.ID,
	}}, nil
}

func (s *fakeServer) lookup(kind, id string) (*apitypes.Resource, *apitypes.ErrorResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := kind + "/" + id
	data, ok := s.stored[key]
	if !ok {
		data, ok = s.resources[key]
	}
	if !ok {
		resp := apitypes.NewErrorResponse("RESOURCE_NOT_FOUND", fmt.Sprintf("%s %q not found", kind, id))
		return nil, &resp
	}
	var resource apitypes.Resource
	if err := json.Unmarshal(data, &resource); err != nil {
		panic(err)
	}
	return &resource, nil
}

func (s *fakeServer) GetResourceWithResponse(_ context.Context, kind adminhttp.ResourceKind, id string, _ ...adminhttp.RequestEditorFn) (*adminhttp.GetResourceResponse, error) {
	s.gets.Add(1)
	if s.transportFailure() {
		return nil, errTransport
	}
	resource, errResp := s.lookup(string(kind), id)
	return &adminhttp.GetResourceResponse{JSON200: resource, JSON404: errResp}, nil
}

func (s *fakeServer) DeleteResourceWithResponse(_ context.Context, kind adminhttp.ResourceKind, id string, _ ...adminhttp.RequestEditorFn) (*adminhttp.DeleteResourceResponse, error) {
	s.deletes.Add(1)
	if s.transportFailure() {
		return nil, errTransport
	}
	resource, errResp := s.lookup(string(kind), id)
	if resource != nil {
		s.mu.Lock()
		delete(s.resources, string(kind)+"/"+id)
		delete(s.stored, string(kind)+"/"+id)
		s.mu.Unlock()
	}
	return &adminhttp.DeleteResourceResponse{JSON200: resource, JSON404: errResp}, nil
}

func (s *fakeServer) put(t *testing.T, raw string) {
	t.Helper()
	var header resourceEnvelope
	if err := json.Unmarshal([]byte(raw), &header); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.resources[header.Kind+"/"+header.Metadata.ID] = json.RawMessage(raw)
	s.mu.Unlock()
}

type fakeConnector struct {
	server   *fakeServer
	connects atomic.Int32
	closes   atomic.Int32
	failures atomic.Int32
	// block, when non-nil, delays connects until closed or ctx is done.
	block chan struct{}
}

func (f *fakeConnector) connect(ctx context.Context, _ contextconn.Options) (resourceConn, error) {
	f.connects.Add(1)
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if n := f.failures.Load(); n > 0 && f.failures.CompareAndSwap(n, n-1) {
		return nil, errors.New("gizclaw: timeout waiting for client readiness")
	}
	return adminresource.NewClient(f.server, func() error {
		f.closes.Add(1)
		return nil
	}), nil
}

func newTestClient(server *fakeServer) (*adminClient, *fakeConnector) {
	connector := &fakeConnector{server: server}
	client := newAdminClient(contextconn.Options{Context: "test"})
	client.connect = connector.connect
	client.retryDelay = 0
	return client, connector
}
