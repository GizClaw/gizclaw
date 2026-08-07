package publiclogin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// CleanupPeer revokes every Public Login credential owned by, targeting, or
// controlled by one canonical Peer identity. It is safe to replay.
func (m *SessionManager) CleanupPeer(ctx context.Context, publicKeyText string) error {
	if m == nil || m.Store == nil {
		return errInvalidSession
	}
	publicKey, err := canonicalPublicKey(publicKeyText)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	keys, err := m.peerCleanupKeys(ctx, publicKey)
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		if err := m.Store.BatchDelete(ctx, keys); err != nil {
			return err
		}
	}
	remaining, err := m.peerCleanupKeys(ctx, publicKey)
	if err != nil {
		return err
	}
	if len(remaining) != 0 {
		return fmt.Errorf("publiclogin: %d peer credentials remain after cleanup", len(remaining))
	}
	return nil
}

func canonicalPublicKey(value string) (giznet.PublicKey, error) {
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(value)); err != nil || publicKey.IsZero() || publicKey.String() != value {
		return giznet.PublicKey{}, errors.New("publiclogin: public key must be canonical")
	}
	return publicKey, nil
}

func (m *SessionManager) peerCleanupKeys(ctx context.Context, publicKey giznet.PublicKey) ([]kv.Key, error) {
	keys := make(map[string]kv.Key)
	add := func(key kv.Key) {
		encoded, _ := json.Marshal([]string(key))
		keys[string(encoded)] = append(kv.Key{}, key...)
	}

	for entry, err := range m.Store.List(ctx, kv.Key{"assertions", publicKey.String()}) {
		if err != nil {
			return nil, err
		}
		if len(entry.Key) != 3 {
			return nil, fmt.Errorf("publiclogin: malformed assertion key %v", entry.Key)
		}
		add(entry.Key)
	}

	for entry, err := range m.Store.List(ctx, primarySessionOwnerPrefix(publicKey)) {
		if err != nil {
			return nil, err
		}
		if len(entry.Key) != 3 {
			return nil, fmt.Errorf("publiclogin: malformed primary session owner key %v", entry.Key)
		}
		token := entry.Key[2]
		data, getErr := m.Store.Get(ctx, sessionKey(token))
		if getErr != nil {
			return nil, fmt.Errorf("publiclogin: primary session owner index %v has no bearer: %w", entry.Key, getErr)
		}
		var sess session
		if err := json.Unmarshal(data, &sess); err != nil || sess.PublicKey != publicKey.String() || (sess.Kind != "" && sess.Kind != SessionKindPrimary) {
			return nil, fmt.Errorf("publiclogin: primary session owner index %v is cross-owned", entry.Key)
		}
		add(entry.Key)
		add(sessionKey(token))
	}

	// Sessions is a bounded credential namespace. Scanning it also discovers
	// compatible rows written before owner/controller indexes existed.
	for entry, err := range m.Store.List(ctx, kv.Key{"sessions"}) {
		if err != nil {
			return nil, err
		}
		if len(entry.Key) != 2 {
			return nil, fmt.Errorf("publiclogin: malformed bearer key %v", entry.Key)
		}
		var sess session
		if err := json.Unmarshal(entry.Value, &sess); err != nil {
			return nil, fmt.Errorf("publiclogin: malformed bearer %v: %w", entry.Key, err)
		}
		owner, err := canonicalPublicKey(sess.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("publiclogin: bearer %v has invalid owner", entry.Key)
		}
		var target giznet.PublicKey
		if sess.TargetPublicKey != "" {
			target, err = canonicalPublicKey(sess.TargetPublicKey)
			if err != nil {
				return nil, fmt.Errorf("publiclogin: bearer %v has invalid target", entry.Key)
			}
		}
		if owner != publicKey && target != publicKey {
			continue
		}
		add(entry.Key)
		if sess.Kind == "" || sess.Kind == SessionKindPrimary {
			if owner != publicKey {
				return nil, fmt.Errorf("publiclogin: primary bearer %v has unexpected target", entry.Key)
			}
			add(primarySessionOwnerKey(owner, entry.Key[1]))
			continue
		}
		if sess.Kind != SessionKindSideControl || target.IsZero() || !validSideControlID(sess.SessionID) {
			return nil, fmt.Errorf("publiclogin: malformed side-control bearer %v", entry.Key)
		}
		add(sideSessionKey(target, sess.SessionID))
		add(sideSessionControllerKey(owner, sess.SessionID))
	}

	if err := m.collectSideSessionIndexes(ctx, sideSessionPrefix(publicKey), publicKey, true, add); err != nil {
		return nil, err
	}
	if err := m.collectSideSessionIndexes(ctx, sideSessionControllerPrefix(publicKey), publicKey, false, add); err != nil {
		return nil, err
	}

	for entry, err := range m.Store.List(ctx, kv.Key{"side-control", "device-tokens", "by-owner", publicKey.String()}) {
		if err != nil {
			return nil, err
		}
		if len(entry.Key) != 5 || !validSideControlID(entry.Key[4]) {
			return nil, fmt.Errorf("publiclogin: malformed device-token owner index %v", entry.Key)
		}
		hash := string(entry.Value)
		data, getErr := m.Store.Get(ctx, deviceTokenHashKey(hash))
		if getErr != nil {
			return nil, fmt.Errorf("publiclogin: device-token owner index %v has no token: %w", entry.Key, getErr)
		}
		var record deviceTokenRecord
		if err := json.Unmarshal(data, &record); err != nil || record.TargetPublicKey != publicKey.String() || record.ID != entry.Key[4] {
			return nil, fmt.Errorf("publiclogin: device-token owner index %v is cross-owned", entry.Key)
		}
		add(entry.Key)
		add(deviceTokenHashKey(hash))
	}
	for entry, err := range m.Store.List(ctx, kv.Key{"side-control", "device-tokens", "by-hash"}) {
		if err != nil {
			return nil, err
		}
		if len(entry.Key) != 4 {
			return nil, fmt.Errorf("publiclogin: malformed device-token hash key %v", entry.Key)
		}
		var record deviceTokenRecord
		if err := json.Unmarshal(entry.Value, &record); err != nil {
			return nil, fmt.Errorf("publiclogin: malformed device-token record %v", entry.Key)
		}
		if record.TargetPublicKey != publicKey.String() {
			continue
		}
		if !validSideControlID(record.ID) {
			return nil, fmt.Errorf("publiclogin: malformed device-token record %v", entry.Key)
		}
		add(entry.Key)
		add(deviceTokenOwnerKey(publicKey, record.ID))
	}

	out := make([]kv.Key, 0, len(keys))
	for _, key := range keys {
		out = append(out, key)
	}
	return out, nil
}

func (m *SessionManager) collectSideSessionIndexes(ctx context.Context, prefix kv.Key, identity giznet.PublicKey, targetIndex bool, add func(kv.Key)) error {
	for entry, err := range m.Store.List(ctx, prefix) {
		if err != nil {
			return err
		}
		if len(entry.Key) != 4 || !validSideControlID(entry.Key[3]) {
			return fmt.Errorf("publiclogin: malformed side-control index %v", entry.Key)
		}
		var index sideSessionIndex
		if err := json.Unmarshal(entry.Value, &index); err != nil || index.AccessToken == "" || index.Session.SessionID != entry.Key[3] {
			return fmt.Errorf("publiclogin: malformed side-control index %v", entry.Key)
		}
		owned := index.Session.PublicKey == identity.String()
		targeted := index.Session.TargetPublicKey == identity.String()
		if (targetIndex && !targeted) || (!targetIndex && !owned) {
			return fmt.Errorf("publiclogin: cross-owned side-control index %v", entry.Key)
		}
		data, err := m.Store.Get(ctx, sessionKey(index.AccessToken))
		if err != nil {
			return fmt.Errorf("publiclogin: side-control index %v has no bearer: %w", entry.Key, err)
		}
		var sess session
		if err := json.Unmarshal(data, &sess); err != nil || sess != index.Session {
			return fmt.Errorf("publiclogin: side-control index %v does not match bearer", entry.Key)
		}
		owner, err := canonicalPublicKey(sess.PublicKey)
		if err != nil {
			return err
		}
		target, err := canonicalPublicKey(sess.TargetPublicKey)
		if err != nil {
			return err
		}
		add(entry.Key)
		add(sessionKey(index.AccessToken))
		add(sideSessionKey(target, sess.SessionID))
		add(sideSessionControllerKey(owner, sess.SessionID))
	}
	return nil
}
