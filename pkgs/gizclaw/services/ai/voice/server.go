package voice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var (
	voicesRoot                  = kv.Key{"by-id"}
	voicesBySourceRoot          = kv.Key{"by-source"}
	voicesByProviderRoot        = kv.Key{"by-provider"}
	voicesByProviderVoiceIDRoot = kv.Key{"by-provider-voice-id"}
)

const (
	defaultListLimit = 50
	maxListLimit     = 200
)

type Server struct {
	Store kv.Store
	Now   func() time.Time
}

type VoiceAdminService interface {
	CreateVoice(context.Context, adminhttp.CreateVoiceRequestObject) (adminhttp.CreateVoiceResponseObject, error)
	ListVoices(context.Context, adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error)
	DeleteVoice(context.Context, adminhttp.DeleteVoiceRequestObject) (adminhttp.DeleteVoiceResponseObject, error)
	GetVoice(context.Context, adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error)
	PutVoice(context.Context, adminhttp.PutVoiceRequestObject) (adminhttp.PutVoiceResponseObject, error)
}

var _ VoiceAdminService = (*Server)(nil)

type Filters struct {
	Source       *string
	ProviderKind *string
	ProviderName *string
}

func (s *Server) CreateVoice(ctx context.Context, request adminhttp.CreateVoiceRequestObject) (adminhttp.CreateVoiceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.CreateVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateVoice400JSONResponse(apitypes.NewErrorResponse("INVALID_VOICE", "request body required")), nil
	}
	voice, err := normalizeVoiceUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.CreateVoice400JSONResponse(apitypes.NewErrorResponse("INVALID_VOICE", err.Error())), nil
	}
	if _, err := store.Get(ctx, voiceKey(string(voice.Id))); err == nil {
		return adminhttp.CreateVoice409JSONResponse(apitypes.NewErrorResponse("VOICE_ALREADY_EXISTS", fmt.Sprintf("voice %q already exists", voice.Id))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	voice.CreatedAt = now
	voice.UpdatedAt = now
	if err := Write(ctx, store, voice, nil); err != nil {
		return adminhttp.CreateVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreateVoice200JSONResponse(voice), nil
}

func (s *Server) ListVoices(ctx context.Context, request adminhttp.ListVoicesRequestObject) (adminhttp.ListVoicesResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.ListVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	filters := Filters{}
	if request.Params.Source != nil {
		source := strings.TrimSpace(string(*request.Params.Source))
		if source != "" {
			filters.Source = &source
		}
	}
	if request.Params.ProviderKind != nil {
		kind := strings.TrimSpace(string(*request.Params.ProviderKind))
		if kind != "" {
			filters.ProviderKind = &kind
		}
	}
	if request.Params.ProviderName != nil {
		name := strings.TrimSpace(string(*request.Params.ProviderName))
		if name != "" {
			filters.ProviderName = &name
		}
	}
	items, hasNext, nextCursor, err := listPage(ctx, store, filters, cursor, limit)
	if err != nil {
		return adminhttp.ListVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListVoices200JSONResponse(adminhttp.VoiceList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) DeleteVoice(ctx context.Context, request adminhttp.DeleteVoiceRequestObject) (adminhttp.DeleteVoiceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.DeleteVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := url.PathUnescape(string(request.Id))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	voice, err := Get(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteVoice404JSONResponse(apitypes.NewErrorResponse("VOICE_NOT_FOUND", fmt.Sprintf("voice %q not found", id))), nil
		}
		return adminhttp.DeleteVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := Delete(ctx, store, voice); err != nil {
		return adminhttp.DeleteVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteVoice200JSONResponse(voice), nil
}

func (s *Server) GetVoice(ctx context.Context, request adminhttp.GetVoiceRequestObject) (adminhttp.GetVoiceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.GetVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	id, err := url.PathUnescape(string(request.Id))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	voice, err := Get(ctx, store, id)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetVoice404JSONResponse(apitypes.NewErrorResponse("VOICE_NOT_FOUND", fmt.Sprintf("voice %q not found", id))), nil
		}
		return adminhttp.GetVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetVoice200JSONResponse(voice), nil
}

func (s *Server) PutVoice(ctx context.Context, request adminhttp.PutVoiceRequestObject) (adminhttp.PutVoiceResponseObject, error) {
	store, err := s.store()
	if err != nil {
		return adminhttp.PutVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutVoice400JSONResponse(apitypes.NewErrorResponse("INVALID_VOICE", "request body required")), nil
	}
	id, err := url.PathUnescape(string(request.Id))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	voice, err := normalizeVoiceUpsert(*request.Body, id)
	if err != nil {
		return adminhttp.PutVoice400JSONResponse(apitypes.NewErrorResponse("INVALID_VOICE", err.Error())), nil
	}
	previous, err := Get(ctx, store, id)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	voice.CreatedAt = now
	voice.UpdatedAt = now
	var previousPtr *apitypes.Voice
	if err == nil {
		if previous.Source == apitypes.VoiceSourceSync {
			return adminhttp.PutVoice409JSONResponse(apitypes.NewErrorResponse("SYNC_VOICE_READ_ONLY", fmt.Sprintf("voice %q has source sync and cannot be modified via API", previous.Id))), nil
		}
		voice.CreatedAt = previous.CreatedAt
		voice.SyncedAt = cloneTime(previous.SyncedAt)
		previousCopy := previous
		previousPtr = &previousCopy
	}
	if err := Write(ctx, store, voice, previousPtr); err != nil {
		return adminhttp.PutVoice500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutVoice200JSONResponse(voice), nil
}

func ProviderData(kind apitypes.VoiceProviderKind, values map[string]any) *apitypes.VoiceProviderData {
	clean := make(map[string]any, len(values))
	for key, value := range values {
		switch typed := value.(type) {
		case nil:
			continue
		case string:
			if strings.TrimSpace(typed) == "" {
				continue
			}
		}
		clean[key] = value
	}
	if len(clean) == 0 {
		return nil
	}
	raw := rawMapFromProviderData(clean)
	out := apitypes.VoiceProviderData{}
	var err error
	switch kind {
	case apitypes.VoiceProviderKindOpenaiTenant:
		err = out.FromOpenAITenantVoiceProviderData(apitypes.OpenAITenantVoiceProviderData{
			VoiceId: stringPtrFromProviderData(clean, "voice_id"),
			Raw:     raw,
		})
	case apitypes.VoiceProviderKindGeminiTenant:
		err = out.FromGeminiTenantVoiceProviderData(apitypes.GeminiTenantVoiceProviderData{
			VoiceId: stringPtrFromProviderData(clean, "voice_id"),
			Raw:     raw,
		})
	case apitypes.VoiceProviderKindDashscopeTenant:
		err = out.FromDashScopeTenantVoiceProviderData(apitypes.DashScopeTenantVoiceProviderData{
			VoiceId: stringPtrFromProviderData(clean, "voice_id"),
			Raw:     raw,
		})
	case apitypes.VoiceProviderKindMinimaxTenant:
		err = out.FromMiniMaxTenantVoiceProviderData(apitypes.MiniMaxTenantVoiceProviderData{
			VoiceId:    stringPtrFromProviderData(clean, "voice_id"),
			VoiceType:  stringPtrFromProviderData(clean, "voice_type"),
			Model:      stringPtrFromProviderData(clean, "model"),
			Format:     stringPtrFromProviderData(clean, "format"),
			SampleRate: intPtrFromProviderData(clean, "sample_rate"),
			Raw:        raw,
		})
	case apitypes.VoiceProviderKindVolcTenant:
		err = out.FromVolcTenantVoiceProviderData(apitypes.VolcTenantVoiceProviderData{
			ResourceId: stringPtrFromProviderData(clean, "resource_id"),
			VoiceId:    stringPtrFromProviderData(clean, "voice_id"),
			State:      stringPtrFromProviderData(clean, "state"),
			Status:     stringPtrFromProviderData(clean, "status"),
			Raw:        raw,
		})
	default:
		return nil
	}
	if err != nil {
		return nil
	}
	return &out
}

func ProviderDataString(voice apitypes.Voice, key string) string {
	if voice.ProviderData == nil {
		return ""
	}
	switch voice.Provider.Kind {
	case apitypes.VoiceProviderKindOpenaiTenant:
		data, err := voice.ProviderData.AsOpenAITenantVoiceProviderData()
		if err != nil {
			return ""
		}
		if key == "voice_id" {
			return stringPtrValue(data.VoiceId)
		}
		return rawProviderDataString(data.Raw, key)
	case apitypes.VoiceProviderKindGeminiTenant:
		data, err := voice.ProviderData.AsGeminiTenantVoiceProviderData()
		if err != nil {
			return ""
		}
		if key == "voice_id" {
			return stringPtrValue(data.VoiceId)
		}
		return rawProviderDataString(data.Raw, key)
	case apitypes.VoiceProviderKindDashscopeTenant:
		data, err := voice.ProviderData.AsDashScopeTenantVoiceProviderData()
		if err != nil {
			return ""
		}
		if key == "voice_id" {
			return stringPtrValue(data.VoiceId)
		}
		return rawProviderDataString(data.Raw, key)
	case apitypes.VoiceProviderKindMinimaxTenant:
		data, err := voice.ProviderData.AsMiniMaxTenantVoiceProviderData()
		if err != nil {
			return ""
		}
		switch key {
		case "voice_id":
			return stringPtrValue(data.VoiceId)
		case "voice_type":
			return stringPtrValue(data.VoiceType)
		case "model":
			return stringPtrValue(data.Model)
		case "format":
			return stringPtrValue(data.Format)
		}
		return rawProviderDataString(data.Raw, key)
	case apitypes.VoiceProviderKindVolcTenant:
		data, err := voice.ProviderData.AsVolcTenantVoiceProviderData()
		if err != nil {
			return ""
		}
		switch key {
		case "resource_id":
			return stringPtrValue(data.ResourceId)
		case "voice_id":
			return stringPtrValue(data.VoiceId)
		case "state":
			return stringPtrValue(data.State)
		case "status":
			return stringPtrValue(data.Status)
		}
		return rawProviderDataString(data.Raw, key)
	default:
		return ""
	}
}

func RawMapValue(in *map[string]any) any {
	if in == nil {
		return nil
	}
	return *in
}

func StableID(kind apitypes.VoiceProviderKind, name string, providerVoiceID string) string {
	return strings.Join([]string{string(kind), string(name), providerVoiceID}, ":")
}

func SemanticEqual(left, right apitypes.Voice) bool {
	return equalStringPtr(left.Description, right.Description) &&
		equalStringPtr(left.Name, right.Name) &&
		left.Provider.Kind == right.Provider.Kind &&
		left.Provider.Name == right.Provider.Name &&
		left.Source == right.Source &&
		providerDataEqual(left.ProviderData, right.ProviderData)
}

func ListProvider(ctx context.Context, store kv.Store, kind apitypes.VoiceProviderKind, name string) ([]apitypes.Voice, error) {
	prefix := voiceByProviderPrefix(string(kind), string(name))
	items := make([]apitypes.Voice, 0)
	for entry, err := range store.List(ctx, prefix) {
		if err != nil {
			return nil, err
		}
		if len(entry.Key) == 0 {
			continue
		}
		id := entry.Key[len(entry.Key)-1]
		voice, err := Get(ctx, store, unescapeStoreSegment(id))
		if err != nil {
			if errors.Is(err, kv.ErrNotFound) {
				continue
			}
			return nil, err
		}
		items = append(items, voice)
	}
	return items, nil
}

func Write(ctx context.Context, store kv.Store, voice apitypes.Voice, previous *apitypes.Voice) error {
	data, err := json.Marshal(voice)
	if err != nil {
		return fmt.Errorf("voice: encode voice %s: %w", voice.Id, err)
	}
	var deletes []kv.Key
	if previous != nil {
		deletes = staleIndexKeys(*previous, voice)
	}
	if len(deletes) > 0 {
		if err := store.BatchDelete(ctx, deletes); err != nil {
			return fmt.Errorf("voice: delete stale indexes %s: %w", voice.Id, err)
		}
	}
	entries := []kv.Entry{
		{Key: voiceKey(string(voice.Id)), Value: data},
		{Key: voiceBySourceKey(string(voice.Source), string(voice.Id)), Value: []byte{}},
		{Key: voiceByProviderKey(string(voice.Provider.Kind), string(voice.Provider.Name), string(voice.Id)), Value: []byte{}},
	}
	if providerVoiceID := ProviderDataString(voice, "voice_id"); providerVoiceID != "" {
		entries = append(entries, kv.Entry{
			Key:   voiceByProviderVoiceIDKey(string(voice.Provider.Kind), string(voice.Provider.Name), providerVoiceID),
			Value: []byte(string(voice.Id)),
		})
	}
	if err := store.BatchSet(ctx, entries); err != nil {
		return fmt.Errorf("voice: write voice %s: %w", voice.Id, err)
	}
	return nil
}

func Delete(ctx context.Context, store kv.Store, voice apitypes.Voice) error {
	keys := []kv.Key{
		voiceKey(string(voice.Id)),
		voiceBySourceKey(string(voice.Source), string(voice.Id)),
		voiceByProviderKey(string(voice.Provider.Kind), string(voice.Provider.Name), string(voice.Id)),
	}
	if providerVoiceID := ProviderDataString(voice, "voice_id"); providerVoiceID != "" {
		keys = append(keys, voiceByProviderVoiceIDKey(
			string(voice.Provider.Kind),
			string(voice.Provider.Name),
			providerVoiceID,
		))
	}
	if err := store.BatchDelete(ctx, keys); err != nil {
		return fmt.Errorf("voice: delete voice %s: %w", voice.Id, err)
	}
	return nil
}

func Get(ctx context.Context, store kv.Store, id string) (apitypes.Voice, error) {
	data, err := store.Get(ctx, voiceKey(id))
	if err != nil {
		return apitypes.Voice{}, err
	}
	var voice apitypes.Voice
	if err := Decode(data, &voice); err != nil {
		return apitypes.Voice{}, fmt.Errorf("voice: decode voice %s: %w", id, err)
	}
	return voice, nil
}

func Decode(data []byte, out *apitypes.Voice) error {
	var decoded struct {
		apitypes.Voice
		ProviderVoiceID   *string         `json:"provider_voice_id,omitempty"`
		ProviderVoiceType *string         `json:"provider_voice_type,omitempty"`
		Raw               *map[string]any `json:"raw,omitempty"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	voice := decoded.Voice
	if voice.ProviderData == nil {
		values := map[string]any{
			"raw":        RawMapValue(decoded.Raw),
			"voice_id":   stringPtrValue(decoded.ProviderVoiceID),
			"voice_type": stringPtrValue(decoded.ProviderVoiceType),
		}
		voice.ProviderData = ProviderData(voice.Provider.Kind, values)
	}
	*out = voice
	return nil
}

func (s *Server) store() (kv.Store, error) {
	if s == nil || s.Store == nil {
		return nil, errors.New("voice: nil store")
	}
	return s.Store, nil
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func normalizeVoiceUpsert(in adminhttp.VoiceUpsert, expectedID string) (apitypes.Voice, error) {
	id := strings.TrimSpace(string(in.Id))
	if id == "" {
		return apitypes.Voice{}, errors.New("id is required")
	}
	if expectedID != "" && id != expectedID {
		return apitypes.Voice{}, fmt.Errorf("id %q must match path id %q", id, expectedID)
	}
	source := apitypes.VoiceSource(strings.TrimSpace(string(in.Source)))
	if source == "" {
		return apitypes.Voice{}, errors.New("source is required")
	}
	if !source.Valid() {
		return apitypes.Voice{}, fmt.Errorf("unsupported source %q", source)
	}
	if source == apitypes.VoiceSourceSync {
		return apitypes.Voice{}, errors.New("voices with source sync cannot be created or updated via API")
	}
	providerKind := strings.TrimSpace(string(in.Provider.Kind))
	if providerKind == "" {
		return apitypes.Voice{}, errors.New("provider.kind is required")
	}
	providerName := strings.TrimSpace(string(in.Provider.Name))
	if providerName == "" {
		return apitypes.Voice{}, errors.New("provider.name is required")
	}
	voice := apitypes.Voice{
		Id: string(id),
		Provider: apitypes.VoiceProvider{
			Kind: apitypes.VoiceProviderKind(providerKind),
			Name: string(providerName),
		},
		Source: source,
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name != "" {
			voice.Name = &name
		}
	}
	if in.Description != nil {
		description := strings.TrimSpace(*in.Description)
		if description != "" {
			voice.Description = &description
		}
	}
	if in.ProviderData != nil {
		voice.ProviderData = cloneProviderData(in.ProviderData)
	}
	return voice, nil
}

func listPage(ctx context.Context, store kv.Store, filters Filters, cursor string, limit int) ([]apitypes.Voice, bool, *string, error) {
	prefix := voicesRoot
	switch {
	case filters.ProviderKind != nil && filters.ProviderName != nil:
		prefix = voiceByProviderPrefix(*filters.ProviderKind, *filters.ProviderName)
	case filters.Source != nil:
		prefix = voiceBySourcePrefix(*filters.Source)
	}
	items := make([]apitypes.Voice, 0, limit+1)
	for entry, err := range store.List(ctx, prefix) {
		if err != nil {
			return nil, false, nil, err
		}
		if len(entry.Key) == 0 {
			continue
		}
		lastSegment := entry.Key[len(entry.Key)-1]
		if cursor != "" && lastSegment <= cursor {
			continue
		}
		var voice apitypes.Voice
		if prefix.String() == voicesRoot.String() {
			if err := Decode(entry.Value, &voice); err != nil {
				return nil, false, nil, fmt.Errorf("voice: decode voice list %s: %w", entry.Key.String(), err)
			}
		} else {
			decodedID := unescapeStoreSegment(lastSegment)
			var err error
			voice, err = Get(ctx, store, decodedID)
			if err != nil {
				if errors.Is(err, kv.ErrNotFound) {
					continue
				}
				return nil, false, nil, err
			}
		}
		if !matchesFilters(voice, filters) {
			continue
		}
		items = append(items, voice)
		if len(items) >= limit+1 {
			break
		}
	}
	if len(items) == 0 {
		return []apitypes.Voice{}, false, nil, nil
	}
	hasNext := len(items) > limit
	if !hasNext {
		return items, false, nil, nil
	}
	page := items[:limit]
	next := escapeStoreSegment(string(page[len(page)-1].Id))
	return page, true, &next, nil
}

func matchesFilters(voice apitypes.Voice, filters Filters) bool {
	if filters.Source != nil && string(voice.Source) != *filters.Source {
		return false
	}
	if filters.ProviderKind != nil && string(voice.Provider.Kind) != *filters.ProviderKind {
		return false
	}
	if filters.ProviderName != nil && string(voice.Provider.Name) != *filters.ProviderName {
		return false
	}
	return true
}

func staleIndexKeys(previous, next apitypes.Voice) []kv.Key {
	var keys []kv.Key
	if previous.Source != next.Source {
		keys = append(keys, voiceBySourceKey(string(previous.Source), string(previous.Id)))
	}
	if previous.Provider.Kind != next.Provider.Kind || previous.Provider.Name != next.Provider.Name {
		keys = append(keys, voiceByProviderKey(string(previous.Provider.Kind), string(previous.Provider.Name), string(previous.Id)))
	}
	previousProviderVoiceID := ProviderDataString(previous, "voice_id")
	if previousProviderVoiceID != "" {
		nextProviderVoiceID := ProviderDataString(next, "voice_id")
		if previous.Provider.Kind != next.Provider.Kind ||
			previous.Provider.Name != next.Provider.Name ||
			previousProviderVoiceID != nextProviderVoiceID {
			keys = append(keys, voiceByProviderVoiceIDKey(
				string(previous.Provider.Kind),
				string(previous.Provider.Name),
				previousProviderVoiceID,
			))
		}
	}
	return keys
}

func providerDataEqual(left, right *apitypes.VoiceProviderData) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	leftJSON, err := left.MarshalJSON()
	if err != nil {
		return false
	}
	rightJSON, err := right.MarshalJSON()
	if err != nil {
		return false
	}
	return string(leftJSON) == string(rightJSON)
}

func providerDataString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func rawProviderDataString(raw *map[string]any, key string) string {
	if raw == nil {
		return ""
	}
	return providerDataString((*raw)[key])
}

func rawMapFromProviderData(values map[string]any) *map[string]any {
	rawValue, ok := values["raw"]
	if !ok || rawValue == nil {
		return nil
	}
	switch typed := rawValue.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		if len(out) == 0 {
			return nil
		}
		return &out
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[key] = value
		}
		if len(out) == 0 {
			return nil
		}
		return &out
	default:
		return nil
	}
}

func stringPtrFromProviderData(values map[string]any, key string) *string {
	value := providerDataString(values[key])
	if value == "" {
		return nil
	}
	return &value
}

func intPtrFromProviderData(values map[string]any, key string) *int {
	switch typed := values[key].(type) {
	case int:
		return &typed
	case int8:
		value := int(typed)
		return &value
	case int16:
		value := int(typed)
		return &value
	case int32:
		value := int(typed)
		return &value
	case int64:
		value := int(typed)
		return &value
	case uint:
		value := int(typed)
		return &value
	case uint8:
		value := int(typed)
		return &value
	case uint16:
		value := int(typed)
		return &value
	case uint32:
		value := int(typed)
		return &value
	case uint64:
		value := int(typed)
		return &value
	case float64:
		value := int(typed)
		if typed == float64(value) {
			return &value
		}
	case float32:
		value := int(typed)
		if typed == float32(value) {
			return &value
		}
	}
	return nil
}

func stringPtrValue(in *string) string {
	if in == nil {
		return ""
	}
	return strings.TrimSpace(*in)
}

func cloneProviderData(in *apitypes.VoiceProviderData) *apitypes.VoiceProviderData {
	if in == nil {
		return nil
	}
	data, err := in.MarshalJSON()
	if err != nil {
		return nil
	}
	var out apitypes.VoiceProviderData
	if err := out.UnmarshalJSON(data); err != nil {
		return nil
	}
	return &out
}

func cloneTime(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func equalStringPtr(left, right *string) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return *left == *right
	}
}

func normalizeListParams(cursor *string, limit *int32) (string, int) {
	normalizedLimit := defaultListLimit
	if limit != nil && int(*limit) > 0 {
		normalizedLimit = int(*limit)
	}
	if normalizedLimit > maxListLimit {
		normalizedLimit = maxListLimit
	}
	normalizedCursor := ""
	if cursor != nil {
		normalizedCursor = strings.TrimSpace(string(*cursor))
	}
	return normalizedCursor, normalizedLimit
}

func voiceKey(id string) kv.Key {
	return append(append(kv.Key{}, voicesRoot...), escapeStoreSegment(id))
}

func voiceBySourcePrefix(source string) kv.Key {
	return append(append(kv.Key{}, voicesBySourceRoot...), escapeStoreSegment(source))
}

func voiceBySourceKey(source, id string) kv.Key {
	return append(voiceBySourcePrefix(source), escapeStoreSegment(id))
}

func voiceByProviderPrefix(kind, name string) kv.Key {
	prefix := append(append(kv.Key{}, voicesByProviderRoot...), escapeStoreSegment(kind))
	return append(prefix, escapeStoreSegment(name))
}

func voiceByProviderKey(kind, name, id string) kv.Key {
	return append(voiceByProviderPrefix(kind, name), escapeStoreSegment(id))
}

func voiceByProviderVoiceIDKey(kind, name, providerVoiceID string) kv.Key {
	key := append(append(kv.Key{}, voicesByProviderVoiceIDRoot...), escapeStoreSegment(kind))
	key = append(key, escapeStoreSegment(name))
	return append(key, escapeStoreSegment(providerVoiceID))
}

func escapeStoreSegment(value string) string {
	value = strings.ReplaceAll(value, "%", "%25")
	return strings.ReplaceAll(value, ":", "%3A")
}

func unescapeStoreSegment(value string) string {
	unescaped, err := url.PathUnescape(value)
	if err != nil {
		return value
	}
	return unescaped
}
