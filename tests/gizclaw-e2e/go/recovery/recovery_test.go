//go:build gizclaw_e2e

package recovery_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	redis "github.com/redis/go-redis/v9"
)

type region struct {
	history memory.HistoryStore
	db      *sqlx.DB
	api     *adminhttp.ClientWithResponses
	info    gizcli.ServerInfoMetadata
	state   string
}
type fixture struct {
	apiKeys *apikey.Server
	secrets map[string]string
	redis   *redis.Client
	records kv.Store
	source  pendingdeletion.KVSource
	regions [2]region
}

func openFixture(t *testing.T) *fixture {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: "redis:6379"})
	t.Cleanup(func() { _ = client.Close() })
	root, err := kv.NewRedisWithClient(client, nil)
	must(t, err)
	f := &fixture{redis: client, records: kv.Prefixed(root, kv.Key{"peers", "records"})}
	f.source = peer.PendingDeletionSource(f.records)
	f.apiKeys = apikey.NewServer(kv.Prefixed(root, kv.Key{"api-keys"}))
	f.secrets = map[string]string{}
	for i, name := range []string{"a", "b"} {
		db, err := sqlx.ConnectContext(t.Context(), "postgres", required(t, "RECOVERY_PG_"+strings.ToUpper(name)))
		must(t, err)
		t.Cleanup(func() { _ = db.Close() })
		info, err := gizcli.FetchServerInfo(t.Context(), "server-"+name+":9820")
		must(t, err)
		key := &giznet.KeyPair{}
		must(t, key.Private.UnmarshalText([]byte(required(t, "RECOVERY_ADMIN_"+strings.ToUpper(name)))))
		key, err = giznet.NewKeyPair(key.Private)
		must(t, err)
		cli := &gizcli.Client{KeyPair: key, DialTransport: func(k *giznet.KeyPair, _ giznet.PublicKey, _ string, policy giznet.SecurityPolicy) (giznet.Listener, giznet.Conn, error) {
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			return gizwebrtc.Dial(ctx, k, info.TransportPublicKey, gizwebrtc.DialConfig{SignalingURL: info.SignalingURL, ICEServers: info.ICEServers, SecurityPolicy: policy})
		}}
		must(t, cli.Dial(info.PublicKey, info.PublicKey.String()))
		go func() { _ = cli.Serve() }()
		t.Cleanup(func() { _ = cli.Close() })
		api, err := cli.ServerAdminClient()
		must(t, err)
		history, err := memory.NewSQLHistoryStore(t.Context(), db, memory.DefaultHistoryTable, 30*24*time.Hour)
		must(t, err)
		f.regions[i] = region{history: history, db: db, api: api, info: info, state: "/evidence/server-" + name}
		response, err := api.CreateRuntimeProfileWithResponse(t.Context(), adminhttp.RuntimeProfileUpsert{Id: "recovery-profile", Spec: apitypes.RuntimeProfileSpec{}})
		must(t, err)
		if response.JSON200 == nil && response.StatusCode() != 409 {
			t.Fatalf("create Profile: HTTP %d", response.StatusCode())
		}
	}
	if f.regions[0].info.PublicKey == f.regions[1].info.PublicKey {
		t.Fatal("regional Server identities are equal")
	}
	return f
}

func (f *fixture) seedPeer(t *testing.T, home int) apitypes.Peer {
	t.Helper()
	key, err := giznet.GenerateKeyPair()
	must(t, err)
	server := &peer.Server{Store: f.records}
	item, err := server.SavePeer(t.Context(), apitypes.Peer{PublicKey: key.Public.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive, AutoRegistered: new(true)})
	must(t, err)
	if home >= 0 {
		root, err := kv.NewRedisWithClient(f.redis, nil)
		must(t, err)
		routes := peerroute.Server{Store: kv.Prefixed(root, kv.Key{"peers", "routes"}), Peers: server, ServerPublicKey: f.regions[home].info.PublicKey, ServerEndpoint: fmt.Sprintf("server-%c:9820", 'a'+home)}
		_, err = routes.Assign(t.Context(), key.Public, nil)
		must(t, err)
		db := f.regions[home].db
		_, err = db.ExecContext(t.Context(), `INSERT INTO runtime_profile_owners(owner_public_key,runtime_profile_id,binding_id,firmware_id) VALUES ($1,'recovery-profile',$2,'registration-firmware')`, item.PublicKey, uuid.NewString())
		must(t, err)
		_, err = db.ExecContext(t.Context(), `INSERT INTO peer_runs(public_key,registered_at) VALUES ($1,$2)`, item.PublicKey, time.Now().UTC().Format(time.RFC3339Nano))
		must(t, err)
	}
	created, err := f.apiKeys.Create(t.Context(), item.PublicKey, "recovery-fixture", false)
	must(t, err)
	f.secrets[item.PublicKey] = created.Secret
	return item
}

func (f *fixture) seedWorkspace(t *testing.T, home int, owner, id string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := f.regions[home].db.ExecContext(t.Context(), `INSERT INTO workspaces(id,owner_public_key,name,workflow_id,system,created_at,updated_at,last_active_at,parameters_json,toolkit_json,icon_json,labels_json,incarnation,revision) VALUES ($1,$2,$1,'system-sfu',0,$3,$3,$3,'null','null','null','null',$4,1)`, id, owner, now, uuid.NewString())
	must(t, err)
	path := filepath.Join(f.regions[home].state, "data", "agenthost", workspace.ObjectPrefix(id), "payload.bin")
	must(t, os.MkdirAll(filepath.Dir(path), 0700))
	must(t, os.WriteFile(path, []byte("owned test artifact"), 0600))
	assetName := workspace.ObjectPrefix(id) + "/history/audio.opus"
	assetPath := filepath.Join(f.regions[home].state, "data", "objects", "workspace-history", assetName)
	must(t, os.MkdirAll(filepath.Dir(assetPath), 0700))
	must(t, os.WriteFile(assetPath, []byte("fixture-audio"), 0600))
	must(t, f.regions[home].history.Append(t.Context(), []memory.HistoryEntry{{ConversationID: id, ID: "history-fixture", Role: memory.RoleUser, Text: "fixture message", CreatedAt: time.Now().UTC(), Assets: []memory.HistoryAsset{{Name: assetName, MIMEType: "audio/opus", Bytes: 13}}}}))
}

func (f *fixture) seedFailed(t *testing.T, item apitypes.Peer, code string, legacy bool) pendingdeletion.Task {
	t.Helper()
	// Future eligibility prevents live workers claiming the fixture between
	// its atomic publication and the controlled terminal transition.
	now := time.Now().Add(time.Hour)
	record, err := pendingdeletion.New(pendingdeletion.KindPeer, item.PublicKey, &item.PublicKey, pendingdeletion.ReasonPeerDelete, map[string]string{"public_key": item.PublicKey}, now)
	must(t, err)
	record, _, err = pendingdeletion.CreateOrGet(t.Context(), f.records, record)
	must(t, err)
	fingerprint, err := pendingdeletion.Fingerprint(record)
	must(t, err)
	claim, ok, err := f.source.Claim(t.Context(), pendingdeletion.Reference{Source: "peer", DeletionID: record.DeletionID, MarkerFingerprint: fingerprint}, now, time.Minute)
	must(t, err)
	if !ok {
		t.Fatal("fixture claim failed")
	}
	if legacy {
		projected := item
		projected.FirmwareId = new("registration-firmware")
		plan := map[string]any{"version": 2, "marker_fingerprint": fingerprint, "peer": projected, "social": map[string]any{"public_key": item.PublicKey}, "workspaces": map[string]any{"public_key": item.PublicKey}}
		data, err := json.Marshal(plan)
		must(t, err)
		must(t, f.records.Set(t.Context(), kv.Key{"peer-retirement-plans", record.DeletionID}, data))
		claim, err = f.source.Checkpoint(t.Context(), claim, pendingdeletion.PhaseFinalize, now)
		must(t, err)
	}
	must(t, f.source.Fail(t.Context(), claim, code, "isolated fixture failure", true, now, now, 10))
	task, err := f.source.GetTask(t.Context(), record.DeletionID)
	must(t, err)
	return task
}

func retryTask(t *testing.T, r region, task pendingdeletion.Task, want int, code string) {
	t.Helper()
	response, err := r.api.RetryPendingDeletionWithResponse(t.Context(), uuid.MustParse(task.Record.DeletionID), &adminhttp.RetryPendingDeletionParams{Source: "peer"})
	must(t, err)
	if response.StatusCode() != want {
		t.Fatalf("retry HTTP %d, want %d", response.StatusCode(), want)
	}
	if code != "" && (response.JSON409 == nil || response.JSON409.Error.Code != code) {
		t.Fatalf("retry error=%+v, want %s", response.JSON409, code)
	}
}

func (f *fixture) requireComplete(t *testing.T, item apitypes.Peer, taskID string, home int) {
	t.Helper()
	await(t, 20*time.Second, func() bool {
		data, err := f.records.Get(t.Context(), kv.Key{"by-pubkey", item.PublicKey})
		return err == nil && string(data) == `{"version":1,"state":"deleted"}`
	})
	if taskID != "" {
		for _, key := range []kv.Key{{"pending-deletion", "task", taskID}, {"pending-deletion", "by-id", taskID}, {"peer-retirement-plans", taskID}} {
			if _, err := f.records.Get(t.Context(), key); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("task-owned key retained %v: %v", key, err)
			}
		}
		// Inspect real sorted sets, including every status and due shard.
		var cursor uint64
		for {
			keys, next, err := f.redis.Scan(t.Context(), cursor, "peers:records:pending-deletion:*", 100).Result()
			must(t, err)
			for _, key := range keys {
				kind, err := f.redis.Type(t.Context(), key).Result()
				must(t, err)
				if kind != "zset" {
					continue
				}
				members, err := f.redis.ZRange(t.Context(), key, 0, -1).Result()
				must(t, err)
				for _, member := range members {
					if strings.Contains(member, taskID) {
						t.Fatalf("completed ID remains in index %s", key)
					}
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	pending, err := pendingdeletion.HasLocator(t.Context(), f.records, pendingdeletion.KindPeer, item.PublicKey)
	must(t, err)
	if pending {
		t.Fatal("completed locator remains")
	}
	var count int
	must(t, f.regions[home].db.GetContext(t.Context(), &count, `SELECT count(*) FROM runtime_profiles WHERE id='recovery-profile'`))
	if count != 1 {
		t.Fatal("global RuntimeProfile removed")
	}
	must(t, f.regions[home].db.GetContext(t.Context(), &count, `SELECT count(*) FROM runtime_profile_owners WHERE owner_public_key=$1`, item.PublicKey))
	if count != 0 {
		t.Fatal("home registration binding remains")
	}
	must(t, f.regions[home].db.GetContext(t.Context(), &count, `SELECT count(*) FROM peer_runs WHERE public_key=$1`, item.PublicKey))
	if count != 1 {
		t.Fatal("diagnostic PeerRun directory lost")
	}
	if exists, err := f.redis.Exists(t.Context(), "peers:routes:by-peer:"+item.PublicKey, "api-keys:retired:"+item.PublicKey).Result(); err != nil || exists != 2 {
		t.Fatalf("permanent route/API-key fences=%d, %v", exists, err)
	}
	var key giznet.PublicKey
	must(t, key.UnmarshalText([]byte(item.PublicKey)))
	if _, err := (&peer.Server{Store: f.records}).EnsureConnectedPeer(t.Context(), key); !errors.Is(err, peer.ErrPeerDeleted) {
		t.Fatalf("tombstone did not fence registration: %v", err)
	}
	root, err := kv.NewRedisWithClient(f.redis, nil)
	must(t, err)
	keys := apikey.NewServer(kv.Prefixed(root, kv.Key{"api-keys"}))
	page, err := keys.ListOwner(t.Context(), item.PublicKey, "", 10)
	must(t, err)
	if len(page.Items) != 0 {
		t.Fatal("retired API Key records remain")
	}
	if secret := f.secrets[item.PublicKey]; secret != "" {
		if _, err := keys.Authenticate(t.Context(), secret); err == nil {
			t.Fatal("retired credential still authenticates")
		}
	}
	if _, err := keys.Create(t.Context(), item.PublicKey, "late-key", false); !errors.Is(err, apikey.ErrOwnerRetired) {
		t.Fatalf("late credential creation=%v", err)
	}
	// Cleanup is idempotent and keeps the concurrent-creation fence.
	must(t, keys.CleanupPeer(t.Context(), item.PublicKey))
}

func TestRegionalRecovery(t *testing.T) {
	f := openFixture(t)
	must(t, f.records.Set(t.Context(), kv.Key{"unrelated", "canary"}, []byte("keep")))
	active := f.seedPeer(t, 1)
	activeData, err := f.records.Get(t.Context(), kv.Key{"by-pubkey", active.PublicKey})
	must(t, err)
	f.seedWorkspace(t, 1, active.PublicKey, "same-workspace-id")

	t.Run("normal_with_firmware_and_owned_assets", func(t *testing.T) {
		item := f.seedPeer(t, 0)
		f.seedWorkspace(t, 0, item.PublicKey, "same-workspace-id")
		response, err := f.regions[0].api.DeletePeerWithResponse(t.Context(), item.PublicKey)
		must(t, err)
		if response.JSON200 == nil {
			t.Fatalf("delete HTTP %d", response.StatusCode())
		}
		// The immutable event ID is deterministic; independent of worker speed.
		record, err := pendingdeletion.New(pendingdeletion.KindPeer, item.PublicKey, &item.PublicKey, pendingdeletion.ReasonAdminDelete, map[string]string{"public_key": item.PublicKey}, time.Now())
		must(t, err)
		f.requireComplete(t, item, record.DeletionID, 0)
		var count int
		must(t, f.regions[0].db.GetContext(t.Context(), &count, `SELECT count(*) FROM workspaces WHERE id='same-workspace-id'`))
		if count != 0 {
			t.Fatal("home Workspace remains")
		}
		must(t, f.regions[0].db.GetContext(t.Context(), &count, `SELECT count(*) FROM workspace_pending_deletions`))
		if count != 0 {
			t.Fatalf("Workspace deletion records remain: %d", count)
		}
		if _, err := os.Stat(filepath.Join(f.regions[0].state, "data", "agenthost", workspace.ObjectPrefix("same-workspace-id"), "payload.bin")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owned artifact remains: %v", err)
		}
		response, err = f.regions[0].api.DeletePeerWithResponse(t.Context(), item.PublicKey)
		must(t, err)
		if response.JSON200 == nil {
			t.Fatalf("repeated delete HTTP %d", response.StatusCode())
		}
		history, err := f.regions[0].history.Query(t.Context(), memory.HistoryQuery{ConversationID: "same-workspace-id", Limit: 10})
		must(t, err)
		if len(history.Entries) != 0 {
			t.Fatal("owned SQL History remains")
		}
		if _, err := os.Stat(filepath.Join(f.regions[0].state, "data", "objects", "workspace-history", workspace.ObjectPrefix("same-workspace-id"), "history", "audio.opus")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owned History audio remains: %v", err)
		}
		t.Log("home PG Workspace/binding/History and object/audio assets removed; API Keys revoked; Redis task/marker/locator/plan/index removed; permanent fences retained")
	})

	t.Run("legacy_failed_recovery_and_foreign_retry", func(t *testing.T) {
		item := f.seedPeer(t, 0)
		task := f.seedFailed(t, item, "peer_replacement_ambiguous", true)
		retryTask(t, f.regions[1], task, 409, "peer_home_mismatch")
		after, err := f.source.GetTask(t.Context(), task.Record.DeletionID)
		must(t, err)
		if after.Status != pendingdeletion.StatusFailed || after.LastErrorCode != task.LastErrorCode {
			t.Fatal("foreign retry changed failed task")
		}
		retryTask(t, f.regions[0], task, 200, "")
		f.requireComplete(t, item, task.Record.DeletionID, 0)
		retryTask(t, f.regions[0], task, 404, "")
	})

	t.Run("second_region_owns_its_cleanup", func(t *testing.T) {
		item := f.seedPeer(t, 1)
		task := f.seedFailed(t, item, "social_cleanup_failed", false)
		retryTask(t, f.regions[0], task, 409, "peer_home_mismatch")
		retryTask(t, f.regions[1], task, 200, "")
		f.requireComplete(t, item, task.Record.DeletionID, 1)
	})

	t.Run("replacement_unknown_missing_and_hidden_home_data", func(t *testing.T) {
		for _, scenario := range []string{"replacement", "unknown", "missing", "hidden_home_data"} {
			t.Run(scenario, func(t *testing.T) {
				home := 0
				if scenario == "unknown" {
					home = -1
				}
				item := f.seedPeer(t, home)
				task := f.seedFailed(t, item, "peer_replacement_ambiguous", true)
				want := ""
				switch scenario {
				case "replacement":
					changed := item
					changed.UpdatedAt = changed.UpdatedAt.Add(time.Second)
					data, err := json.Marshal(changed)
					must(t, err)
					must(t, f.records.Set(t.Context(), kv.Key{"by-pubkey", item.PublicKey}, data))
					want = "peer_replacement_ambiguous"
				case "unknown":
					want = "peer_home_unknown"
				case "missing":
					must(t, f.records.Delete(t.Context(), kv.Key{"by-pubkey", item.PublicKey}))
					want = "peer_record_missing"
				case "hidden_home_data":
					f.seedWorkspace(t, 0, item.PublicKey, "hidden-home-data")
					want = "retirement_scope_ambiguous"
				}
				if scenario == "hidden_home_data" {
					retryTask(t, f.regions[0], task, 200, "")
					await(t, 10*time.Second, func() bool {
						state, err := f.source.GetTask(t.Context(), task.Record.DeletionID)
						return err == nil && state.Status == pendingdeletion.StatusFailed && state.LastErrorCode == want
					})
				} else {
					retryTask(t, f.regions[0], task, 409, want)
				}
				if _, err := f.source.GetTask(t.Context(), task.Record.DeletionID); err != nil {
					t.Fatal("unsafe task lost")
				}
				if pending, err := pendingdeletion.HasLocator(t.Context(), f.records, pendingdeletion.KindPeer, item.PublicKey); err != nil || !pending {
					t.Fatal("unsafe identity fence lost")
				}
			})
		}
	})
	canary, err := f.records.Get(t.Context(), kv.Key{"unrelated", "canary"})
	must(t, err)
	if string(canary) != "keep" {
		t.Fatal("unrelated Redis record changed")
	}
	after, err := f.records.Get(t.Context(), kv.Key{"by-pubkey", active.PublicKey})
	must(t, err)
	if string(after) != string(activeData) {
		t.Fatal("active foreign Peer changed")
	}
	var count int
	must(t, f.regions[1].db.GetContext(t.Context(), &count, `SELECT count(*) FROM workspaces WHERE id='same-workspace-id' AND owner_public_key=$1`, active.PublicKey))
	if count != 1 {
		t.Fatal("independent regional Workspace lost")
	}
	must(t, f.regions[1].db.GetContext(t.Context(), &count, `SELECT count(*) FROM runtime_profile_owners WHERE owner_public_key=$1`, active.PublicKey))
	if count != 1 {
		t.Fatal("foreign owner binding lost")
	}
	if _, err := os.Stat(filepath.Join(f.regions[1].state, "data", "agenthost", workspace.ObjectPrefix("same-workspace-id"), "payload.bin")); err != nil {
		t.Fatal("foreign artifact lost")
	}
	history, err := f.regions[1].history.Query(t.Context(), memory.HistoryQuery{ConversationID: "same-workspace-id", Limit: 10})
	must(t, err)
	if len(history.Entries) != 1 {
		t.Fatal("foreign SQL History changed")
	}
	if _, err := os.Stat(filepath.Join(f.regions[1].state, "data", "objects", "workspace-history", workspace.ObjectPrefix("same-workspace-id"), "history", "audio.opus")); err != nil {
		t.Fatal("foreign History audio removed")
	}
	if _, err := f.apiKeys.Authenticate(t.Context(), f.secrets[active.PublicKey]); err != nil {
		t.Fatal("active foreign API Key was revoked")
	}
	t.Log("two independent regional PG pools, active Peer/Workspace/binding/assets preserved")
}

func TestOnlineRecoveryCLI(t *testing.T) {
	f := openFixture(t)
	admin, err := giznet.GenerateKeyPair()
	must(t, err)
	_, err = (&peer.Server{Store: f.records}).SavePeer(t.Context(), apitypes.Peer{PublicKey: admin.Public.String(), Role: apitypes.PeerRoleAdmin, Status: apitypes.PeerRegistrationStatusActive})
	must(t, err)
	// Wait for the real read-only replica used by ordinary transport admission.
	replica := redis.NewClient(&redis.Options{Addr: "replica:6379"})
	defer replica.Close()
	await(t, 10*time.Second, func() bool {
		count, err := replica.Exists(t.Context(), "peers:records:by-pubkey:"+admin.Public.String()).Result()
		return err == nil && count == 1
	})
	item := f.seedPeer(t, 0)
	task := f.seedFailed(t, item, "cli_fixture", false)
	configDir := t.TempDir()
	contextDir := filepath.Join(configDir, "gizclaw", "recovery")
	must(t, os.MkdirAll(contextDir, 0700))
	must(t, os.WriteFile(filepath.Join(contextDir, "config.yaml"), fmt.Appendf(nil, "identity:\n  private-key: %s\nserver:\n  endpoint: http://server-a:9820\n", admin.Private.String()), 0600))
	run := func(execute bool) string {
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		args := []string{"admin", "pending-deletions", "recover", "--context", "recovery", "--source", "peer", "--error-code", "cli_fixture", "--limit", "500"}
		if execute {
			args = append(args, "--execute", "--interval", "100ms", "--wait", "15s")
		}
		cmd := exec.CommandContext(ctx, "/binaries/gizclaw", args...)
		cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+configDir)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("online recovery CLI: %v: %s", err, output)
		}
		return string(output)
	}
	preview := run(false)
	if !strings.Contains(preview, `"selected":1`) || strings.Contains(preview, `"type":"retry"`) {
		t.Fatalf("preview=%s", preview)
	}
	state, err := f.source.GetTask(t.Context(), task.Record.DeletionID)
	must(t, err)
	if state.Status != pendingdeletion.StatusFailed {
		t.Fatal("preview changed task")
	}
	result := run(true)
	if !strings.Contains(result, `"outcome":"completed"`) {
		t.Fatalf("recovery did not report completion: %s", result)
	}
	f.requireComplete(t, item, task.Record.DeletionID, 0)
	if repeated := run(true); strings.Contains(repeated, `"type":"retry"`) {
		t.Fatalf("second execution retried completed task: %s", repeated)
	}
	t.Log("real online CLI preview left failed state unchanged; execute completed domain cleanup; repeated execute selected no completed task")
}

func TestPrepareInterruptedRecovery(t *testing.T) {
	if os.Getenv("RECOVERY_STAGE") != "prepare-interruption" {
		t.Skip("runner controls Server interruption")
	}
	f := openFixture(t)
	item := f.seedPeer(t, 0)
	task := f.seedFailed(t, item, "social_cleanup_failed", false)
	tx, err := f.regions[0].db.BeginTxx(t.Context(), nil)
	must(t, err)
	defer tx.Rollback()
	_, err = tx.ExecContext(t.Context(), `SELECT owner_public_key FROM runtime_profile_owners WHERE owner_public_key=$1 FOR UPDATE`, item.PublicKey)
	must(t, err)
	retryTask(t, f.regions[0], task, 200, "")
	await(t, 10*time.Second, func() bool {
		state, err := f.source.GetTask(t.Context(), task.Record.DeletionID)
		exists, _ := f.redis.Exists(t.Context(), "api-keys:retired:"+item.PublicKey).Result()
		return err == nil && state.Status == pendingdeletion.StatusRunning && exists == 1
	})
	data, err := json.Marshal(struct {
		Peer apitypes.Peer
		Task string
	}{item, task.Record.DeletionID})
	must(t, err)
	must(t, os.WriteFile("/evidence/interrupted.json", data, 0600))
	await(t, 30*time.Second, func() bool { _, err := os.Stat("/evidence/server-killed"); return err == nil })
	state, err := f.source.GetTask(t.Context(), task.Record.DeletionID)
	must(t, err)
	if state.Status != pendingdeletion.StatusRunning {
		t.Fatalf("crash did not preserve live lease: %s", state.Status)
	}
	must(t, tx.Rollback())
	t.Log("real home worker killed after API-key retirement while SQL binding deletion was blocked; task and lease retained")
}

func TestResumeInterruptedRecovery(t *testing.T) {
	if os.Getenv("RECOVERY_STAGE") != "resume-interruption" {
		t.Skip("requires killed home worker")
	}
	f := openFixture(t)
	var state struct {
		Peer apitypes.Peer
		Task string
	}
	data, err := os.ReadFile("/evidence/interrupted.json")
	must(t, err)
	must(t, json.Unmarshal(data, &state))
	f.requireComplete(t, state.Peer, state.Task, 0)
	t.Log("restarted home worker reclaimed expired lease and completed exact task")
}

func TestRecoveryReadsCentralWriterDespiteStaleReplica(t *testing.T) {
	f := openFixture(t)
	replica := redis.NewClient(&redis.Options{Addr: "replica:6379"})
	defer replica.Close()
	// A real read-only replica is disconnected after initial synchronization.
	must(t, replica.ReplicaOf(t.Context(), "127.0.0.1", "1").Err())
	defer func() { must(t, replica.ReplicaOf(context.Background(), "redis", "6379").Err()) }()
	item := f.seedPeer(t, 0)
	task := f.seedFailed(t, item, "peer_replacement_ambiguous", true)
	if count, err := replica.Exists(t.Context(), "peers:records:pending-deletion:task:"+task.Record.DeletionID).Result(); err != nil || count != 0 {
		t.Fatalf("replica fixture not stale: %d %v", count, err)
	}
	page, err := f.regions[0].api.ListPendingDeletionsWithResponse(t.Context(), &adminhttp.ListPendingDeletionsParams{Source: new("peer"), Status: new(apitypes.PendingDeletionStatusFailed), Limit: new(int32(500))})
	must(t, err)
	found := false
	if page.JSON200 != nil {
		for _, v := range page.JSON200.Items {
			found = found || v.DeletionId.String() == task.Record.DeletionID
		}
	}
	if !found {
		t.Fatal("Admin list read stale regional replica")
	}
	retryTask(t, f.regions[0], task, 200, "")
	f.requireComplete(t, item, task.Record.DeletionID, 0)
	t.Log("Admin selection, retry, handler and atomic finalization used central writer while regional reader lacked the task")
}

func await(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	t.Fatal("timed out waiting for backend invariant")
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func required(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Fatalf("%s is required", name)
	}
	return v
}
