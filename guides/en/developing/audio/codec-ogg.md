# pkgs/audio/codec/ogg

Implements page, packet and stream framing of Ogg container, does not explain packet internal use of Opus or other codecs.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/codec/ogg)

## Core structure and main function

| Symbol | Function |
| --- | --- |
| `Page` / `ParsePage` / `ParsePages` | Parse Ogg pages. |
| `Packet` / `ExtractPackets` | Reassemble packets from pages. |
| `BuildPacketPages` / `MarshalPages` | Paging and encoding the packet. |
| `StreamReader` / `NewStreamReader` | Incrementally read Ogg stream. |
| `StreamWriter` / `NewStreamWriter` | Manage serial, sequence and page output. |
| `Packets` | Read packets using iterator. |

Ogg package has container framing, checksum and page sequencing; Opus header and PCM conversion belong to `codecconv` and `codec/opus`.
