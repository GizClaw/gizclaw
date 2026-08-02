package rpcapi

type SpeechTranscribeRequest struct {
	ModelName   string  `json:"model_name"`
	ContentType string  `json:"content_type"`
	Language    *string `json:"language,omitempty"`
}

type SpeechTranscribeResponse struct {
	Transcript string `json:"transcript"`
}

type SpeechExtractRequest struct {
	ASRModelName     string  `json:"asr_model_name"`
	ExtractModelName string  `json:"extract_model_name"`
	ContentType      string  `json:"content_type"`
	Language         *string `json:"language,omitempty"`
	SchemaJSON       string  `json:"schema_json"`
	Instruction      *string `json:"instruction,omitempty"`
}

type SpeechExtractResponse struct {
	Transcript string `json:"transcript"`
	ResultJSON string `json:"result_json"`
}

type SpeechSynthesizeRequest struct {
	VoiceName            string   `json:"voice_name"`
	Text                 string   `json:"text"`
	AcceptedContentTypes []string `json:"accepted_content_types"`
}

type SpeechSynthesizeResponse struct {
	ContentType  string `json:"content_type"`
	SampleRateHz *int32 `json:"sample_rate_hz,omitempty"`
	Channels     *int32 `json:"channels,omitempty"`
}

func (p RPCPayload) AsSpeechTranscribeRequest() (SpeechTranscribeRequest, error) {
	var out SpeechTranscribeRequest
	err := p.decode("SpeechTranscribeRequest", &out)
	return out, err
}

func (p *RPCPayload) FromSpeechTranscribeRequest(value SpeechTranscribeRequest) error {
	return p.encode("SpeechTranscribeRequest", value)
}

func (p RPCPayload) AsSpeechTranscribeResponse() (SpeechTranscribeResponse, error) {
	var out SpeechTranscribeResponse
	err := p.decode("SpeechTranscribeResponse", &out)
	return out, err
}

func (p *RPCPayload) FromSpeechTranscribeResponse(value SpeechTranscribeResponse) error {
	return p.encode("SpeechTranscribeResponse", value)
}

func (p RPCPayload) AsSpeechExtractRequest() (SpeechExtractRequest, error) {
	var out SpeechExtractRequest
	err := p.decode("SpeechExtractRequest", &out)
	return out, err
}

func (p *RPCPayload) FromSpeechExtractRequest(value SpeechExtractRequest) error {
	return p.encode("SpeechExtractRequest", value)
}

func (p RPCPayload) AsSpeechExtractResponse() (SpeechExtractResponse, error) {
	var out SpeechExtractResponse
	err := p.decode("SpeechExtractResponse", &out)
	return out, err
}

func (p *RPCPayload) FromSpeechExtractResponse(value SpeechExtractResponse) error {
	return p.encode("SpeechExtractResponse", value)
}

func (p RPCPayload) AsSpeechSynthesizeRequest() (SpeechSynthesizeRequest, error) {
	var out SpeechSynthesizeRequest
	err := p.decode("SpeechSynthesizeRequest", &out)
	return out, err
}

func (p *RPCPayload) FromSpeechSynthesizeRequest(value SpeechSynthesizeRequest) error {
	return p.encode("SpeechSynthesizeRequest", value)
}

func (p RPCPayload) AsSpeechSynthesizeResponse() (SpeechSynthesizeResponse, error) {
	var out SpeechSynthesizeResponse
	err := p.decode("SpeechSynthesizeResponse", &out)
	return out, err
}

func (p *RPCPayload) FromSpeechSynthesizeResponse(value SpeechSynthesizeResponse) error {
	return p.encode("SpeechSynthesizeResponse", value)
}
