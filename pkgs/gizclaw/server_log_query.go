package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

const (
	defaultServerLogStreamLimit = 100
	maxServerLogStreamLimit     = 1000
)

type ServerLogOrder string

const (
	ServerLogOrderAsc  ServerLogOrder = "asc"
	ServerLogOrderDesc ServerLogOrder = "desc"
)

type ServerLogStreamRequest struct {
	Filter       string
	FilterSet    bool
	StartTimeMs  int64
	StartTimeSet bool
	EndTimeMs    int64
	EndTimeSet   bool
	Limit        int
	Order        ServerLogOrder
	OrderSet     bool
	Cursor       string
}

type ServerLogQueryService interface {
	StreamServerLogs(ctx context.Context, req ServerLogStreamRequest, emit func(apitypes.ServerLogEntry) error) (apitypes.ServerLogStreamEnd, error)
}

type ServerLogQueryError struct {
	StatusCode int
	Code       string
	Message    string
	Err        error
}

func (e *ServerLogQueryError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *ServerLogQueryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func InvalidServerLogQuery(code, message string) *ServerLogQueryError {
	return &ServerLogQueryError{StatusCode: http.StatusBadRequest, Code: code, Message: message}
}

func LogQueryNotConfigured() *ServerLogQueryError {
	return &ServerLogQueryError{StatusCode: http.StatusNotImplemented, Code: "LOG_QUERY_NOT_CONFIGURED", Message: "server log query backend is not configured"}
}

func ServerLogBackendError(err error) *ServerLogQueryError {
	if err == nil {
		return nil
	}
	return &ServerLogQueryError{StatusCode: http.StatusBadGateway, Code: "LOG_QUERY_BACKEND_ERROR", Message: "server log query backend failed", Err: err}
}

func serverLogQueryErrorResponse(err error) (int, apitypes.ErrorResponse) {
	var queryErr *ServerLogQueryError
	if errors.As(err, &queryErr) {
		return queryErr.StatusCode, apitypes.NewErrorResponse(queryErr.Code, queryErr.Message)
	}
	return http.StatusBadGateway, apitypes.NewErrorResponse("LOG_QUERY_BACKEND_ERROR", "server log query backend failed")
}
