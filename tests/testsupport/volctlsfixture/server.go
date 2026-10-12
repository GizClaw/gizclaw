// Package volctlsfixture provides an in-memory TLS HTTP protocol fixture for
// local tests. It is not a LogStore backend and is never linked into GizClaw.
package volctlsfixture

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/volcengine/volc-sdk-golang/service/tls"
)

type entry struct {
	topic     string
	timestamp int64
	fields    map[string]any
}

type continuation struct {
	topic, query, sort string
	start, end         int64
	entries            []entry
}

type handler struct {
	mu       sync.Mutex
	entries  []entry
	pages    map[string]continuation
	sequence uint64
}

// NewHandler serves DescribeIndex, PutLogs and SearchLogs using the real TLS
// SDK wire format. Search supports the structured expressions GizClaw emits.
func NewHandler() http.Handler {
	s := &handler{pages: make(map[string]continuation)}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /DescribeIndex", s.index)
	mux.HandleFunc("POST /PutLogs", s.put)
	mux.HandleFunc("POST /SearchLogs", s.search)
	return mux
}

func (*handler) index(w http.ResponseWriter, _ *http.Request) {
	fields := []tls.KeyValueInfo{}
	for _, name := range []string{"id", "stream", "kind", "level"} {
		fields = append(fields, tls.KeyValueInfo{Key: name, Value: tls.Value{ValueType: "text", CaseSensitive: true}})
	}
	fields = append(fields,
		tls.KeyValueInfo{Key: "msg", Value: tls.Value{ValueType: "text", Delimiter: " \t\r\n", CaseSensitive: true, IncludeChinese: true}},
		tls.KeyValueInfo{Key: "attributes", Value: tls.Value{ValueType: "json", CaseSensitive: true, IndexAll: true}},
	)
	inner := []tls.KeyValueInfo{}
	writeJSON(w, &tls.DescribeIndexResponse{KeyValue: &fields, UserInnerKeyValue: &inner, EnablePhraseIndex: true})
}

func (s *handler) put(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	rawSize, sizeErr := strconv.ParseInt(r.Header.Get("x-tls-bodyrawsize"), 10, 64)
	if err != nil || sizeErr != nil || rawSize < 0 || rawSize > 4<<20 {
		http.Error(w, "invalid log body", http.StatusBadRequest)
		return
	}
	groups, err := tls.GetLogGroupList(r.Header.Get("x-tls-compresstype"), rawSize, body)
	if err != nil {
		http.Error(w, "invalid protobuf", http.StatusBadRequest)
		return
	}
	var received []entry
	for _, group := range groups.LogGroups {
		for _, item := range group.Logs {
			timestamp := time.Unix(item.Time, int64(item.GetTimeNs())).UnixNano()
			fields := map[string]any{"__time__": item.Time, "__time_ns__": item.GetTimeNs(), "__source__": group.Source, "__path__": group.FileName}
			for _, field := range item.Contents {
				fields[field.Key] = field.Value
			}
			received = append(received, entry{topic: r.URL.Query().Get("TopicId"), timestamp: timestamp, fields: fields})
		}
	}
	s.mu.Lock()
	s.entries = append(s.entries, received...)
	if len(s.entries) > 100000 {
		s.entries = slices.Clone(s.entries[len(s.entries)-100000:])
	}
	s.mu.Unlock()
	writeJSON(w, struct{}{})
}

func (s *handler) search(w http.ResponseWriter, r *http.Request) {
	var request tls.SearchLogsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil || request.Limit < 1 || request.Limit > 1000 {
		http.Error(w, "invalid search", http.StatusBadRequest)
		return
	}
	match, err := parseQuery(request.Query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var entries []entry
	s.mu.Lock()
	if request.Context != "" {
		page, ok := s.pages[request.Context]
		if !ok || page.topic != request.TopicID || page.query != request.Query || page.sort != request.Sort || page.start != request.StartTime || page.end != request.EndTime {
			s.mu.Unlock()
			http.Error(w, "invalid continuation", http.StatusBadRequest)
			return
		}
		entries = page.entries
	} else {
		entries = slices.Clone(s.entries)
	}
	s.mu.Unlock()
	if request.Context == "" {
		entries = slices.DeleteFunc(entries, func(item entry) bool {
			return item.topic != request.TopicID || item.timestamp < request.StartTime*int64(time.Millisecond) || item.timestamp >= request.EndTime*int64(time.Millisecond) || !match(item.fields)
		})
		slices.SortStableFunc(entries, func(a, b entry) int {
			order := 0
			if a.timestamp < b.timestamp {
				order = -1
			} else if a.timestamp > b.timestamp {
				order = 1
			}
			if request.Sort == "desc" {
				return -order
			}
			return order
		})
	}
	count := min(request.Limit, len(entries))
	response := tls.SearchLogsResponse{Status: "complete", ListOver: count == len(entries), Count: count, HitCount: len(entries), Limit: request.Limit, Logs: make([]map[string]any, 0, count)}
	for _, item := range entries[:count] {
		response.Logs = append(response.Logs, item.fields)
	}
	if !response.ListOver {
		s.mu.Lock()
		s.sequence++
		response.Context = strconv.FormatUint(s.sequence, 10)
		s.pages[response.Context] = continuation{topic: request.TopicID, query: request.Query, sort: request.Sort, start: request.StartTime, end: request.EndTime, entries: entries[count:]}
		if s.sequence > 512 {
			delete(s.pages, strconv.FormatUint(s.sequence-512, 10))
		}
		s.mu.Unlock()
	}
	writeJSON(w, response)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		fmt.Printf("TLS fixture encode: %v\n", err)
	}
}
