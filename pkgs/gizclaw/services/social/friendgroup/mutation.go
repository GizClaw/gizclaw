package friendgroup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/google/uuid"
)

// ErrGroupChanged reports a rejected stale snapshot. Read the group again before retrying.
var ErrGroupChanged = errors.New("social: friend group changed concurrently; retry")
var errGroupCreateUncertain = errors.New("social: group creation outcome could not be confirmed")

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
		return groupMutation{}, err
	}
	if len(state.revision) == 0 {
		return groupMutation{}, errors.New("social: empty group revision")
	}
	state.groupData, err = store.Get(ctx, state.groupKey)
	return state, err
}

func (state groupMutation) apply(ctx context.Context, mutation kv.Mutation, retire bool) (bool, error) {
	mutation.Conditions = append(mutation.Conditions,
		kv.Condition{Key: state.groupKey, Expected: state.groupData},
		kv.Condition{Key: state.revisionKey, Expected: state.revision})
	if retire {
		mutation.DeleteKeys = append(mutation.DeleteKeys, state.revisionKey)
	} else {
		mutation.Entries = append(mutation.Entries, kv.Entry{Key: state.revisionKey, Value: []byte(uuid.NewString())})
	}
	return state.store.ApplyMutation(ctx, mutation)
}

// createGroupWithOwner publishes a usable group in one shared transaction.
// No other Server can observe a group without its owner, binding or indexes.
func (s *Server) createGroupWithOwner(ctx context.Context, id string, group rpcapi.FriendGroupObject, binding workspaceBinding, owner, name string) error {
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
	admin := adminGroupMembership(id)
	admin.Key = s.relationshipKey(prefixes[0], admin.Key)
	mutation.AddOrderedMembers = []kv.SetMembers{admin}
	locators, err := socialutil.WorkspaceLocatorEntries(socialutil.WorkspaceBindingLocator{ResourceID: id, WorkspaceID: binding.WorkspaceID, WorkspaceName: binding.WorkspaceName})
	if err != nil {
		return err
	}
	for _, entry := range locators {
		entry.Key = s.relationshipKey(prefixes[3], entry.Key)
		mutation.Conditions = append(mutation.Conditions, kv.Condition{Key: entry.Key})
		mutation.Entries = append(mutation.Entries, entry)
	}
	created, err := store.ApplyMutation(ctx, mutation)
	if err != nil {
		// A lost write acknowledgement must not trigger deletion of a Workspace
		// that the now-published group is already using.
		stored, readErr := store.Get(ctx, bindingKey)
		if readErr == nil && bytes.Equal(stored, bindingData) {
			return nil
		}
		return errors.Join(errGroupCreateUncertain, err, readErr)
	}
	if !created {
		if _, err := store.Get(ctx, nameKey); err == nil {
			return errors.New("social: friend group name already exists")
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
		return errors.New("social: friend group name already exists")
	} else if !errors.Is(err, kv.ErrNotFound) {
		return err
	}
	return nil
}
