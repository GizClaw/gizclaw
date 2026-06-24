# Play UI Story Sets

Play UI e2e tests are browser tests against the setup-started Play UI.

Each child directory maps to a visible Play UI page or major surface and owns its own `USER_STORIES.md` plus `_test.go` files.

- `openai-gateway/`: gateway shell, resource navigation, and the OpenAI drawer.
- `workspace-drawer/`: active workspace drawer, voice controls, history, memory, and recall.
- `workspaces/`: workspace list section.
- `social/`: Friends, Groups, invite-token flows, and the social Chat drawer.
