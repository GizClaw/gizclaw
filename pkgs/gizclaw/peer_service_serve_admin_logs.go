package gizclaw

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type serverLogEvent struct {
	name string
	data any
}

type streamServerLogsResponse struct {
	ctx     context.Context
	service ServerLogQueryService
	request ServerLogStreamRequest
}

func (s *adminService) StreamServerLogs(ctx context.Context, request adminhttp.StreamServerLogsRequestObject) (adminhttp.StreamServerLogsResponseObject, error) {
	streamReq, err := serverLogStreamRequestFromParams(request.Params)
	if err != nil {
		return adminhttp.StreamServerLogs400JSONResponse(apitypes.NewErrorResponse("INVALID_LOG_QUERY", err.Error())), nil
	}
	if s.ServerLogs == nil {
		return adminhttp.StreamServerLogs501JSONResponse(LogQueryNotConfiguredResponse()), nil
	}
	return streamServerLogsResponse{ctx: ctx, service: s.ServerLogs, request: streamReq}, nil
}

func LogQueryNotConfiguredResponse() apitypes.ErrorResponse {
	return apitypes.NewErrorResponse("LOG_QUERY_NOT_CONFIGURED", "server log query backend is not configured")
}

func serverLogStreamRequestFromParams(params adminhttp.StreamServerLogsParams) (ServerLogStreamRequest, error) {
	req := ServerLogStreamRequest{
		Filter: "*",
		Limit:  defaultServerLogStreamLimit,
		Order:  ServerLogOrderAsc,
	}
	if params.Filter != nil {
		if filter := strings.TrimSpace(*params.Filter); filter != "" {
			req.Filter = filter
		}
		req.FilterSet = true
	}
	if params.StartTimeMs != nil {
		req.StartTimeMs = *params.StartTimeMs
		req.StartTimeSet = true
	}
	if params.EndTimeMs != nil {
		req.EndTimeMs = *params.EndTimeMs
		req.EndTimeSet = true
	}
	if params.Limit != nil {
		if *params.Limit <= 0 {
			return ServerLogStreamRequest{}, errors.New("limit must be positive")
		}
		if *params.Limit > maxServerLogStreamLimit {
			req.Limit = maxServerLogStreamLimit
		} else {
			req.Limit = int(*params.Limit)
		}
	}
	if params.Order != nil {
		switch strings.ToLower(strings.TrimSpace(*params.Order)) {
		case string(ServerLogOrderAsc):
			req.Order = ServerLogOrderAsc
			req.OrderSet = true
		case string(ServerLogOrderDesc):
			req.Order = ServerLogOrderDesc
			req.OrderSet = true
		default:
			return ServerLogStreamRequest{}, errors.New("order must be asc or desc")
		}
	}
	if params.Cursor != nil {
		req.Cursor = strings.TrimSpace(*params.Cursor)
	}
	if req.Cursor == "" {
		if params.StartTimeMs == nil {
			return ServerLogStreamRequest{}, errors.New("start_time_ms is required when cursor is not set")
		}
		if params.EndTimeMs == nil {
			return ServerLogStreamRequest{}, errors.New("end_time_ms is required when cursor is not set")
		}
		if req.EndTimeMs <= req.StartTimeMs {
			return ServerLogStreamRequest{}, errors.New("end_time_ms must be greater than start_time_ms")
		}
	}
	return req, nil
}

func (response streamServerLogsResponse) VisitStreamServerLogsResponse(ctx *fiber.Ctx) error {
	streamCtx, cancel := context.WithCancel(response.ctx)
	events := make(chan serverLogEvent, 16)
	done := make(chan error, 1)
	go func() {
		defer close(events)
		end, err := response.service.StreamServerLogs(streamCtx, response.request, func(entry apitypes.ServerLogEntry) error {
			select {
			case <-streamCtx.Done():
				return streamCtx.Err()
			case events <- serverLogEvent{name: "log", data: entry}:
				return nil
			}
		})
		if err == nil {
			select {
			case <-streamCtx.Done():
				err = streamCtx.Err()
			case events <- serverLogEvent{name: "end", data: end}:
			}
		}
		done <- err
	}()

	first, err, hasFirst, donePending := waitFirstServerLogEvent(streamCtx, events, done)
	if err != nil && !hasFirst {
		cancel()
		status, body := serverLogQueryErrorResponse(err)
		switch status {
		case http.StatusBadRequest:
			return ctx.Status(status).JSON(adminhttp.StreamServerLogs400JSONResponse(body))
		case http.StatusNotImplemented:
			return ctx.Status(status).JSON(adminhttp.StreamServerLogs501JSONResponse(body))
		default:
			return ctx.Status(http.StatusBadGateway).JSON(adminhttp.StreamServerLogs502JSONResponse(body))
		}
	}

	ctx.Response().Header.Set("Content-Type", "text/event-stream")
	ctx.Response().Header.Set("Cache-Control", "no-cache")
	ctx.Response().Header.Set("X-Accel-Buffering", "no")
	ctx.Status(http.StatusOK)
	ctx.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer cancel()
		if hasFirst {
			if err := writeServerLogSSE(w, first.name, first.data); err != nil {
				return
			}
		}
		for {
			select {
			case event, ok := <-events:
				if !ok {
					if donePending {
						err = <-done
					}
					if err != nil && !errors.Is(err, context.Canceled) {
						_ = writeServerLogSSE(w, "error", postStartServerLogError(err))
					}
					return
				}
				if err := writeServerLogSSE(w, event.name, event.data); err != nil {
					return
				}
			case <-streamCtx.Done():
				return
			}
		}
	})
	return nil
}

func waitFirstServerLogEvent(ctx context.Context, events <-chan serverLogEvent, done <-chan error) (serverLogEvent, error, bool, bool) {
	select {
	case event, ok := <-events:
		if !ok {
			return serverLogEvent{}, <-done, false, false
		}
		return event, nil, true, true
	default:
	}

	select {
	case event, ok := <-events:
		if !ok {
			return serverLogEvent{}, <-done, false, false
		}
		return event, nil, true, true
	case err := <-done:
		select {
		case event, ok := <-events:
			if ok {
				return event, err, true, false
			}
		default:
		}
		return serverLogEvent{}, err, false, false
	case <-ctx.Done():
		return serverLogEvent{}, ctx.Err(), false, false
	}
}

func postStartServerLogError(err error) apitypes.ErrorResponse {
	_, body := serverLogQueryErrorResponse(err)
	return body
}

func writeServerLogSSE(w *bufio.Writer, event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return err
	}
	return w.Flush()
}
