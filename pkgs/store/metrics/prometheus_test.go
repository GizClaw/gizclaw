package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPrometheusLatestTranslatesSelectorPrivately(t *testing.T) {
	t.Parallel()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query().Get("query")
		if !strings.Contains(q, `m{peer="a"}`) {
			t.Errorf("query=%q", q)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(q, "[1ms]") {
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
			return
		}
		w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[{"metric":{"__name__":"m","peer":"a"},"values":[[100,"2"]]}]}}`))
	}))
	defer server.Close()
	s, err := NewPrometheusStore(PrometheusConfig{RemoteWriteURL: server.URL, QueryURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Latest(context.Background(), LatestQuery{Selector: Selector{Name: "m", Matchers: []LabelMatcher{{Name: "peer", Op: MatchEqual, Value: "a"}}}, At: time.Unix(101, 0), Lookback: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(got) != 1 || got[0].Points[0].Value != 2 {
		t.Fatalf("calls=%d got=%+v", calls, got)
	}
}
func TestPrometheusConfigValidation(t *testing.T) {
	t.Parallel()
	if _, err := NewPrometheusStore(PrometheusConfig{}); err == nil {
		t.Fatal("expected error")
	}
}
