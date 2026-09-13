//go:build gizclaw_genx_e2e

package doubaoasr

import "context"

// ObserveSessionForE2E returns an isolated copy with local SDK boundary probes.
// Callbacks run synchronously and must not block. Open means the WebSocket dial
// and initial configuration write completed, not receipt of an ASR result.
// This diagnostic surface is excluded from normal production builds.
func (t *Transformer) ObserveSessionForE2E(opening, opened func(), sent func(int)) *Transformer {
	copy := *t
	copy.newSession = func(ctx context.Context, cfg doubaoASRSessionConfig) (doubaoASRSession, error) {
		opening()
		session, err := t.openSession(ctx, cfg)
		if err != nil {
			return nil, err
		}
		opened()
		return &measurementSession{doubaoASRSession: session, sent: sent}, nil
	}
	return &copy
}

type measurementSession struct {
	doubaoASRSession
	sent func(int)
}

func (s *measurementSession) SendAudio(ctx context.Context, data []byte, last bool) error {
	if err := s.doubaoASRSession.SendAudio(ctx, data, last); err != nil {
		return err
	}
	if len(data) > 0 {
		s.sent(len(data))
	}
	return nil
}
