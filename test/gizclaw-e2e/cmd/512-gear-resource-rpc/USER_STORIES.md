# Gear Resource RPC

## Story

A registered gear can manage workspace, workflow, model, and credential resources through the Gear Service RPC surface when ACL bindings explicitly allow those resources. A different gear without matching bindings must be denied.

## Coverage

- Admin seeds existing resources before gear RPC reads them.
- Gear RPC lists and gets seeded resources.
- Gear RPC creates, updates, gets, lists, and deletes its own allowed resources.
- ACL denies access for an unbound gear.
