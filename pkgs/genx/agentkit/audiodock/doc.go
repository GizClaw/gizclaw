// Package audiodock composes optional streaming ASR and TTS around a
// text-oriented GenX Transformer.
//
// The package owns only provider-neutral stream composition. Product resource
// lookup, model aliases, credentials, Workspaces, and provider protocols stay
// with callers and concrete Transformers.
package audiodock
