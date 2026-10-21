package infraflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
)

// Config binds a dedicated KV namespace for the public read-only feed.
// An empty Store disables the feed; it never creates an implicit Store.
type Config struct {
	Store       string               `yaml:"store"`
	Environment infraapi.Environment `yaml:"environment"`
	Capture     *CaptureConfig       `yaml:"capture"`
	Producer    *ProducerConfig      `yaml:"producer"`
	// Deprecated: ReadToken is ignored. Infrastructure aggregates are public.
	ReadToken string `yaml:"read_token" json:"-"`
}

// Validate checks the optional logical Store name. Legacy ReadToken is ignored.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.Store) != c.Store {
		return errors.New("infra_flow.store must not contain surrounding whitespace")
	}
	return c.validateCollection()
}

type flowServer struct{ feed *Feed }

func (s *flowServer) PreflightInfraFlows(context.Context, infraapi.PreflightInfraFlowsRequestObject) (infraapi.PreflightInfraFlowsResponseObject, error) {
	return infraapi.PreflightInfraFlows204Response{}, nil
}

func (s *flowServer) ReadInfraFlows(ctx context.Context, request infraapi.ReadInfraFlowsRequestObject) (infraapi.ReadInfraFlowsResponseObject, error) {
	cursor := ""
	if request.Params.Cursor != nil {
		cursor = *request.Params.Cursor
	}
	if !request.Params.Environment.Valid() || !validCursor(cursor) {
		return infraapi.ReadInfraFlows400JSONResponse{Error: infraapi.INVALIDFLOWQUERY}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	page, err := s.feed.Read(ctx, request.Params.Environment, cursor)
	if err != nil {
		return infraapi.ReadInfraFlows503JSONResponse{Error: infraapi.FLOWFEEDUNAVAILABLE}, nil
	}
	return infraapi.ReadInfraFlows200JSONResponse(page), nil
}

func validCursor(cursor string) bool {
	if cursor == "" {
		return true
	}
	value, err := strconv.ParseInt(cursor, 10, 64)
	return err == nil && len(cursor) <= 16 && value > 0 && value <= 8640000000000000 && value%Window.Milliseconds() == 0 && strconv.FormatInt(value, 10) == cursor
}

// Handler mounts the generated public read-only AP API. It borrows Feed, starts
// no producer, and validates bounded queries before reading KV. It ignores credentials.
func Handler(cfg Config, feed *Feed, next http.Handler) http.Handler {
	validConfig := cfg.Validate() == nil
	if next == nil {
		next = http.NotFoundHandler()
	}
	api := infraapi.HandlerWithOptions(infraapi.NewStrictHandler(&flowServer{feed: feed}, nil), infraapi.StdHTTPServerOptions{ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, _ error) {
		writeError(w, http.StatusBadRequest, infraapi.INVALIDFLOWQUERY)
	}})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != Path {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		} else {
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,OPTIONS")
		if r.Method == http.MethodOptions {
			api.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET,OPTIONS")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !validConfig || feed == nil || cfg.Store == "" {
			writeError(w, http.StatusServiceUnavailable, infraapi.FLOWFEEDDISABLED)
			return
		}
		query, err := urlValues(r)
		if err != nil || !infraapi.Environment(query.Get("environment")).Valid() || !validCursor(query.Get("cursor")) {
			writeError(w, http.StatusBadRequest, infraapi.INVALIDFLOWQUERY)
			return
		}
		api.ServeHTTP(w, r)
	})
}

func writeError(w http.ResponseWriter, status int, code infraapi.ErrorError) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(infraapi.Error{Error: code})
}

func urlValues(r *http.Request) (url.Values, error) {
	if len(r.URL.RawQuery) > 160 {
		return nil, errors.New("oversized query")
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, err
	}
	for key, items := range values {
		if (key != "environment" && key != "cursor") || len(items) != 1 || items[0] == "" {
			return nil, errors.New("invalid query")
		}
	}
	return values, nil
}
