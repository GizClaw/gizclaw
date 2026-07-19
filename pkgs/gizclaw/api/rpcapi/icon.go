package rpcapi

func asIconPayload[T any](payload RPCPayload, name string) (T, error) {
	var body T
	err := payload.decode(name, &body)
	return body, err
}

func (t RPCPayload) AsWorkspaceIconDownloadRequest() (WorkspaceIconDownloadRequest, error) {
	return asIconPayload[WorkspaceIconDownloadRequest](t, "WorkspaceIconDownloadRequest")
}

func (t *RPCPayload) FromWorkspaceIconDownloadRequest(v WorkspaceIconDownloadRequest) error {
	return t.encode("WorkspaceIconDownloadRequest", v)
}

func (t *RPCPayload) MergeWorkspaceIconDownloadRequest(v WorkspaceIconDownloadRequest) error {
	return t.merge("WorkspaceIconDownloadRequest", v)
}

func (t RPCPayload) AsWorkspaceIconDownloadResponse() (WorkspaceIconDownloadResponse, error) {
	return asIconPayload[WorkspaceIconDownloadResponse](t, "WorkspaceIconDownloadResponse")
}

func (t *RPCPayload) FromWorkspaceIconDownloadResponse(v WorkspaceIconDownloadResponse) error {
	return t.encode("WorkspaceIconDownloadResponse", v)
}

func (t *RPCPayload) MergeWorkspaceIconDownloadResponse(v WorkspaceIconDownloadResponse) error {
	return t.merge("WorkspaceIconDownloadResponse", v)
}
