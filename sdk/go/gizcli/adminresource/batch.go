package adminresource

import (
	"context"
	"fmt"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
)

// BatchConcurrency is the maximum number of concurrent reads GetResources
// issues on one connection.
const BatchConcurrency = 8

// Reference addresses one concrete resource.
type Reference struct {
	Kind apitypes.ResourceKind `json:"kind"`
	ID   string                `json:"id"`
}

// ParseReference validates kind and id as an addressable resource reference.
func ParseReference(kind, id string) (Reference, error) {
	resourceKind := apitypes.ResourceKind(kind)
	if !resourceKind.Valid() {
		return Reference{}, fmt.Errorf("unknown resource kind %q", kind)
	}
	if resourceKind == apitypes.ResourceKindResourceList {
		return Reference{}, fmt.Errorf("resource kind %q cannot be addressed by ID", resourceKind)
	}
	if err := customid.ValidateResourceID(id); err != nil {
		return Reference{}, fmt.Errorf("invalid resource ID: %w", err)
	}
	return Reference{Kind: resourceKind, ID: id}, nil
}

// Getter reads one resource.
type Getter interface {
	GetResource(ctx context.Context, kind apitypes.ResourceKind, id string) (apitypes.Resource, error)
}

// Result is the outcome of reading one Reference. Exactly one of Resource and
// Err is set.
type Result struct {
	Resource *apitypes.Resource
	Err      error
}

// GetResources reads refs through getter with at most BatchConcurrency
// requests in flight and returns one Result per reference in input order.
// Duplicate references are read independently. References not started before
// ctx is done report ctx.Err().
func GetResources(ctx context.Context, getter Getter, refs []Reference) []Result {
	results := make([]Result, len(refs))
	var workers sync.WaitGroup
	for worker := range min(BatchConcurrency, len(refs)) {
		workers.Go(func() {
			// Each worker owns disjoint indices.
			for i := worker; i < len(refs); i += BatchConcurrency {
				ref := refs[i]
				if err := ctx.Err(); err != nil {
					results[i].Err = err
					continue
				}
				resource, err := getter.GetResource(ctx, ref.Kind, ref.ID)
				if err != nil {
					results[i].Err = err
					continue
				}
				results[i].Resource = &resource
			}
		})
	}
	workers.Wait()
	return results
}
