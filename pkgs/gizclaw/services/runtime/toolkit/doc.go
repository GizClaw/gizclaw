// Package toolkit defines the runtime Tool and ToolKit model used by agents.
//
// Tools are persisted configuration resources backed directly by pkgs/store/kv.
// A ToolKit is a per-agent runtime view filtered by ACL, enabled state,
// optional workspace policy, and executor availability.
package toolkit
