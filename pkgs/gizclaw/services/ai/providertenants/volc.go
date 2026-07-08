package providertenants

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/volcengine/volcengine-go-sdk/service/speechsaasprod"
	"github.com/volcengine/volcengine-go-sdk/volcengine"
	"github.com/volcengine/volcengine-go-sdk/volcengine/credentials"
	"github.com/volcengine/volcengine-go-sdk/volcengine/session"
	"github.com/volcengine/volcengine-go-sdk/volcengine/universal"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	voicecatalog "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/voice"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var volcTenantsRoot = kv.Key{"volc-by-name"}

const (
	defaultVolcRegion         = "cn-beijing"
	volcPublicTTSV1ResourceID = "seed-tts-1.0"
	volcPublicTTSResourceID   = "seed-tts-2.0"
	volcPublicICLResourceID   = "seed-icl-2.0"
	volcProviderKind          = apitypes.VoiceProviderKind("volc-tenant")
)

type VolcSpeakerClient interface {
	ListSpeakersWithContext(context.Context, []string, int32, int32) (*volcSpeakersPage, error)
	ListBigModelTTSTimbresWithContext(context.Context) ([]volcPublicTimbre, error)
	BatchListMegaTTSTrainStatusWithContext(context.Context, string, []string, int32, int32) (*volcMegaTTSTrainStatusPage, error)
}

type VolcSpeakerClientFactory func(context.Context, apitypes.Credential, apitypes.VolcTenant) (VolcSpeakerClient, error)

func (s *Server) ListVolcTenants(ctx context.Context, request adminhttp.ListVolcTenantsRequestObject) (adminhttp.ListVolcTenantsResponseObject, error) {
	store, err := s.volcTenantStore()
	if err != nil {
		return adminhttp.ListVolcTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	cursor, limit := normalizeListParams(request.Params.Cursor, request.Params.Limit)
	items, hasNext, nextCursor, err := listVolcTenantsPage(ctx, store, cursor, limit)
	if err != nil {
		return adminhttp.ListVolcTenants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListVolcTenants200JSONResponse(adminhttp.VolcTenantList{
		HasNext:    hasNext,
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

func (s *Server) CreateVolcTenant(ctx context.Context, request adminhttp.CreateVolcTenantRequestObject) (adminhttp.CreateVolcTenantResponseObject, error) {
	store, err := s.volcTenantStore()
	if err != nil {
		return adminhttp.CreateVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.CreateVolcTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_VOLC_TENANT", "request body required")), nil
	}
	tenant, err := normalizeVolcTenantUpsert(*request.Body, "")
	if err != nil {
		return adminhttp.CreateVolcTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_VOLC_TENANT", err.Error())), nil
	}
	credentialStore, err := s.credentialStore()
	if err != nil {
		return adminhttp.CreateVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := validateVolcTenantReferences(ctx, credentialStore, tenant); err != nil {
		return adminhttp.CreateVolcTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_VOLC_TENANT", err.Error())), nil
	}
	if _, err := store.Get(ctx, volcTenantKey(string(tenant.Name))); err == nil {
		return adminhttp.CreateVolcTenant409JSONResponse(apitypes.NewErrorResponse("VOLC_TENANT_ALREADY_EXISTS", fmt.Sprintf("Volcengine tenant %q already exists", tenant.Name))), nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.CreateVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	if err := writeVolcTenant(ctx, store, tenant); err != nil {
		return adminhttp.CreateVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.CreateVolcTenant200JSONResponse(tenant), nil
}

func (s *Server) DeleteVolcTenant(ctx context.Context, request adminhttp.DeleteVolcTenantRequestObject) (adminhttp.DeleteVolcTenantResponseObject, error) {
	store, err := s.volcTenantStore()
	if err != nil {
		return adminhttp.DeleteVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getVolcTenant(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.DeleteVolcTenant404JSONResponse(apitypes.NewErrorResponse("VOLC_TENANT_NOT_FOUND", fmt.Sprintf("Volcengine tenant %q not found", name))), nil
		}
		return adminhttp.DeleteVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	voiceStore, err := s.voiceStore()
	if err != nil {
		return adminhttp.DeleteVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := deleteVolcTenantVoices(ctx, voiceStore, tenant.Name); err != nil {
		return adminhttp.DeleteVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := store.Delete(ctx, volcTenantKey(string(tenant.Name))); err != nil {
		return adminhttp.DeleteVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteVolcTenant200JSONResponse(tenant), nil
}

func (s *Server) GetVolcTenant(ctx context.Context, request adminhttp.GetVolcTenantRequestObject) (adminhttp.GetVolcTenantResponseObject, error) {
	store, err := s.volcTenantStore()
	if err != nil {
		return adminhttp.GetVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getVolcTenant(ctx, store, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.GetVolcTenant404JSONResponse(apitypes.NewErrorResponse("VOLC_TENANT_NOT_FOUND", fmt.Sprintf("Volcengine tenant %q not found", name))), nil
		}
		return adminhttp.GetVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetVolcTenant200JSONResponse(tenant), nil
}

func (s *Server) PutVolcTenant(ctx context.Context, request adminhttp.PutVolcTenantRequestObject) (adminhttp.PutVolcTenantResponseObject, error) {
	store, err := s.volcTenantStore()
	if err != nil {
		return adminhttp.PutVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if request.Body == nil {
		return adminhttp.PutVolcTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_VOLC_TENANT", "request body required")), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := normalizeVolcTenantUpsert(*request.Body, name)
	if err != nil {
		return adminhttp.PutVolcTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_VOLC_TENANT", err.Error())), nil
	}
	credentialStore, err := s.credentialStore()
	if err != nil {
		return adminhttp.PutVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if err := validateVolcTenantReferences(ctx, credentialStore, tenant); err != nil {
		return adminhttp.PutVolcTenant400JSONResponse(apitypes.NewErrorResponse("INVALID_VOLC_TENANT", err.Error())), nil
	}
	previous, err := getVolcTenant(ctx, store, name)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return adminhttp.PutVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	now := s.now()
	tenant.CreatedAt = now
	tenant.UpdatedAt = now
	if err == nil {
		tenant.CreatedAt = previous.CreatedAt
		tenant.LastSyncedAt = cloneTime(previous.LastSyncedAt)
	}
	if err := writeVolcTenant(ctx, store, tenant); err != nil {
		return adminhttp.PutVolcTenant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutVolcTenant200JSONResponse(tenant), nil
}

func (s *Server) SyncVolcTenantVoices(ctx context.Context, request adminhttp.SyncVolcTenantVoicesRequestObject) (adminhttp.SyncVolcTenantVoicesResponseObject, error) {
	tenantStore, err := s.volcTenantStore()
	if err != nil {
		return adminhttp.SyncVolcTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	voiceStore, err := s.voiceStore()
	if err != nil {
		return adminhttp.SyncVolcTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	credentialStore, err := s.credentialStore()
	if err != nil {
		return adminhttp.SyncVolcTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	name, err := url.PathUnescape(string(request.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}
	tenant, err := getVolcTenant(ctx, tenantStore, name)
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.SyncVolcTenantVoices404JSONResponse(apitypes.NewErrorResponse("VOLC_TENANT_NOT_FOUND", fmt.Sprintf("Volcengine tenant %q not found", name))), nil
		}
		return adminhttp.SyncVolcTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	credential, err := getCredential(ctx, credentialStore, string(tenant.CredentialName))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return adminhttp.SyncVolcTenantVoices400JSONResponse(apitypes.NewErrorResponse("INVALID_VOLC_TENANT", fmt.Sprintf("credential %q not found", tenant.CredentialName))), nil
		}
		return adminhttp.SyncVolcTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	appID, _, _, err := volcCredentialValues(credential)
	if err != nil {
		return adminhttp.SyncVolcTenantVoices400JSONResponse(apitypes.NewErrorResponse("INVALID_VOLC_TENANT", err.Error())), nil
	}
	client, err := s.volcSpeakerClientForTenant(ctx, credential, tenant)
	if err != nil {
		return adminhttp.SyncVolcTenantVoices400JSONResponse(apitypes.NewErrorResponse("INVALID_VOLC_TENANT", err.Error())), nil
	}
	upstream, err := listAllVolcSpeakers(ctx, client, tenant, appID)
	if err != nil {
		return adminhttp.SyncVolcTenantVoices502JSONResponse(apitypes.NewErrorResponse("VOLC_SYNC_FAILED", err.Error())), nil
	}
	now := s.now()
	createdCount, updatedCount, deletedCount, err := reconcileVolcTenantVoices(ctx, voiceStore, tenant, upstream, now)
	if err != nil {
		return adminhttp.SyncVolcTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	tenant.LastSyncedAt = &now
	tenant.UpdatedAt = now
	if err := writeVolcTenant(ctx, tenantStore, tenant); err != nil {
		return adminhttp.SyncVolcTenantVoices500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.SyncVolcTenantVoices200JSONResponse(adminhttp.VolcSyncVoicesResult{
		CreatedCount: createdCount,
		DeletedCount: deletedCount,
		SyncedAt:     now,
		TenantName:   tenant.Name,
		UpdatedCount: updatedCount,
	}), nil
}

func listVolcTenantsPage(ctx context.Context, store kv.Store, cursor string, limit int) ([]apitypes.VolcTenant, bool, *string, error) {
	entries, err := kv.ListAfter(ctx, store, volcTenantsRoot, cursorAfterKey(volcTenantsRoot, cursor), limit+1)
	if err != nil {
		return nil, false, nil, err
	}
	pageEntries, hasNext, nextCursor := paginateEntries(entries, limit)
	items := make([]apitypes.VolcTenant, 0, len(pageEntries))
	for _, entry := range pageEntries {
		var tenant apitypes.VolcTenant
		if err := json.Unmarshal(entry.Value, &tenant); err != nil {
			return nil, false, nil, fmt.Errorf("mmx: decode volc tenant list %s: %w", entry.Key.String(), err)
		}
		items = append(items, tenant)
	}
	return items, hasNext, nextCursor, nil
}

func normalizeVolcTenantUpsert(in adminhttp.VolcTenantUpsert, expectedName string) (apitypes.VolcTenant, error) {
	name := strings.TrimSpace(string(in.Name))
	if name == "" {
		return apitypes.VolcTenant{}, errors.New("name is required")
	}
	if expectedName != "" && name != expectedName {
		return apitypes.VolcTenant{}, fmt.Errorf("name %q must match path name %q", name, expectedName)
	}
	credentialName := strings.TrimSpace(string(in.CredentialName))
	if credentialName == "" {
		return apitypes.VolcTenant{}, errors.New("credential_name is required")
	}
	tenant := apitypes.VolcTenant{
		CredentialName: string(credentialName),
		Name:           string(name),
	}
	if in.Region != nil {
		region := strings.TrimSpace(*in.Region)
		if region != "" {
			tenant.Region = &region
		}
	}
	if in.Endpoint != nil {
		endpoint := strings.TrimSpace(*in.Endpoint)
		if endpoint != "" {
			parsed, err := url.Parse(endpoint)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				return apitypes.VolcTenant{}, errors.New("endpoint must be an absolute URL")
			}
			tenant.Endpoint = &endpoint
		}
	}
	if in.ResourceIds != nil {
		resourceIDs := normalizeVolcResourceIDs(*in.ResourceIds)
		if len(resourceIDs) > 0 {
			tenant.ResourceIds = &resourceIDs
		}
	}
	if in.Description != nil {
		description := strings.TrimSpace(*in.Description)
		if description != "" {
			tenant.Description = &description
		}
	}
	return tenant, nil
}

func validateVolcTenantReferences(ctx context.Context, store kv.Store, tenant apitypes.VolcTenant) error {
	if _, err := store.Get(ctx, credentialKey(string(tenant.CredentialName))); err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return fmt.Errorf("credential %q not found", tenant.CredentialName)
		}
		return err
	}
	return nil
}

func writeVolcTenant(ctx context.Context, store kv.Store, tenant apitypes.VolcTenant) error {
	data, err := json.Marshal(tenant)
	if err != nil {
		return fmt.Errorf("mmx: encode volc tenant %s: %w", tenant.Name, err)
	}
	if err := store.Set(ctx, volcTenantKey(string(tenant.Name)), data); err != nil {
		return fmt.Errorf("mmx: write volc tenant %s: %w", tenant.Name, err)
	}
	return nil
}

func getVolcTenant(ctx context.Context, store kv.Store, name string) (apitypes.VolcTenant, error) {
	data, err := store.Get(ctx, volcTenantKey(name))
	if err != nil {
		return apitypes.VolcTenant{}, err
	}
	var tenant apitypes.VolcTenant
	if err := json.Unmarshal(data, &tenant); err != nil {
		return apitypes.VolcTenant{}, fmt.Errorf("mmx: decode volc tenant %s: %w", name, err)
	}
	return tenant, nil
}

func (s *Server) volcSpeakerClientForTenant(ctx context.Context, credential apitypes.Credential, tenant apitypes.VolcTenant) (VolcSpeakerClient, error) {
	if s != nil && s.VolcSpeakerClientFactory != nil {
		return s.VolcSpeakerClientFactory(ctx, credential, tenant)
	}
	provider := strings.TrimSpace(string(credential.Provider))
	if provider != "" && provider != "volc" && provider != "volcengine" {
		return nil, fmt.Errorf("credential %q provider must be volcengine", tenant.CredentialName)
	}
	_, ak, sk, err := volcCredentialValues(credential)
	if err != nil {
		return nil, err
	}
	cfg := volcengine.NewConfig().
		WithCredentials(credentials.NewStaticCredentials(ak, sk, "")).
		WithRegion(volcRegion(tenant))
	if s != nil && s.HTTPClient != nil {
		cfg.WithHTTPClient(s.HTTPClient)
	}
	if tenant.Endpoint != nil && strings.TrimSpace(*tenant.Endpoint) != "" {
		cfg.WithEndpoint(strings.TrimSpace(*tenant.Endpoint))
	}
	sess, err := session.NewSession(cfg)
	if err != nil {
		return nil, fmt.Errorf("create Volcengine session: %w", err)
	}
	return volcSpeechSDKClient{
		speech:    speechsaasprod.New(sess),
		universal: universal.New(sess),
	}, nil
}

type volcSpeechSDKClient struct {
	speech    *speechsaasprod.SPEECHSAASPROD
	universal *universal.Universal
}

func (c volcSpeechSDKClient) ListSpeakersWithContext(ctx context.Context, resourceIDs []string, pageNumber, pageSize int32) (*volcSpeakersPage, error) {
	input := &speechsaasprod.ListSpeakersInput{}
	if pageNumber > 0 {
		input.Page = &pageNumber
	}
	if pageSize > 0 {
		limit := fmt.Sprint(pageSize)
		input.Limit = &limit
	}
	if len(resourceIDs) > 0 {
		input.ResourceIDs = volcResourceIDStringPtrs(resourceIDs)
	}
	out, err := c.speech.ListSpeakersWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	page := &volcSpeakersPage{PageNumber: pageNumber, PageSize: pageSize}
	if out.Total != nil {
		page.Total = *out.Total
	}
	page.Speakers = make([]volcSpeaker, 0, len(out.Speakers))
	for _, speaker := range out.Speakers {
		if speaker == nil {
			continue
		}
		voiceType := strings.TrimSpace(stringValue(speaker.VoiceType))
		resourceID := strings.TrimSpace(stringValue(speaker.ResourceID))
		if voiceType == "" || resourceID == "" {
			continue
		}
		raw := rawStructToMap(speaker)
		page.Speakers = append(page.Speakers, volcSpeaker{
			VoiceType:   voiceType,
			Name:        strings.TrimSpace(stringValue(speaker.Name)),
			Description: strings.TrimSpace(stringValue(speaker.Description)),
			ResourceID:  resourceID,
			Raw:         voicecatalog.RawMapValue(raw),
		})
	}
	return page, nil
}

func (c volcSpeechSDKClient) ListBigModelTTSTimbresWithContext(ctx context.Context) ([]volcPublicTimbre, error) {
	out, err := c.speech.ListBigModelTTSTimbresWithContext(ctx, &speechsaasprod.ListBigModelTTSTimbresInput{})
	if err != nil {
		return nil, err
	}
	timbres := make([]volcPublicTimbre, 0, len(out.Timbres))
	for _, timbre := range out.Timbres {
		if timbre == nil {
			continue
		}
		speakerID := strings.TrimSpace(stringValue(timbre.SpeakerID))
		if speakerID == "" {
			continue
		}
		raw := rawStructToMap(timbre)
		timbres = append(timbres, volcPublicTimbre{
			SpeakerID: speakerID,
			Name:      firstVolcTimbreSpeakerName(timbre.TimbreInfos),
			Raw:       voicecatalog.RawMapValue(raw),
		})
	}
	return timbres, nil
}

func (c volcSpeechSDKClient) BatchListMegaTTSTrainStatusWithContext(ctx context.Context, appID string, resourceIDs []string, pageNumber, pageSize int32) (*volcMegaTTSTrainStatusPage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body := map[string]interface{}{
		"AppID":       appID,
		"ResourceIDs": volcResourceIDStrings(resourceIDs),
		"PageNumber":  pageNumber,
		"PageSize":    pageSize,
	}
	out, err := c.universal.DoCall(universal.RequestUniversal{
		ServiceName: "speech_saas_prod",
		Action:      "BatchListMegaTTSTrainStatus",
		Version:     "2023-11-07",
		HttpMethod:  universal.POST,
		ContentType: universal.ApplicationJSON,
	}, &body)
	if err != nil {
		return nil, err
	}
	result, ok := (*out)["Result"]
	if !ok {
		return nil, errors.New("Volcengine response missing Result")
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode Volcengine Result: %w", err)
	}
	var page volcMegaTTSTrainStatusPage
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, fmt.Errorf("decode Volcengine Result: %w", err)
	}
	if err := page.captureRawStatuses(); err != nil {
		return nil, err
	}
	return &page, nil
}

func volcCredentialValues(credential apitypes.Credential) (string, string, string, error) {
	body, err := credential.Body.AsVolcCredentialBody()
	if err != nil {
		return "", "", "", err
	}
	appID := ptrString(body.AppId)
	ak := ptrString(body.OpenapiAccessKeyId)
	sk := ptrString(body.OpenapiAccessKey)
	if appID == "" {
		return "", "", "", fmt.Errorf("credential %q is missing app_id", credential.Name)
	}
	if ak == "" || sk == "" {
		return "", "", "", fmt.Errorf("credential %q is missing openapi_access_key_id/openapi_access_key", credential.Name)
	}
	return appID, ak, sk, nil
}

type volcSpeakerRecord struct {
	appID      string
	resourceID string
	source     string
	status     *volcSpeakerStatus
	speaker    *volcSpeaker
	timbre     *volcPublicTimbre
}

type volcSpeaker struct {
	VoiceType   string
	Name        string
	Description string
	ResourceID  string
	Raw         interface{}
}

type volcSpeakersPage struct {
	PageNumber int32
	PageSize   int32
	Speakers   []volcSpeaker
	Total      int32
}

type volcPublicTimbre struct {
	SpeakerID string
	Name      string
	Raw       interface{}
}

type volcMegaTTSTrainStatusPage struct {
	AppID       string              `json:"AppID"`
	NextToken   string              `json:"NextToken"`
	PageNumber  int32               `json:"PageNumber"`
	PageSize    int32               `json:"PageSize"`
	Statuses    []volcSpeakerStatus `json:"Statuses"`
	TotalCount  int32               `json:"TotalCount"`
	rawStatuses []json.RawMessage
}

type volcSpeakerStatus struct {
	Alias                  string                 `json:"Alias"`
	AvailableTrainingTimes int32                  `json:"AvailableTrainingTimes"`
	CreateTime             int64                  `json:"CreateTime"`
	DemoAudio              string                 `json:"DemoAudio"`
	Description            string                 `json:"Description"`
	ExpireTime             int64                  `json:"ExpireTime"`
	InstanceNO             string                 `json:"InstanceNO"`
	InstanceStatus         string                 `json:"InstanceStatus"`
	IsActivatable          bool                   `json:"IsActivatable"`
	ModelTypeDetails       []volcModelTypeDetail  `json:"ModelTypeDetails"`
	OrderTime              int64                  `json:"OrderTime"`
	ResourceID             string                 `json:"ResourceID"`
	SpeakerID              string                 `json:"SpeakerID"`
	State                  string                 `json:"State"`
	Version                string                 `json:"Version"`
	raw                    map[string]interface{} `json:"-"`
}

type volcModelTypeDetail struct {
	DemoAudio    string `json:"DemoAudio"`
	IclSpeakerId string `json:"IclSpeakerId"`
	ModelType    int32  `json:"ModelType"`
	ResourceID   string `json:"ResourceID"`
}

func (p *volcMegaTTSTrainStatusPage) UnmarshalJSON(data []byte) error {
	type pageAlias volcMegaTTSTrainStatusPage
	var raw struct {
		*pageAlias
		Statuses []json.RawMessage `json:"Statuses"`
	}
	raw.pageAlias = (*pageAlias)(p)
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.rawStatuses = raw.Statuses
	p.Statuses = make([]volcSpeakerStatus, 0, len(raw.Statuses))
	for _, item := range raw.Statuses {
		var status volcSpeakerStatus
		if err := json.Unmarshal(item, &status); err != nil {
			return err
		}
		var rawStatus map[string]interface{}
		if err := json.Unmarshal(item, &rawStatus); err != nil {
			return err
		}
		status.raw = rawStatus
		p.Statuses = append(p.Statuses, status)
	}
	return nil
}

func (p *volcMegaTTSTrainStatusPage) captureRawStatuses() error {
	if len(p.rawStatuses) == len(p.Statuses) {
		return nil
	}
	for i := range p.Statuses {
		if p.Statuses[i].raw != nil {
			continue
		}
		raw := rawStructToMap(p.Statuses[i])
		if raw == nil {
			continue
		}
		p.Statuses[i].raw = *raw
	}
	return nil
}

func listAllVolcSpeakers(ctx context.Context, client VolcSpeakerClient, tenant apitypes.VolcTenant, appID string) ([]volcSpeakerRecord, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return nil, errors.New("Volcengine credential app_id is required")
	}
	resourceIDs := volcTenantResourceIDs(tenant)
	resourceFilter := volcResourceIDSet(resourceIDs)
	byVoiceID := make(map[string]volcSpeakerRecord)
	const pageSize int32 = 30
	listSpeakersFailed := false
	for pageNumber := int32(1); ; pageNumber++ {
		page, err := client.ListSpeakersWithContext(ctx, nil, pageNumber, pageSize)
		if err != nil {
			if pageNumber == 1 {
				listSpeakersFailed = true
				break
			}
			return nil, err
		}
		for _, speaker := range page.Speakers {
			voiceType := strings.TrimSpace(speaker.VoiceType)
			resourceID := strings.TrimSpace(speaker.ResourceID)
			if voiceType == "" {
				return nil, errors.New("Volcengine returned speaker without VoiceType")
			}
			if resourceID == "" {
				return nil, fmt.Errorf("Volcengine returned speaker %q without ResourceID", voiceType)
			}
			if len(resourceFilter) > 0 {
				if _, ok := resourceFilter[resourceID]; !ok {
					continue
				}
			}
			speakerCopy := speaker
			byVoiceID[voiceType] = volcSpeakerRecord{appID: appID, resourceID: resourceID, source: "speakers", speaker: &speakerCopy}
		}
		if page.Total == 0 || len(page.Speakers) == 0 || pageNumber*pageSize >= page.Total {
			break
		}
	}
	if listSpeakersFailed {
		publicTimbres, err := client.ListBigModelTTSTimbresWithContext(ctx)
		if err != nil {
			return nil, err
		}
		for _, timbre := range publicTimbres {
			speakerID := strings.TrimSpace(timbre.SpeakerID)
			if speakerID == "" {
				continue
			}
			resourceID := volcResourceIDForPublicTimbre(speakerID)
			if len(resourceFilter) > 0 {
				if _, ok := resourceFilter[resourceID]; !ok {
					continue
				}
			}
			timbreCopy := timbre
			byVoiceID[speakerID] = volcSpeakerRecord{appID: appID, resourceID: resourceID, source: "timbres", timbre: &timbreCopy}
		}
	}
	if len(resourceIDs) == 0 {
		return sortedVolcSpeakerRecords(byVoiceID), nil
	}
	for pageNumber := int32(1); ; pageNumber++ {
		page, err := client.BatchListMegaTTSTrainStatusWithContext(ctx, appID, resourceIDs, pageNumber, pageSize)
		if err != nil {
			return nil, err
		}
		for _, status := range page.Statuses {
			speakerID := strings.TrimSpace(status.SpeakerID)
			if speakerID == "" {
				return nil, errors.New("Volcengine returned speaker status without SpeakerID")
			}
			resourceID := string(strings.TrimSpace(status.ResourceID))
			if len(resourceFilter) > 0 {
				if _, ok := resourceFilter[string(resourceID)]; !ok {
					continue
				}
			}
			statusCopy := status
			byVoiceID[speakerID] = volcSpeakerRecord{appID: appID, resourceID: resourceID, source: "app", status: &statusCopy}
		}
		if page.TotalCount == 0 || len(page.Statuses) == 0 || pageNumber*pageSize >= page.TotalCount {
			break
		}
	}
	return sortedVolcSpeakerRecords(byVoiceID), nil
}

func sortedVolcSpeakerRecords(byVoiceID map[string]volcSpeakerRecord) []volcSpeakerRecord {
	all := make([]volcSpeakerRecord, 0, len(byVoiceID))
	for _, record := range byVoiceID {
		all = append(all, record)
	}
	sort.Slice(all, func(i, j int) bool {
		left := all[i].providerVoiceID()
		right := all[j].providerVoiceID()
		if left == right {
			return string(all[i].resourceID) < string(all[j].resourceID)
		}
		return left < right
	})
	return all
}

func (r volcSpeakerRecord) providerVoiceID() string {
	if r.status != nil {
		return strings.TrimSpace(r.status.SpeakerID)
	}
	if r.speaker != nil {
		return strings.TrimSpace(r.speaker.VoiceType)
	}
	if r.timbre != nil {
		return strings.TrimSpace(r.timbre.SpeakerID)
	}
	return ""
}

func reconcileVolcTenantVoices(ctx context.Context, store kv.Store, tenant apitypes.VolcTenant, upstream []volcSpeakerRecord, now time.Time) (int32, int32, int32, error) {
	existing, err := voicecatalog.ListProvider(ctx, store, volcProviderKind, string(tenant.Name))
	if err != nil {
		return 0, 0, 0, err
	}
	existingByProviderVoiceID := make(map[string]apitypes.Voice, len(existing))
	for _, voice := range existing {
		if voice.Source != apitypes.VoiceSourceSync {
			continue
		}
		providerVoiceID := voicecatalog.ProviderDataString(voice, "voice_id")
		if providerVoiceID == "" {
			continue
		}
		existingByProviderVoiceID[providerVoiceID] = voice
	}

	seen := make(map[string]struct{}, len(upstream))
	var createdCount, updatedCount int32
	for _, upstreamVoice := range upstream {
		providerVoiceID := upstreamVoice.providerVoiceID()
		if providerVoiceID == "" {
			return 0, 0, 0, errors.New("Volcengine returned voice without speaker id")
		}
		seen[providerVoiceID] = struct{}{}
		record := voiceFromVolc(tenant.Name, upstreamVoice, now)
		if previous, ok := existingByProviderVoiceID[providerVoiceID]; ok {
			record.CreatedAt = previous.CreatedAt
			if voicecatalog.SemanticEqual(previous, record) {
				record.UpdatedAt = previous.UpdatedAt
			} else {
				updatedCount++
			}
			previousCopy := previous
			if err := voicecatalog.Write(ctx, store, record, &previousCopy); err != nil {
				return 0, 0, 0, err
			}
			continue
		}
		if occupied, err := voicecatalog.Get(ctx, store, string(record.Id)); err == nil {
			if occupied.Source != apitypes.VoiceSourceSync {
				return 0, 0, 0, fmt.Errorf("voice id %q is occupied by non-sync resource", record.Id)
			}
			previousCopy := occupied
			if err := voicecatalog.Write(ctx, store, record, &previousCopy); err != nil {
				return 0, 0, 0, err
			}
			updatedCount++
			continue
		} else if !errors.Is(err, kv.ErrNotFound) {
			return 0, 0, 0, err
		}
		createdCount++
		if err := voicecatalog.Write(ctx, store, record, nil); err != nil {
			return 0, 0, 0, err
		}
	}

	var deletedCount int32
	for providerVoiceID, voice := range existingByProviderVoiceID {
		if _, ok := seen[providerVoiceID]; ok {
			continue
		}
		if err := voicecatalog.Delete(ctx, store, voice); err != nil {
			return 0, 0, 0, err
		}
		deletedCount++
	}
	return createdCount, updatedCount, deletedCount, nil
}

func voiceFromVolc(tenantName string, upstream volcSpeakerRecord, now time.Time) apitypes.Voice {
	providerVoiceID := upstream.providerVoiceID()
	voiceID := voicecatalog.StableID(volcProviderKind, string(tenantName), providerVoiceID)
	name, description, raw := volcVoiceDisplay(upstream)
	resourceID := strings.TrimSpace(string(upstream.resourceID))
	syncedAt := now
	voice := apitypes.Voice{
		CreatedAt: now,
		Id:        string(voiceID),
		Provider: apitypes.VoiceProvider{
			Kind: volcProviderKind,
			Name: string(tenantName),
		},
		ProviderData: voicecatalog.ProviderData(volcProviderKind, map[string]interface{}{
			"raw":         raw,
			"resource_id": resourceID,
			"state":       upstream.state(),
			"status":      upstream.statusText(),
			"voice_id":    providerVoiceID,
		}),
		Source:    apitypes.VoiceSourceSync,
		SyncedAt:  &syncedAt,
		UpdatedAt: now,
	}
	if name != "" {
		voice.Name = &name
	}
	if description != "" {
		voice.Description = &description
	}
	return voice
}

func volcVoiceDisplay(record volcSpeakerRecord) (string, string, interface{}) {
	if record.status != nil {
		name := strings.TrimSpace(record.status.Alias)
		if name == "" {
			name = strings.TrimSpace(record.status.SpeakerID)
		}
		description := strings.TrimSpace(record.status.Description)
		raw := record.status.raw
		if len(raw) == 0 {
			if rawMap := rawStructToMap(record.status); rawMap != nil {
				raw = *rawMap
			}
		}
		return name, description, raw
	}
	if record.speaker != nil {
		name := strings.TrimSpace(record.speaker.Name)
		if name == "" {
			name = strings.TrimSpace(record.speaker.VoiceType)
		}
		return name, strings.TrimSpace(record.speaker.Description), record.speaker.Raw
	}
	if record.timbre != nil {
		name := strings.TrimSpace(record.timbre.Name)
		if name == "" {
			name = strings.TrimSpace(record.timbre.SpeakerID)
		}
		return name, "", record.timbre.Raw
	}
	return "", "", nil
}

func (r volcSpeakerRecord) state() string {
	if r.status == nil {
		return ""
	}
	return strings.TrimSpace(r.status.State)
}

func (r volcSpeakerRecord) statusText() string {
	if r.status == nil {
		return ""
	}
	return strings.TrimSpace(r.status.InstanceStatus)
}

func deleteVolcTenantVoices(ctx context.Context, store kv.Store, tenantName string) error {
	voices, err := voicecatalog.ListProvider(ctx, store, volcProviderKind, string(tenantName))
	if err != nil {
		return err
	}
	for _, voice := range voices {
		if voice.Source != apitypes.VoiceSourceSync {
			continue
		}
		if err := voicecatalog.Delete(ctx, store, voice); err != nil {
			return err
		}
	}
	return nil
}

func volcTenantResourceIDs(tenant apitypes.VolcTenant) []string {
	if tenant.ResourceIds == nil {
		return nil
	}
	return normalizeVolcResourceIDs(*tenant.ResourceIds)
}

func volcResourceIDSet(resourceIDs []string) map[string]struct{} {
	if len(resourceIDs) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		out[string(resourceID)] = struct{}{}
	}
	return out
}

func volcResourceIDStrings(resourceIDs []string) []string {
	normalized := normalizeVolcResourceIDs(resourceIDs)
	out := make([]string, 0, len(normalized))
	for _, resourceID := range normalized {
		out = append(out, string(resourceID))
	}
	return out
}

func volcResourceIDStringPtrs(resourceIDs []string) []*string {
	strings := volcResourceIDStrings(resourceIDs)
	out := make([]*string, 0, len(strings))
	for _, value := range strings {
		value := value
		out = append(out, &value)
	}
	return out
}

func volcResourceIDForPublicTimbre(speakerID string) string {
	normalized := strings.TrimSpace(speakerID)
	lower := strings.ToLower(normalized)
	if strings.HasPrefix(normalized, "S_") {
		return volcPublicICLResourceID
	}
	if strings.Contains(lower, "_uranus_bigtts") || strings.HasPrefix(lower, "saturn_") {
		return volcPublicTTSResourceID
	}
	return volcPublicTTSV1ResourceID
}

func normalizeVolcResourceIDs(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, item := range in {
		value := strings.TrimSpace(string(item))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, string(value))
	}
	return out
}

func volcRegion(tenant apitypes.VolcTenant) string {
	if tenant.Region != nil {
		region := strings.TrimSpace(*tenant.Region)
		if region != "" {
			return region
		}
	}
	return defaultVolcRegion
}

func rawStructToMap(in interface{}) *map[string]interface{} {
	data, err := json.Marshal(in)
	if err != nil {
		return nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil || len(out) == 0 {
		return nil
	}
	return &out
}

func stringValue(in *string) string {
	if in == nil {
		return ""
	}
	return *in
}

func firstVolcTimbreSpeakerName(infos []*speechsaasprod.TimbreInfoForListBigModelTTSTimbresOutput) string {
	for _, info := range infos {
		if info == nil {
			continue
		}
		if name := strings.TrimSpace(stringValue(info.SpeakerName)); name != "" {
			return name
		}
	}
	return ""
}

func volcTenantKey(name string) kv.Key {
	return append(append(kv.Key{}, volcTenantsRoot...), escapeStoreSegment(name))
}

func (s *Server) volcTenantStore() (kv.Store, error) {
	if s == nil {
		return nil, errors.New("Volcengine tenant store not configured")
	}
	if s.VolcTenantStore != nil {
		return s.VolcTenantStore, nil
	}
	if s.TenantStore != nil {
		return s.TenantStore, nil
	}
	if s.Store == nil {
		return nil, errors.New("Volcengine tenant store not configured")
	}
	return s.Store, nil
}
