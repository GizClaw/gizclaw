package rpcapi

// PetPixaDownloadRequest defines model for PetPixaDownloadRequest.
type PetPixaDownloadRequest struct {
	PetId string `json:"pet_id"`
}

// PetPixaDownloadResponse defines model for PetPixaDownloadResponse.
type PetPixaDownloadResponse struct {
	PetId     string  `json:"pet_id"`
	PetdefId  string  `json:"petdef_id"`
	PixaPath  *string `json:"pixa_path,omitempty"`
	SizeBytes int64   `json:"size_bytes"`
}

type PetVisualBindings struct {
	Feed  string  `json:"feed"`
	Bathe string  `json:"bathe"`
	Play  string  `json:"play"`
	Heal  string  `json:"heal"`
	Idle  string  `json:"idle"`
	Sick  string  `json:"sick"`
	Dead  string  `json:"dead"`
	Sleep *string `json:"sleep,omitempty"`
}

type PetActions struct {
	PetId           string            `json:"pet_id"`
	PetdefId        string            `json:"petdef_id"`
	Bindings        PetVisualBindings `json:"bindings"`
	ClipNames       map[string]string `json:"clip_names"`
	PetdefUpdatedAt string            `json:"petdef_updated_at"`
}

type ServerPetPixaDownloadRequest = PetPixaDownloadRequest
type ServerPetPixaDownloadResponse = PetPixaDownloadResponse
type ServerPetActionsGetRequest = PetGetRequest
type ServerPetActionsGetResponse = PetActions

func (t RPCPayload) AsServerPetPixaDownloadRequest() (ServerPetPixaDownloadRequest, error) {
	var body ServerPetPixaDownloadRequest
	err := t.decode("ServerPetPixaDownloadRequest", &body)
	return body, err
}

func (t *RPCPayload) FromServerPetPixaDownloadRequest(v ServerPetPixaDownloadRequest) error {
	return t.encode("ServerPetPixaDownloadRequest", v)
}

func (t *RPCPayload) MergeServerPetPixaDownloadRequest(v ServerPetPixaDownloadRequest) error {
	return t.merge("ServerPetPixaDownloadRequest", v)
}

func (t RPCPayload) AsServerPetPixaDownloadResponse() (ServerPetPixaDownloadResponse, error) {
	var body ServerPetPixaDownloadResponse
	err := t.decode("ServerPetPixaDownloadResponse", &body)
	return body, err
}

func (t *RPCPayload) FromServerPetPixaDownloadResponse(v ServerPetPixaDownloadResponse) error {
	return t.encode("ServerPetPixaDownloadResponse", v)
}

func (t *RPCPayload) MergeServerPetPixaDownloadResponse(v ServerPetPixaDownloadResponse) error {
	return t.merge("ServerPetPixaDownloadResponse", v)
}

func (t RPCPayload) AsServerPetActionsGetRequest() (ServerPetActionsGetRequest, error) {
	var body ServerPetActionsGetRequest
	err := t.decode("ServerPetActionsGetRequest", &body)
	return body, err
}

func (t *RPCPayload) FromServerPetActionsGetRequest(v ServerPetActionsGetRequest) error {
	return t.encode("ServerPetActionsGetRequest", v)
}

func (t *RPCPayload) MergeServerPetActionsGetRequest(v ServerPetActionsGetRequest) error {
	return t.merge("ServerPetActionsGetRequest", v)
}

func (t RPCPayload) AsServerPetActionsGetResponse() (ServerPetActionsGetResponse, error) {
	var body ServerPetActionsGetResponse
	err := t.decode("ServerPetActionsGetResponse", &body)
	return body, err
}

func (t *RPCPayload) FromServerPetActionsGetResponse(v ServerPetActionsGetResponse) error {
	return t.encode("ServerPetActionsGetResponse", v)
}

func (t *RPCPayload) MergeServerPetActionsGetResponse(v ServerPetActionsGetResponse) error {
	return t.merge("ServerPetActionsGetResponse", v)
}
