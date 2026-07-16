# Stream Processing

Stream Processing holds Transformer composition capabilities that do not belong to a specific provider. General `Mux` is responsible for selecting the Adapter according to the pattern; the selected Adapter directly consumes the input `genx.Stream` and returns the output `genx.Stream`.

## Core structure and main function

| Structure or function | Function |
| --- | --- |
| [`TTSAudioNormalizer`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#TTSAudioNormalizer) | Unify the audio MIME type and chunk boundary of TTS output stream. |
| [`Mux`](https://pkg.go.dev/github.com/GizClaw/gizclaw-go@v0.0.0-20260707135347-b9bf1fb24b9f/pkgs/genx/transformers#Mux) | Select one `genx.Transformer` according to pattern and do not create a second set of ASR/TTS registry. |
| `runTTSTransform` | Public TTS pipeline inside Package; consumes text Stream, aggregates and splits text by StreamID, calls Adapter synthesize, and outputs audio Stream. |

`ASR` and `TTS` are capability categories and do not require additional export of facade, session or segment types. All Adapters are uniformly registered to the Transformer registry, and the caller uses the BOS, data, EOS and StreamID of `genx.Stream` to express continuous input and segmentation. Provider connection/session only exists as an internal implementation of Adapter.

## TTS Stream Processing

The public TTS pipeline consumes GenX text Stream and maintains sentence segmenters according to StreamID. During the input process, the complete sentence can be handed over to the Adapter for synthesis in advance; after receiving the EOS of the StreamID, the pipeline flushes the remaining text and outputs the corresponding audio EOS.

Text segmentation, audio normalization, and debug wrapper are common pipelines. Universal StreamID, BOS, EOS and Stream close contract are defined in [GenX Overview](../overview#streamid-and-eos); how ASR, Realtime and other Adapters map provider events is explained by each Adapter document.
