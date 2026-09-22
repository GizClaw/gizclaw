# pkgs/audio/resampler

Streaming signed PCM16 little-endian sample-rate and mono/stereo conversion, implemented as a pure Go polyphase FIR without cgo or libsoxr.

[Go API References](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/audio/resampler)

## Core types and functions

| Symbol | Purpose |
| --- | --- |
| `Format` | Input/output sample rate and mono/stereo layout. |
| `Resampler` | Streaming conversion and close contract. |
| `Soxr` | Pure Go implementation; the type name is retained for source compatibility. |
| `New` | Creates a converter from a source reader and the two formats. |

This package converts PCM representation. It does not decode compressed audio or choose device/network formats. Reuse a converter for a continuous stream, create a new instance when the input format changes, and call `Close` when finished. Concurrent `Read` calls on one instance are unsupported.

## Sampling and boundaries

- Equal sample rates pass PCM through, with channel conversion when needed. Stereo downmix averages the two channels; mono upmix duplicates samples. Resampling processes channels independently.
- Rate conversion uses a Kaiser-windowed sinc (beta 9) with cutoff at 94% of the lower Nyquist frequency. Filter length is `2 * ceil(48 * max(1, inputRate/outputRate)) + 1`. Conversion from 24 to 16 kHz uses two active phases and 145 multiply-accumulates per output sample.
- An integer phase clock avoids accumulated timing drift. Small denominators use exact phases; large denominators interpolate adjacent precomputed phases. The bank has at most 256 intervals and 262144 coefficients to bound memory use.
- The filter needs half a window of future samples: 3 ms for 24→16 kHz. Stream boundaries use zero extension. EOF drains remaining frames with filter delay removed; `N` input frames produce `ceil(N * outputRate/inputRate)` frames without an appended silent tail.
- Transport chunks may split PCM frames. An incomplete final frame returns `io.ErrUnexpectedEOF`. Output buffers must hold at least one complete destination frame.
- Sample rates must be in `[1, 2147483647]`, with output/input ratio in `[1/256, 256]`. Noninteger ratios need not come from a fixed list of sample rates.

## Validation

`go test ./pkgs/audio/resampler` covers frame counts, arbitrary chunk boundaries, channel isolation, frequency response, empty streams, EOF, and errors. Check the pure Go path with `CGO_ENABLED=0 go test ./pkgs/audio/resampler`.

`BenchmarkSpeech` measures complete speech streams, including construction, 60 ms output reads, and drain. Set `GIZCLAW_RESAMPLER_SPEECH_DIR` to a directory containing PCM16LE fixtures named `RATE-CHANNELS.pcm`. Prepare every rate/channel combination listed in the benchmark from the same speech excerpt with equal duration. Run `go test ./pkgs/audio/resampler -run '^$' -bench BenchmarkSpeech -benchtime=2s -count=5`. On a shared benchmark host, hold the common benchmark lock during measurement and record load averages and variation across repeated runs.
