package redis8

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GizClaw/flowcraft/memory/retrieval"
)

const (
	metadataExactField  = "metadata_exact"
	metadataValuesField = "metadata_values"
	metadataExistsField = "metadata_exists"
	metadataNumericList = "metadata_numeric_fields"
)

type compiledFilter struct {
	query     string
	supported bool
}

func metadataKeyToken(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func metadataValueToken(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func metadataPredicateToken(key string, value any) (string, error) {
	valueToken, err := metadataValueToken(value)
	if err != nil {
		return "", err
	}
	return metadataKeyToken(key) + "_" + valueToken, nil
}

func metadataNumericField(key string) string {
	return "metadata_numeric_" + metadataKeyToken(key)
}

func metadataIndexFields(metadata map[string]any) (map[string]any, []string, error) {
	fields := make(map[string]any, 4+len(metadata))
	exact := make([]string, 0, len(metadata))
	values := make([]string, 0, len(metadata))
	exists := make([]string, 0, len(metadata))
	numericFields := make([]string, 0, len(metadata))
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := metadata[key]
		keyToken := metadataKeyToken(key)
		exists = append(exists, keyToken)
		exactToken, err := metadataPredicateToken(key, value)
		if err != nil {
			return nil, nil, fmt.Errorf("encode metadata %q: %w", key, err)
		}
		exact = append(exact, exactToken)
		for _, element := range metadataElements(value) {
			token, err := metadataPredicateToken(key, element)
			if err != nil {
				return nil, nil, fmt.Errorf("encode metadata element %q: %w", key, err)
			}
			values = append(values, token)
		}
		if number, ok := finiteMetadataNumber(value); ok {
			field := metadataNumericField(key)
			fields[field] = number
			numericFields = append(numericFields, field)
		}
	}
	fields[metadataExactField] = strings.Join(exact, "|")
	fields[metadataValuesField] = strings.Join(values, "|")
	fields[metadataExistsField] = strings.Join(exists, "|")
	rawNumericFields, err := json.Marshal(numericFields)
	if err != nil {
		return nil, nil, err
	}
	fields[metadataNumericList] = rawNumericFields
	return fields, numericFields, nil
}

func metadataElements(value any) []any {
	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() || (reflected.Kind() != reflect.Array && reflected.Kind() != reflect.Slice) {
		return []any{value}
	}
	elements := make([]any, reflected.Len())
	for i := range reflected.Len() {
		elements[i] = reflected.Index(i).Interface()
	}
	return elements
}

func metadataNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int8:
		return float64(number), true
	case int16:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint8:
		return float64(number), true
	case uint16:
		return float64(number), true
	case uint32:
		return float64(number), true
	case uint64:
		if number > 1<<53 {
			return 0, false
		}
		return float64(number), true
	default:
		return 0, false
	}
}

func (index *Index) ensureNumericMetadataField(ctx context.Context, key string) error {
	field := metadataNumericField(key)
	stateKey := index.prefix + ":retrieval:schema:" + field
	created, err := index.client.SetNX(ctx, stateKey, "creating", time.Minute).Result()
	if err != nil {
		return err
	}
	if created {
		_, err = index.client.Do(ctx, "FT.ALTER", index.indexName, "SCHEMA", "ADD", field, "NUMERIC").Result()
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
			_ = index.client.Del(ctx, stateKey).Err()
			return fmt.Errorf("add numeric metadata field %q: %w", key, err)
		}
		return index.client.Set(ctx, stateKey, "ready", 0).Err()
	}
	deadline := time.Now().Add(time.Second)
	for {
		state, err := index.client.Get(ctx, stateKey).Result()
		if err != nil {
			return err
		}
		if state == "ready" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("flowcraft redis8 retrieval: metadata schema %q did not become ready", key)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (index *Index) compileFilter(ctx context.Context, filter retrieval.Filter) (compiledFilter, error) {
	if !supportsRedisFilter(filter) {
		return compiledFilter{supported: false}, nil
	}
	query, err := index.compileSupportedFilter(ctx, filter)
	if err != nil {
		return compiledFilter{}, err
	}
	if query == "" {
		query = "*"
	}
	return compiledFilter{query: query, supported: true}, nil
}

func supportsRedisFilter(filter retrieval.Filter) bool {
	if len(filter.Match) > 0 || len(filter.Contains) > 0 || len(filter.IContains) > 0 {
		return false
	}
	for _, values := range filter.Eq {
		if _, err := metadataValueToken(values); err != nil {
			return false
		}
	}
	for _, values := range filter.Neq {
		if _, err := metadataValueToken(values); err != nil {
			return false
		}
	}
	for _, lists := range []map[string][]any{filter.In, filter.NotIn, filter.ContainsAny, filter.ContainsAll} {
		for _, values := range lists {
			for _, value := range values {
				if _, err := metadataValueToken(value); err != nil {
					return false
				}
			}
		}
	}
	for _, bounds := range filter.Range {
		for _, value := range []any{bounds.Gt, bounds.Gte, bounds.Lt, bounds.Lte} {
			if value != nil {
				if _, ok := finiteMetadataNumber(value); !ok {
					return false
				}
			}
		}
	}
	if filter.Not != nil && !supportsRedisFilter(*filter.Not) {
		return false
	}
	for _, children := range [][]retrieval.Filter{filter.And, filter.Or} {
		for _, child := range children {
			if !supportsRedisFilter(child) {
				return false
			}
		}
	}
	return true
}

func (index *Index) compileSupportedFilter(ctx context.Context, filter retrieval.Filter) (string, error) {
	clauses := make([]string, 0)
	if filter.Not != nil {
		clause, err := index.compileSupportedFilter(ctx, *filter.Not)
		if err != nil {
			return "", err
		}
		if clause == "" {
			return impossibleFilterQuery, nil
		}
		if clause == impossibleFilterQuery {
			return "", nil
		}
		clauses = append(clauses, "-("+clause+")")
	}
	for _, child := range filter.And {
		clause, err := index.compileSupportedFilter(ctx, child)
		if err != nil {
			return "", err
		}
		if clause != "" {
			clauses = append(clauses, "("+clause+")")
		}
	}
	if len(filter.Or) > 0 {
		alternatives := make([]string, 0, len(filter.Or))
		for _, child := range filter.Or {
			clause, err := index.compileSupportedFilter(ctx, child)
			if err != nil {
				return "", err
			}
			if clause == "" {
				alternatives = nil
				break
			}
			alternatives = append(alternatives, "("+clause+")")
		}
		if len(alternatives) > 0 {
			clauses = append(clauses, "("+strings.Join(alternatives, "|")+")")
		}
	}
	for key, value := range filter.Eq {
		clause, err := equalityClause(key, value)
		if err != nil {
			return "", err
		}
		clauses = append(clauses, clause)
	}
	for key, value := range filter.Neq {
		clause, err := equalityClause(key, value)
		if err != nil {
			return "", err
		}
		clauses = append(clauses, "-("+clause+")")
	}
	for key, values := range filter.In {
		clause, err := membershipClause(metadataExactField, key, values)
		if err != nil {
			return "", err
		}
		clauses = append(clauses, clause)
	}
	for key, values := range filter.NotIn {
		if len(values) == 0 {
			continue
		}
		clause, err := membershipClause(metadataExactField, key, values)
		if err != nil {
			return "", err
		}
		clauses = append(clauses, "-("+clause+")")
	}
	for key, bounds := range filter.Range {
		if err := index.ensureNumericMetadataField(ctx, key); err != nil {
			return "", err
		}
		clause, err := numericRangeClause(key, bounds)
		if err != nil {
			return "", err
		}
		clauses = append(clauses, clause)
	}
	for _, key := range filter.Exists {
		clauses = append(clauses, tagClause(metadataExistsField, []string{metadataKeyToken(key)}))
	}
	for _, key := range filter.Missing {
		clauses = append(clauses, "-"+tagClause(metadataExistsField, []string{metadataKeyToken(key)}))
	}
	for key, values := range filter.ContainsAny {
		clause, err := membershipClause(metadataValuesField, key, values)
		if err != nil {
			return "", err
		}
		clauses = append(clauses, clause)
	}
	for key, values := range filter.ContainsAll {
		if len(values) == 0 {
			continue
		}
		for _, value := range values {
			token, err := metadataPredicateToken(key, value)
			if err != nil {
				return "", err
			}
			clauses = append(clauses, tagClause(metadataValuesField, []string{token}))
		}
	}
	sort.Strings(clauses)
	return strings.Join(clauses, " "), nil
}

func (index *Index) SupportsFilter(filter retrieval.Filter) bool {
	return supportsRedisFilter(filter)
}

const impossibleFilterQuery = "-@present:{1}"

func equalityClause(key string, value any) (string, error) {
	token, err := metadataPredicateToken(key, value)
	if err != nil {
		return "", err
	}
	exact := tagClause(metadataExactField, []string{token})
	if value != nil {
		return exact, nil
	}
	missing := "-" + tagClause(metadataExistsField, []string{metadataKeyToken(key)})
	return "(" + exact + "|" + missing + ")", nil
}

func membershipClause(field, key string, values []any) (string, error) {
	if len(values) == 0 {
		return impossibleFilterQuery, nil
	}
	tokens := make([]string, 0, len(values))
	hasNil := false
	for _, value := range values {
		if value == nil {
			hasNil = true
		}
		token, err := metadataPredicateToken(key, value)
		if err != nil {
			return "", err
		}
		tokens = append(tokens, token)
	}
	clause := tagClause(field, tokens)
	if hasNil && field == metadataExactField {
		missing := "-" + tagClause(metadataExistsField, []string{metadataKeyToken(key)})
		clause = "(" + clause + "|" + missing + ")"
	}
	return clause, nil
}

func tagClause(field string, tokens []string) string {
	sort.Strings(tokens)
	return "@" + field + ":{" + strings.Join(tokens, "|") + "}"
}

func numericRangeClause(key string, bounds retrieval.Range) (string, error) {
	lower := "-inf"
	upper := "+inf"
	if bounds.Gt != nil {
		value, _ := finiteMetadataNumber(bounds.Gt)
		lower = "(" + strconv.FormatFloat(value, 'g', -1, 64)
	} else if bounds.Gte != nil {
		value, _ := finiteMetadataNumber(bounds.Gte)
		lower = strconv.FormatFloat(value, 'g', -1, 64)
	}
	if bounds.Lt != nil {
		value, _ := finiteMetadataNumber(bounds.Lt)
		upper = "(" + strconv.FormatFloat(value, 'g', -1, 64)
	} else if bounds.Lte != nil {
		value, _ := finiteMetadataNumber(bounds.Lte)
		upper = strconv.FormatFloat(value, 'g', -1, 64)
	}
	if lower == "-inf" && upper == "+inf" {
		return tagClause(metadataExistsField, []string{metadataKeyToken(key)}), nil
	}
	return fmt.Sprintf("@%s:[%s %s]", metadataNumericField(key), lower, upper), nil
}

func finiteMetadataNumber(value any) (float64, bool) {
	number, ok := metadataNumber(value)
	return number, ok && !math.IsNaN(number) && !math.IsInf(number, 0)
}
