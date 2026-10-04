package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// dialogue observes actual PeerStream EOS through Edge and Server. This fixture
// helper gives Giztest the existing EventError fields without changing its DSL.
func (f *fixture) dialogue(w http.ResponseWriter, r *http.Request) {
	driver := r.PathValue("driver")
	if driver != "eino" {
		http.Error(w, "unknown driver", 400)
		return
	}
	var request struct {
		Unavailable bool `json:"unavailable"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	defer r.Body.Close()
	result, err := f.runDialogue(r.Context(), driver, request.Unavailable)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, result)
}

func (f *fixture) runDialogue(parent context.Context, driver string, unavailable bool) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	info, err := gizcli.FetchServerInfo(ctx, "edge:9821")
	if err != nil {
		return nil, err
	}
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	client := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		return gizwebrtc.Dial(ctx, key, info.TransportPublicKey, gizwebrtc.DialConfig{SignalingURL: info.SignalingURL, ICEServers: info.ICEServers, SecurityPolicy: policy})
	}}
	if err := client.Dial(info.PublicKey, "edge:9821"); err != nil {
		return nil, err
	}
	defer client.Close()
	done := make(chan error, 1)
	go func() { done <- client.Serve() }()
	defer func() { _ = client.Close(); <-done }()
	token := "quota-dialogue-" + driver
	if unavailable {
		token = "quota-failure"
	}
	if _, err := client.Register(ctx, "register", token); err != nil {
		return nil, err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer stop()
		_, _ = client.DeletePeer(cleanup, "delete", rpcapi.ServerPeerDeleteRequest{})
	}()
	name := "quota-dialogue-" + key.Public.ShortString()
	if _, err := client.CreateWorkspace(ctx, "create", rpcapi.WorkspaceCreateRequest{Name: name, WorkflowName: "quota-" + driver}); err != nil {
		return nil, err
	}
	stream, err := client.OpenPeerStream(32)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	stopRead := context.AfterFunc(ctx, func() { _ = stream.Close() })
	defer stopRead()
	if _, err := client.SetServerRunWorkspace(ctx, "select", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: name}); err != nil {
		return nil, err
	}
	turn := func(inputID string) (*genx.StreamCtrl, string, error) {
		if err := stream.Push(ctx, &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text("hello"), Ctrl: &genx.StreamCtrl{StreamID: inputID, BeginOfStream: true}}); err != nil {
			return nil, "", err
		}
		if err := stream.Push(ctx, &genx.MessageChunk{Role: genx.RoleUser, Part: genx.Text(""), Ctrl: &genx.StreamCtrl{StreamID: inputID, EndOfStream: true}}); err != nil {
			return nil, "", err
		}
		var text strings.Builder
		for {
			chunk, err := stream.Next()
			if err != nil {
				return nil, "", err
			}
			if chunk == nil || chunk.Role == genx.RoleUser {
				continue
			}
			if value, ok := chunk.Part.(genx.Text); ok {
				text.WriteString(string(value))
			}
			if chunk.IsEndOfStream() {
				return chunk.Ctrl, text.String(), nil
			}
		}
	}
	var first *genx.StreamCtrl
	var text string
	if !unavailable {
		first, text, err = turn("allowed-input")
		if err != nil {
			return nil, err
		}
		if first.ErrorCode != "" || text != "quota fixture answer" {
			return nil, fmt.Errorf("first turn: %+v text=%q", first, text)
		}
		select {
		case <-time.After(2200 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	before := f.providerCalls.Load()
	last, _, err := turn("expired-input")
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(last)
	if err != nil {
		return nil, err
	}
	var terminal map[string]any
	if err := json.Unmarshal(encoded, &terminal); err != nil {
		return nil, err
	}
	// Include false retryable explicitly so Giztest asserts the actual bool.
	terminal["error_retryable"] = last.ErrorRetryable
	return map[string]any{"allowed_text": text, "error": terminal, "new_response": last.StreamID != "" && (first == nil || first.StreamID != last.StreamID), "provider_calls_after_expiry": f.providerCalls.Load() - before}, nil
}
