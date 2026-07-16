package peerresource

import (
	"bytes"
	"context"
	"io"
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/asset"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestPrepareAssetDownloadAuthorizesLiveResourceBinding(t *testing.T) {
	ctx := context.Background()
	assets, err := asset.New(kv.NewMemory(nil), objectstore.Dir(t.TempDir()), asset.Options{})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := assets.Put(ctx, asset.PutRequest{MediaType: "image/png", MaxBytes: 1024}, bytes.NewBufferString("payload"))
	if err != nil {
		t.Fatal(err)
	}
	owner := asset.Owner{Kind: asset.OwnerKindResource, ID: "Workflow/demo"}
	resolver := assetResolverFunc(func(context.Context, asset.Owner) (asset.OwnerSnapshot, error) {
		return asset.OwnerSnapshot{Exists: true, Refs: []asset.Ref{stored.Metadata.Ref}}, nil
	})
	if err := assets.RegisterOwnerResolver(asset.OwnerKindResource, resolver); err != nil {
		t.Fatal(err)
	}
	if err := assets.Bind(ctx, stored.Metadata.Ref, asset.Binding{Owner: owner}); err != nil {
		t.Fatal(err)
	}
	server := &Server{ACL: allowAllAuthorizer{}, Assets: assets, AssetDisplays: displayAssetResolverFunc(func(context.Context, asset.Owner, asset.Ref) (bool, error) {
		return true, nil
	})}
	response, reader, rpcErr, err := server.PrepareAssetDownload(ctx, rpcpb.AssetDownloadRequest{Ref: stored.Metadata.Ref.String()})
	if err != nil || rpcErr != nil {
		t.Fatalf("PrepareAssetDownload() error=%v rpcError=%v", err, rpcErr)
	}
	defer reader.Close()
	payload, err := io.ReadAll(reader)
	if err != nil || string(payload) != "payload" || response.GetMetadata().GetRef() != stored.Metadata.Ref.String() {
		t.Fatalf("download metadata=%#v payload=%q err=%v", response.GetMetadata(), payload, err)
	}
}

func TestPrepareAssetDownloadDeniesBeforeOpeningBytes(t *testing.T) {
	ctx := context.Background()
	assets, err := asset.New(kv.NewMemory(nil), objectstore.Dir(t.TempDir()), asset.Options{})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := assets.Put(ctx, asset.PutRequest{MediaType: "image/png", MaxBytes: 1024}, bytes.NewBufferString("payload"))
	if err != nil {
		t.Fatal(err)
	}
	owner := asset.Owner{Kind: asset.OwnerKindResource, ID: "Workflow/hidden"}
	if err := assets.RegisterOwnerResolver(asset.OwnerKindResource, assetResolverFunc(func(context.Context, asset.Owner) (asset.OwnerSnapshot, error) {
		return asset.OwnerSnapshot{Exists: true, Refs: []asset.Ref{stored.Metadata.Ref}}, nil
	})); err != nil {
		t.Fatal(err)
	}
	if err := assets.Bind(ctx, stored.Metadata.Ref, asset.Binding{Owner: owner}); err != nil {
		t.Fatal(err)
	}
	server := &Server{ACL: newRuleAuthorizer(), Assets: assets, AssetDisplays: displayAssetResolverFunc(func(context.Context, asset.Owner, asset.Ref) (bool, error) {
		return true, nil
	})}
	_, reader, rpcErr, err := server.PrepareAssetDownload(ctx, rpcpb.AssetDownloadRequest{Ref: stored.Metadata.Ref.String()})
	if err != nil || rpcErr == nil || reader != nil {
		t.Fatalf("denied download reader=%v rpcError=%v err=%v", reader, rpcErr, err)
	}
}

func TestPrepareAssetDownloadRejectsNonDisplayResourceReference(t *testing.T) {
	ctx := context.Background()
	assets, err := asset.New(kv.NewMemory(nil), objectstore.Dir(t.TempDir()), asset.Options{})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := assets.Put(ctx, asset.PutRequest{MediaType: "application/octet-stream", MaxBytes: 1024}, bytes.NewBufferString("internal"))
	if err != nil {
		t.Fatal(err)
	}
	owner := asset.Owner{Kind: asset.OwnerKindResource, ID: "Workflow/internal"}
	if err := assets.RegisterOwnerResolver(asset.OwnerKindResource, assetResolverFunc(func(context.Context, asset.Owner) (asset.OwnerSnapshot, error) {
		return asset.OwnerSnapshot{Exists: true, Refs: []asset.Ref{stored.Metadata.Ref}}, nil
	})); err != nil {
		t.Fatal(err)
	}
	if err := assets.Bind(ctx, stored.Metadata.Ref, asset.Binding{Owner: owner}); err != nil {
		t.Fatal(err)
	}
	server := &Server{ACL: allowAllAuthorizer{}, Assets: assets, AssetDisplays: displayAssetResolverFunc(func(context.Context, asset.Owner, asset.Ref) (bool, error) {
		return false, nil
	})}
	_, reader, rpcErr, err := server.PrepareAssetDownload(ctx, rpcpb.AssetDownloadRequest{Ref: stored.Metadata.Ref.String()})
	if err != nil || rpcErr == nil || reader != nil {
		t.Fatalf("non-display download reader=%v rpcError=%v err=%v", reader, rpcErr, err)
	}
}

type assetResolverFunc func(context.Context, asset.Owner) (asset.OwnerSnapshot, error)

func (f assetResolverFunc) ResolveAssetOwner(ctx context.Context, owner asset.Owner) (asset.OwnerSnapshot, error) {
	return f(ctx, owner)
}

type displayAssetResolverFunc func(context.Context, asset.Owner, asset.Ref) (bool, error)

func (f displayAssetResolverFunc) ResourceHasDisplayAsset(ctx context.Context, owner asset.Owner, ref asset.Ref) (bool, error) {
	return f(ctx, owner, ref)
}
