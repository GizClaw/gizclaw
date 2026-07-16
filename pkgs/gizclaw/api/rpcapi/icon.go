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

func (t RPCPayload) AsServerInfoIconDownloadRequest() (ServerInfoIconDownloadRequest, error) {
	return asIconPayload[ServerInfoIconDownloadRequest](t, "ServerInfoIconDownloadRequest")
}

func (t *RPCPayload) FromServerInfoIconDownloadRequest(v ServerInfoIconDownloadRequest) error {
	return t.encode("ServerInfoIconDownloadRequest", v)
}

func (t *RPCPayload) MergeServerInfoIconDownloadRequest(v ServerInfoIconDownloadRequest) error {
	return t.merge("ServerInfoIconDownloadRequest", v)
}

func (t RPCPayload) AsServerInfoIconDownloadResponse() (ServerInfoIconDownloadResponse, error) {
	return asIconPayload[ServerInfoIconDownloadResponse](t, "ServerInfoIconDownloadResponse")
}

func (t *RPCPayload) FromServerInfoIconDownloadResponse(v ServerInfoIconDownloadResponse) error {
	return t.encode("ServerInfoIconDownloadResponse", v)
}

func (t *RPCPayload) MergeServerInfoIconDownloadResponse(v ServerInfoIconDownloadResponse) error {
	return t.merge("ServerInfoIconDownloadResponse", v)
}

func (t RPCPayload) AsServerInfoIconUploadRequest() (ServerInfoIconUploadRequest, error) {
	return asIconPayload[ServerInfoIconUploadRequest](t, "ServerInfoIconUploadRequest")
}

func (t *RPCPayload) FromServerInfoIconUploadRequest(v ServerInfoIconUploadRequest) error {
	return t.encode("ServerInfoIconUploadRequest", v)
}

func (t *RPCPayload) MergeServerInfoIconUploadRequest(v ServerInfoIconUploadRequest) error {
	return t.merge("ServerInfoIconUploadRequest", v)
}

func (t RPCPayload) AsServerInfoIconUploadResponse() (ServerInfoIconUploadResponse, error) {
	return asIconPayload[ServerInfoIconUploadResponse](t, "ServerInfoIconUploadResponse")
}

func (t *RPCPayload) FromServerInfoIconUploadResponse(v ServerInfoIconUploadResponse) error {
	return t.encode("ServerInfoIconUploadResponse", v)
}

func (t *RPCPayload) MergeServerInfoIconUploadResponse(v ServerInfoIconUploadResponse) error {
	return t.merge("ServerInfoIconUploadResponse", v)
}

func (t RPCPayload) AsServerInfoIconDeleteRequest() (ServerInfoIconDeleteRequest, error) {
	return asIconPayload[ServerInfoIconDeleteRequest](t, "ServerInfoIconDeleteRequest")
}

func (t *RPCPayload) FromServerInfoIconDeleteRequest(v ServerInfoIconDeleteRequest) error {
	return t.encode("ServerInfoIconDeleteRequest", v)
}

func (t *RPCPayload) MergeServerInfoIconDeleteRequest(v ServerInfoIconDeleteRequest) error {
	return t.merge("ServerInfoIconDeleteRequest", v)
}

func (t RPCPayload) AsServerInfoIconDeleteResponse() (ServerInfoIconDeleteResponse, error) {
	return asIconPayload[ServerInfoIconDeleteResponse](t, "ServerInfoIconDeleteResponse")
}

func (t *RPCPayload) FromServerInfoIconDeleteResponse(v ServerInfoIconDeleteResponse) error {
	return t.encode("ServerInfoIconDeleteResponse", v)
}

func (t *RPCPayload) MergeServerInfoIconDeleteResponse(v ServerInfoIconDeleteResponse) error {
	return t.merge("ServerInfoIconDeleteResponse", v)
}
