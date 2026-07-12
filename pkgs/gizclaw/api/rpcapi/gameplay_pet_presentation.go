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

type PetPresentation struct {
	PetId           string                      `json:"pet_id"`
	PetdefId        string                      `json:"petdef_id"`
	DefaultLocale   string                      `json:"default_locale"`
	Attr            PetPresentationAttrSpec     `json:"attr"`
	Drive           PetPresentationDriveSpec    `json:"drive"`
	PixaMetadata    PetPresentationPixaMetadata `json:"pixa_metadata"`
	I18n            PetPresentationI18nSpec     `json:"i18n"`
	PixaPath        *string                     `json:"pixa_path,omitempty"`
	PetdefUpdatedAt string                      `json:"petdef_updated_at"`
}

type PetPresentationActionEffectSpec struct {
	AttrDelta   *PetPresentationAttrDelta `json:"attr_delta,omitempty"`
	PetExpDelta *int64                    `json:"pet_exp_delta,omitempty"`
}

type PetPresentationActionSpec struct {
	Id           string                           `json:"id"`
	Cost         int64                            `json:"cost"`
	Effect       *PetPresentationActionEffectSpec `json:"effect,omitempty"`
	VisualClipId *string                          `json:"visual_clip_id,omitempty"`
	Icon         *string                          `json:"icon,omitempty"`
}

type PetPresentationAttrDelta struct {
	Life *PetLife `json:"life,omitempty"`
}

type PetPresentationAttrGroupSpec map[string]PetPresentationAttrValueSpec

type PetPresentationAttrSpec struct {
	Life        PetPresentationAttrGroupSpec `json:"life"`
	Progression PetPresentationAttrGroupSpec `json:"progression"`
}

type PetPresentationAttrValueSpec struct {
	Initial int64 `json:"initial"`
}

type PetPresentationDriveSpec struct {
	Actions []PetPresentationActionSpec `json:"actions"`
}

type PetPresentationI18nAttrGroup map[string]PetPresentationI18nDisplayText

type PetPresentationI18nAttrSpec struct {
	Life        *PetPresentationI18nAttrGroup `json:"life,omitempty"`
	Progression *PetPresentationI18nAttrGroup `json:"progression,omitempty"`
}

type PetPresentationI18nCatalog struct {
	DisplayName *string                       `json:"display_name,omitempty"`
	Description *string                       `json:"description,omitempty"`
	Attr        *PetPresentationI18nAttrSpec  `json:"attr,omitempty"`
	Drive       *PetPresentationI18nDriveSpec `json:"drive,omitempty"`
}

type PetPresentationI18nDisplayText struct {
	DisplayName string `json:"display_name"`
}

type PetPresentationI18nDriveSpec struct {
	Actions map[string]PetPresentationI18nDisplayText `json:"actions"`
}

type PetPresentationI18nSpec map[string]PetPresentationI18nCatalog

type PetPresentationPixaCanvasMetadata struct {
	Width  int64 `json:"width"`
	Height int64 `json:"height"`
}

type PetPresentationPixaClipMetadata struct {
	Id           string  `json:"id"`
	ActionId     *string `json:"action_id,omitempty"`
	PixaClipName string  `json:"pixa_clip_name"`
}

type PetPresentationPixaMetadata struct {
	Version string                            `json:"version"`
	Canvas  PetPresentationPixaCanvasMetadata `json:"canvas"`
	Clips   []PetPresentationPixaClipMetadata `json:"clips"`
}

type ServerPetPixaDownloadRequest = PetPixaDownloadRequest
type ServerPetPixaDownloadResponse = PetPixaDownloadResponse
type ServerPetPresentationGetRequest = PetGetRequest
type ServerPetPresentationGetResponse = PetPresentation

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

func (t RPCPayload) AsServerPetPresentationGetRequest() (ServerPetPresentationGetRequest, error) {
	var body ServerPetPresentationGetRequest
	err := t.decode("ServerPetPresentationGetRequest", &body)
	return body, err
}

func (t *RPCPayload) FromServerPetPresentationGetRequest(v ServerPetPresentationGetRequest) error {
	return t.encode("ServerPetPresentationGetRequest", v)
}

func (t *RPCPayload) MergeServerPetPresentationGetRequest(v ServerPetPresentationGetRequest) error {
	return t.merge("ServerPetPresentationGetRequest", v)
}

func (t RPCPayload) AsServerPetPresentationGetResponse() (ServerPetPresentationGetResponse, error) {
	var body ServerPetPresentationGetResponse
	err := t.decode("ServerPetPresentationGetResponse", &body)
	return body, err
}

func (t *RPCPayload) FromServerPetPresentationGetResponse(v ServerPetPresentationGetResponse) error {
	return t.encode("ServerPetPresentationGetResponse", v)
}

func (t *RPCPayload) MergeServerPetPresentationGetResponse(v ServerPetPresentationGetResponse) error {
	return t.merge("ServerPetPresentationGetResponse", v)
}
