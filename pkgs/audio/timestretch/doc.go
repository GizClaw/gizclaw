// Package timestretch changes the duration of interleaved little-endian PCM16
// audio without changing its pitch.
//
// Stretcher implements streaming WSOLA (waveform similarity overlap-add):
// Write accepts PCM in any chunk size and returns the stretched audio that is
// final so far, and Flush returns the remainder. The output does not depend on
// how the input was chunked, and its length is the input length divided by the
// speed factor. It is intended for synthesized speech whose provider has no
// native speaking-rate control; providers with a native rate do not need it.
package timestretch
