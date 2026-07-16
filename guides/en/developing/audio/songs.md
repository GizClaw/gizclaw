# pkgs/audio/songs

Stores built-in song, note, tempo, voice and metronome definitions, and render scores as PCM chunks.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/songs)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `Song` / `Note` / `Voice` | Express song metadata and multi-voice notes. |
| `Tempo` / `TimeSignature` / `Metronome` | Describe the beat and time structure. |
| `BeatNote` / `BeatVoice` / `N` | Construct a melody with beats. |
| `All` / `ByID` / `ByName` | Index built-in songs. |
| `RenderOptions` / `DefaultRenderOptions` | Configure PCM render. |
| `VoiceToChunk` | Render voice to the specified PCM format. |

The Songs package has built-in musical notation and synthesis logic, but does not own playback devices, user playlists, or product resource storage.
