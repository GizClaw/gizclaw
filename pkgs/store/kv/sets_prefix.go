package kv

import "context"

func (s *prefixedStore) AddMembers(ctx context.Context, key Key, members ...string) error {
	return s.base.AddMembers(ctx, s.prefixedKey(key), members...)
}
func (s *prefixedStore) RemoveMembers(ctx context.Context, key Key, members ...string) error {
	return s.base.RemoveMembers(ctx, s.prefixedKey(key), members...)
}
func (s *prefixedStore) HasMember(ctx context.Context, key Key, member string) (bool, error) {
	return s.base.HasMember(ctx, s.prefixedKey(key), member)
}
func (s *prefixedStore) ListMembers(ctx context.Context, key Key) ([]string, error) {
	return s.base.ListMembers(ctx, s.prefixedKey(key))
}
func (s *prefixedStore) ApplyMutation(ctx context.Context, mutation Mutation) (bool, error) {
	scoped := Mutation{}
	for _, condition := range mutation.Conditions {
		scoped.Conditions = append(scoped.Conditions, Condition{Key: s.prefixedKey(condition.Key), Expected: condition.Expected})
	}
	for _, entry := range mutation.Entries {
		entry.Key = s.prefixedKey(entry.Key)
		scoped.Entries = append(scoped.Entries, entry)
	}
	for _, key := range mutation.DeleteKeys {
		scoped.DeleteKeys = append(scoped.DeleteKeys, s.prefixedKey(key))
	}
	for _, group := range mutation.AddMembers {
		group.Key = s.prefixedKey(group.Key)
		scoped.AddMembers = append(scoped.AddMembers, group)
	}
	for _, group := range mutation.RemoveMembers {
		group.Key = s.prefixedKey(group.Key)
		scoped.RemoveMembers = append(scoped.RemoveMembers, group)
	}
	for _, group := range mutation.AddOrderedMembers {
		group.Key = s.prefixedKey(group.Key)
		scoped.AddOrderedMembers = append(scoped.AddOrderedMembers, group)
	}
	for _, group := range mutation.RemoveOrderedMembers {
		group.Key = s.prefixedKey(group.Key)
		scoped.RemoveOrderedMembers = append(scoped.RemoveOrderedMembers, group)
	}
	return s.base.ApplyMutation(ctx, scoped)
}
