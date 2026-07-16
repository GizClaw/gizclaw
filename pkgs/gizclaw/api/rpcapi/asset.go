package rpcapi

import rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"

// RPCMethodServerAssetDownload downloads an authorized Resource display asset.
const RPCMethodServerAssetDownload RPCMethod = "server.asset.download"

// AsAssetDownloadRequest decodes the RPCPayload as an AssetDownloadRequest.
func (t RPCPayload) AsAssetDownloadRequest() (rpcpb.AssetDownloadRequest, error) {
	var body rpcpb.AssetDownloadRequest
	err := t.decode("AssetDownloadRequest", &body)
	return body, err
}

// FromAssetDownloadRequest overwrites any protobuf payload as the provided AssetDownloadRequest.
func (t *RPCPayload) FromAssetDownloadRequest(value rpcpb.AssetDownloadRequest) error {
	return t.encode("AssetDownloadRequest", &value)
}

// MergeAssetDownloadRequest merges an AssetDownloadRequest into the protobuf payload.
func (t *RPCPayload) MergeAssetDownloadRequest(value rpcpb.AssetDownloadRequest) error {
	return t.merge("AssetDownloadRequest", &value)
}

// AsAssetDownloadResponse decodes the RPCPayload as an AssetDownloadResponse.
func (t RPCPayload) AsAssetDownloadResponse() (rpcpb.AssetDownloadResponse, error) {
	var body rpcpb.AssetDownloadResponse
	err := t.decode("AssetDownloadResponse", &body)
	return body, err
}

// FromAssetDownloadResponse overwrites any protobuf payload as the provided AssetDownloadResponse.
func (t *RPCPayload) FromAssetDownloadResponse(value rpcpb.AssetDownloadResponse) error {
	return t.encode("AssetDownloadResponse", &value)
}

// MergeAssetDownloadResponse merges an AssetDownloadResponse into the protobuf payload.
func (t *RPCPayload) MergeAssetDownloadResponse(value rpcpb.AssetDownloadResponse) error {
	return t.merge("AssetDownloadResponse", &value)
}
