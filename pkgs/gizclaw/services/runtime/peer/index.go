package peer

import (
	"sort"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func peerSN(peer apitypes.Peer) string {
	if peer.Device.Identifiers == nil || peer.Device.Identifiers.Sn == nil {
		return ""
	}
	return *peer.Device.Identifiers.Sn
}

func peerIMEIs(peer apitypes.Peer) []apitypes.PeerIMEI {
	if peer.Device.Identifiers == nil || peer.Device.Identifiers.Imeis == nil {
		return nil
	}
	return *peer.Device.Identifiers.Imeis
}

func peerLabels(peer apitypes.Peer) []apitypes.PeerLabel {
	if peer.Device.Identifiers == nil || peer.Device.Identifiers.Labels == nil {
		return nil
	}
	return *peer.Device.Identifiers.Labels
}

func dedupeIMEIs(items []apitypes.PeerIMEI) []apitypes.PeerIMEI {
	seen := make(map[[2]string]struct{}, len(items))
	out := make([]apitypes.PeerIMEI, 0, len(items))
	for _, item := range items {
		if item.Tac == "" || item.Serial == "" {
			continue
		}
		key := [2]string{item.Tac, item.Serial}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Tac == out[j].Tac {
			return out[i].Serial < out[j].Serial
		}
		return out[i].Tac < out[j].Tac
	})
	return out
}

func dedupeLabels(items []apitypes.PeerLabel) []apitypes.PeerLabel {
	seen := make(map[[2]string]struct{}, len(items))
	out := make([]apitypes.PeerLabel, 0, len(items))
	for _, item := range items {
		if item.Key == "" || item.Value == "" {
			continue
		}
		key := [2]string{item.Key, item.Value}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key == out[j].Key {
			return out[i].Value < out[j].Value
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func peerKey(publicKey string) kv.Key {
	return kv.Key{"by-pubkey", publicKey}
}

func peersPrefix() kv.Key {
	return kv.Key{"by-pubkey"}
}

func snKey(sn string) kv.Key {
	return kv.Key{"by-sn", escapeIndexSegment(sn)}
}

func imeiKey(tac, serial string) kv.Key {
	return kv.Key{"by-imei", escapeIndexSegment(tac), escapeIndexSegment(serial)}
}

func labelPrefix(key, value string) kv.Key {
	return kv.Key{"by-label", escapeIndexSegment(key), escapeIndexSegment(value)}
}

func labelKey(item apitypes.PeerLabel, publicKey string) kv.Key {
	return append(labelPrefix(item.Key, item.Value), publicKey)
}

func rolePrefix(role apitypes.PeerRole) kv.Key {
	return kv.Key{"by-role", string(role)}
}

func roleKey(role apitypes.PeerRole, publicKey string) kv.Key {
	return append(rolePrefix(role), publicKey)
}

func statusPrefix(status apitypes.PeerRegistrationStatus) kv.Key {
	return kv.Key{"by-status", string(status)}
}

func statusKey(status apitypes.PeerRegistrationStatus, publicKey string) kv.Key {
	return append(statusPrefix(status), publicKey)
}

func indexEntries(peer apitypes.Peer) []kv.Entry {
	publicKey := peer.PublicKey
	entries := make([]kv.Entry, 0)
	if sn := peerSN(peer); sn != "" {
		entries = append(entries, kv.Entry{Key: snKey(sn), Value: []byte(publicKey)})
	}
	for _, item := range dedupeIMEIs(peerIMEIs(peer)) {
		entries = append(entries, kv.Entry{Key: imeiKey(item.Tac, item.Serial), Value: []byte(publicKey)})
	}
	for _, item := range dedupeLabels(peerLabels(peer)) {
		entries = append(entries, kv.Entry{Key: labelKey(item, publicKey), Value: []byte{1}})
	}
	if peer.Role != "" {
		entries = append(entries, kv.Entry{Key: roleKey(peer.Role, publicKey), Value: []byte{1}})
	}
	if peer.Status != "" {
		entries = append(entries, kv.Entry{Key: statusKey(peer.Status, publicKey), Value: []byte{1}})
	}
	return entries
}

func indexKeys(peer apitypes.Peer) []kv.Key {
	publicKey := peer.PublicKey
	keys := make([]kv.Key, 0, 2+len(peerIMEIs(peer))+len(peerLabels(peer)))
	if sn := peerSN(peer); sn != "" {
		keys = append(keys, snKey(sn))
	}
	for _, item := range dedupeIMEIs(peerIMEIs(peer)) {
		keys = append(keys, imeiKey(item.Tac, item.Serial))
	}
	for _, item := range dedupeLabels(peerLabels(peer)) {
		keys = append(keys, labelKey(item, publicKey))
	}
	if peer.Role != "" {
		keys = append(keys, roleKey(peer.Role, publicKey))
	}
	if peer.Status != "" {
		keys = append(keys, statusKey(peer.Status, publicKey))
	}
	return keys
}

func escapeIndexSegment(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	return strings.ReplaceAll(value, ":", "%3A")
}
