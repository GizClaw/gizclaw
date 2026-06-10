# Audio Output

Tracking issue: https://github.com/GizClaw/gizclaw-go/issues/21

This package is reserved for server-provided audio output APIs for gears.

Planned scope:

- `audio.say`
- Voice/model/credential ACL checks for TTS.
- Routing generated audio into peer mixer tracks.

This package should use the existing peer mixer path instead of creating a
separate audio transport.
