package flowcraft

import memorystore "github.com/GizClaw/gizclaw-go/pkgs/store/memory"

type (
	DeleteRequest = memorystore.DeleteRequest
	Fact          = memorystore.Fact
	Filter        = memorystore.Filter
	Observation   = memorystore.Observation
	Operation     = memorystore.Operation
	Query         = memorystore.Query
	Role          = memorystore.Role
	Scope         = memorystore.Scope
	Turn          = memorystore.Turn
	UpdateRequest = memorystore.UpdateRequest
)

const (
	FilterEqual        = memorystore.FilterEqual
	OperationPending   = memorystore.OperationPending
	OperationSucceeded = memorystore.OperationSucceeded
	RoleUser           = memorystore.RoleUser
	RoleAssistant      = memorystore.RoleAssistant
)

var (
	ErrConflict     = memorystore.ErrConflict
	ErrInvalidInput = memorystore.ErrInvalidInput
	ErrNotFound     = memorystore.ErrNotFound
	ErrUnavailable  = memorystore.ErrUnavailable
	ErrUnsupported  = memorystore.ErrUnsupported
)
