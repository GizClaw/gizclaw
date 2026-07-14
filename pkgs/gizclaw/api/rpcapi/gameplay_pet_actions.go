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

type PetActionEffectSpec struct {
	AttrDeltaLife *PetLife `json:"attr_delta_life,omitempty"`
	PetExpDelta   *int64   `json:"pet_exp_delta,omitempty"`
}

type PetAction struct {
	Id           string               `json:"id"`
	Cost         int64                `json:"cost"`
	Effect       *PetActionEffectSpec `json:"effect,omitempty"`
	VisualClipId *string              `json:"visual_clip_id,omitempty"`
	PixaClipName *string              `json:"pixa_clip_name,omitempty"`
}

type PetActionI18nText struct {
	Name string `json:"name"`
}

type PetActionsI18nCatalog struct {
	Actions map[string]PetActionI18nText `json:"actions"`
}

type PetActionsI18n map[string]PetActionsI18nCatalog

type PetActions struct {
	PetId           string         `json:"pet_id"`
	PetdefId        string         `json:"petdef_id"`
	DefaultLocale   string         `json:"default_locale"`
	Actions         []PetAction    `json:"actions"`
	I18n            PetActionsI18n `json:"i18n"`
	PetdefUpdatedAt string         `json:"petdef_updated_at"`
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
