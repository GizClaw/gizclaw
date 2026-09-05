// monitor-fixture prepares a retained audio asset in the local Docker test
// stores. It is not linked into GizClaw and exposes no production HTTP route.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	initDir := flag.String("init", "", "initialize an isolated Monitor Docker workspace")
	endpoint := flag.String("endpoint", "server:9820", "local Docker Server endpoint")
	dataDir := flag.String("data", "", "local Docker Server data directory")
	audioFile := flag.String("audio", "", "retained Ogg fixture")
	flag.Parse()
	if *initDir != "" {
		return initialize(*initDir)
	}
	if *dataDir == "" || *audioFile == "" {
		return fmt.Errorf("data and audio are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	info, err := gizcli.FetchServerInfo(ctx, *endpoint)
	if err != nil {
		return err
	}
	key, err := giznet.GenerateKeyPair()
	if err != nil {
		return err
	}
	client := &gizcli.Client{KeyPair: key, DialTransport: func(key *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
		return gizwebrtc.Dial(ctx, key, info.TransportPublicKey, gizwebrtc.DialConfig{SignalingURL: info.SignalingURL, ICEServers: info.ICEServers, SecurityPolicy: policy})
	}}
	if err := client.Dial(info.PublicKey, *endpoint); err != nil {
		return err
	}
	defer client.Close()
	go func() { _ = client.Serve() }()
	if _, err := client.Register(ctx, "monitor-fixture-register", os.Getenv("GIZCLAW_TEST_REGISTRATION_TOKEN")); err != nil {
		return err
	}
	if _, err := client.PutServerRuntime(ctx, "monitor-fixture-debug", rpcapi.ServerPutRuntimeRequest{DebugMode: "readonly"}); err != nil {
		return err
	}
	name := "monitor-audio-" + key.Public.ShortString()
	if _, err := client.CreateWorkspace(ctx, "monitor-fixture-workspace", rpcapi.WorkspaceCreateRequest{Name: name, Collection: "assistants", WorkflowName: "monitor-echo"}); err != nil {
		return err
	}
	api, err := peerhttp.NewClientWithResponses("http://"+*endpoint, peerhttp.WithRequestEditorFn(func(_ context.Context, request *http.Request) error {
		request.Header.Set("Authorization", "Bearer gizclaw_pk_"+key.Public.String())
		return nil
	}))
	if err != nil {
		return err
	}
	response, err := api.ListDeviceWorkspacesWithResponse(ctx)
	if err != nil {
		return err
	}
	if response.JSON200 == nil || len(*response.JSON200) != 1 {
		return fmt.Errorf("fixture workspace lookup status %d: %s", response.StatusCode(), response.Body)
	}
	id := (*response.JSON200)[0].Id
	db, err := sqlx.Open("sqlite", filepath.Join(*dataDir, "gameplay.sqlite")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	records, err := logstore.NewSQLStoreWithDB(db, "workspace_history")
	if err != nil {
		return err
	}
	rootDir := filepath.Join(*dataDir, "objects", "workspace-history")
	if err := os.MkdirAll(rootDir, 0700); err != nil {
		return err
	}
	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return err
	}
	defer root.Close()
	objects, err := objectstore.NewRoot(root)
	if err != nil {
		return err
	}
	audio, err := os.ReadFile(*audioFile)
	if err != nil {
		return err
	}
	history := workspace.NewHistoryStore(records, objects, id)
	entry, err := history.Append(ctx, workspace.AppendHistoryRequest{Type: "agent", Name: "audio-fixture", Text: "Retained Monitor audio", Asset: &workspace.AppendHistoryAsset{MIMEType: "audio/ogg", Data: audio}})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(audio)
	return json.NewEncoder(os.Stdout).Encode(map[string]string{
		"GIZCLAW_MONITOR_AUDIO_KEY": key.Public.String(), "GIZCLAW_MONITOR_AUDIO_WORKSPACE": id,
		"GIZCLAW_MONITOR_AUDIO_HISTORY": entry.ID, "GIZCLAW_MONITOR_AUDIO_SHA256": hex.EncodeToString(digest[:]),
	})
}
