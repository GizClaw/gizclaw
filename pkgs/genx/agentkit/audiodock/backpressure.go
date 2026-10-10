package audiodock

import "github.com/GizClaw/gizclaw-go/pkgs/genx"

const maxPendingSpeechBytes = 4 << 20

type ttsDeliveryStream interface {
	DeferOutputObservation()
	ObserveOutput(*genx.MessageChunk)
	AbandonOutputObservation(*genx.MessageChunk)
}
