package rpcapi

import rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"

// AsClientContentAgeGetRequest decodes the typed invoke payload.
func (p RPCPayload) AsClientContentAgeGetRequest() (*rpcpb.ClientContentAgeGetRequest, error) {
	result := new(rpcpb.ClientContentAgeGetRequest)
	err := p.decode("ClientContentAgeGetRequest", result)
	return result, err
}

// FromClientContentAgeGetRequest encodes the typed invoke payload.
func (p *RPCPayload) FromClientContentAgeGetRequest(value *rpcpb.ClientContentAgeGetRequest) error {
	return p.encode("ClientContentAgeGetRequest", value)
}

// AsClientContentAgeGetResponse decodes the typed invoke payload.
func (p RPCPayload) AsClientContentAgeGetResponse() (*rpcpb.ClientContentAgeGetResponse, error) {
	result := new(rpcpb.ClientContentAgeGetResponse)
	err := p.decode("ClientContentAgeGetResponse", result)
	return result, err
}

// FromClientContentAgeGetResponse encodes the typed invoke payload.
func (p *RPCPayload) FromClientContentAgeGetResponse(value *rpcpb.ClientContentAgeGetResponse) error {
	return p.encode("ClientContentAgeGetResponse", value)
}

// AsClientContentAgeSetRequest decodes the typed invoke payload.
func (p RPCPayload) AsClientContentAgeSetRequest() (*rpcpb.ClientContentAgeSetRequest, error) {
	result := new(rpcpb.ClientContentAgeSetRequest)
	err := p.decode("ClientContentAgeSetRequest", result)
	return result, err
}

// FromClientContentAgeSetRequest encodes the typed invoke payload.
func (p *RPCPayload) FromClientContentAgeSetRequest(value *rpcpb.ClientContentAgeSetRequest) error {
	return p.encode("ClientContentAgeSetRequest", value)
}

// AsClientContentAgeSetResponse decodes the typed invoke payload.
func (p RPCPayload) AsClientContentAgeSetResponse() (*rpcpb.ClientContentAgeSetResponse, error) {
	result := new(rpcpb.ClientContentAgeSetResponse)
	err := p.decode("ClientContentAgeSetResponse", result)
	return result, err
}

// FromClientContentAgeSetResponse encodes the typed invoke payload.
func (p *RPCPayload) FromClientContentAgeSetResponse(value *rpcpb.ClientContentAgeSetResponse) error {
	return p.encode("ClientContentAgeSetResponse", value)
}

// AsClientDeviceFeaturesGetRequest decodes the typed invoke payload.
func (p RPCPayload) AsClientDeviceFeaturesGetRequest() (*rpcpb.ClientDeviceFeaturesGetRequest, error) {
	result := new(rpcpb.ClientDeviceFeaturesGetRequest)
	err := p.decode("ClientDeviceFeaturesGetRequest", result)
	return result, err
}

// FromClientDeviceFeaturesGetRequest encodes the typed invoke payload.
func (p *RPCPayload) FromClientDeviceFeaturesGetRequest(value *rpcpb.ClientDeviceFeaturesGetRequest) error {
	return p.encode("ClientDeviceFeaturesGetRequest", value)
}

// AsClientDeviceFeaturesGetResponse decodes the typed invoke payload.
func (p RPCPayload) AsClientDeviceFeaturesGetResponse() (*rpcpb.ClientDeviceFeaturesGetResponse, error) {
	result := new(rpcpb.ClientDeviceFeaturesGetResponse)
	err := p.decode("ClientDeviceFeaturesGetResponse", result)
	return result, err
}

// FromClientDeviceFeaturesGetResponse encodes the typed invoke payload.
func (p *RPCPayload) FromClientDeviceFeaturesGetResponse(value *rpcpb.ClientDeviceFeaturesGetResponse) error {
	return p.encode("ClientDeviceFeaturesGetResponse", value)
}

// AsClientDeviceFeatureSetRequest decodes the typed invoke payload.
func (p RPCPayload) AsClientDeviceFeatureSetRequest() (*rpcpb.ClientDeviceFeatureSetRequest, error) {
	result := new(rpcpb.ClientDeviceFeatureSetRequest)
	err := p.decode("ClientDeviceFeatureSetRequest", result)
	return result, err
}

// FromClientDeviceFeatureSetRequest encodes the typed invoke payload.
func (p *RPCPayload) FromClientDeviceFeatureSetRequest(value *rpcpb.ClientDeviceFeatureSetRequest) error {
	return p.encode("ClientDeviceFeatureSetRequest", value)
}

// AsClientDeviceFeatureSetResponse decodes the typed invoke payload.
func (p RPCPayload) AsClientDeviceFeatureSetResponse() (*rpcpb.ClientDeviceFeatureSetResponse, error) {
	result := new(rpcpb.ClientDeviceFeatureSetResponse)
	err := p.decode("ClientDeviceFeatureSetResponse", result)
	return result, err
}

// FromClientDeviceFeatureSetResponse encodes the typed invoke payload.
func (p *RPCPayload) FromClientDeviceFeatureSetResponse(value *rpcpb.ClientDeviceFeatureSetResponse) error {
	return p.encode("ClientDeviceFeatureSetResponse", value)
}

// AsClientUsageTimeGetRequest decodes the typed invoke payload.
func (p RPCPayload) AsClientUsageTimeGetRequest() (*rpcpb.ClientUsageTimeGetRequest, error) {
	result := new(rpcpb.ClientUsageTimeGetRequest)
	err := p.decode("ClientUsageTimeGetRequest", result)
	return result, err
}

// FromClientUsageTimeGetRequest encodes the typed invoke payload.
func (p *RPCPayload) FromClientUsageTimeGetRequest(value *rpcpb.ClientUsageTimeGetRequest) error {
	return p.encode("ClientUsageTimeGetRequest", value)
}

// AsClientUsageTimeGetResponse decodes the typed invoke payload.
func (p RPCPayload) AsClientUsageTimeGetResponse() (*rpcpb.ClientUsageTimeGetResponse, error) {
	result := new(rpcpb.ClientUsageTimeGetResponse)
	err := p.decode("ClientUsageTimeGetResponse", result)
	return result, err
}

// FromClientUsageTimeGetResponse encodes the typed invoke payload.
func (p *RPCPayload) FromClientUsageTimeGetResponse(value *rpcpb.ClientUsageTimeGetResponse) error {
	return p.encode("ClientUsageTimeGetResponse", value)
}

// AsClientUsageTimeSetRequest decodes the typed invoke payload.
func (p RPCPayload) AsClientUsageTimeSetRequest() (*rpcpb.ClientUsageTimeSetRequest, error) {
	result := new(rpcpb.ClientUsageTimeSetRequest)
	err := p.decode("ClientUsageTimeSetRequest", result)
	return result, err
}

// FromClientUsageTimeSetRequest encodes the typed invoke payload.
func (p *RPCPayload) FromClientUsageTimeSetRequest(value *rpcpb.ClientUsageTimeSetRequest) error {
	return p.encode("ClientUsageTimeSetRequest", value)
}

// AsClientUsageTimeSetResponse decodes the typed invoke payload.
func (p RPCPayload) AsClientUsageTimeSetResponse() (*rpcpb.ClientUsageTimeSetResponse, error) {
	result := new(rpcpb.ClientUsageTimeSetResponse)
	err := p.decode("ClientUsageTimeSetResponse", result)
	return result, err
}

// FromClientUsageTimeSetResponse encodes the typed invoke payload.
func (p *RPCPayload) FromClientUsageTimeSetResponse(value *rpcpb.ClientUsageTimeSetResponse) error {
	return p.encode("ClientUsageTimeSetResponse", value)
}

// AsClientAISafetyGetRequest decodes the typed invoke payload.
func (p RPCPayload) AsClientAISafetyGetRequest() (*rpcpb.ClientAISafetyGetRequest, error) {
	result := new(rpcpb.ClientAISafetyGetRequest)
	err := p.decode("ClientAISafetyGetRequest", result)
	return result, err
}

// FromClientAISafetyGetRequest encodes the typed invoke payload.
func (p *RPCPayload) FromClientAISafetyGetRequest(value *rpcpb.ClientAISafetyGetRequest) error {
	return p.encode("ClientAISafetyGetRequest", value)
}

// AsClientAISafetyGetResponse decodes the typed invoke payload.
func (p RPCPayload) AsClientAISafetyGetResponse() (*rpcpb.ClientAISafetyGetResponse, error) {
	result := new(rpcpb.ClientAISafetyGetResponse)
	err := p.decode("ClientAISafetyGetResponse", result)
	return result, err
}

// FromClientAISafetyGetResponse encodes the typed invoke payload.
func (p *RPCPayload) FromClientAISafetyGetResponse(value *rpcpb.ClientAISafetyGetResponse) error {
	return p.encode("ClientAISafetyGetResponse", value)
}

// AsClientAISafetySetRequest decodes the typed invoke payload.
func (p RPCPayload) AsClientAISafetySetRequest() (*rpcpb.ClientAISafetySetRequest, error) {
	result := new(rpcpb.ClientAISafetySetRequest)
	err := p.decode("ClientAISafetySetRequest", result)
	return result, err
}

// FromClientAISafetySetRequest encodes the typed invoke payload.
func (p *RPCPayload) FromClientAISafetySetRequest(value *rpcpb.ClientAISafetySetRequest) error {
	return p.encode("ClientAISafetySetRequest", value)
}

// AsClientAISafetySetResponse decodes the typed invoke payload.
func (p RPCPayload) AsClientAISafetySetResponse() (*rpcpb.ClientAISafetySetResponse, error) {
	result := new(rpcpb.ClientAISafetySetResponse)
	err := p.decode("ClientAISafetySetResponse", result)
	return result, err
}

// FromClientAISafetySetResponse encodes the typed invoke payload.
func (p *RPCPayload) FromClientAISafetySetResponse(value *rpcpb.ClientAISafetySetResponse) error {
	return p.encode("ClientAISafetySetResponse", value)
}

// AsClientManagementPasswordGetRequest decodes the typed invoke payload.
func (p RPCPayload) AsClientManagementPasswordGetRequest() (*rpcpb.ClientManagementPasswordGetRequest, error) {
	result := new(rpcpb.ClientManagementPasswordGetRequest)
	err := p.decode("ClientManagementPasswordGetRequest", result)
	return result, err
}

// FromClientManagementPasswordGetRequest encodes the typed invoke payload.
func (p *RPCPayload) FromClientManagementPasswordGetRequest(value *rpcpb.ClientManagementPasswordGetRequest) error {
	return p.encode("ClientManagementPasswordGetRequest", value)
}

// AsClientManagementPasswordGetResponse decodes the typed invoke payload.
func (p RPCPayload) AsClientManagementPasswordGetResponse() (*rpcpb.ClientManagementPasswordGetResponse, error) {
	result := new(rpcpb.ClientManagementPasswordGetResponse)
	err := p.decode("ClientManagementPasswordGetResponse", result)
	return result, err
}

// FromClientManagementPasswordGetResponse encodes the typed invoke payload.
func (p *RPCPayload) FromClientManagementPasswordGetResponse(value *rpcpb.ClientManagementPasswordGetResponse) error {
	return p.encode("ClientManagementPasswordGetResponse", value)
}

// AsClientManagementPasswordSetRequest decodes the typed invoke payload.
func (p RPCPayload) AsClientManagementPasswordSetRequest() (*rpcpb.ClientManagementPasswordSetRequest, error) {
	result := new(rpcpb.ClientManagementPasswordSetRequest)
	err := p.decode("ClientManagementPasswordSetRequest", result)
	return result, err
}

// FromClientManagementPasswordSetRequest encodes the typed invoke payload.
func (p *RPCPayload) FromClientManagementPasswordSetRequest(value *rpcpb.ClientManagementPasswordSetRequest) error {
	return p.encode("ClientManagementPasswordSetRequest", value)
}

// AsClientManagementPasswordSetResponse decodes the typed invoke payload.
func (p RPCPayload) AsClientManagementPasswordSetResponse() (*rpcpb.ClientManagementPasswordSetResponse, error) {
	result := new(rpcpb.ClientManagementPasswordSetResponse)
	err := p.decode("ClientManagementPasswordSetResponse", result)
	return result, err
}

// FromClientManagementPasswordSetResponse encodes the typed invoke payload.
func (p *RPCPayload) FromClientManagementPasswordSetResponse(value *rpcpb.ClientManagementPasswordSetResponse) error {
	return p.encode("ClientManagementPasswordSetResponse", value)
}

// AsClientManagementPasswordResetRequest decodes the typed invoke payload.
func (p RPCPayload) AsClientManagementPasswordResetRequest() (*rpcpb.ClientManagementPasswordResetRequest, error) {
	result := new(rpcpb.ClientManagementPasswordResetRequest)
	err := p.decode("ClientManagementPasswordResetRequest", result)
	return result, err
}

// FromClientManagementPasswordResetRequest encodes the typed invoke payload.
func (p *RPCPayload) FromClientManagementPasswordResetRequest(value *rpcpb.ClientManagementPasswordResetRequest) error {
	return p.encode("ClientManagementPasswordResetRequest", value)
}

// AsClientManagementPasswordResetResponse decodes the typed invoke payload.
func (p RPCPayload) AsClientManagementPasswordResetResponse() (*rpcpb.ClientManagementPasswordResetResponse, error) {
	result := new(rpcpb.ClientManagementPasswordResetResponse)
	err := p.decode("ClientManagementPasswordResetResponse", result)
	return result, err
}

// FromClientManagementPasswordResetResponse encodes the typed invoke payload.
func (p *RPCPayload) FromClientManagementPasswordResetResponse(value *rpcpb.ClientManagementPasswordResetResponse) error {
	return p.encode("ClientManagementPasswordResetResponse", value)
}
