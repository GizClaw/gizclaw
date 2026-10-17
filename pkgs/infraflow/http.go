package infraflow

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	infraapi "github.com/GizClaw/gizclaw-go/pkgs/infraflow/api"
)

// Config binds a dedicated KV namespace and an independent read credential.
// An empty configuration disables the feed; it never creates an implicit Store.
type Config struct {
	Store     string `yaml:"store"`
	ReadToken string `yaml:"read_token" json:"-"`
}

// Validate expands the token and requires an explicit Store/credential pair.
func (c *Config) Validate() error {
	c.ReadToken = os.ExpandEnv(c.ReadToken)
	if c.Store == "" && c.ReadToken == "" {
		return nil
	}
	if c.Store == "" || !strings.HasPrefix(c.ReadToken, "gizclaw_mk_") || len(c.ReadToken) < 43 || len(c.ReadToken) > 512 {
		return errors.New("infra_flow requires a KV store and independent gizclaw_mk_ read_token")
	}
	return nil
}

type flowServer struct{ feed *Feed }

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

// Handler mounts the generated read-only AP API. It borrows Feed, starts no
// producer, and authenticates before reading KV. User/API-key tokens grant no access.
func Handler(cfg Config, feed *Feed, next http.Handler) http.Handler {
	validConfig := cfg.Validate() == nil
	if next == nil {
		next = http.NotFoundHandler()
	}
	hash := sha256.Sum256([]byte(cfg.ReadToken))
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
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET,OPTIONS")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !validConfig || feed == nil || cfg.ReadToken == "" {
			writeError(w, http.StatusServiceUnavailable, infraapi.FLOWFEEDDISABLED)
			return
		}
		scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		candidate := sha256.Sum256([]byte(token))
		if !ok || !strings.EqualFold(scheme, "Bearer") || subtle.ConstantTimeCompare(hash[:], candidate[:]) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, infraapi.INVALIDFLOWTOKEN)
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
