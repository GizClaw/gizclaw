// Package flowcraft adapts a Flowcraft graph runtime to a GenX Transformer.
//
// A Transformer is safe for concurrent Transform calls. Each call owns an
// ephemeral conversation context; turns inside that call are ordered, while
// separate calls share only the configured History, Memory, and State stores.
package flowcraft
