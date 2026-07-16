package gizcli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func TestDownloadAssetRejectsNilOutput(t *testing.T) {
	client := &rpcClient{}
	_, err := client.DownloadAsset(context.Background(), nil, "asset-download", rpcpb.AssetDownloadRequest{}, nil)
	if err == nil || !strings.Contains(err.Error(), "asset download output is required") {
		t.Fatalf("DownloadAsset(nil output) error = %v", err)
	}
}

func TestDownloadAssetStreamsMetadataAndBytes(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	payload := []byte("asset-payload")
	digest := sha256.Sum256(payload)
	serverErr := make(chan error, 1)
	go func() {
		request, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			serverErr <- err
			return
		}
		response := resourceResponse(request.Id, rpcpb.AssetDownloadResponse{Metadata: &rpcpb.AssetMetadata{
			Ref:       "asset://01010101010101010101010101010101",
			MediaType: "image/png",
			SizeBytes: int64(len(payload)),
			Sha256:    hex.EncodeToString(digest[:]),
		}}, (*rpcapi.RPCPayload).FromAssetDownloadResponse)
		if err := rpcapi.WriteResponseForMethod(serverSide, rpcapi.RPCMethodServerAssetDownload, response); err != nil {
			serverErr <- err
			return
		}
		if err := rpcapi.WriteFrame(serverSide, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: payload}); err != nil {
			serverErr <- err
			return
		}
		serverErr <- rpcapi.WriteEOS(serverSide)
	}()
	var out bytes.Buffer
	result, err := (&rpcClient{}).DownloadAsset(context.Background(), clientSide, "asset-download", rpcpb.AssetDownloadRequest{Ref: "asset://01010101010101010101010101010101"}, &out)
	if err != nil {
		t.Fatalf("DownloadAsset() error = %v", err)
	}
	if result.Metadata.GetMediaType() != "image/png" || result.Bytes != int64(len(payload)) || !bytes.Equal(out.Bytes(), payload) {
		t.Fatalf("DownloadAsset() = %#v payload=%q", result, out.Bytes())
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server error = %v", err)
	}
}

func TestDownloadAssetRejectsContentThatDoesNotMatchMetadata(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	payload := []byte("corrupt")
	expected := sha256.Sum256([]byte("expected"))
	serverErr := make(chan error, 1)
	go func() {
		request, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			serverErr <- err
			return
		}
		response := resourceResponse(request.Id, rpcpb.AssetDownloadResponse{Metadata: &rpcpb.AssetMetadata{
			Ref:       "asset://01010101010101010101010101010101",
			MediaType: "image/png",
			SizeBytes: int64(len(payload)),
			Sha256:    hex.EncodeToString(expected[:]),
		}}, (*rpcapi.RPCPayload).FromAssetDownloadResponse)
		if err := rpcapi.WriteResponseForMethod(serverSide, rpcapi.RPCMethodServerAssetDownload, response); err != nil {
			serverErr <- err
			return
		}
		if err := rpcapi.WriteFrame(serverSide, rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: payload}); err != nil {
			serverErr <- err
			return
		}
		serverErr <- rpcapi.WriteEOS(serverSide)
	}()
	var out bytes.Buffer
	_, err := (&rpcClient{}).DownloadAsset(context.Background(), clientSide, "asset-download", rpcpb.AssetDownloadRequest{Ref: "asset://01010101010101010101010101010101"}, &out)
	if err == nil || !strings.Contains(err.Error(), "does not match metadata") {
		t.Fatalf("DownloadAsset(corrupt) error = %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server error = %v", err)
	}
}

func TestDownloadAssetRejectsMissingMetadataBeforeBinary(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	serverErr := make(chan error, 1)
	go func() {
		request, err := readRPCRequestWithEOS(serverSide)
		if err != nil {
			serverErr <- err
			return
		}
		response := resourceResponse(request.Id, rpcpb.AssetDownloadResponse{}, (*rpcapi.RPCPayload).FromAssetDownloadResponse)
		serverErr <- rpcapi.WriteResponseForMethod(serverSide, rpcapi.RPCMethodServerAssetDownload, response)
	}()
	var out bytes.Buffer
	_, err := (&rpcClient{}).DownloadAsset(context.Background(), clientSide, "asset-download", rpcpb.AssetDownloadRequest{Ref: "asset://01010101010101010101010101010101"}, &out)
	if err == nil || !strings.Contains(err.Error(), "missing metadata") {
		t.Fatalf("DownloadAsset(missing metadata) error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("DownloadAsset(missing metadata) wrote %d bytes", out.Len())
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("server error = %v", err)
	}
}
