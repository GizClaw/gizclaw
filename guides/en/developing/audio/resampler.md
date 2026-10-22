# pkgs/audio/resampler

[Source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/audio/resampler)

Streaming signed PCM16 little-endian sample-rate and mono/stereo conversion, implemented as a pure Go polyphase FIR without cgo or libsoxr.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go/pkgs/audio/resampler)

This package converts PCM representation. It does not decode compressed audio or choose device/network formats. Reuse a converter for a continuous stream, create a new instance when the input format changes, and call `Close` when finished. Concurrent `Read` calls on one instance are unsupported.

## Sampling and boundaries

Reuse a converter across a continuous PCM stream, recreate it when the input format changes, and close it after consumption. Sample-rate and channel conversion can be combined; callers still choose the target format and handle truncated input or insufficient output buffers.

See the [resampler source](https://github.com/GizClaw/gizclaw/tree/main/pkgs/audio/resampler) for filters, output-frame calculations, and supported ranges.

## Validation

`go test ./pkgs/audio/resampler` covers frame counts, arbitrary chunk boundaries, channel isolation, frequency response, empty streams, EOF, and errors. Check the pure Go path with `CGO_ENABLED=0 go test ./pkgs/audio/resampler`.

`BenchmarkSpeech` measures complete speech streams, including construction, 60 ms output reads, and drain. Set `GIZCLAW_RESAMPLER_SPEECH_DIR` to a directory containing PCM16LE fixtures named `RATE-CHANNELS.pcm`. Prepare every rate/channel combination listed in the benchmark from the same speech excerpt with equal duration. Run `go test ./pkgs/audio/resampler -run '^$' -bench BenchmarkSpeech -benchtime=2s -count=5`. On a shared benchmark host, hold the common benchmark lock during measurement and record load averages and variation across repeated runs.
