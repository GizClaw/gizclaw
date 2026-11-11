package friendgroup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/google/uuid"
)

// ErrGroupChanged reports a rejected stale snapshot. Read the group again before retrying.
var ErrGroupChanged = errors.New("social: friend group changed concurrently; retry")
var errGroupCreateUncertain = errors.New("social: group creation outcome could not be confirmed")

// retryGroupMutation retries only a definitively rejected snapshot. The
// attempt must release its keyed locks before returning, so the primary attempt
// repeats permission checks and acquires the current participant locks.
func retryGroupMutation[T any](ctx context.Context, attempt func(context.Context) (T, error)) (T, error) {
	result, err := attempt(ctx)
	if !errors.Is(err, ErrGroupChanged) {
		return result, err
	}
	return attempt(kv.WithPrimaryReads(ctx))
}

// retryNamedGroupMutation resolves a name inside the retry boundary and pins the
// first canonical ID it observes. A missing reader index gets one authority
// attempt, while later conflicts cannot follow a reassigned name.
func retryNamedGroupMutation[T any](ctx context.Context, resolve func(context.Context) (string, error), attempt func(context.Context, string) (T, error)) (T, error) {
	var id string
	return retryGroupMutation(ctx, func(ctx context.Context) (T, error) {
		if id == "" {
			resolved, err := resolve(ctx)
			if err != nil {
				var zero T
				return zero, groupMutationReadError(ctx, err)
			}
			id = resolved
		}
		return attempt(ctx, id)
	})
}

// groupMutationReadError marks a reader's missing precommit observation for
// bounded recovery. A primary miss keeps its original not-found classification.
// Do not use it on writes or postcommit projections: an uncertain write
// acknowledgement must never cause a semantic replay.
func groupMutationReadError(ctx context.Context, err error) error {
	if errors.Is(err, kv.ErrNotFound) && kv.WithPrimaryReads(ctx) != ctx {
		return errors.Join(ErrGroupChanged, err)
	}
	return err
}

func groupRevisionKey(id string) kv.Key {
	return kv.Key{"group-revisions", socialutil.EscapeStoreSegment(id)}
}

// groupMutation holds a shared version captured before reading related rows.
// Every membership mutation advances it, so a capacity check or retirement
// snapshot cannot commit after another Server changes the group.
type groupMutation struct {
	store                 kv.Store
	prefixes              []kv.Key
	groupPrefix           kv.Key
	groupKey, revisionKey kv.Key
	groupData, revision   []byte
}

func (s *Server) readGroupMutation(ctx context.Context, id string, participants ...kv.Store) (groupMutation, error) {
	groups, err := s.groupsStore()
	if err != nil {
		return groupMutation{}, err
	}
	stores := append([]kv.Store{groups}, participants...)
	store, prefixes, ok := kv.SharedAtomicStore(stores...)
	if !ok {
		return groupMutation{}, errors.New("social: group stores do not share an atomic store")
	}
	state := groupMutation{store: store, prefixes: prefixes[1:],
		groupPrefix: prefixes[0],
		groupKey:    s.relationshipKey(prefixes[0], socialutil.GroupKey(id)),
		revisionKey: s.relationshipKey(prefixes[0], groupRevisionKey(id)),
	}
	state.revision, err = store.Get(ctx, state.revisionKey)
	if err != nil {
		return groupMutation{}, groupMutationReadError(ctx, err)
	}
	if len(state.revision) == 0 {
		return groupMutation{}, errors.New("social: empty group revision")
	}
	state.groupData, err = store.Get(ctx, state.groupKey)
	if errors.Is(err, kv.ErrNotFound) {
		// A revision without its atomically published group is a conflicted
		// read, not an ordinary miss.
		return groupMutation{}, errors.Join(ErrGroupChanged, err)
	}
	return state, err
}

func (state groupMutation) conditions() []kv.Condition {
	return []kv.Condition{{Key: state.groupKey, Expected: state.groupData}, {Key: state.revisionKey, Expected: state.revision}}
}

func (state groupMutation) peerRetirementConditions(ctx context.Context, belongPrefix kv.Key, peers ...string) ([]kv.Condition, error) {
	var group rpcapi.FriendGroupObject
	if err := json.Unmarshal(state.groupData, &group); err != nil {
		return nil, err
	}
	owner := socialutil.StringValue(group.CreatedByPeerPublicKey)
	if owner == "" {
		return nil, errors.New("social: Friend Group owner is missing")
	}
	conditions := make([]kv.Condition, 0, len(peers))
	seen := make(map[string]bool, len(peers))
	for _, peer := range append([]string{owner}, peers...) {
		if seen[peer] {
			continue
		}
		seen[peer] = true
		condition := socialutil.PeerRetirementCondition(peer)
		condition.Key = append(append(kv.Key{}, belongPrefix...), condition.Key...)
		if _, err := state.store.Get(ctx, condition.Key); err == nil {
			return nil, ownership.ErrOwnerRetired
		} else if !errors.Is(err, kv.ErrNotFound) {
			return nil, err
		}
		conditions = append(conditions, condition)
	}
	return conditions, nil
}

func (state groupMutation) apply(ctx context.Context, mutation kv.Mutation, retire bool) (bool, error) {
	mutation.Conditions = append(mutation.Conditions, state.conditions()...)
	if retire {
		mutation.DeleteKeys = append(mutation.DeleteKeys, state.revisionKey)
	} else {
		mutation.Entries = append(mutation.Entries, kv.Entry{Key: state.revisionKey, Value: []byte(uuid.NewString())})
	}
	return state.store.ApplyMutation(ctx, mutation)
}

// groupInviteMutationStore keeps invitation keys and serialized locators local
// to their namespace while applying Group guards at the shared atomic root.
type groupInviteMutationStore struct {
	kv.Store
	authority  kv.Store
	prefix     kv.Key
	conditions []kv.Condition
}

func (store groupInviteMutationStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	key := func(local kv.Key) kv.Key { return append(append(kv.Key{}, store.prefix...), local...) }
	root := kv.Mutation{Conditions: append([]kv.Condition(nil), store.conditions...)}
	for _, condition := range mutation.Conditions {
		condition.Key = key(condition.Key)
		root.Conditions = append(root.Conditions, condition)
	}
	for _, entry := range mutation.Entries {
		entry.Key = key(entry.Key)
		root.Entries = append(root.Entries, entry)
	}
	for _, local := range mutation.DeleteKeys {
		root.DeleteKeys = append(root.DeleteKeys, key(local))
	}
	for _, collection := range []struct {
		local []kv.SetMembers
		root  *[]kv.SetMembers
	}{
		{mutation.AddMembers, &root.AddMembers},
		{mutation.RemoveMembers, &root.RemoveMembers},
		{mutation.AddOrderedMembers, &root.AddOrderedMembers},
		{mutation.RemoveOrderedMembers, &root.RemoveOrderedMembers},
	} {
		for _, members := range collection.local {
			members.Key = key(members.Key)
			*collection.root = append(*collection.root, members)
		}
	}
	return store.authority.ApplyMutation(ctx, root)
}

// updateGroupInviteToken makes one guarded invitation attempt. The caller owns
// semantic retries, so a conflict refreshes Group permissions and recomputes the
// token from the current bytes instead of replaying a precomputed replacement.
func (s *Server) updateGroupInviteToken(ctx context.Context, owner, id string, update func([]byte) ([]byte, error)) ([]byte, error) {
	owner = strings.TrimSpace(owner)
	unlock, err := s.lockGroup(ctx, id)
	if err != nil {
		return nil, err
	}
	defer unlock()
	invites, err := s.groupInviteTokensStore()
	if err != nil {
		return nil, err
	}
	relationships, err := s.relationshipStore()
	if err != nil {
		return nil, err
	}
	belongs, err := s.belongsStore()
	if err != nil {
		return nil, err
	}
	members, err := s.membersStore()
	if err != nil {
		return nil, err
	}
	guard, err := s.readGroupMutation(ctx, id, invites, relationships, belongs, members)
	if err != nil {
		return nil, err
	}
	var group rpcapi.FriendGroupObject
	if err := json.Unmarshal(guard.groupData, &group); err != nil {
		return nil, err
	}
	conditions := guard.conditions()
	if owner != "" {
		key := s.relationshipKey(guard.prefixes[3], socialutil.GroupMemberKey(id, owner))
		data, err := guard.store.Get(ctx, key)
		if err != nil {
			return nil, groupMutationReadError(ctx, err)
		}
		var member friendGroupMemberRecord
		if err := json.Unmarshal(data, &member); err != nil {
			return nil, err
		}
		if err := member.validate(); err != nil {
			return nil, err
		}
		if member.FriendGroupID != id || member.PeerPublicKey != owner || member.Role != rpcapi.FriendGroupMemberRoleOwner {
			return nil, ErrFriendGroupPermissionDenied
		}
		conditions = append(conditions, kv.Condition{Key: key, Expected: data})
	}
	lockedCtx, releasePeers, err := s.lockGroupPeers(ctx, group)
	if err != nil {
		return nil, err
	}
	defer releasePeers()
	ctx = lockedCtx
	fences, err := guard.peerRetirementConditions(ctx, guard.prefixes[2])
	if err != nil {
		return nil, err
	}
	conditions = append(conditions, fences...)
	pending := pendingdeletion.AbsentLocatorCondition(pendingdeletion.KindFriendGroup, id)
	pending.Key = s.relationshipKey(guard.prefixes[1], pending.Key)
	conditions = append(conditions, pending,
		kv.Condition{Key: s.relationshipKey(guard.prefixes[1], groupRetirementIntentKey(id))},
		kv.Condition{Key: s.relationshipKey(guard.prefixes[1], groupRetirementReceiptKey(id))})
	store := groupInviteMutationStore{Store: kv.Prefixed(guard.store, guard.prefixes[0]),
		authority: guard.store, prefix: guard.prefixes[0], conditions: conditions}
	data, err := socialutil.UpdateInviteToken(ctx, store, socialutil.GroupInviteTokenKey(id), nil, update)
	if errors.Is(err, socialutil.ErrInviteTokenChanged) {
		return nil, ErrGroupChanged
	}
	return data, err
}

// proveGroupJoin acknowledges an existing membership only while its exact
// lifecycle, member record, projections and invitation are still authoritative.
// A rejected proof goes through the same bounded retry as a new admission.
func (s *Server) proveGroupJoin(ctx context.Context, id, peer, name string, invitation []kv.Condition) (rpcapi.FriendGroupJoinResponse, error) {
	members, err := s.membersStore()
	if err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	belongs, err := s.belongsStore()
	if err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	invites, err := s.groupInviteTokensStore()
	if err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	guard, err := s.readGroupMutation(ctx, id, members, belongs, invites)
	if err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	memberKey := s.relationshipKey(guard.prefixes[0], socialutil.GroupMemberKey(id, peer))
	data, err := guard.store.Get(ctx, memberKey)
	if err != nil {
		return rpcapi.FriendGroupJoinResponse{}, groupMutationReadError(ctx, err)
	}
	var member friendGroupMemberRecord
	if err := json.Unmarshal(data, &member); err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	if err := member.validate(); err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	if member.FriendGroupID != id || member.PeerPublicKey != peer {
		return rpcapi.FriendGroupJoinResponse{}, errors.New("social: Group join membership identity mismatch")
	}
	conditions, err := guard.peerRetirementConditions(ctx, guard.prefixes[1], peer)
	if err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	conditions = append(conditions, guard.conditions()...)
	conditions = append(conditions,
		kv.Condition{Key: memberKey, Expected: data},
		kv.Condition{Key: s.relationshipKey(guard.prefixes[1], socialutil.GroupBelongKey(peer, id)), Expected: data},
		kv.Condition{Key: s.relationshipKey(guard.prefixes[1], socialutil.GroupNameKey(peer, member.FriendGroupName)), Expected: []byte(id)})
	for _, condition := range invitation {
		condition.Key = s.relationshipKey(guard.prefixes[2], condition.Key)
		conditions = append(conditions, condition)
	}
	proved, err := guard.store.ApplyMutation(ctx, kv.Mutation{Conditions: conditions})
	if err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	if !proved {
		return rpcapi.FriendGroupJoinResponse{}, ErrGroupChanged
	}
	if member.FriendGroupName != name {
		return rpcapi.FriendGroupJoinResponse{}, ErrFriendGroupMembershipNameImmutable
	}
	var group rpcapi.FriendGroupObject
	if err := json.Unmarshal(guard.groupData, &group); err != nil {
		return rpcapi.FriendGroupJoinResponse{}, err
	}
	group.Name, group.MyRole = member.FriendGroupName, new(member.Role)
	return rpcapi.FriendGroupJoinResponse{Group: group, Member: member.peerObject()}, nil
}

func rejectPeerGroupRetirement(ctx context.Context, belongs kv.Store, peerID string) error {
	_, err := belongs.Get(ctx, socialutil.PeerRetirementFenceKey(peerID))
	if err == nil {
		return ownership.ErrOwnerRetired
	}
	if errors.Is(err, kv.ErrNotFound) {
		return nil
	}
	return err
}

func validatePeerRetirementGroup(snapshot PeerRetirementGroup, id string, group rpcapi.FriendGroupObject, binding workspaceBinding) error {
	if snapshot.FriendGroupID != id || socialutil.StringValue(group.CreatedByPeerPublicKey) != snapshot.OwnerPublicKey ||
		socialutil.StringValue(group.WorkspaceName) != snapshot.WorkspaceName || binding.FriendGroupID != id ||
		binding.Owner != snapshot.OwnerPublicKey || binding.WorkspaceID != snapshot.WorkspaceID || binding.WorkspaceName != snapshot.WorkspaceName ||
		(snapshot.OwnerPublicKey == snapshot.PeerPublicKey && group.Name != snapshot.FriendGroupName) {
		return errors.New("social: Friend Group no longer matches Peer retirement snapshot")
	}
	return nil
}

func validatePeerRetirementMember(snapshot PeerRetirementGroup, member friendGroupMemberRecord, active bool) error {
	if member.FriendGroupID != snapshot.FriendGroupID || member.PeerPublicKey != snapshot.PeerPublicKey ||
		member.FriendGroupName != snapshot.FriendGroupName || member.Role != snapshot.Role ||
		(snapshot.CreatedAt != nil && !snapshot.CreatedAt.Equal(member.CreatedAt)) {
		return errors.New("social: Friend Group membership no longer matches Peer retirement snapshot")
	}
	if active && (snapshot.CreatedAt == nil || snapshot.CreatedAt.IsZero()) {
		return errors.New("social: Peer retirement snapshot lacks membership incarnation")
	}
	return nil
}

// peerRetirementMutationConditions checks the complete captured identity against
// the records used by this mutation, then guards the Workspace binding as well.
func (s *Server) peerRetirementMutationConditions(ctx context.Context, state groupMutation, id string, relationshipPrefix kv.Key, member *friendGroupMemberRecord) ([]kv.Condition, error) {
	snapshot, ok := peerRetirementFromContext(ctx)
	if !ok {
		return nil, nil
	}
	var group rpcapi.FriendGroupObject
	if err := json.Unmarshal(state.groupData, &group); err != nil {
		return nil, err
	}
	bindingKey := s.relationshipKey(relationshipPrefix, workspaceBindingKey(id))
	bindingData, err := state.store.Get(ctx, bindingKey)
	if errors.Is(err, kv.ErrNotFound) {
		return nil, errors.Join(ErrGroupChanged, err)
	}
	if err != nil {
		return nil, err
	}
	var binding workspaceBinding
	if err := json.Unmarshal(bindingData, &binding); err != nil {
		return nil, err
	}
	if err := validateWorkspaceBinding(binding, id); err != nil {
		return nil, err
	}
	if err := validatePeerRetirementGroup(snapshot, id, group, binding); err != nil {
		return nil, err
	}
	if member != nil {
		if err := validatePeerRetirementMember(snapshot, *member, true); err != nil {
			return nil, err
		}
	}
	return []kv.Condition{{Key: bindingKey, Expected: bindingData}}, nil
}

func validatePeerRetirementIntent(ctx context.Context, intent retirementIntent) error {
	snapshot, ok := peerRetirementFromContext(ctx)
	if !ok {
		return nil
	}
	binding := workspaceBinding{FriendGroupID: intent.FriendGroupID, WorkspaceID: intent.WorkspaceID,
		WorkspaceName: intent.WorkspaceName, Owner: socialutil.StringValue(intent.FriendGroup.CreatedByPeerPublicKey)}
	if err := validatePeerRetirementGroup(snapshot, intent.FriendGroupID, intent.FriendGroup, binding); err != nil {
		return err
	}
	for _, member := range intent.Members {
		if member.PeerPublicKey == snapshot.PeerPublicKey {
			return validatePeerRetirementMember(snapshot, member, false)
		}
	}
	return errors.New("social: Friend Group retirement proof is missing captured membership")
}

func (s *Server) provePeerRetirementMemberAbsent(ctx context.Context, id, peer string) error {
	snapshot, ok := peerRetirementFromContext(ctx)
	if !ok || snapshot.FriendGroupID != id || snapshot.PeerPublicKey != peer || snapshot.OwnerPublicKey == peer {
		return errors.New("social: invalid foreign membership retirement proof")
	}
	members, err := s.membersStore()
	if err != nil {
		return err
	}
	relationships, err := s.relationshipStore()
	if err != nil {
		return err
	}
	belongs, err := s.belongsStore()
	if err != nil {
		return err
	}
	guard, err := s.readGroupMutation(ctx, id, members, relationships, belongs)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return errors.Join(ErrGroupChanged, err)
		}
		return err
	}
	conditions, err := s.peerRetirementMutationConditions(ctx, guard, id, guard.prefixes[1], nil)
	if err != nil {
		return err
	}
	belongKey := s.relationshipKey(guard.prefixes[2], socialutil.GroupBelongKey(peer, id))
	if _, err := guard.store.Get(ctx, belongKey); err == nil {
		return ErrGroupChanged
	} else if !errors.Is(err, kv.ErrNotFound) {
		return err
	}
	nameKey := s.relationshipKey(guard.prefixes[2], socialutil.GroupNameKey(peer, snapshot.FriendGroupName))
	nameData, err := guard.store.Get(ctx, nameKey)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return err
	}
	if string(nameData) == id {
		return ErrGroupChanged
	}
	for _, item := range []struct {
		key    kv.Key
		member string
	}{
		{s.relationshipKey(guard.prefixes[0], memberCollectionKey(id)), peer},
		{s.relationshipKey(guard.prefixes[2], belongCollectionKey(peer)), id},
	} {
		present, err := guard.store.HasMember(ctx, item.key, item.member)
		if err != nil {
			return err
		}
		if present {
			return ErrGroupChanged
		}
	}
	conditions = append(conditions, guard.conditions()...)
	conditions = append(conditions,
		kv.Condition{Key: s.relationshipKey(guard.prefixes[0], socialutil.GroupMemberKey(id, peer))},
		kv.Condition{Key: belongKey}, kv.Condition{Key: nameKey, Expected: nameData})
	absent, err := guard.store.ApplyMutation(ctx, kv.Mutation{Conditions: conditions})
	if err != nil {
		return err
	}
	if !absent {
		return ErrGroupChanged
	}
	return nil
}

// createGroupWithOwner publishes a usable group in one shared transaction.
// No other Server can observe a group without its owner, binding or indexes.
func (s *Server) createGroupWithOwner(ctx context.Context, id string, group rpcapi.FriendGroupObject, binding workspaceBinding, owner, name string) error {
	_, err := retryGroupMutation(ctx, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, s.createGroupWithOwnerAttempt(ctx, id, group, binding, owner, name)
	})
	return err
}

func (s *Server) createGroupWithOwnerAttempt(ctx context.Context, id string, group rpcapi.FriendGroupObject, binding workspaceBinding, owner, name string) error {
	store, prefixes, err := s.groupAtomicStore()
	if err != nil {
		return err
	}
	groupData, err := json.Marshal(group)
	if err != nil {
		return err
	}
	if err := validateWorkspaceBinding(binding, id); err != nil {
		return err
	}
	bindingData, err := json.Marshal(binding)
	if err != nil {
		return err
	}
	peerRevision, err := s.readPeerGroupCapacity(ctx, owner)
	if err != nil {
		return err
	}
	now := s.now()
	member := friendGroupMemberRecord{FriendGroupID: id, PeerPublicKey: owner, FriendGroupName: name,
		Role: rpcapi.FriendGroupMemberRoleOwner, CreatedAt: now, UpdatedAt: now}
	memberData, err := json.Marshal(member)
	if err != nil {
		return err
	}
	groupKey := s.relationshipKey(prefixes[0], socialutil.GroupKey(id))
	revisionKey := s.relationshipKey(prefixes[0], groupRevisionKey(id))
	memberKey := s.relationshipKey(prefixes[1], socialutil.GroupMemberKey(id, owner))
	nameKey := s.relationshipKey(prefixes[2], socialutil.GroupNameKey(owner, name))
	bindingKey := s.relationshipKey(prefixes[3], workspaceBindingKey(id))
	mutation := kv.Mutation{
		Conditions: []kv.Condition{{Key: groupKey}, {Key: revisionKey}, {Key: memberKey}, {Key: nameKey}, {Key: bindingKey}},
		Entries: []kv.Entry{{Key: groupKey, Value: groupData}, {Key: revisionKey, Value: []byte(uuid.NewString())},
			{Key: memberKey, Value: memberData}, {Key: nameKey, Value: []byte(id)},
			{Key: s.relationshipKey(prefixes[2], socialutil.GroupBelongKey(owner, id)), Value: memberData},
			{Key: bindingKey, Value: bindingData}},
		AddMembers: []kv.SetMembers{
			{Key: s.relationshipKey(prefixes[1], memberCollectionKey(id)), Members: []string{owner}},
			{Key: s.relationshipKey(prefixes[2], belongCollectionKey(owner)), Members: []string{id}},
		},
	}
	// A stale negative deletion check must not reuse this lifecycle's ID.
	// These fences share the group creation's atomic transaction boundary.
	fence := pendingdeletion.AbsentLocatorCondition(pendingdeletion.KindFriendGroup, id)
	fence.Key = s.relationshipKey(prefixes[3], fence.Key)
	mutation.Conditions = append(mutation.Conditions, fence,
		kv.Condition{Key: s.relationshipKey(prefixes[3], groupRetirementIntentKey(id))},
		kv.Condition{Key: s.relationshipKey(prefixes[3], groupRetirementReceiptKey(id))})
	admin := adminGroupMembership(id)
	admin.Key = s.relationshipKey(prefixes[0], admin.Key)
	mutation.AddOrderedMembers = []kv.SetMembers{admin, {Key: s.relationshipKey(prefixes[2], belongPageKey(owner)), Members: []string{socialutil.EscapeStoreSegment(id)}}}
	locators, err := socialutil.WorkspaceLocatorEntries(socialutil.WorkspaceBindingLocator{ResourceID: id, WorkspaceID: binding.WorkspaceID, WorkspaceName: binding.WorkspaceName})
	if err != nil {
		return err
	}
	for _, entry := range locators {
		entry.Key = s.relationshipKey(prefixes[3], entry.Key)
		mutation.Conditions = append(mutation.Conditions, kv.Condition{Key: entry.Key})
		mutation.Entries = append(mutation.Entries, entry)
	}
	peerRevisionKey := s.admitPeerToGroup(&mutation, prefixes[2], owner, peerRevision)
	created, err := store.ApplyMutation(ctx, mutation)
	if err != nil {
		// A lost write acknowledgement must not trigger deletion of a Workspace
		// that the now-published group is already using.
		stored, readErr := store.Get(kv.WithPrimaryReads(ctx), bindingKey)
		if readErr == nil && bytes.Equal(stored, bindingData) {
			return nil
		}
		return errors.Join(errGroupCreateUncertain, err, readErr)
	}
	if !created {
		ctx = kv.WithPrimaryReads(ctx)
		if err := rejectPeerGroupRetirement(ctx, kv.Prefixed(store, prefixes[2]), owner); err != nil {
			return err
		}
		if err := s.rejectDataPendingDeletion(ctx, id); err != nil {
			return err
		}
		for _, key := range []kv.Key{groupRetirementIntentKey(id), groupRetirementReceiptKey(id)} {
			if _, err := store.Get(ctx, s.relationshipKey(prefixes[3], key)); err == nil {
				return ErrFriendGroupPendingDeletion
			} else if !errors.Is(err, kv.ErrNotFound) {
				return err
			}
		}
		if changed, err := peerGroupRevisionChanged(ctx, store, peerRevisionKey, peerRevision); err != nil {
			return err
		} else if changed {
			return ErrGroupChanged
		}
		if _, err := store.Get(ctx, nameKey); err == nil {
			return ErrFriendGroupNameExists
		} else if !errors.Is(err, kv.ErrNotFound) {
			return err
		}
		return socialutil.ErrResourceAlreadyExists
	}
	return nil
}

func (s *Server) groupAtomicStore() (kv.Store, []kv.Key, error) {
	groups, err := s.groupsStore()
	if err != nil {
		return nil, nil, err
	}
	members, err := s.membersStore()
	if err != nil {
		return nil, nil, err
	}
	belongs, err := s.belongsStore()
	if err != nil {
		return nil, nil, err
	}
	relationships, err := s.relationshipStore()
	if err != nil {
		return nil, nil, err
	}
	store, prefixes, ok := kv.SharedAtomicStore(groups, members, belongs, relationships)
	if !ok {
		return nil, nil, errors.New("social: group stores do not share an atomic store")
	}
	return store, prefixes, nil
}

func (s *Server) checkGroupCreate(ctx context.Context, id, owner, name string) error {
	store, prefixes, err := s.groupAtomicStore()
	if err != nil {
		return err
	}
	if _, err := store.Get(ctx, s.relationshipKey(prefixes[0], socialutil.GroupKey(id))); err == nil {
		return socialutil.ErrResourceAlreadyExists
	} else if !errors.Is(err, kv.ErrNotFound) {
		return err
	}
	if _, err := store.Get(ctx, s.relationshipKey(prefixes[2], socialutil.GroupNameKey(owner, name))); err == nil {
		return ErrFriendGroupNameExists
	} else if !errors.Is(err, kv.ErrNotFound) {
		return err
	}
	// Reject before the caller creates the Workspace; createGroupWithOwner
	// repeats the check under the Peer revision when it commits.
	_, err = s.readPeerGroupCapacity(ctx, owner)
	return err
}
