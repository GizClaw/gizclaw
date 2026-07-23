package eventpb

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestPeerEventValidate(t *testing.T) {
	t.Parallel()
	valid := &PeerEvent{
		Version: Version,
		Type:    PeerEventType_PEER_EVENT_TYPE_BOS,
		Payload: &PeerEvent_Bos{Bos: &StreamBegin{StreamId: "turn-1"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	for name, event := range map[string]*PeerEvent{
		"version": {
			Type:    PeerEventType_PEER_EVENT_TYPE_BOS,
			Payload: &PeerEvent_Bos{Bos: &StreamBegin{}},
		},
		"unknown type": {
			Version: Version,
			Type:    PeerEventType(99),
			Payload: &PeerEvent_Bos{Bos: &StreamBegin{}},
		},
		"mismatch": {
			Version: Version,
			Type:    PeerEventType_PEER_EVENT_TYPE_EOS,
			Payload: &PeerEvent_Bos{Bos: &StreamBegin{}},
		},
		"missing workspace": {
			Version: Version,
			Type:    PeerEventType_PEER_EVENT_TYPE_WORKSPACE_HISTORY_UPDATED,
			Payload: &PeerEvent_WorkspaceHistoryUpdated{
				WorkspaceHistoryUpdated: &WorkspaceHistoryUpdated{},
			},
		},
		"missing stream": {
			Version: Version,
			Type:    PeerEventType_PEER_EVENT_TYPE_BOS,
			Payload: &PeerEvent_Bos{Bos: &StreamBegin{}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := event.Validate(); err == nil {
				t.Fatal("Validate() succeeded")
			}
		})
	}
}

func TestPeerEventGoldenVectors(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../../api/proto/events/testdata/peer_event_vectors.json")
	if err != nil {
		t.Fatalf("ReadFile(golden vectors): %v", err)
	}
	var vectors []struct {
		Name string `json:"name"`
		Hex  string `json:"hex"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("Unmarshal(golden vectors): %v", err)
	}
	if len(vectors) != 7 {
		t.Fatalf("golden vector count = %d, want every oneof arm", len(vectors))
	}
	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			want, err := hex.DecodeString(vector.Hex)
			if err != nil {
				t.Fatalf("DecodeString: %v", err)
			}
			var event PeerEvent
			if err := proto.Unmarshal(want, &event); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if err := event.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
			got, err := proto.MarshalOptions{Deterministic: true}.Marshal(&event)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if hex.EncodeToString(got) != vector.Hex {
				t.Fatalf("encoded bytes = %x, want %s", got, vector.Hex)
			}
		})
	}
}

func TestPeerEventValidateErrors(t *testing.T) {
	t.Parallel()
	if err := (*PeerEvent)(nil).Validate(); !errors.Is(err, ErrInvalidVersion) {
		t.Fatalf("nil Validate() error = %v", err)
	}
}

func TestPeerEventValidateReceivedAllowsFutureType(t *testing.T) {
	t.Parallel()
	event := &PeerEvent{Version: Version, Type: PeerEventType(99)}
	if err := event.ValidateReceived(); err != nil {
		t.Fatalf("ValidateReceived(future type): %v", err)
	}
	if err := event.Validate(); !errors.Is(err, ErrUnknownType) {
		t.Fatalf("Validate(future type) = %v, want ErrUnknownType", err)
	}
	event.Payload = &PeerEvent_Bos{Bos: &StreamBegin{}}
	if err := event.ValidateReceived(); !errors.Is(err, ErrPayloadMismatch) {
		t.Fatalf("ValidateReceived(future type with known payload) = %v, want ErrPayloadMismatch", err)
	}
}
