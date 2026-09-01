package redis8

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GizClaw/flowcraft/memory/retrieval"
	"github.com/GizClaw/flowcraft/memory/retrieval/scoring"
	"github.com/GizClaw/flowcraft/sdk/errdefs"
	"github.com/redis/go-redis/v9"
)

const docField = "doc"

type Index struct {
	client                                     *redis.Client
	prefix, indexName, docPrefix, dimensionKey string
}

func newIndex(client *redis.Client, prefix string) *Index {
	sum := sha256.Sum256([]byte(prefix))
	return &Index{client: client, prefix: prefix, indexName: "giz_fc8_" + hex.EncodeToString(sum[:8]), docPrefix: prefix + ":retrieval:doc:", dimensionKey: prefix + ":retrieval:vector_dim"}
}
func (index *Index) ensure(ctx context.Context) error {
	_, err := index.client.FTInfo(ctx, index.indexName).Result()
	if err == nil {
		return nil
	}
	_, err = index.client.FTCreate(ctx, index.indexName, &redis.FTCreateOptions{OnHash: true, Prefix: []any{index.docPrefix}},
		&redis.FieldSchema{FieldName: "present", FieldType: redis.SearchFieldTypeTag},
		&redis.FieldSchema{FieldName: "namespace", FieldType: redis.SearchFieldTypeTag},
		&redis.FieldSchema{FieldName: "content", FieldType: redis.SearchFieldTypeText},
		&redis.FieldSchema{FieldName: "timestamp", FieldType: redis.SearchFieldTypeNumeric, Sortable: true},
		&redis.FieldSchema{FieldName: metadataExactField, FieldType: redis.SearchFieldTypeTag, Separator: "|", CaseSensitive: true},
		&redis.FieldSchema{FieldName: metadataValuesField, FieldType: redis.SearchFieldTypeTag, Separator: "|", CaseSensitive: true},
		&redis.FieldSchema{FieldName: metadataExistsField, FieldType: redis.SearchFieldTypeTag, Separator: "|", CaseSensitive: true},
	).Result()
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "index already exists") {
		return err
	}
	return nil
}
func encodePart(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
func (index *Index) docKey(namespace, id string) string {
	return index.docPrefix + encodePart(namespace) + ":" + encodePart(id)
}
func (index *Index) namespaceKey(namespace string) string {
	return index.prefix + ":retrieval:namespace:" + encodePart(namespace)
}
func vectorBytes(vector []float32) []byte {
	out := make([]byte, len(vector)*4)
	for i, value := range vector {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(value))
	}
	return out
}
func (index *Index) ensureVector(ctx context.Context, dimension int) error {
	if dimension <= 0 {
		return nil
	}
	value := strconv.Itoa(dimension)
	created, err := index.client.SetNX(ctx, index.dimensionKey, value, 0).Result()
	if err != nil {
		return err
	}
	if !created {
		existing, err := index.client.Get(ctx, index.dimensionKey).Result()
		if err != nil {
			return err
		}
		if existing != value {
			return errdefs.Validationf("flowcraft redis8 retrieval: vector dimension %d does not match index dimension %s", dimension, existing)
		}
		return nil
	}
	_, err = index.client.Do(ctx, "FT.ALTER", index.indexName, "SCHEMA", "ADD", "vector", "VECTOR", "HNSW", "6", "TYPE", "FLOAT32", "DIM", dimension, "DISTANCE_METRIC", "COSINE").Result()
	if err != nil {
		_, _ = index.client.Eval(ctx, `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) end return 0`, []string{index.dimensionKey}, value).Result()
		return fmt.Errorf("add vector schema: %w", err)
	}
	return nil
}
func (index *Index) Upsert(ctx context.Context, namespace string, docs []retrieval.Doc) error {
	if namespace == "" {
		return errdefs.Validationf("flowcraft redis8 retrieval: namespace is required")
	}
	if len(docs) == 0 {
		return nil
	}
	dimension := 0
	for _, doc := range docs {
		if doc.ID == "" {
			return errdefs.Validationf("flowcraft redis8 retrieval: document id is required")
		}
		if len(doc.Vector) > 0 {
			if dimension == 0 {
				dimension = len(doc.Vector)
			} else if dimension != len(doc.Vector) {
				return errdefs.Validationf("flowcraft redis8 retrieval: mixed vector dimensions")
			}
		}
	}
	if err := index.ensureVector(ctx, dimension); err != nil {
		return err
	}
	type preparedDoc struct {
		doc              retrieval.Doc
		key              string
		raw              []byte
		metadataFields   map[string]any
		oldNumericFields []string
	}
	prepared := make([]preparedDoc, len(docs))
	reads := index.client.Pipeline()
	oldNumeric := make([]*redis.StringCmd, len(docs))
	for i, doc := range docs {
		for key, value := range doc.Metadata {
			if _, ok := finiteMetadataNumber(value); ok {
				if err := index.ensureNumericMetadataField(ctx, key); err != nil {
					return err
				}
			}
		}
		raw, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		metadataFields, _, err := metadataIndexFields(doc.Metadata)
		if err != nil {
			return err
		}
		key := index.docKey(namespace, doc.ID)
		prepared[i] = preparedDoc{doc: doc, key: key, raw: raw, metadataFields: metadataFields}
		oldNumeric[i] = reads.HGet(ctx, key, metadataNumericList)
	}
	if _, err := reads.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	for i, command := range oldNumeric {
		if raw, err := command.Result(); err == nil && raw != "" {
			if err := json.Unmarshal([]byte(raw), &prepared[i].oldNumericFields); err != nil {
				return fmt.Errorf("decode prior numeric metadata fields: %w", err)
			}
		}
	}
	pipe := index.client.TxPipeline()
	for _, item := range prepared {
		fields := map[string]any{
			"present":   "1",
			"namespace": encodePart(namespace),
			"content":   item.doc.Content,
			"timestamp": item.doc.Timestamp.UnixNano(),
			docField:    item.raw,
		}
		maps.Copy(fields, item.metadataFields)
		if len(item.oldNumericFields) > 0 {
			pipe.HDel(ctx, item.key, item.oldNumericFields...)
		}
		if len(item.doc.Vector) > 0 {
			fields["vector"] = vectorBytes(item.doc.Vector)
		} else {
			pipe.HDel(ctx, item.key, "vector")
		}
		pipe.HSet(ctx, item.key, fields)
		pipe.SAdd(ctx, index.namespaceKey(namespace), item.doc.ID)
	}
	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}
	keys := make([]any, len(docs))
	for i, doc := range docs {
		keys[i] = index.docKey(namespace, doc.ID)
	}
	deadline := time.Now().Add(250 * time.Millisecond)
	for {
		result, searchErr := index.client.FTSearchWithArgs(ctx, index.indexName, "*", &redis.FTSearchOptions{
			CountOnly: true, InKeys: keys, DialectVersion: 2,
		}).Result()
		if searchErr != nil {
			return searchErr
		}
		if result.Total == len(docs) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("flowcraft redis8 retrieval: indexing timeout: indexed %d of %d documents", result.Total, len(docs))
		}
		time.Sleep(5 * time.Millisecond)
	}
}
func (index *Index) Delete(ctx context.Context, namespace string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	pipe := index.client.TxPipeline()
	members := make([]any, len(ids))
	for i, id := range ids {
		pipe.Del(ctx, index.docKey(namespace, id))
		members[i] = id
	}
	pipe.SRem(ctx, index.namespaceKey(namespace), members...)
	_, err := pipe.Exec(ctx)
	return err
}
func (index *Index) Get(ctx context.Context, namespace, id string) (retrieval.Doc, bool, error) {
	raw, err := index.client.HGet(ctx, index.docKey(namespace, id), docField).Result()
	if errors.Is(err, redis.Nil) {
		return retrieval.Doc{}, false, nil
	}
	if err != nil {
		return retrieval.Doc{}, false, err
	}
	var doc retrieval.Doc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return retrieval.Doc{}, false, err
	}
	return doc, true, nil
}
func (index *Index) all(ctx context.Context, namespace string) ([]retrieval.Doc, error) {
	ids, err := index.client.SMembers(ctx, index.namespaceKey(namespace)).Result()
	if err != nil {
		return nil, err
	}
	sort.Strings(ids)
	out := make([]retrieval.Doc, 0, len(ids))
	for _, id := range ids {
		doc, ok, err := index.Get(ctx, namespace, id)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, doc)
		}
	}
	return out, nil
}
func (index *Index) List(ctx context.Context, namespace string, request retrieval.ListRequest) (*retrieval.ListResponse, error) {
	docs, err := index.all(ctx, namespace)
	if err != nil {
		return nil, err
	}
	filtered := docs[:0]
	for _, doc := range docs {
		if retrieval.DocMatchesFilter(doc, request.Filter) {
			filtered = append(filtered, doc)
		}
	}
	docs = filtered
	sort.SliceStable(docs, func(i, j int) bool {
		switch request.OrderBy {
		case retrieval.OrderByTimestampAsc:
			return docs[i].Timestamp.Before(docs[j].Timestamp)
		case retrieval.OrderByIDAsc:
			return docs[i].ID < docs[j].ID
		default:
			return docs[i].Timestamp.After(docs[j].Timestamp)
		}
	})
	offset, err := retrieval.DecodeListPageTokenFor(request.PageToken, request)
	if err != nil {
		return nil, err
	}
	if offset > len(docs) {
		offset = len(docs)
	}
	size := request.PageSize
	if size <= 0 || size > 10000 {
		size = 100
	}
	end := min(offset+size, len(docs))
	items := make([]retrieval.Doc, end-offset)
	copy(items, docs[offset:end])
	if !request.WithVector {
		for i := range items {
			items[i].Vector = nil
			items[i].SparseVector = nil
		}
	}
	response := &retrieval.ListResponse{Items: items, Total: int64(len(docs))}
	if end < len(docs) {
		response.NextPageToken, err = retrieval.EncodeListPageTokenFor(end, request)
	}
	return response, err
}
func escapeText(value string) string {
	const special = `,.<>{}[]"':;!@#$%^&*()-+=~|/\\`
	var escaped strings.Builder
	for _, char := range strings.TrimSpace(value) {
		if strings.ContainsRune(special, char) {
			escaped.WriteByte('\\')
		}
		escaped.WriteRune(char)
	}
	return escaped.String()
}
func (index *Index) searchLane(ctx context.Context, namespace, query string, options *redis.FTSearchOptions, scoreName string) ([]retrieval.Hit, error) {
	result, err := index.client.FTSearchWithArgs(ctx, index.indexName, query, options).Result()
	if err != nil {
		return nil, err
	}
	hits := make([]retrieval.Hit, 0, len(result.Docs))
	for _, record := range result.Docs {
		raw := record.Fields[docField]
		var doc retrieval.Doc
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			return nil, err
		}
		score := 0.0
		if record.Score != nil {
			score = *record.Score
		}
		hit := retrieval.Hit{Doc: doc, Score: score, Scores: map[string]float64{scoreName: score}}
		if distanceRaw := record.Fields["vector_distance"]; distanceRaw != "" {
			distance, _ := strconv.ParseFloat(distanceRaw, 64)
			hit.Distance = distance
			hit.Score = 1 - distance
			hit.Scores[scoreName] = hit.Score
		}
		hits = append(hits, hit)
	}
	return hits, nil
}

func namespaceFilter(namespace string) string {
	return "@namespace:{" + encodePart(namespace) + "}"
}

func combineQueryFilter(namespace string, filter compiledFilter) string {
	query := namespaceFilter(namespace)
	if filter.query != "" && filter.query != "*" {
		query += " (" + filter.query + ")"
	}
	return query
}

func searchCandidateWindow(topK int, params map[string]any) int {
	window := max(topK*4, 20)
	if configured, ok := params["rrf_window"]; ok {
		if value, ok := metadataNumber(configured); ok && value >= float64(topK) && value <= 10000 {
			window = int(value)
		}
	}
	return window
}

func rrfConstant(params map[string]any) float64 {
	if configured, ok := params["rrf_constant"]; ok {
		if value, ok := finiteMetadataNumber(configured); ok && value > 0 {
			return value
		}
	}
	return scoring.DefaultRRFK
}

func resultString(result map[string]any, key string) string {
	switch value := result[key].(type) {
	case string:
		return value
	case []byte:
		return string(value)
	default:
		return ""
	}
}

func resultFloat(result map[string]any, key string) (float64, bool) {
	value, ok := result[key]
	if !ok {
		return 0, false
	}
	if number, ok := metadataNumber(value); ok {
		return number, true
	}
	text, ok := value.(string)
	if !ok {
		return 0, false
	}
	number, err := strconv.ParseFloat(text, 64)
	return number, err == nil
}

func decodeHybridHits(result redis.FTHybridResult) ([]retrieval.Hit, error) {
	hits := make([]retrieval.Hit, 0, len(result.Results))
	for _, record := range result.Results {
		raw := resultString(record, docField)
		if raw == "" {
			return nil, fmt.Errorf("flowcraft redis8 retrieval: FT.HYBRID result omitted %q", docField)
		}
		var doc retrieval.Doc
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			return nil, err
		}
		scores := make(map[string]float64, 3)
		combined, _ := resultFloat(record, "__combined_score")
		if score, ok := resultFloat(record, "__score"); ok {
			combined = score
		}
		if score, ok := resultFloat(record, "giz_bm25_score"); ok {
			scores["bm25"] = score
		}
		distance := 0.0
		if score, ok := resultFloat(record, "giz_vector_score"); ok && score > 0 {
			// FT.HYBRID exposes the vector lane as 1/(1+distance), while
			// retrieval.Hit uses cosine similarity and cosine distance.
			distance = 1/score - 1
			scores["cos"] = 1 - distance
		}
		if scores["bm25"] <= 0 && scores["cos"] <= 0 {
			continue
		}
		hits = append(hits, retrieval.Hit{Doc: doc, Score: combined, Distance: distance, Scores: scores})
	}
	return hits, nil
}

func (index *Index) searchHybridNative(ctx context.Context, namespace string, request retrieval.SearchRequest, topK int, filter compiledFilter) ([]retrieval.Hit, error) {
	if err := index.ensureVector(ctx, len(request.QueryVector)); err != nil {
		return nil, err
	}
	queryFilter := combineQueryFilter(namespace, filter)
	window := searchCandidateWindow(topK, request.HybridParam)
	result, err := index.client.FTHybridWithArgs(ctx, index.indexName, &redis.FTHybridOptions{
		CountExpressions: 2,
		SearchExpressions: []redis.FTHybridSearchExpression{{
			Query:        queryFilter + " @content:(" + escapeText(request.QueryText) + ")",
			Scorer:       "BM25STD",
			YieldScoreAs: "giz_bm25_score",
		}},
		VectorExpressions: []redis.FTHybridVectorExpression{{
			VectorField:     "vector",
			VectorData:      &redis.VectorFP32{Val: vectorBytes(request.QueryVector)},
			VectorParamName: "query_vector",
			Method:          "KNN",
			MethodParams:    []any{"K", window},
			Filter:          queryFilter,
			YieldScoreAs:    "giz_vector_score",
		}},
		Combine: &redis.FTHybridCombineOptions{
			Method:   redis.FTHybridCombineRRF,
			Window:   window,
			Constant: rrfConstant(request.HybridParam),
		},
		Load:        []string{"@" + docField, "@__score", "@__combined_score", "@giz_bm25_score", "@giz_vector_score"},
		LimitOffset: 0,
		Limit:       topK,
	}).Result()
	if err != nil {
		return nil, err
	}
	return decodeHybridHits(result)
}

func (index *Index) searchLegacy(ctx context.Context, namespace string, request retrieval.SearchRequest, topK int) ([]retrieval.Hit, error) {
	candidateCount, err := index.client.SCard(ctx, index.namespaceKey(namespace)).Result()
	if err != nil {
		return nil, err
	}
	limit := max(int(candidateCount), topK)
	tag := namespaceFilter(namespace)
	var lanes [][]retrieval.Hit
	if text := escapeText(request.QueryText); text != "" {
		hits, err := index.searchLane(ctx, namespace, tag+" @content:("+text+")", &redis.FTSearchOptions{WithScores: true, Return: []redis.FTSearchReturn{{FieldName: docField}}, Limit: limit, DialectVersion: 2}, "bm25")
		if err != nil {
			return nil, err
		}
		lanes = append(lanes, hits)
	}
	if len(request.QueryVector) > 0 {
		if err := index.ensureVector(ctx, len(request.QueryVector)); err != nil {
			return nil, err
		}
		query := fmt.Sprintf("(%s)=>[KNN %d @vector $query_vector AS vector_distance]", tag, limit)
		hits, err := index.searchLane(ctx, namespace, query, &redis.FTSearchOptions{Return: []redis.FTSearchReturn{{FieldName: docField}, {FieldName: "vector_distance"}}, SortBy: []redis.FTSearchSortBy{{FieldName: "vector_distance", Asc: true}}, Limit: limit, Params: map[string]any{"query_vector": vectorBytes(request.QueryVector)}, DialectVersion: 2}, "cos")
		if err != nil {
			return nil, err
		}
		lanes = append(lanes, hits)
	}
	for i := range lanes {
		filtered := lanes[i][:0]
		for _, hit := range lanes[i] {
			if hit.Score <= 0 || !retrieval.DocMatchesFilter(hit.Doc, request.Filter) {
				continue
			}
			if len(lanes) == 1 && request.MinScore > 0 && hit.Score < request.MinScore {
				continue
			}
			filtered = append(filtered, hit)
		}
		lanes[i] = filtered
	}
	if len(lanes) == 1 {
		return lanes[0], nil
	}
	return scoring.RRF(lanes, scoring.DefaultRRFK), nil
}

func (index *Index) Search(ctx context.Context, namespace string, request retrieval.SearchRequest) (*retrieval.SearchResponse, error) {
	started := time.Now()
	if strings.TrimSpace(request.QueryText) == "" && len(request.QueryVector) == 0 && len(request.SparseVec) == 0 {
		return nil, retrieval.ErrNoQuery
	}
	if len(request.SparseVec) > 0 {
		return nil, errdefs.Validationf("flowcraft redis8 retrieval: sparse vectors are unsupported")
	}
	topK := request.TopK
	if topK <= 0 {
		topK = 10
	}
	filter, err := index.compileFilter(ctx, request.Filter)
	if err != nil {
		return nil, err
	}
	if !filter.supported {
		hits, err := index.searchLegacy(ctx, namespace, request, topK)
		if err != nil {
			return nil, err
		}
		if len(hits) > topK {
			hits = hits[:topK]
		}
		return &retrieval.SearchResponse{Hits: hits, Took: time.Since(started)}, nil
	}
	queryFilter := combineQueryFilter(namespace, filter)
	var hits []retrieval.Hit
	if strings.TrimSpace(request.QueryText) != "" && len(request.QueryVector) > 0 {
		hits, err = index.searchHybridNative(ctx, namespace, request, topK, filter)
	} else if text := escapeText(request.QueryText); text != "" {
		hits, err = index.searchLane(ctx, namespace, queryFilter+" @content:("+text+")", &redis.FTSearchOptions{WithScores: true, Return: []redis.FTSearchReturn{{FieldName: docField}}, Limit: topK, DialectVersion: 2}, "bm25")
	} else {
		if err = index.ensureVector(ctx, len(request.QueryVector)); err == nil {
			query := fmt.Sprintf("(%s)=>[KNN %d @vector $query_vector AS vector_distance]", queryFilter, topK)
			hits, err = index.searchLane(ctx, namespace, query, &redis.FTSearchOptions{Return: []redis.FTSearchReturn{{FieldName: docField}, {FieldName: "vector_distance"}}, SortBy: []redis.FTSearchSortBy{{FieldName: "vector_distance", Asc: true}}, Limit: topK, Params: map[string]any{"query_vector": vectorBytes(request.QueryVector)}, DialectVersion: 2}, "cos")
		}
	}
	if err != nil {
		return nil, err
	}
	if request.MinScore > 0 && len(request.QueryVector) == 0 {
		filtered := hits[:0]
		for _, hit := range hits {
			if hit.Score >= request.MinScore {
				filtered = append(filtered, hit)
			}
		}
		hits = filtered
	}
	if len(hits) > topK {
		hits = hits[:topK]
	}
	return &retrieval.SearchResponse{Hits: hits, Took: time.Since(started)}, nil
}

func (index *Index) SearchHybrid(ctx context.Context, namespace string, request retrieval.HybridRequest) (*retrieval.SearchResponse, error) {
	if request.Mode != retrieval.HybridDefault && request.Mode != retrieval.HybridRRF {
		return nil, errdefs.Validationf("flowcraft redis8 retrieval: hybrid mode %q is unsupported", request.Mode)
	}
	return index.Search(ctx, namespace, retrieval.SearchRequest{
		QueryText:   request.QueryText,
		QueryVector: request.QueryVector,
		Filter:      request.Filter,
		TopK:        request.TopK,
		HybridMode:  retrieval.HybridRRF,
		HybridParam: request.Param,
		Debug:       request.Debug,
	})
}
func (index *Index) Iterate(ctx context.Context, namespace, cursor string, batch int) ([]retrieval.Doc, string, error) {
	request := retrieval.ListRequest{PageSize: batch, PageToken: cursor, OrderBy: retrieval.OrderByIDAsc, WithVector: true}
	response, err := index.List(ctx, namespace, request)
	if err != nil {
		return nil, "", err
	}
	return response.Items, response.NextPageToken, nil
}
func (index *Index) Count(ctx context.Context, namespace string, filter retrieval.Filter) (int64, error) {
	docs, err := index.all(ctx, namespace)
	if err != nil {
		return 0, err
	}
	var count int64
	for _, doc := range docs {
		if retrieval.DocMatchesFilter(doc, filter) {
			count++
		}
	}
	return count, nil
}
func emptyFilter(filter retrieval.Filter) bool {
	return reflect.DeepEqual(filter, retrieval.Filter{})
}
func (index *Index) DeleteByFilter(ctx context.Context, namespace string, filter retrieval.Filter) (int64, error) {
	if emptyFilter(filter) {
		return 0, retrieval.ErrEmptyDeleteFilter
	}
	docs, err := index.all(ctx, namespace)
	if err != nil {
		return 0, err
	}
	var ids []string
	for _, doc := range docs {
		if retrieval.DocMatchesFilter(doc, filter) {
			ids = append(ids, doc.ID)
		}
	}
	return int64(len(ids)), index.Delete(ctx, namespace, ids)
}
func (index *Index) Drop(ctx context.Context, namespace string) error {
	ids, err := index.client.SMembers(ctx, index.namespaceKey(namespace)).Result()
	if err != nil {
		return err
	}
	if err := index.Delete(ctx, namespace, ids); err != nil {
		return err
	}
	return index.client.Del(ctx, index.namespaceKey(namespace)).Err()
}
func (index *Index) WarmNamespace(context.Context, string) error { return nil }
func (index *Index) Capabilities() retrieval.Capabilities {
	return retrieval.Capabilities{BM25: true, Vector: true, Hybrid: true, FilterPushdown: true, SupportedOps: []string{"and", "or", "not", "eq", "neq", "in", "not_in", "range", "exists", "missing", "contains_any", "contains_all"}, BatchUpsertMax: 0, WriteIsAtomic: false, MaxListPageSize: 10000, NativeDeleteByFilter: false, SupportedListOrders: []retrieval.ListOrderBy{retrieval.OrderByTimestampDesc, retrieval.OrderByTimestampAsc, retrieval.OrderByIDAsc}, ReadAfterWrite: true, Distributed: true, Extensions: retrieval.ExtensionCapabilities{DocGetter: true, Iterable: true, Count: true, DeleteByFilter: true, DropNamespace: true, NamespaceWarm: true}}
}
func (index *Index) Close() error { return nil }

var _ retrieval.Index = (*Index)(nil)
var _ retrieval.Hybridable = (*Index)(nil)
var _ retrieval.Filterable = (*Index)(nil)
var _ retrieval.DocGetter = (*Index)(nil)
var _ retrieval.Iterable = (*Index)(nil)
var _ retrieval.Countable = (*Index)(nil)
var _ retrieval.DeletableByFilter = (*Index)(nil)
var _ retrieval.Droppable = (*Index)(nil)
var _ retrieval.NamespaceWarmer = (*Index)(nil)
