package rpcapi

// ClientDeviceStatusGetRequest asks the device for its current PeerStatus.
type ClientDeviceStatusGetRequest struct{}

// ClientDeviceStatusGetResponse carries the PeerStatus reported by the device.
type ClientDeviceStatusGetResponse struct {
	Value PeerStatus `json:"value"`
}

// ClientDeviceVolumeSetRequest sets an absolute volume level and mute state.
type ClientDeviceVolumeSetRequest struct {
	Level int64 `json:"level"`
	Muted bool  `json:"muted"`
}

// ClientDeviceVolumeSetResponse carries the PeerStatus after the volume change.
type ClientDeviceVolumeSetResponse struct {
	Value PeerStatus `json:"value"`
}

// ClientDeviceSoundPlayRequest asks the device to play a device-defined sound.
type ClientDeviceSoundPlayRequest struct {
	Sound      string `json:"sound"`
	DurationMs *int64 `json:"duration_ms,omitempty"`
}

// ClientDeviceSoundPlayResponse acknowledges a sound playback request.
type ClientDeviceSoundPlayResponse struct{}

// ClientDeviceRebootRequest asks the device to reboot after the response is sent.
type ClientDeviceRebootRequest struct {
	DelayMs *int64 `json:"delay_ms,omitempty"`
}

// ClientDeviceRebootResponse acknowledges a reboot request.
type ClientDeviceRebootResponse struct{}

// WifiStatus describes the device's current Wi-Fi connection.
type WifiStatus struct {
	Connected bool    `json:"connected"`
	Ssid      *string `json:"ssid,omitempty"`
	RssiDbm   *int64  `json:"rssi_dbm,omitempty"`
	Ip        *string `json:"ip,omitempty"`
	Bssid     *string `json:"bssid,omitempty"`
}

// WifiSavedNetwork identifies one Wi-Fi network remembered by the device.
type WifiSavedNetwork struct {
	Ssid string `json:"ssid"`
}

// ClientWifiStatusGetRequest asks the device for its Wi-Fi status.
type ClientWifiStatusGetRequest struct{}

// ClientWifiStatusGetResponse carries the device Wi-Fi status.
type ClientWifiStatusGetResponse struct {
	Value WifiStatus `json:"value"`
}

// ClientWifiSavedListRequest asks the device for its saved Wi-Fi networks.
type ClientWifiSavedListRequest struct{}

// ClientWifiSavedListResponse lists the device's saved Wi-Fi networks.
type ClientWifiSavedListResponse struct {
	Networks []WifiSavedNetwork `json:"networks"`
}

// ClientWifiSavedForgetRequest asks the device to forget one saved Wi-Fi network.
type ClientWifiSavedForgetRequest struct {
	Ssid string `json:"ssid"`
}

// ClientWifiSavedForgetResponse acknowledges a forget request.
type ClientWifiSavedForgetResponse struct{}

// WifiScanResult describes one nearby Wi-Fi network reported by the device.
type WifiScanResult struct {
	Ssid         string  `json:"ssid"`
	Bssid        *string `json:"bssid,omitempty"`
	RssiDbm      *int64  `json:"rssi_dbm,omitempty"`
	FrequencyMhz *int64  `json:"frequency_mhz,omitempty"`
	Security     *string `json:"security,omitempty"`
}

// ClientWifiScanRequest asks the device to scan for nearby Wi-Fi networks.
type ClientWifiScanRequest struct {
	TimeoutMs *int64 `json:"timeout_ms,omitempty"`
}

// ClientWifiScanResponse carries nearby Wi-Fi networks reported by the device.
type ClientWifiScanResponse struct {
	Networks []WifiScanResult `json:"networks"`
}

// ClientWifiConnectRequest asks the device to join a Wi-Fi network.
type ClientWifiConnectRequest struct {
	Ssid       string  `json:"ssid"`
	Passphrase *string `json:"passphrase,omitempty"`
}

// ClientWifiConnectResponse acknowledges that the device accepted credentials.
type ClientWifiConnectResponse struct{}

// AsClientDeviceStatusGetRequest decodes the RPCPayload as a ClientDeviceStatusGetRequest.
func (t RPCPayload) AsClientDeviceStatusGetRequest() (ClientDeviceStatusGetRequest, error) {
	var body ClientDeviceStatusGetRequest
	err := t.decode("ClientDeviceStatusGetRequest", &body)
	return body, err
}

// FromClientDeviceStatusGetRequest encodes the ClientDeviceStatusGetRequest into the RPCPayload.
func (t *RPCPayload) FromClientDeviceStatusGetRequest(v ClientDeviceStatusGetRequest) error {
	return t.encode("ClientDeviceStatusGetRequest", v)
}

// AsClientDeviceStatusGetResponse decodes the RPCPayload as a ClientDeviceStatusGetResponse.
func (t RPCPayload) AsClientDeviceStatusGetResponse() (ClientDeviceStatusGetResponse, error) {
	var body ClientDeviceStatusGetResponse
	err := t.decode("ClientDeviceStatusGetResponse", &body)
	return body, err
}

// FromClientDeviceStatusGetResponse encodes the ClientDeviceStatusGetResponse into the RPCPayload.
func (t *RPCPayload) FromClientDeviceStatusGetResponse(v ClientDeviceStatusGetResponse) error {
	return t.encode("ClientDeviceStatusGetResponse", v)
}

// AsClientDeviceVolumeSetRequest decodes the RPCPayload as a ClientDeviceVolumeSetRequest.
func (t RPCPayload) AsClientDeviceVolumeSetRequest() (ClientDeviceVolumeSetRequest, error) {
	var body ClientDeviceVolumeSetRequest
	err := t.decode("ClientDeviceVolumeSetRequest", &body)
	return body, err
}

// FromClientDeviceVolumeSetRequest encodes the ClientDeviceVolumeSetRequest into the RPCPayload.
func (t *RPCPayload) FromClientDeviceVolumeSetRequest(v ClientDeviceVolumeSetRequest) error {
	return t.encode("ClientDeviceVolumeSetRequest", v)
}

// AsClientDeviceVolumeSetResponse decodes the RPCPayload as a ClientDeviceVolumeSetResponse.
func (t RPCPayload) AsClientDeviceVolumeSetResponse() (ClientDeviceVolumeSetResponse, error) {
	var body ClientDeviceVolumeSetResponse
	err := t.decode("ClientDeviceVolumeSetResponse", &body)
	return body, err
}

// FromClientDeviceVolumeSetResponse encodes the ClientDeviceVolumeSetResponse into the RPCPayload.
func (t *RPCPayload) FromClientDeviceVolumeSetResponse(v ClientDeviceVolumeSetResponse) error {
	return t.encode("ClientDeviceVolumeSetResponse", v)
}

// AsClientDeviceSoundPlayRequest decodes the RPCPayload as a ClientDeviceSoundPlayRequest.
func (t RPCPayload) AsClientDeviceSoundPlayRequest() (ClientDeviceSoundPlayRequest, error) {
	var body ClientDeviceSoundPlayRequest
	err := t.decode("ClientDeviceSoundPlayRequest", &body)
	return body, err
}

// FromClientDeviceSoundPlayRequest encodes the ClientDeviceSoundPlayRequest into the RPCPayload.
func (t *RPCPayload) FromClientDeviceSoundPlayRequest(v ClientDeviceSoundPlayRequest) error {
	return t.encode("ClientDeviceSoundPlayRequest", v)
}

// AsClientDeviceSoundPlayResponse decodes the RPCPayload as a ClientDeviceSoundPlayResponse.
func (t RPCPayload) AsClientDeviceSoundPlayResponse() (ClientDeviceSoundPlayResponse, error) {
	var body ClientDeviceSoundPlayResponse
	err := t.decode("ClientDeviceSoundPlayResponse", &body)
	return body, err
}

// FromClientDeviceSoundPlayResponse encodes the ClientDeviceSoundPlayResponse into the RPCPayload.
func (t *RPCPayload) FromClientDeviceSoundPlayResponse(v ClientDeviceSoundPlayResponse) error {
	return t.encode("ClientDeviceSoundPlayResponse", v)
}

// AsClientDeviceRebootRequest decodes the RPCPayload as a ClientDeviceRebootRequest.
func (t RPCPayload) AsClientDeviceRebootRequest() (ClientDeviceRebootRequest, error) {
	var body ClientDeviceRebootRequest
	err := t.decode("ClientDeviceRebootRequest", &body)
	return body, err
}

// FromClientDeviceRebootRequest encodes the ClientDeviceRebootRequest into the RPCPayload.
func (t *RPCPayload) FromClientDeviceRebootRequest(v ClientDeviceRebootRequest) error {
	return t.encode("ClientDeviceRebootRequest", v)
}

// AsClientDeviceRebootResponse decodes the RPCPayload as a ClientDeviceRebootResponse.
func (t RPCPayload) AsClientDeviceRebootResponse() (ClientDeviceRebootResponse, error) {
	var body ClientDeviceRebootResponse
	err := t.decode("ClientDeviceRebootResponse", &body)
	return body, err
}

// FromClientDeviceRebootResponse encodes the ClientDeviceRebootResponse into the RPCPayload.
func (t *RPCPayload) FromClientDeviceRebootResponse(v ClientDeviceRebootResponse) error {
	return t.encode("ClientDeviceRebootResponse", v)
}

// AsClientWifiStatusGetRequest decodes the RPCPayload as a ClientWifiStatusGetRequest.
func (t RPCPayload) AsClientWifiStatusGetRequest() (ClientWifiStatusGetRequest, error) {
	var body ClientWifiStatusGetRequest
	err := t.decode("ClientWifiStatusGetRequest", &body)
	return body, err
}

// FromClientWifiStatusGetRequest encodes the ClientWifiStatusGetRequest into the RPCPayload.
func (t *RPCPayload) FromClientWifiStatusGetRequest(v ClientWifiStatusGetRequest) error {
	return t.encode("ClientWifiStatusGetRequest", v)
}

// AsClientWifiStatusGetResponse decodes the RPCPayload as a ClientWifiStatusGetResponse.
func (t RPCPayload) AsClientWifiStatusGetResponse() (ClientWifiStatusGetResponse, error) {
	var body ClientWifiStatusGetResponse
	err := t.decode("ClientWifiStatusGetResponse", &body)
	return body, err
}

// FromClientWifiStatusGetResponse encodes the ClientWifiStatusGetResponse into the RPCPayload.
func (t *RPCPayload) FromClientWifiStatusGetResponse(v ClientWifiStatusGetResponse) error {
	return t.encode("ClientWifiStatusGetResponse", v)
}

// AsClientWifiSavedListRequest decodes the RPCPayload as a ClientWifiSavedListRequest.
func (t RPCPayload) AsClientWifiSavedListRequest() (ClientWifiSavedListRequest, error) {
	var body ClientWifiSavedListRequest
	err := t.decode("ClientWifiSavedListRequest", &body)
	return body, err
}

// FromClientWifiSavedListRequest encodes the ClientWifiSavedListRequest into the RPCPayload.
func (t *RPCPayload) FromClientWifiSavedListRequest(v ClientWifiSavedListRequest) error {
	return t.encode("ClientWifiSavedListRequest", v)
}

// AsClientWifiSavedListResponse decodes the RPCPayload as a ClientWifiSavedListResponse.
func (t RPCPayload) AsClientWifiSavedListResponse() (ClientWifiSavedListResponse, error) {
	var body ClientWifiSavedListResponse
	err := t.decode("ClientWifiSavedListResponse", &body)
	return body, err
}

// FromClientWifiSavedListResponse encodes the ClientWifiSavedListResponse into the RPCPayload.
func (t *RPCPayload) FromClientWifiSavedListResponse(v ClientWifiSavedListResponse) error {
	return t.encode("ClientWifiSavedListResponse", v)
}

// AsClientWifiSavedForgetRequest decodes the RPCPayload as a ClientWifiSavedForgetRequest.
func (t RPCPayload) AsClientWifiSavedForgetRequest() (ClientWifiSavedForgetRequest, error) {
	var body ClientWifiSavedForgetRequest
	err := t.decode("ClientWifiSavedForgetRequest", &body)
	return body, err
}

// FromClientWifiSavedForgetRequest encodes the ClientWifiSavedForgetRequest into the RPCPayload.
func (t *RPCPayload) FromClientWifiSavedForgetRequest(v ClientWifiSavedForgetRequest) error {
	return t.encode("ClientWifiSavedForgetRequest", v)
}

// AsClientWifiSavedForgetResponse decodes the RPCPayload as a ClientWifiSavedForgetResponse.
func (t RPCPayload) AsClientWifiSavedForgetResponse() (ClientWifiSavedForgetResponse, error) {
	var body ClientWifiSavedForgetResponse
	err := t.decode("ClientWifiSavedForgetResponse", &body)
	return body, err
}

// FromClientWifiSavedForgetResponse encodes the ClientWifiSavedForgetResponse into the RPCPayload.
func (t *RPCPayload) FromClientWifiSavedForgetResponse(v ClientWifiSavedForgetResponse) error {
	return t.encode("ClientWifiSavedForgetResponse", v)
}

// AsClientWifiScanRequest decodes the RPCPayload as a ClientWifiScanRequest.
func (t RPCPayload) AsClientWifiScanRequest() (ClientWifiScanRequest, error) {
	var body ClientWifiScanRequest
	err := t.decode("ClientWifiScanRequest", &body)
	return body, err
}

// FromClientWifiScanRequest encodes the ClientWifiScanRequest into the RPCPayload.
func (t *RPCPayload) FromClientWifiScanRequest(v ClientWifiScanRequest) error {
	return t.encode("ClientWifiScanRequest", v)
}

// AsClientWifiScanResponse decodes the RPCPayload as a ClientWifiScanResponse.
func (t RPCPayload) AsClientWifiScanResponse() (ClientWifiScanResponse, error) {
	var body ClientWifiScanResponse
	err := t.decode("ClientWifiScanResponse", &body)
	return body, err
}

// FromClientWifiScanResponse encodes the ClientWifiScanResponse into the RPCPayload.
func (t *RPCPayload) FromClientWifiScanResponse(v ClientWifiScanResponse) error {
	return t.encode("ClientWifiScanResponse", v)
}

// AsClientWifiConnectRequest decodes the RPCPayload as a ClientWifiConnectRequest.
func (t RPCPayload) AsClientWifiConnectRequest() (ClientWifiConnectRequest, error) {
	var body ClientWifiConnectRequest
	err := t.decode("ClientWifiConnectRequest", &body)
	return body, err
}

// FromClientWifiConnectRequest encodes the ClientWifiConnectRequest into the RPCPayload.
func (t *RPCPayload) FromClientWifiConnectRequest(v ClientWifiConnectRequest) error {
	return t.encode("ClientWifiConnectRequest", v)
}

// AsClientWifiConnectResponse decodes the RPCPayload as a ClientWifiConnectResponse.
func (t RPCPayload) AsClientWifiConnectResponse() (ClientWifiConnectResponse, error) {
	var body ClientWifiConnectResponse
	err := t.decode("ClientWifiConnectResponse", &body)
	return body, err
}

// FromClientWifiConnectResponse encodes the ClientWifiConnectResponse into the RPCPayload.
func (t *RPCPayload) FromClientWifiConnectResponse(v ClientWifiConnectResponse) error {
	return t.encode("ClientWifiConnectResponse", v)
}
