package giztest

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestHTTPEventStreamSharedVectors(t *testing.T) {
	data, err := os.ReadFile("../../api/giztest/testdata/http_sse_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Name     string         `json:"name"`
		Text     string         `json:"text"`
		Expected map[string]any `json:"expected"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			value, err := DecodeHTTPEventStream(vector.Text)
			if err != nil || !reflect.DeepEqual(value, vector.Expected) {
				t.Fatalf("SSE projection = %#v, %v; want %#v", value, err, vector.Expected)
			}
		})
	}
}

func TestHTTPEventStreamLimits(t *testing.T) {
	for _, text := range []string{strings.Repeat("x", maxHTTPSSEResponseBytes+1), strings.Repeat("data:x\n\n", maxHTTPSSEResponseEvents+1)} {
		if _, err := DecodeHTTPEventStream(text); err == nil {
			t.Fatal("unbounded SSE response accepted")
		}
	}
}
