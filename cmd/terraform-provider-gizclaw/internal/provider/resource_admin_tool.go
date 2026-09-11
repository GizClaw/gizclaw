package provider

import (
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli/adminresource"
)

// The Tool rules below mirror the Server normalization in
// pkgs/gizclaw/services/runtime/toolkit (normalizeTool, normalizeHTTPRequest,
// and the API conversion defaults). A configuration that omits a defaulted
// field, or writes a value in a form the Server normalizes, stays consistent
// with the Server read; any other observed value is still recorded as drift.

// toolObservedDefault reports whether observed is the value the Server stores
// for a Tool field that the configuration omits or sets to null.
func toolObservedDefault(kind string, fieldPath []string, observed any) bool {
	if kind != "Tool" {
		return false
	}
	switch {
	case pathEquals(fieldPath, "enabled"):
		return observed == true
	case pathEquals(fieldPath, "http", "headers"):
		headers, ok := observed.(map[string]any)
		return ok && len(headers) == 0
	case pathEquals(fieldPath, "http", "success_status_codes"):
		return isDefaultSuccessStatusCodes(observed)
	case toolBindingRequiredPath(fieldPath):
		return observed == true
	}
	return false
}

// toolNormalizedStringMatches reports whether configured, after environment
// expansion, normalizes to observed under the Server rule for its Tool field.
func toolNormalizedStringMatches(kind string, fieldPath []string, configured, observed string) bool {
	if kind != "Tool" {
		return false
	}
	var normalize func(string) (string, bool)
	switch {
	case pathEquals(fieldPath, "http", "method"):
		normalize = func(value string) (string, bool) { return strings.ToUpper(strings.TrimSpace(value)), true }
	case pathEquals(fieldPath, "http", "url"):
		normalize = func(value string) (string, bool) {
			parsed, err := url.Parse(value)
			if err != nil {
				return "", false
			}
			return parsed.String(), true
		}
	case pathEquals(fieldPath, "http", "timeout"):
		normalize = func(value string) (string, bool) {
			duration, err := time.ParseDuration(value)
			if err != nil {
				return "", false
			}
			return duration.String(), true
		}
	case pathEquals(fieldPath, "http", "auth", "header"):
		normalize = func(value string) (string, bool) {
			return http.CanonicalHeaderKey(strings.TrimSpace(value)), true
		}
	default:
		return false
	}
	if adminresource.HasEnvReference(configured) {
		expanded, err := adminresource.ExpandEnvString(configured)
		if err != nil {
			return false
		}
		configured = expanded
	}
	normalized, ok := normalize(configured)
	return ok && normalized == observed
}

// toolCanonicalHeaders returns configured fixed headers keyed by the canonical
// names the Server stores. It reports false when two configured names collapse
// to the same canonical name, because the Server keeps only one of them.
func toolCanonicalHeaders(kind string, fieldPath []string, configured map[string]any) (map[string]any, bool) {
	if kind != "Tool" || !pathEquals(fieldPath, "http", "headers") {
		return configured, true
	}
	canonical := make(map[string]any, len(configured))
	for name, value := range configured {
		key := http.CanonicalHeaderKey(strings.TrimSpace(name))
		if _, duplicate := canonical[key]; duplicate {
			return nil, false
		}
		canonical[key] = value
	}
	return canonical, true
}

// toolNormalizedSuccessStatusCodes returns configured success status codes in
// the sorted, de-duplicated form the Server stores, with an empty list
// replaced by the default [200].
func toolNormalizedSuccessStatusCodes(kind string, fieldPath []string, configured []any) []any {
	if kind != "Tool" || !pathEquals(fieldPath, "http", "success_status_codes") {
		return configured
	}
	codes := make([]float64, 0, len(configured))
	for _, entry := range configured {
		code, ok := entry.(float64)
		if !ok || code != math.Trunc(code) {
			return configured
		}
		codes = append(codes, code)
	}
	if len(codes) == 0 {
		codes = []float64{http.StatusOK}
	}
	slices.Sort(codes)
	codes = slices.Compact(codes)
	normalized := make([]any, len(codes))
	for index, code := range codes {
		normalized[index] = code
	}
	return normalized
}

func isDefaultSuccessStatusCodes(observed any) bool {
	codes, ok := observed.([]any)
	return ok && len(codes) == 1 && codes[0] == float64(http.StatusOK)
}

func toolBindingRequiredPath(fieldPath []string) bool {
	if len(fieldPath) != 4 || fieldPath[0] != "http" ||
		(fieldPath[1] != "query" && fieldPath[1] != "body") || fieldPath[3] != "required" {
		return false
	}
	_, err := strconv.Atoi(fieldPath[2])
	return err == nil
}

func pathEquals(fieldPath []string, want ...string) bool {
	return slices.Equal(fieldPath, want)
}
