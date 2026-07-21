package rpcapi

type SpeechTranscribeRequest struct {
	ModelAlias  string  `json:"model_alias"`
	ContentType string  `json:"content_type"`
	Language    *string `json:"language,omitempty"`
}

type SpeechTranscribeResponse struct {
	Transcript string `json:"transcript"`
}

type SpeechSynthesizeRequest struct {
	VoiceAlias           string   `json:"voice_alias"`
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
