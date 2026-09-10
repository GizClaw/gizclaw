package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/contextconn"
	"golang.org/x/sync/singleflight"
)

const (
	maxOperationAttempts = 3
	defaultRetryDelay    = time.Second
)

// resourceConn is one connected Admin resource client.
type resourceConn interface {
	ApplyResource(context.Context, apitypes.Resource) (apitypes.ApplyResult, error)
	DeleteResource(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error)
	GetResource(context.Context, apitypes.ResourceKind, string) (apitypes.Resource, error)
	Close() error
}

type resourceEnvelope struct {
	APIVersion string           `json:"apiVersion"`
	Kind       string           `json:"kind"`
	Metadata   resourceMetadata `json:"metadata"`
	Spec       json.RawMessage  `json:"spec"`
}

type resourceMetadata struct {
	ID          string            `json:"id"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// adminClient owns one lazily opened, long-lived Server connection for a
// provider process. A GizClaw context has one Peer identity, and a second
// connection with the same identity replaces the first on the Server, so all
// operations share this connection instead of dialing per resource.
type adminClient struct {
	opts       contextconn.Options
	connect    func(contextconn.Options) (resourceConn, error)
	retryDelay time.Duration

	dial   singleflight.Group
	connMu sync.Mutex
	conn   resourceConn

	// slots bounds concurrent requests on the shared connection.
	slots chan struct{}

	readMu       sync.Mutex
	pendingReads []resourceRead
}

func newAdminClient(opts contextconn.Options) *adminClient {
	return &adminClient{
		opts: opts,
		connect: func(opts contextconn.Options) (resourceConn, error) {
			conn, err := adminresource.Connect(opts)
			if err != nil {
				return nil, err
			}
			return conn, nil
		},
		retryDelay: defaultRetryDelay,
		slots:      make(chan struct{}, adminresource.BatchConcurrency),
	}
}

// connection returns the shared connection, opening it when needed.
// Concurrent callers share one dial; connMu never covers the dial itself.
func (c *adminClient) connection() (resourceConn, error) {
	if conn := c.current(); conn != nil {
		return conn, nil
	}
	value, err, _ := c.dial.Do("connect", func() (any, error) {
		if conn := c.current(); conn != nil {
			return conn, nil
		}
		conn, err := c.connect(c.opts)
		if err != nil {
			return nil, fmt.Errorf("connect to GizClaw Server: %w", err)
		}
		c.connMu.Lock()
		c.conn = conn
		c.connMu.Unlock()
		return conn, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(resourceConn), nil
}

func (c *adminClient) current() resourceConn {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	return c.conn
}

// discard closes conn if it is still the shared connection, so the next
// operation reconnects. Operations that already hold a newer connection are
// not affected.
func (c *adminClient) discard(conn resourceConn) {
	c.connMu.Lock()
	current := c.conn == conn
	if current {
		c.conn = nil
	}
	c.connMu.Unlock()
	if current {
		_ = conn.Close()
	}
}

// do runs op on the shared connection. Structured Admin API errors are
// returned immediately. Connection and transport failures discard the
// connection and retry with a fresh one, up to maxOperationAttempts. Every
// operation used by the provider is safe to repeat: apply is declarative, get
// is read-only, and a repeated delete reports NOT_FOUND.
func (c *adminClient) do(ctx context.Context, op func(resourceConn) error) error {
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		conn, err := c.connection()
		if err == nil {
			err = c.run(ctx, conn, op)
			if err == nil || isResponseError(err) || ctx.Err() != nil {
				return err
			}
			c.discard(conn)
		}
		if attempt == maxOperationAttempts {
			return err
		}
		if c.retryDelay > 0 {
			timer := time.NewTimer(c.retryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
}

func (c *adminClient) run(ctx context.Context, conn resourceConn, op func(resourceConn) error) error {
	select {
	case c.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-c.slots }()
	return op(conn)
}

func isResponseError(err error) bool {
	var responseErr *adminresource.ResponseError
	return errors.As(err, &responseErr)
}

// apply prepares manifest like `gizclaw admin apply --file <name>.json`,
// applies it, and returns the stored resource.
func (c *adminClient) apply(ctx context.Context, manifest []byte) (resourceEnvelope, error) {
	desired, err := adminresource.DecodeManifest(adminresource.FormatJSON, manifest)
	if err != nil {
		return resourceEnvelope{}, err
	}
	header, err := toEnvelope(desired)
	if err != nil {
		return resourceEnvelope{}, fmt.Errorf("decode desired resource: %w", err)
	}
	if err := c.do(ctx, func(conn resourceConn) error {
		_, err := conn.ApplyResource(ctx, desired)
		return err
	}); err != nil {
		return resourceEnvelope{}, err
	}
	return c.get(ctx, header.Kind, header.Metadata.ID)
}

func (c *adminClient) get(ctx context.Context, kind, resourceID string) (resourceEnvelope, error) {
	ref, err := adminresource.ParseReference(kind, resourceID)
	if err != nil {
		return resourceEnvelope{}, err
	}
	var stored apitypes.Resource
	if err := c.do(ctx, func(conn resourceConn) error {
		var err error
		stored, err = conn.GetResource(ctx, ref.Kind, ref.ID)
		return err
	}); err != nil {
		return resourceEnvelope{}, err
	}
	return toEnvelope(stored)
}

// GetResource implements adminresource.Getter for batch reads.
func (c *adminClient) GetResource(ctx context.Context, kind apitypes.ResourceKind, id string) (apitypes.Resource, error) {
	var stored apitypes.Resource
	err := c.do(ctx, func(conn resourceConn) error {
		var err error
		stored, err = conn.GetResource(ctx, kind, id)
		return err
	})
	return stored, err
}

func (c *adminClient) delete(ctx context.Context, kind, resourceID string) error {
	ref, err := adminresource.ParseReference(kind, resourceID)
	if err != nil {
		return err
	}
	return c.do(ctx, func(conn resourceConn) error {
		_, err := conn.DeleteResource(ctx, ref.Kind, ref.ID)
		return err
	})
}

// toEnvelope decodes a resource through its JSON form, matching the output of
// `gizclaw admin show`.
func toEnvelope(resource apitypes.Resource) (resourceEnvelope, error) {
	data, err := json.Marshal(resource)
	if err != nil {
		return resourceEnvelope{}, err
	}
	var envelope resourceEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return resourceEnvelope{}, fmt.Errorf("decode gizclaw resource: %w", err)
	}
	return envelope, nil
}
