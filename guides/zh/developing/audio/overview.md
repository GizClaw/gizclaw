# pkgs/audio 总览

`pkgs/audio` provides reusable audio codecs, PCM pipelines, device I/O, resampling, and music rendering. Product layers can combine these packages, while Peer, Agent, workspace, and transport lifecycles remain with their owners.

## Package 结构

```text
pkgs/audio/
├── codec/
│   ├── mp3/         # MP3 encode / decode
│   ├── ogg/         # Ogg pages, packets and streams
│   └── opus/        # Opus encode / decode
├── codecconv/       # PCM, Ogg and Opus conversion
├── pcm/             # PCM formats, chunks, tracks and mixer
├── portaudio/       # Native capture and playback
├── resampler/       # Sample-rate and channel conversion
├── songs/           # Song definitions and PCM rendering
└── timestretch/     # Pitch-preserving PCM16 time-stretch
```

## 数据流关系

```mermaid
flowchart LR
    Device["Audio device"] <--> PortAudio["portaudio"]
    PortAudio <--> PCM["pcm"]
    PCM --> Resampler["resampler"]
    PCM --> TimeStretch["timestretch"]
    PCM --> CodecConv["codecconv"]
    CodecConv --> Ogg["codec/ogg"]
    CodecConv --> Opus["codec/opus"]
    PCM --> MP3["codec/mp3"]
    Songs["songs"] --> PCM
```

## Package 导航

- Codec：[mp3](./codec-mp3)、[ogg](./codec-ogg)、[opus](./codec-opus)
- Conversion 与 PCM：[codecconv](./codecconv)、[pcm](./pcm)、[resampler](./resampler)、[timestretch](./timestretch)
- Device 与产品素材：[portaudio](./portaudio)、[songs](./songs)

Audio packages 只拥有格式、frame、sample、device stream 与 signal-processing contract。WebRTC track、Peer connection、Agent realtime stream 和产品资源分别属于 giznet、gizclaw runtime 与领域 services。
