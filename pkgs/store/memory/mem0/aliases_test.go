package mem0

import memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"

type (
	DeleteRequest = memorystore.DeleteRequest
	Filter        = memorystore.Filter
	Observation   = memorystore.Observation
	Query         = memorystore.Query
	Scope         = memorystore.Scope
	UpdateRequest = memorystore.UpdateRequest
)

const (
	FilterEqual        = memorystore.FilterEqual
	OperationSucceeded = memorystore.OperationSucceeded
)

var (
	ErrInvalidInput = memorystore.ErrInvalidInput
	ErrUnavailable  = memorystore.ErrUnavailable
	ErrUnsupported  = memorystore.ErrUnsupported
)
