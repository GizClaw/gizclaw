//go:build gizclaw_e2e

// User story: As a Play UI user, I can use the social pages and chat drawer
// built around invite tokens and workspace history.
package playui_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
	. "github.com/GizClaw/gizclaw-go/test/gizclaw-e2e/ui/internal/harness"
	"github.com/goccy/go-yaml"
	"github.com/playwright-community/playwright-go"
)

func playSocialStories() []Story {
	return []Story{
		{
			Name: "205-play-social-shell",
			Run: func(_ testing.TB, page *Page) {
				page.GotoPlay("/")

				page.ClickRoleLike("button", "Contacts")
				page.ExpectText("New Contact")

				page.ClickRoleLike("button", "Friends")
				page.ExpectText("Invite Token")
				page.ExpectText("Add Friend")
				page.ExpectNoText("Request")

				page.ClickRoleLike("tab", "Invite Token")
				page.ExpectText("Invite token")
				page.ClickRoleLike("tab", "Add Friend")
				page.ExpectText("Add Friend")

				page.ClickRoleLike("button", "Groups")
				page.ExpectText("Create Group")
				page.ExpectText("Join Group")
				page.ExpectNoText("Request")

				page.ClickRole("button", "Chat")
				page.ExpectText("Append voice messages and replay history through the selected social workspace.")
			},
		},
		{
			Name: "206-play-social-contacts-crud",
			Run: func(t testing.TB, page *Page) {
				contactName := fmt.Sprintf("UI Contact %d", time.Now().UnixNano())
				renamedContact := contactName + " Renamed"

				page.GotoPlay("/")
				page.ClickRoleLike("button", "Contacts")
				page.ClickRole("button", "New Contact")
				fillLabel(t, page, "Display name", contactName)
				fillLabel(t, page, "Phone number", fmt.Sprintf("+1555%d", time.Now().UnixNano()%1000000000))
				page.ClickRole("button", "Create")
				page.ExpectText(contactName)

				clickContactAction(t, page, contactName, "Edit")
				fillLabel(t, page, "Display name", renamedContact)
				page.ClickRole("button", "Save")
				page.ExpectText(renamedContact)

				clickContactAction(t, page, renamedContact, "Delete")
			},
		},
		{
			Name: "207-play-social-friend-invite-token-flow",
			Run: func(t testing.TB, page *Page) {
				t.Cleanup(func() {
					restorePlayHTTPWorkspace(t, page.Seed.PlayURL, SeedWorkspaceName)
				})
				other := newPlaySocialRPCPeer(t, "friend-target")
				otherToken := other.createFriendInviteToken(t)

				page.GotoPlay("/")
				page.ClickRoleLike("button", "Friends")
				page.ClickRoleLike("tab", "Invite Token")
				clickRoleLikeAt(t, page, "button", "Refresh", 1)
				expectInputValue(t, page, "#friend-invite-token")
				page.ClickRole("button", "Clear")
				expectInputValueEquals(t, page, "#friend-invite-token", "")

				page.ClickRoleLike("tab", "Add Friend")
				selfToken := createPlayHTTPFriendInviteToken(t, page)
				page.Fill("#friend-add-token", selfToken)
				page.ClickRole("button", "Add Friend")
				page.ExpectText("cannot friend self")

				page.Fill("#friend-add-token", otherToken)
				page.ClickRole("button", "Add Friend")
				page.ExpectText(other.publicKey)
				page.ExpectText("History")

				clickRoleAt(t, page, "button", "Chat", 1)
				page.ExpectText("Composer")
				page.ExpectText("History")
				page.ExpectText("Friend /")
			},
		},
		{
			Name: "208-play-social-group-invite-token-flow",
			Run: func(t testing.TB, page *Page) {
				t.Cleanup(func() {
					restorePlayHTTPWorkspace(t, page.Seed.PlayURL, SeedWorkspaceName)
				})
				owner := newPlaySocialRPCPeer(t, "group-owner")
				groupName := fmt.Sprintf("ui-play-social-join-%d", time.Now().UnixNano())
				group := owner.createFriendGroup(t, groupName)
				token := owner.createFriendGroupInviteToken(t, stringValue(group.Id))

				page.GotoPlay("/")
				page.ClickRoleLike("button", "Groups")
				page.ClickRole("button", "Join Group")
				page.Fill("#group-join-token", token)
				page.ClickRole("button", "Join Group")
				page.ExpectText(groupName)
				page.ExpectText("member")

				page.ClickRoleLike("tab", "Invite Token")
				page.ExpectText("Invite token unavailable")

				clickRoleAt(t, page, "button", "Chat", 1)
				page.ExpectText("Composer")
				page.ExpectText("History")
				page.ExpectText("Group /")
			},
		},
		{
			Name: "209-play-social-owner-group-token-flow",
			Run: func(t testing.TB, page *Page) {
				groupName := fmt.Sprintf("ui-play-social-owned-%d", time.Now().UnixNano())

				page.GotoPlay("/")
				page.ClickRoleLike("button", "Groups")
				page.ClickRole("button", "Create Group")
				fillLabel(t, page, "Name", groupName)
				fillLabel(t, page, "Description", "Created by Play UI social e2e")
				page.ClickRole("button", "Create Group")
				page.ExpectText(groupName)
				page.ExpectText("owner")

				page.ClickRoleLike("tab", "Invite Token")
				clickRoleLikeAt(t, page, "button", "Refresh", 1)
				expectInputValue(t, page, "#group-invite-token")
				page.ClickRole("button", "Clear")
				expectInputValueEquals(t, page, "#group-invite-token", "")
			},
		},
	}
}

func clickContactAction(t testing.TB, page *Page, contactName string, action string) {
	t.Helper()
	row := page.Raw().Locator(fmt.Sprintf(`table tbody tr:has-text(%q)`, contactName)).First()
	if err := row.GetByRole(playwright.AriaRole("button"), playwright.LocatorGetByRoleOptions{
		Name:  action,
		Exact: playwright.Bool(true),
	}).Click(); err != nil {
		t.Fatalf("click contact %q action %q: %v", contactName, action, err)
	}
}

func TestPlaySocialStories(t *testing.T) {
	RunPlayStories(t, playSocialStories())
}

type playSocialRPCPeer struct {
	publicKey string
	client    *gizcli.Client
	done      chan error
}

type playSocialEndpoint struct {
	address         string
	serverPublicKey giznet.PublicKey
}

type playContextConfig struct {
	Server struct {
		Address   string `yaml:"address"`
		PublicKey string `yaml:"public-key"`
	} `yaml:"server"`
}

func newPlaySocialRPCPeer(t testing.TB, label string) *playSocialRPCPeer {
	t.Helper()

	endpoint := loadPlaySocialEndpoint(t)
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatalf("generate social peer key: %v", err)
	}
	client := &gizcli.Client{KeyPair: keyPair}
	if err := client.Dial(endpoint.serverPublicKey, endpoint.address); err != nil {
		t.Fatalf("dial social peer: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- client.Serve()
	}()
	peer := &playSocialRPCPeer{client: client, done: done, publicKey: keyPair.Public.String()}
	t.Cleanup(func() { peer.close(t) })

	waitForSocialRPCReady(t, peer, label)
	name := "ui-play-social-" + label
	sn := name + "-" + strings.ReplaceAll(peer.publicKey[:12], " ", "-")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := peer.client.PutServerInfo(ctx, "ui.play.social.info.put", rpcapi.ServerPutInfoRequest{Name: &name, Sn: &sn}); err != nil {
		t.Fatalf("register social peer %q: %v", label, err)
	}
	return peer
}

func (p *playSocialRPCPeer) close(t testing.TB) {
	t.Helper()
	if p == nil || p.client == nil {
		return
	}
	_ = p.client.Close()
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Logf("social peer client for %s did not stop before timeout", p.publicKey)
	}
}

func (p *playSocialRPCPeer) createFriendInviteToken(t testing.TB) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token, err := p.client.CreateFriendInviteToken(ctx, "ui.play.friend.invite_token.create", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("create friend invite token for %s: %v", p.publicKey, err)
	}
	if token == nil || token.InviteToken == "" {
		t.Fatalf("empty friend invite token for %s: %#v", p.publicKey, token)
	}
	return token.InviteToken
}

func (p *playSocialRPCPeer) createFriendGroup(t testing.TB, name string) rpcapi.FriendGroupObject {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	group, err := p.client.CreateFriendGroup(ctx, "ui.play.friend_group.create", rpcapi.FriendGroupCreateRequest{Name: name})
	if err != nil {
		t.Fatalf("create friend group for %s: %v", p.publicKey, err)
	}
	if group == nil || stringValue(group.Id) == "" || stringValue(group.WorkspaceName) == "" {
		t.Fatalf("invalid friend group for %s: %#v", p.publicKey, group)
	}
	return *group
}

func (p *playSocialRPCPeer) createFriendGroupInviteToken(t testing.TB, groupID string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token, err := p.client.CreateFriendGroupInviteToken(ctx, "ui.play.friend_group.invite_token.create", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupId: groupID})
	if err != nil {
		t.Fatalf("create group invite token for %s: %v", groupID, err)
	}
	if token == nil || token.InviteToken == "" {
		t.Fatalf("empty group invite token for %s: %#v", groupID, token)
	}
	return token.InviteToken
}

func waitForSocialRPCReady(t testing.TB, peer *playSocialRPCPeer, label string) {
	t.Helper()
	if err := WaitUntil(15*time.Second, func() error {
		select {
		case err := <-peer.done:
			if err == nil {
				return fmt.Errorf("social peer %q stopped early", label)
			}
			return fmt.Errorf("social peer %q stopped early: %w", label, err)
		default:
		}
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		_, err := peer.client.Ping(ctx, "ui.play.social.ping")
		return err
	}); err != nil {
		t.Fatalf("social peer %q did not become ready: %v", label, err)
	}
}

func loadPlaySocialEndpoint(t testing.TB) playSocialEndpoint {
	t.Helper()
	repoRoot := findRepoRoot(t)
	configHome := getenvDefaultLocal("GIZCLAW_E2E_PLAY_UI_CONFIG_HOME", filepath.Join(repoRoot, "test", "gizclaw-e2e", "testdata", "gizclaw-config-home"))
	contextName := getenvDefaultLocal("GIZCLAW_E2E_PLAY_UI_CONTEXT", "e2e-client")
	if !filepath.IsAbs(configHome) {
		configHome = filepath.Join(repoRoot, configHome)
	}
	data, err := os.ReadFile(filepath.Join(configHome, "gizclaw", contextName, "config.yaml"))
	if err != nil {
		t.Fatalf("read Play UI context config: %v", err)
	}
	var cfg playContextConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse Play UI context config: %v", err)
	}
	if strings.TrimSpace(cfg.Server.Address) == "" {
		t.Fatal("Play UI context server address is empty")
	}
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(strings.TrimSpace(cfg.Server.PublicKey))); err != nil {
		t.Fatalf("parse Play UI context server public key: %v", err)
	}
	if publicKey.IsZero() {
		t.Fatal("Play UI context server public key is zero")
	}
	return playSocialEndpoint{address: strings.TrimSpace(cfg.Server.Address), serverPublicKey: publicKey}
}

func expectInputValue(t testing.TB, page *Page, selector string) string {
	if t != nil {
		t.Helper()
	}
	var value string
	if err := WaitUntil(10*time.Second, func() error {
		got, err := page.Raw().Locator(selector).InputValue()
		if err != nil {
			return err
		}
		if strings.TrimSpace(got) == "" {
			return fmt.Errorf("input %s is empty", selector)
		}
		value = got
		return nil
	}); err != nil {
		if t == nil {
			panic(err)
		}
		t.Fatalf("wait for non-empty %s: %v", selector, err)
	}
	return value
}

func expectInputValueEquals(t testing.TB, page *Page, selector, want string) {
	if t != nil {
		t.Helper()
	}
	if err := WaitUntil(10*time.Second, func() error {
		got, err := page.Raw().Locator(selector).InputValue()
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("input %s = %q, want %q", selector, got, want)
		}
		return nil
	}); err != nil {
		if t == nil {
			panic(err)
		}
		t.Fatalf("wait for %s = %q: %v", selector, want, err)
	}
}

func createPlayHTTPFriendInviteToken(t testing.TB, page *Page) string {
	t.Helper()
	client := http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(page.Seed.PlayURL, "/")+"/peer-resources/friends/@invite-token", nil)
	if err != nil {
		t.Fatalf("create current Play UI friend invite token request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("create current Play UI friend invite token: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		t.Fatalf("read current Play UI friend invite token body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create current Play UI friend invite token status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		InviteToken string `json:"invite_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode current Play UI friend invite token: %v", err)
	}
	if strings.TrimSpace(out.InviteToken) == "" {
		t.Fatalf("current Play UI friend invite token is empty: %s", strings.TrimSpace(string(body)))
	}
	return out.InviteToken
}

func restorePlayHTTPWorkspace(t testing.TB, playURL, workspaceName string) {
	t.Helper()
	body := strings.NewReader(fmt.Sprintf(`{"workspace_name":%q}`, workspaceName))
	client := http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodPut, strings.TrimRight(playURL, "/")+"/peer-run/workspace", body)
	if err != nil {
		t.Fatalf("restore Play UI workspace request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("restore Play UI workspace %q: %v", workspaceName, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		t.Fatalf("read restore Play UI workspace body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restore Play UI workspace %q status=%d body=%s", workspaceName, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
}

func fillLabel(t testing.TB, page *Page, label, value string) {
	t.Helper()
	if err := page.Raw().GetByLabel(label).Fill(value); err != nil {
		t.Fatalf("fill label %q: %v", label, err)
	}
}

func clickRoleAt(t testing.TB, page *Page, role, name string, index int) {
	t.Helper()
	if err := page.Raw().GetByRole(playwright.AriaRole(role), pageGetByRoleOptions(name, true)).Nth(index).Click(); err != nil {
		t.Fatalf("click role=%s name=%q index=%d: %v", role, name, index, err)
	}
}

func clickRoleLikeAt(t testing.TB, page *Page, role, name string, index int) {
	t.Helper()
	if err := page.Raw().GetByRole(playwright.AriaRole(role), pageGetByRoleOptions(name, false)).Nth(index).Click(); err != nil {
		t.Fatalf("click role=%s name~=%q index=%d: %v", role, name, index, err)
	}
}

func pageGetByRoleOptions(name string, exact bool) playwright.PageGetByRoleOptions {
	return playwright.PageGetByRoleOptions{
		Name:  name,
		Exact: playwright.Bool(exact),
	}
}

func findRepoRoot(t testing.TB) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			t.Fatal("repo root with go.mod not found")
		}
		wd = parent
	}
}

func getenvDefaultLocal(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
