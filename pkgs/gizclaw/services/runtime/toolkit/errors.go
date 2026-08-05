package toolkit

import "errors"

var (
	ErrNotConfigured = errors.New("toolkit: not configured")
	ErrInvalidTool   = errors.New("toolkit: invalid tool")
	ErrToolConflict  = errors.New("toolkit: tool conflict")
	ErrToolNotFound  = errors.New("toolkit: tool not found")
)
