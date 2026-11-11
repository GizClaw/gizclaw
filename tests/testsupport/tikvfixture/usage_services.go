package tikvfixture

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerpresence"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// UsageServices borrows the real services and SQL handle of one Docker Server.
// Controls call these implementations directly only for canonical competitors
// and private diagnostics; tested device requests still use Giztest RPC/HTTP.
// No control operation writes raw KV rows or synthetic successful responses.
type UsageServices struct {
	Stores        map[string]kv.Store
	Workspaces    *sql.DB
	AdminEndpoint string
	Materialize   func(context.Context, string, string, string) (any, error)
	Peers         *peer.Server
	Routes        *peerroute.Server
	Friends       *friend.Server
	Groups        *friendgroup.Server
	Contacts      *contact.Server
	Presence      *peerpresence.Service
	mu            sync.Mutex
	publishers    map[string]*peerpresence.Publisher
}

// AttachUsageServices connects private controls to production service instances.
// It is called only by the build overlay, after the Server initialized them.
func AttachUsageServices(services *UsageServices) error {
	gate := usageTransport.Load()
	if gate == nil {
		return errors.New("TiKV usage transport was not installed")
	}
	services.publishers = make(map[string]*peerpresence.Publisher)
	gate.services.Store(services)
	return nil
}

// UsageAction is a bounded private diagnostic or a canonical competing service
// call. Primary affects this explicitly marked control call only. Wait polls a
// real read until Found/Equals match; it never delays application responses.
type UsageAction struct {
	Operation      string            `json:"operation"`
	Primary        bool              `json:"primary,omitempty"`
	RequireLearner bool              `json:"require_learner,omitempty"`
	ExpectedError  bool              `json:"expected_error,omitempty"`
	Store          string            `json:"store,omitempty"`
	Key            []string          `json:"key,omitempty"`
	Found          *bool             `json:"found,omitempty"`
	Equals         any               `json:"equals,omitempty"`
	Field          string            `json:"field,omitempty"`
	Wait           string            `json:"wait,omitempty"`
	ID             string            `json:"id,omitempty"`
	Owner          string            `json:"owner,omitempty"`
	Peer           string            `json:"peer,omitempty"`
	Name           string            `json:"name,omitempty"`
	Token          string            `json:"token,omitempty"`
	TTLSeconds     int               `json:"ttl_seconds,omitempty"`
	DisplayName    *string           `json:"display_name,omitempty"`
	Description    *string           `json:"description,omitempty"`
	Snapshot       json.RawMessage   `json:"snapshot,omitempty"`
	Online         bool              `json:"online,omitempty"`
	Publisher      string            `json:"publisher,omitempty"`
	Workspace      string            `json:"workspace,omitempty"`
	Endpoint       string            `json:"endpoint,omitempty"`
	Method         string            `json:"method,omitempty"`
	Path           string            `json:"path,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Body           any               `json:"body,omitempty"`
}

type usageActionResult struct {
	Diagnostic bool   `json:"diagnostic"`
	Operation  string `json:"operation"`
	Value      any    `json:"value"`
	Error      string `json:"error"`
}

func (g *UsageGate) invoke(ctx context.Context, action UsageAction) (usageActionResult, error) {
	services := g.services.Load()
	if services == nil {
		return usageActionResult{}, errors.New("Server services not ready")
	}
	ctx = context.WithValue(ctx, usageDiagnosticKey{}, true)
	if action.Primary {
		ctx = kv.WithPrimaryReads(ctx)
	}
	value, err := services.invoke(ctx, action)
	result := usageActionResult{Diagnostic: action.Operation != "http" && action.Operation != "admin.friend.create", Operation: action.Operation, Value: value}
	if err != nil {
		result.Error = err.Error()
	}
	if action.ExpectedError == (err == nil) {
		if err == nil {
			return result, fmt.Errorf("%s unexpectedly succeeded", action.Operation)
		}
		count := -1
		if action.Name != "" && services.Workspaces != nil {
			observation, cancel := context.WithTimeout(ctx, time.Second)
			_ = services.Workspaces.QueryRowContext(observation, services.WorkspacesQuery(), action.Name).Scan(&count)
			cancel()
		}
		slog.Error("TiKV usage control failed", "operation", action.Operation, "error", usageRedactFailure(err.Error()), "exact_workspace_rows", count)
		return result, fmt.Errorf("%s: %w", action.Operation, err)
	}
	return result, nil
}

func usagePeer(value string) (giznet.PublicKey, error) {
	var key giznet.PublicKey
	err := key.UnmarshalText([]byte(value))
	return key, err
}

func usageRelation(a, b string) string {
	parts := []string{a, b}
	sort.Strings(parts)
	return parts[0] + ":" + parts[1]
}

func (s *UsageServices) invoke(ctx context.Context, a UsageAction) (any, error) {
	switch a.Operation {
	case "http":
		return usageForward(ctx, a)
	case "admin.friend.create":
		return s.adminFriendCreate(ctx, a)
	case "fixture.invoke":
		var result usageActionResult
		err := usageControl(ctx, a.Endpoint, "/invoke", a.Body, &result)
		return result, err
	case "group.materialize", "friend.materialize":
		if s.Materialize == nil {
			return nil, errors.New("real Workspace materializer is not attached")
		}
		name := a.Name
		if a.Operation == "friend.materialize" && name == "" {
			item, err := s.Friends.GetFriendRelation(ctx, a.Owner, a.Peer)
			if err != nil {
				return nil, err
			}
			if item.WorkspaceName == nil {
				return nil, errors.New("active Friend has no Workspace name")
			}
			name = *item.WorkspaceName
		}
		return s.Materialize(ctx, a.Operation, name, a.Owner)
	case "peers.visible":
		return s.peersVisible(ctx)
	case "friend.relation":
		return map[string]string{"id": usageRelation(a.Owner, a.Peer)}, nil
	case "friend.capacity.changed":
		if a.Owner == "" || a.Token == "" {
			return nil, errors.New("owner and observed capacity revision required")
		}
		data, err := s.Stores["friend"].Get(kv.WithPrimaryReads(ctx), kv.Key{"friends", "friend-peer-revisions", url.QueryEscape(a.Owner)})
		if err != nil {
			return nil, err
		}
		if string(data) == a.Token {
			return nil, errors.New("competing Friend did not advance the observed capacity revision")
		}
		return map[string]any{"revision": string(data), "changed": true}, nil
	case "credential.unknown":
		if len(a.Token) < 43 {
			return nil, errors.New("an actual issued fixture API key is required")
		}
		prefix, suffix := a.Token[:len(a.Token)-43], a.Token[len(a.Token)-43:]
		decoded, err := base64.RawURLEncoding.DecodeString(suffix)
		if err != nil || len(decoded) != 32 {
			return nil, errors.New("fixture API key must contain a 32-byte secret")
		}
		decoded[31] ^= 1
		candidate := prefix + base64.RawURLEncoding.EncodeToString(decoded)
		if _, err := s.Stores["apikey"].Get(kv.WithPrimaryReads(ctx), kv.Key{"secrets", candidate}); !errors.Is(err, kv.ErrNotFound) {
			return nil, fmt.Errorf("derived fixture credential is not provably absent: %v", err)
		}
		return map[string]string{"api_key": candidate}, nil
	case "identity.new":
		key, err := giznet.GenerateKeyPair()
		if err != nil {
			return nil, err
		}
		return map[string]string{"public_key": key.Public.String()}, nil
	case "route.lookup", "route.resolve":
		key, err := usagePeer(a.Peer)
		if err != nil {
			return nil, err
		}
		if a.Operation == "route.lookup" {
			return s.Routes.Lookup(ctx, key)
		}
		return s.Routes.Resolve(ctx, key)
	case "read", "members":
		return s.read(ctx, a)
	case "friend.creation.workspace.count":
		if s.Workspaces == nil || a.Owner == "" || a.Peer == "" {
			return nil, errors.New("exact creation participants and Workspace SQL are required")
		}
		relation := usageRelation(a.Owner, a.Peer)
		raw, err := s.Stores["friend"].Get(kv.WithPrimaryReads(ctx), kv.Key{"friends", "friend-creation-intents", url.QueryEscape(relation)})
		if err != nil {
			return nil, err
		}
		var intent struct {
			RelationID     string `json:"relation_id"`
			WorkspaceName  string `json:"workspace_name"`
			WorkspaceOwner string `json:"workspace_owner"`
			IncarnationID  string `json:"incarnation_id"`
		}
		if err := json.Unmarshal(raw, &intent); err != nil {
			return nil, err
		}
		if intent.RelationID != relation || intent.WorkspaceName == "" || intent.WorkspaceOwner == "" || intent.IncarnationID == "" {
			return nil, errors.New("primary creation intent lacks its exact identity")
		}
		var count int
		if err := s.Workspaces.QueryRowContext(ctx, "SELECT COUNT(*) FROM workspaces WHERE name = $1 AND owner_public_key = $2", intent.WorkspaceName, intent.WorkspaceOwner).Scan(&count); err != nil {
			return nil, err
		}
		if a.Equals != nil && !reflect.DeepEqual(float64(count), a.Equals) {
			return nil, fmt.Errorf("exact pending creation Workspace count = %d, want %v", count, a.Equals)
		}
		return map[string]any{"count": count, "workspace_name": intent.WorkspaceName, "workspace_owner": intent.WorkspaceOwner, "incarnation_id": intent.IncarnationID}, nil
	case "workspace.count":
		if s.Workspaces == nil || a.Name == "" {
			return nil, errors.New("Workspace database and exact name required")
		}
		if a.Wait != "" {
			duration, err := time.ParseDuration(a.Wait)
			if err != nil || duration <= 0 || duration > 80*time.Second {
				return nil, errors.New("invalid SQL observation wait")
			}
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, duration)
			defer cancel()
		}
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			var count int
			if err := s.Workspaces.QueryRowContext(ctx, s.WorkspacesQuery(), a.Name).Scan(&count); err != nil {
				return nil, err
			}
			if a.Wait == "" || a.Equals == nil || reflect.DeepEqual(float64(count), a.Equals) {
				return map[string]int{"count": count}, nil
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-ticker.C:
			}
		}
	case "social.prepare":
		return (social.PeerRetirement{Contacts: s.Contacts, Friends: s.Friends, FriendGroups: s.Groups}).PreparePeerSocialRetirement(ctx, a.Peer)
	case "peer.tasks":
		tasks, err := peer.PendingDeletionSource(kv.Prefixed(s.Stores["peer"], kv.Key{"records"})).ListTasks(ctx, pendingdeletion.SourceListOptions{Kinds: map[pendingdeletion.Kind]bool{pendingdeletion.KindPeer: true}, Limit: 200})
		if err != nil {
			return nil, err
		}
		count := 0
		for _, task := range tasks {
			if task.Record.ResourceID == a.Peer {
				count++
			}
		}
		return map[string]int{"count": count}, nil
	case "peer.available":
		key, err := usagePeer(a.Peer)
		if err != nil {
			return nil, err
		}
		return nil, s.Peers.EnsureAvailable(ctx, key)
	case "peer.delete":
		key, err := usagePeer(a.Peer)
		if err != nil {
			return nil, err
		}
		return nil, s.Peers.DeleteSelf(ctx, key)
	case "group.create":
		return s.Groups.AdminCreateFriendGroup(ctx, a.ID, a.Owner, a.Name, a.DisplayName, a.Description)
	case "group.get":
		return s.Groups.AdminGetFriendGroupObject(ctx, a.ID)
	case "group.delete":
		return s.Groups.AdminDeleteFriendGroup(ctx, a.ID)
	case "group.replace":
		if _, err := s.Groups.AdminDeleteFriendGroup(ctx, a.ID); err != nil {
			return nil, err
		}
		// A distinct canonical ID is supplied by the document. Both calls use
		// actual Group mutation/retirement and actual SQL Workspace services.
		return s.Groups.AdminCreateFriendGroup(ctx, a.Token, a.Owner, a.Name, a.DisplayName, a.Description)
	case "group.invite.put":
		return s.Groups.AdminPutFriendGroupInviteToken(ctx, a.ID, a.Token, time.Now().UTC().Add(time.Duration(a.TTLSeconds)*time.Second))
	case "group.invite.create":
		return s.Groups.CreateFriendGroupInviteTokenWithTTL(ctx, a.Owner, rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: a.Name}, time.Duration(a.TTLSeconds)*time.Second)
	case "group.invite.clear":
		return s.Groups.AdminDeleteFriendGroupInviteToken(ctx, a.ID)
	case "group.member.add":
		return s.Groups.AdminCreateFriendGroupMember(ctx, a.ID, a.Peer, a.Name, rpcapi.FriendGroupMemberRoleMember)
	case "group.member.delete":
		return s.Groups.AdminDeleteFriendGroupMember(ctx, a.ID, a.Peer)
	case "group.snapshot":
		return s.Groups.SnapshotPeerGroups(ctx, a.Peer)
	case "group.retire":
		var snapshot friendgroup.PeerRetirementGroup
		if err := json.Unmarshal(a.Snapshot, &snapshot); err != nil {
			return nil, err
		}
		if a.Workspace != "" {
			snapshot.WorkspaceName = a.Workspace
		}
		return nil, s.Groups.RetirePeerGroup(ctx, snapshot)
	case "friend.create":
		return s.Friends.AdminCreateFriendResource(ctx, usageRelation(a.Owner, a.Peer), a.Owner, a.Peer)
	case "friend.get":
		return s.Friends.AdminGetFriend(ctx, a.Owner, usageRelation(a.Owner, a.Peer))
	case "friend.delete":
		return s.Friends.AdminDeleteFriend(ctx, a.Owner, usageRelation(a.Owner, a.Peer))
	case "friend.delete.device":
		return s.Friends.DeleteFriend(ctx, a.Owner, rpcapi.FriendDeleteRequest{Name: a.Peer})
	case "friend.invite.create":
		return s.Friends.CreateFriendInviteTokenWithTTL(ctx, a.Owner, time.Duration(a.TTLSeconds)*time.Second)
	case "friend.invite.clear":
		return s.Friends.ClearFriendInviteToken(ctx, a.Owner, rpcapi.FriendInviteTokenClearRequest{})
	case "friend.reconcile":
		return nil, errors.Join(s.Friends.ReconcileCreationIntents(ctx), s.Friends.ReconcileRetirementIntents(ctx))
	case "friend.creation.reconcile":
		return nil, s.Friends.ReconcileCreationIntents(ctx)
	case "friend.retirement.reconcile":
		return nil, s.Friends.ReconcileRetirementIntents(ctx)
	case "friend.snapshot":
		return s.Friends.SnapshotPeerFriends(ctx, a.Owner)
	case "friend.retire":
		var snapshot friend.PeerRetirementFriend
		if err := json.Unmarshal(a.Snapshot, &snapshot); err != nil {
			return nil, err
		}
		return nil, s.Friends.RetirePeerFriend(ctx, a.Owner, snapshot)
	case "contact.create":
		return s.Contacts.CreateContact(ctx, a.Owner, rpcapi.ContactCreateRequest{Name: a.Name})
	case "presence.publish":
		key, err := usagePeer(a.Peer)
		if err != nil || a.Publisher == "" {
			return nil, errors.New("valid Peer and publisher name required")
		}
		s.mu.Lock()
		publisher := s.publishers[a.Publisher]
		if publisher == nil {
			publisher = s.Presence.NewPublisher(key)
			s.publishers[a.Publisher] = publisher
		}
		s.mu.Unlock()
		publisher.Publish(peerpresence.Snapshot{Online: a.Online, WorkspaceName: a.Workspace, LastSeenAt: time.Now().UTC()})
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown diagnostic operation %q", a.Operation)
	}
}

func (s *UsageServices) peersVisible(ctx context.Context) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	var cursor *string
	count := 0
	for range 20 {
		response, err := s.Peers.ListPeers(kv.WithPrimaryReads(ctx), adminhttp.ListPeersRequestObject{Params: adminhttp.ListPeersParams{Cursor: cursor, Limit: new(int32(200))}})
		if err != nil {
			return nil, err
		}
		page, ok := response.(adminhttp.ListPeers200JSONResponse)
		if !ok {
			return nil, fmt.Errorf("primary Peer list returned %T", response)
		}
		for _, item := range page.Items {
			tombstone, err := item.AsExternalRef0RegistrationTombstone()
			if err != nil {
				return nil, err
			}
			if tombstone.Status == "deleted" {
				continue
			}
			registration, err := item.AsExternalRef0Registration()
			if err != nil {
				return nil, err
			}
			if registration.PublicKey == "" {
				return nil, errors.New("Peer registration has no public key")
			}
			if _, err := s.read(ctx, UsageAction{Operation: "read", Store: "peer", Key: []string{"records", "by-pubkey", registration.PublicKey}, Found: new(true), RequireLearner: true, Wait: "20s"}); err != nil {
				return nil, err
			}
			count++
		}
		if !page.HasNext {
			return map[string]int{"visible": count}, nil
		}
		cursor = page.NextCursor
	}
	return nil, errors.New("fixture Peer list exceeded 20 pages")
}

// WorkspacesQuery returns a read-only exact-name query understood by both
// fixture SQL drivers. SQLite and PostgreSQL both accept this dollar parameter.
func (s *UsageServices) WorkspacesQuery() string {
	return "SELECT COUNT(*) FROM workspaces WHERE name = $1"
}

func (s *UsageServices) read(ctx context.Context, a UsageAction) (any, error) {
	store := s.Stores[a.Store]
	if store == nil || len(a.Key) == 0 {
		return nil, errors.New("known store and exact key required")
	}
	key := make(kv.Key, len(a.Key))
	for i, part := range a.Key {
		key[i] = url.QueryEscape(part)
	}
	if a.Wait != "" {
		duration, err := time.ParseDuration(a.Wait)
		if err != nil || duration <= 0 || duration > 80*time.Second {
			return nil, errors.New("wait must be positive and at most 80s")
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var value any
		receipt := new(usageReadReceipt)
		readContext := ctx
		if a.RequireLearner {
			readContext = context.WithValue(ctx, usageReadReceiptKey{}, receipt)
		}
		found := true
		if a.Operation == "members" {
			members, err := store.ListMembers(ctx, key)
			if err != nil {
				return nil, err
			}
			sort.Strings(members)
			if members == nil {
				members = []string{}
			}
			value = members
			found = len(members) != 0
		} else {
			raw, err := store.Get(readContext, key)
			if err != nil && !errors.Is(err, kv.ErrNotFound) {
				return nil, err
			}
			found = err == nil
			if found {
				if json.Unmarshal(raw, &value) != nil {
					value = string(raw)
				}
			}
		}
		result := map[string]any{"found": found, "data": value}
		if a.RequireLearner {
			result["native_learner"] = receipt.learner.Load()
		}
		comparison := value
		if a.Field != "" {
			object, ok := value.(map[string]any)
			if ok {
				comparison = object[a.Field]
			} else {
				comparison = nil
			}
		}
		matched := (a.Found == nil || found == *a.Found) && (a.Equals == nil || reflect.DeepEqual(comparison, a.Equals)) && (!a.RequireLearner || receipt.learner.Load())
		if matched {
			return result, nil
		}
		if a.Wait == "" {
			if a.RequireLearner && !receipt.learner.Load() {
				return nil, errors.New("read had no successful native Learner response")
			}
			return result, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%s %s did not reach expected observation: %w", a.Store, key, ctx.Err())
		case <-ticker.C:
		}
	}
}

// usageForward is a test-owned envelope for a real HTTP response. It is used
// when a race may legitimately return different 4xx codes. No status or body
// is synthesized; transport errors remain errors and cannot pass a 4xx check.
func usageForward(ctx context.Context, action UsageAction) (any, error) {
	if action.Endpoint == "" || action.Method == "" || len(action.Path) == 0 || action.Path[0] != '/' {
		return nil, errors.New("HTTP endpoint, method and absolute path required")
	}
	var body io.Reader
	if action.Body != nil {
		raw, err := json.Marshal(action.Body)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, action.Method, action.Endpoint+action.Path, body)
	if err != nil {
		return nil, err
	}
	for name, value := range action.Headers {
		req.Header.Set(name, value)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, errors.New("HTTP response exceeds 4 MiB")
	}
	var value any
	if len(raw) != 0 && json.Unmarshal(raw, &value) != nil {
		value = string(raw)
	}
	return map[string]any{"status": response.StatusCode, "body": value}, nil
}

type usageAfter struct {
	WaitEndpoint    string        `json:"wait_endpoint"`
	Gate            string        `json:"gate"`
	Actions         []UsageAction `json:"actions"`
	ReleaseEndpoint string        `json:"release_endpoint"`
}

// usageControl uses the existing private fixture endpoint without forwarding
// cookies, credentials or headers from the incoming HTTP control request.
func usageControl(ctx context.Context, endpoint, path string, input, output any) error {
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		text, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return fmt.Errorf("fixture %s: status %d: %s", path, response.StatusCode, text)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output)
}

func (g *UsageGate) after(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var input usageAfter
	if err := usageDecode(r, &input); err != nil || input.Gate == "" || input.WaitEndpoint == "" || len(input.Actions) == 0 || len(input.Actions) > 8 {
		http.Error(w, "wait_endpoint, gate and 1..8 real service actions required", http.StatusBadRequest)
		return
	}
	release := input.ReleaseEndpoint
	if release == "" {
		release = input.WaitEndpoint
	}
	// Release even on a competing service failure. The failed action remains
	// a failing HTTP step; it cannot turn a blocked request into a false pass.
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = usageControl(cleanup, release, "/release", map[string]string{"id": input.Gate}, new(UsageObservation))
	}()
	var observation UsageObservation
	if err := usageControl(ctx, input.WaitEndpoint, "/wait", map[string]string{"id": input.Gate}, &observation); err != nil {
		http.Error(w, err.Error(), http.StatusGatewayTimeout)
		return
	}
	results := make([]usageActionResult, 0, len(input.Actions))
	for _, action := range input.Actions {
		result, err := g.invoke(ctx, action)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		results = append(results, result)
	}
	usageJSON(w, map[string]any{"diagnostic": true, "boundary": observation, "actions": results})
}
