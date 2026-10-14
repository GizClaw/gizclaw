// Package tikvfixture configures an isolated center-voter/regional-Learner
// TiKV test cluster. It never configures a production cluster implicitly.
package tikvfixture

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Topology records the verified placement of every Region in the fixture.
type Topology struct {
	CenterStoreID  uint64                  `json:"center_store_id"`
	LearnerStoreID uint64                  `json:"learner_store_id"`
	Regions        int                     `json:"regions"`
	LearnerPeers   map[uint64]RegionalPeer `json:"learner_peers"`
}

// RegionalPeer identifies the actual Learner for a Region's diagnostic reads.
type RegionalPeer struct {
	ID      uint64 `json:"id"`
	StoreID uint64 `json:"store_id"`
}

// HTTP makes a bounded request to the fixture's PD or replication proxy.
func HTTP(ctx context.Context, method, endpoint string, input, output any) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("TiKV fixture HTTP status %d: %s", resp.StatusCode, detail)
	}
	if output != nil {
		return json.NewDecoder(resp.Body).Decode(output)
	}
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

// Configure waits for bootstrap, assigns the center voter and regional Learner,
// then verifies that all Regions have a center leader and the local Learner.
// The caller supplies the startup deadline.
func Configure(ctx context.Context, address string) (Topology, error) {
	base := "http://" + address + "/pd/api/v1"
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		var regions, stores struct {
			Count int `json:"count"`
		}
		err := HTTP(ctx, http.MethodGet, base+"/regions", nil, &regions)
		if err == nil {
			err = HTTP(ctx, http.MethodGet, base+"/stores", nil, &stores)
		}
		if err == nil && regions.Count > 0 && stores.Count >= 2 {
			break
		}
		select {
		case <-ctx.Done():
			return Topology{}, fmt.Errorf("TiKV bootstrap: %w", ctx.Err())
		case <-ticker.C:
		}
	}
	if err := HTTP(ctx, http.MethodPost, base+"/config/replicate", map[string]any{"max-replicas": 1, "enable-placement-rules": "true"}, nil); err != nil {
		return Topology{}, err
	}
	for _, rule := range []map[string]any{
		{"group_id": "pd", "id": "default", "role": "voter", "count": 1, "label_constraints": []map[string]any{{"key": "region", "op": "in", "values": []string{"center"}}}},
		{"group_id": "gizclaw-e2e", "id": "regional", "role": "learner", "count": 1, "label_constraints": []map[string]any{{"key": "region", "op": "in", "values": []string{"local"}}}},
	} {
		if err := HTTP(ctx, http.MethodPost, base+"/config/rule", rule, nil); err != nil {
			return Topology{}, err
		}
	}
	for {
		topology, ready, err := ReadTopology(ctx, address)
		if err == nil && ready {
			return topology, nil
		}
		select {
		case <-ctx.Done():
			return topology, fmt.Errorf("TiKV placement: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// ReadTopology checks live PD placement without changing it.
func ReadTopology(ctx context.Context, address string) (Topology, bool, error) {
	base := "http://" + address + "/pd/api/v1"
	var regions struct {
		Regions []struct {
			ID     uint64 `json:"id"`
			Leader struct {
				StoreID uint64 `json:"store_id"`
			} `json:"leader"`
			Peers []struct {
				ID       uint64 `json:"id"`
				StoreID  uint64 `json:"store_id"`
				RoleName string `json:"role_name"`
			} `json:"peers"`
		} `json:"regions"`
	}
	var stores struct {
		Stores []struct {
			Store struct {
				ID     uint64                        `json:"id"`
				Labels []struct{ Key, Value string } `json:"labels"`
			} `json:"store"`
		} `json:"stores"`
	}
	if err := HTTP(ctx, http.MethodGet, base+"/regions", nil, &regions); err != nil {
		return Topology{}, false, err
	}
	if err := HTTP(ctx, http.MethodGet, base+"/stores", nil, &stores); err != nil {
		return Topology{}, false, err
	}
	topology := Topology{Regions: len(regions.Regions), LearnerPeers: make(map[uint64]RegionalPeer)}
	for _, store := range stores.Stores {
		for _, label := range store.Store.Labels {
			if label.Key == "region" {
				switch label.Value {
				case "center":
					topology.CenterStoreID = store.Store.ID
				case "local":
					topology.LearnerStoreID = store.Store.ID
				}
			}
		}
	}
	ready := topology.CenterStoreID != 0 && topology.LearnerStoreID != 0 && topology.Regions > 0
	for _, region := range regions.Regions {
		learner := false
		for _, peer := range region.Peers {
			if peer.StoreID == topology.LearnerStoreID && peer.RoleName == "Learner" {
				learner = peer.ID != 0
				topology.LearnerPeers[region.ID] = RegionalPeer{ID: peer.ID, StoreID: peer.StoreID}
			}
		}
		ready = ready && region.ID != 0 && learner && region.Leader.StoreID == topology.CenterStoreID
	}
	return topology, ready, nil
}
