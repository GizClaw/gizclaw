package acl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
)

const (
	defaultCleanupQueueSize = 1024
	defaultListLimit        = 50
	maxListLimit            = 200
)

var (
	errNilServer                  = errors.New("acl: nil server")
	errNilDB                      = errors.New("acl: sql db not configured")
	ErrViewNotFound               = errors.New("acl: view not found")
	ErrViewAlreadyExists          = errors.New("acl: view already exists")
	ErrRoleNotFound               = errors.New("acl: role not found")
	ErrRoleAlreadyExists          = errors.New("acl: role already exists")
	ErrPolicyBindingNotFound      = errors.New("acl: policy binding not found")
	ErrPolicyBindingAlreadyExists = errors.New("acl: policy binding already exists")
)

type Server struct {
	DB     *sql.DB
	Now    func() time.Time
	Logger *slog.Logger

	cleanupOnce sync.Once
	cleanupCh   chan string
}

type AuthorizeRequest struct {
	Subject    apitypes.ACLSubject
	Resource   apitypes.ACLResource
	Permission apitypes.ACLPermission
}

func (s *Server) Authorize(ctx context.Context, request AuthorizeRequest) error {
	if s == nil {
		return errNilServer
	}
	if s.DB == nil {
		return errNilDB
	}
	if _, err := CanonicalSubject(request.Subject); err != nil {
		return err
	}
	if _, err := CanonicalResource(request.Resource); err != nil {
		return err
	}
	request.Permission = apitypes.ACLPermission(strings.TrimSpace(string(request.Permission)))
	if request.Permission == "" {
		return errors.New("acl: permission is required")
	}
	if !request.Permission.Valid() {
		return errors.New("acl: unsupported permission")
	}
	subjectKind := strings.TrimSpace(string(request.Subject.Kind))
	subjectID := strings.TrimSpace(request.Subject.Id)
	resourceKind := strings.TrimSpace(string(request.Resource.Kind))
	resourceID := strings.TrimSpace(request.Resource.Id)
	now := s.now().Format(time.RFC3339Nano)
	subjects := []apitypes.ACLSubject{request.Subject}
	if request.Subject.Kind != SubjectKindAllPeers {
		subjects = append(subjects, AllPeersSubject())
	}
	var count int
	for _, subject := range subjects {
		subjectKind = strings.TrimSpace(string(subject.Kind))
		subjectID = strings.TrimSpace(subject.Id)
		if err := s.DB.QueryRowContext(ctx, `
SELECT count(*)
FROM acl_binding_permissions
WHERE subject_kind = ?
  AND subject_id = ?
  AND resource_kind = ?
  AND resource_id = ?
  AND permission = ?
  AND (not_before IS NULL OR not_before <= ?)
  AND (expires_at IS NULL OR expires_at > ?)`,
			subjectKind, subjectID, resourceKind, resourceID, string(request.Permission), now, now,
		).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
	}
	return ErrDenied
}

func normalizeListParams(cursor string, limit int) (string, int) {
	cursor = strings.TrimSpace(cursor)
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	return cursor, limit
}

func validateName(name, label string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("acl: %s is required", label)
	}
	return name, nil
}

func (s *Server) EnqueueExpiredPolicyBinding(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	ch := s.cleanupQueue()
	select {
	case ch <- id:
	default:
	}
}

func (s *Server) Run(ctx context.Context) error {
	if s == nil {
		return nil
	}
	ch := s.cleanupQueue()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case id := <-ch:
			if err := s.deletePolicyBinding(context.WithoutCancel(ctx), id); err != nil {
				s.logger().Error("acl: delete expired policy binding failed", "id", id, "error", err)
			}
		}
	}
}

func (s *Server) CleanupExpired(ctx context.Context, limit int) (int, error) {
	if s == nil {
		return 0, errNilServer
	}
	if s.DB == nil {
		return 0, errNilDB
	}
	if limit <= 0 {
		return 0, nil
	}
	now := s.now().Format(time.RFC3339Nano)
	rows, err := s.DB.QueryContext(ctx, `
SELECT id
FROM acl_policy_bindings
WHERE expires_at IS NOT NULL AND expires_at <= ?
ORDER BY expires_at, id
LIMIT ?`, now, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err := s.deletePolicyBinding(ctx, id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

func (s *Server) cleanupQueue() chan string {
	s.cleanupOnce.Do(func() {
		s.cleanupCh = make(chan string, defaultCleanupQueueSize)
	})
	return s.cleanupCh
}

func (s *Server) deletePolicyBinding(ctx context.Context, id string) error {
	if s == nil || s.DB == nil {
		return nil
	}
	return deletePolicyBinding(ctx, s.DB, id)
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *Server) logger() *slog.Logger {
	if s != nil && s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}
