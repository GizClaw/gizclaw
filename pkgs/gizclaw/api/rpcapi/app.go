package rpcapi

import rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"

// App methods are provided by the connected client.
const (
	RPCMethodClientAppList      RPCMethod = "client.app.list"
	RPCMethodClientAppInstall   RPCMethod = "client.app.install"
	RPCMethodClientAppUninstall RPCMethod = "client.app.uninstall"
	RPCMethodClientAppInvoke    RPCMethod = "client.app.invoke"
	RPCMethodClientAppJobStart  RPCMethod = "client.app.job.start"
	RPCMethodClientAppJobCancel RPCMethod = "client.app.job.cancel"
)

// AsClientAppListRequest decodes the App payload.
func (p RPCPayload) AsClientAppListRequest() (*rpcpb.ClientAppListRequest, error) {
	value := new(rpcpb.ClientAppListRequest)
	err := p.decode("ClientAppListRequest", value)
	return value, err
}

// FromClientAppListRequest encodes the App payload.
func (p *RPCPayload) FromClientAppListRequest(value *rpcpb.ClientAppListRequest) error {
	return p.encode("ClientAppListRequest", value)
}

// AsClientAppListResponse decodes the App payload.
func (p RPCPayload) AsClientAppListResponse() (*rpcpb.ClientAppListResponse, error) {
	value := new(rpcpb.ClientAppListResponse)
	err := p.decode("ClientAppListResponse", value)
	return value, err
}

// FromClientAppListResponse encodes the App payload.
func (p *RPCPayload) FromClientAppListResponse(value *rpcpb.ClientAppListResponse) error {
	return p.encode("ClientAppListResponse", value)
}

// AsClientAppInstallRequest decodes the App payload.
func (p RPCPayload) AsClientAppInstallRequest() (*rpcpb.ClientAppInstallRequest, error) {
	value := new(rpcpb.ClientAppInstallRequest)
	err := p.decode("ClientAppInstallRequest", value)
	return value, err
}

// FromClientAppInstallRequest encodes the App payload.
func (p *RPCPayload) FromClientAppInstallRequest(value *rpcpb.ClientAppInstallRequest) error {
	return p.encode("ClientAppInstallRequest", value)
}

// AsClientAppInstallResponse decodes the App payload.
func (p RPCPayload) AsClientAppInstallResponse() (*rpcpb.ClientAppInstallResponse, error) {
	value := new(rpcpb.ClientAppInstallResponse)
	err := p.decode("ClientAppInstallResponse", value)
	return value, err
}

// FromClientAppInstallResponse encodes the App payload.
func (p *RPCPayload) FromClientAppInstallResponse(value *rpcpb.ClientAppInstallResponse) error {
	return p.encode("ClientAppInstallResponse", value)
}

// AsClientAppUninstallRequest decodes the App payload.
func (p RPCPayload) AsClientAppUninstallRequest() (*rpcpb.ClientAppUninstallRequest, error) {
	value := new(rpcpb.ClientAppUninstallRequest)
	err := p.decode("ClientAppUninstallRequest", value)
	return value, err
}

// FromClientAppUninstallRequest encodes the App payload.
func (p *RPCPayload) FromClientAppUninstallRequest(value *rpcpb.ClientAppUninstallRequest) error {
	return p.encode("ClientAppUninstallRequest", value)
}

// AsClientAppUninstallResponse decodes the App payload.
func (p RPCPayload) AsClientAppUninstallResponse() (*rpcpb.ClientAppUninstallResponse, error) {
	value := new(rpcpb.ClientAppUninstallResponse)
	err := p.decode("ClientAppUninstallResponse", value)
	return value, err
}

// FromClientAppUninstallResponse encodes the App payload.
func (p *RPCPayload) FromClientAppUninstallResponse(value *rpcpb.ClientAppUninstallResponse) error {
	return p.encode("ClientAppUninstallResponse", value)
}

// AsClientAppInvokeRequest decodes the App payload.
func (p RPCPayload) AsClientAppInvokeRequest() (*rpcpb.ClientAppInvokeRequest, error) {
	value := new(rpcpb.ClientAppInvokeRequest)
	err := p.decode("ClientAppInvokeRequest", value)
	return value, err
}

// FromClientAppInvokeRequest encodes the App payload.
func (p *RPCPayload) FromClientAppInvokeRequest(value *rpcpb.ClientAppInvokeRequest) error {
	return p.encode("ClientAppInvokeRequest", value)
}

// AsClientAppInvokeResponse decodes the App payload.
func (p RPCPayload) AsClientAppInvokeResponse() (*rpcpb.ClientAppInvokeResponse, error) {
	value := new(rpcpb.ClientAppInvokeResponse)
	err := p.decode("ClientAppInvokeResponse", value)
	return value, err
}

// FromClientAppInvokeResponse encodes the App payload.
func (p *RPCPayload) FromClientAppInvokeResponse(value *rpcpb.ClientAppInvokeResponse) error {
	return p.encode("ClientAppInvokeResponse", value)
}

// AsClientAppJobStartRequest decodes the App payload.
func (p RPCPayload) AsClientAppJobStartRequest() (*rpcpb.ClientAppJobStartRequest, error) {
	value := new(rpcpb.ClientAppJobStartRequest)
	err := p.decode("ClientAppJobStartRequest", value)
	return value, err
}

// FromClientAppJobStartRequest encodes the App payload.
func (p *RPCPayload) FromClientAppJobStartRequest(value *rpcpb.ClientAppJobStartRequest) error {
	return p.encode("ClientAppJobStartRequest", value)
}

// AsClientAppJobStartResponse decodes the App payload.
func (p RPCPayload) AsClientAppJobStartResponse() (*rpcpb.ClientAppJobStartResponse, error) {
	value := new(rpcpb.ClientAppJobStartResponse)
	err := p.decode("ClientAppJobStartResponse", value)
	return value, err
}

// FromClientAppJobStartResponse encodes the App payload.
func (p *RPCPayload) FromClientAppJobStartResponse(value *rpcpb.ClientAppJobStartResponse) error {
	return p.encode("ClientAppJobStartResponse", value)
}

// AsClientAppJobCancelRequest decodes the App payload.
func (p RPCPayload) AsClientAppJobCancelRequest() (*rpcpb.ClientAppJobCancelRequest, error) {
	value := new(rpcpb.ClientAppJobCancelRequest)
	err := p.decode("ClientAppJobCancelRequest", value)
	return value, err
}

// FromClientAppJobCancelRequest encodes the App payload.
func (p *RPCPayload) FromClientAppJobCancelRequest(value *rpcpb.ClientAppJobCancelRequest) error {
	return p.encode("ClientAppJobCancelRequest", value)
}

// AsClientAppJobCancelResponse decodes the App payload.
func (p RPCPayload) AsClientAppJobCancelResponse() (*rpcpb.ClientAppJobCancelResponse, error) {
	value := new(rpcpb.ClientAppJobCancelResponse)
	err := p.decode("ClientAppJobCancelResponse", value)
	return value, err
}

// FromClientAppJobCancelResponse encodes the App payload.
func (p *RPCPayload) FromClientAppJobCancelResponse(value *rpcpb.ClientAppJobCancelResponse) error {
	return p.encode("ClientAppJobCancelResponse", value)
}
