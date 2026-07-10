package toolkit

import "errors"

var (
	ErrNotConfigured       = errors.New("toolkit: not configured")
	ErrInvalidTool         = errors.New("toolkit: invalid tool")
	ErrToolNotFound        = errors.New("toolkit: tool not found")
	ErrExecutorNotFound    = errors.New("toolkit: executor not found")
	ErrExecutorUnavailable = errors.New("toolkit: executor unavailable")
	ErrDuplicateToolName   = errors.New("toolkit: duplicate effective tool name")
)
