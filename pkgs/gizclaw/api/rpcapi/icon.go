package rpcapi

func asIconPayload[T any](payload RPCPayload, name string) (T, error) {
	var body T
	err := payload.decode(name, &body)
	return body, err
}

func (t RPCPayload) AsWorkflowIconDownloadRequest() (WorkflowIconDownloadRequest, error) {
	return asIconPayload[WorkflowIconDownloadRequest](t, "WorkflowIconDownloadRequest")
}

func (t *RPCPayload) FromWorkflowIconDownloadRequest(v WorkflowIconDownloadRequest) error {
	return t.encode("WorkflowIconDownloadRequest", v)
}

func (t *RPCPayload) MergeWorkflowIconDownloadRequest(v WorkflowIconDownloadRequest) error {
	return t.merge("WorkflowIconDownloadRequest", v)
}

func (t RPCPayload) AsWorkflowIconDownloadResponse() (WorkflowIconDownloadResponse, error) {
	return asIconPayload[WorkflowIconDownloadResponse](t, "WorkflowIconDownloadResponse")
}

func (t *RPCPayload) FromWorkflowIconDownloadResponse(v WorkflowIconDownloadResponse) error {
	return t.encode("WorkflowIconDownloadResponse", v)
}

func (t *RPCPayload) MergeWorkflowIconDownloadResponse(v WorkflowIconDownloadResponse) error {
	return t.merge("WorkflowIconDownloadResponse", v)
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
