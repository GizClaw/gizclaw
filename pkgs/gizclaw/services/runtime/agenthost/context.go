package agenthost

import (
	"context"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

type aclSubjectContextKey struct{}

// WithACLSubject attaches the authenticated caller subject to an agent runtime context.
func WithACLSubject(ctx context.Context, subject apitypes.ACLSubject) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, aclSubjectContextKey{}, subject)
}

func aclSubjectFromContext(ctx context.Context) (apitypes.ACLSubject, bool) {
	if ctx == nil {
		return apitypes.ACLSubject{}, false
	}
	subject, ok := ctx.Value(aclSubjectContextKey{}).(apitypes.ACLSubject)
	if !ok || subject.Kind == "" || strings.TrimSpace(subject.Id) == "" {
		return apitypes.ACLSubject{}, false
	}
	return subject, true
}
