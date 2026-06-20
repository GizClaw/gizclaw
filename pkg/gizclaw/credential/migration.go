package credential

import (
	"context"
)

// Migration is currently reserved for future credential storage migrations.
func (s *Server) Migration(ctx context.Context) error {
	_ = ctx
	return nil
}
