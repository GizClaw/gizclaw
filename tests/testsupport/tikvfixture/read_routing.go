// Package-local fixture support for the two-label read-routing lane.
package tikvfixture

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// ReadRoutingTopology describes the two healthy Learners in every Region.
type ReadRoutingTopology struct {
	Center  uint64            `json:"center"`
	Stores  map[string]uint64 `json:"stores"`
	Regions int               `json:"regions"`
}

// ConfigureReadRouting adds a second regional Learner to the isolated fixture.
func ConfigureReadRouting(ctx context.Context, address string) (ReadRoutingTopology, error) {
	if _, err := Configure(ctx, address); err != nil {
		return ReadRoutingTopology{}, err
	}
	rule := map[string]any{"group_id": "gizclaw-e2e", "id": "remote", "role": "learner", "count": 1,
		"label_constraints": []map[string]any{{"key": "region", "op": "in", "values": []string{"remote"}}}}
	if err := HTTP(ctx, http.MethodPost, "http://"+address+"/pd/api/v1/config/rule", rule, nil); err != nil {
		return ReadRoutingTopology{}, err
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		topology, ready, err := ReadRouting(ctx, address)
		if err == nil && ready {
			return topology, nil
		}
		select {
		case <-ctx.Done():
			return topology, fmt.Errorf("two-Learner placement: %w (last error: %v)", ctx.Err(), err)
		case <-ticker.C:
		}
	}
}

// ReadRouting verifies Up stores, two distinct label-matching Learners and a
// center voter/leader in every Region, with no pending or down peers.
func ReadRouting(ctx context.Context, address string) (ReadRoutingTopology, bool, error) {
	base := "http://" + address + "/pd/api/v1"
	var stores struct {
		Stores []struct {
			Store struct {
				ID     uint64                        `json:"id"`
				State  string                        `json:"state_name"`
				Labels []struct{ Key, Value string } `json:"labels"`
			} `json:"store"`
		} `json:"stores"`
	}
	var regions struct {
		Regions []struct {
			ID     uint64       `json:"id"`
			Leader RegionalPeer `json:"leader"`
			Peers  []struct {
				StoreID uint64 `json:"store_id"`
				Role    string `json:"role_name"`
			} `json:"peers"`
			Pending []any `json:"pending_peers"`
			Down    []any `json:"down_peers"`
		} `json:"regions"`
	}
	if err := HTTP(ctx, http.MethodGet, base+"/stores", nil, &stores); err != nil {
		return ReadRoutingTopology{}, false, err
	}
	if err := HTTP(ctx, http.MethodGet, base+"/regions", nil, &regions); err != nil {
		return ReadRoutingTopology{}, false, err
	}
	topology := ReadRoutingTopology{Stores: make(map[string]uint64), Regions: len(regions.Regions)}
	for _, store := range stores.Stores {
		if store.Store.State != "Up" {
			continue
		}
		for _, label := range store.Store.Labels {
			if label.Key != "region" {
				continue
			}
			switch label.Value {
			case "center":
				topology.Center = store.Store.ID
			case "local", "remote":
				topology.Stores[label.Value] = store.Store.ID
			}
		}
	}
	ready := topology.Center != 0 && topology.Stores["local"] != 0 && topology.Stores["remote"] != 0 && topology.Stores["local"] != topology.Stores["remote"] && topology.Regions > 0
	for _, region := range regions.Regions {
		learners := map[uint64]bool{}
		voters := 0
		for _, peer := range region.Peers {
			if peer.Role == "Learner" {
				learners[peer.StoreID] = true
			} else if peer.StoreID == topology.Center && peer.Role == "Voter" {
				voters++
			}
		}
		ready = ready && region.ID != 0 && region.Leader.StoreID == topology.Center && len(region.Peers) == 3 && voters == 1 && learners[topology.Stores["local"]] && learners[topology.Stores["remote"]] && len(region.Pending) == 0 && len(region.Down) == 0
	}
	return topology, ready, nil
}
