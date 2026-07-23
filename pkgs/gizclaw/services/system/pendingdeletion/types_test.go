package pendingdeletion

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRecordValidateRejectsInvalidEnvelope(t *testing.T) {
	valid, err := New(KindPeer, "peer-a", nil, ReasonPeerDelete, map[string]string{"public_key": "peer-a"}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Record)
	}{
		{name: "deletion ID", mutate: func(record *Record) { record.DeletionID = "invalid" }},
		{name: "kind", mutate: func(record *Record) { record.Kind = "unknown" }},
		{name: "resource ID", mutate: func(record *Record) { record.ResourceID = " " }},
		{name: "non-canonical resource ID", mutate: func(record *Record) { record.ResourceID = " peer-a " }},
		{name: "owner public key", mutate: func(record *Record) {
			owner := " "
			record.OwnerPublicKey = &owner
		}},
		{name: "non-canonical owner public key", mutate: func(record *Record) {
			owner := " peer-a "
			record.OwnerPublicKey = &owner
		}},
		{name: "reason", mutate: func(record *Record) { record.Reason = "unknown" }},
		{name: "deleted at", mutate: func(record *Record) { record.DeletedAt = time.Time{} }},
		{name: "descriptor version", mutate: func(record *Record) { record.DescriptorVersion++ }},
		{name: "descriptor", mutate: func(record *Record) { record.Descriptor = json.RawMessage("{") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := valid
			test.mutate(&record)
			if err := record.Validate(); err == nil {
				t.Fatal("Validate error = nil")
			}
			if _, err := KVEntries(record); err == nil {
				t.Fatal("KVEntries error = nil")
			}
		})
	}
}

func TestNewRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name       string
		kind       Kind
		resourceID string
		reason     Reason
		descriptor any
	}{
		{name: "kind", kind: "unknown", resourceID: "resource", reason: ReasonResourceDelete, descriptor: struct{}{}},
		{name: "resource ID", kind: KindPeer, resourceID: " ", reason: ReasonPeerDelete, descriptor: struct{}{}},
		{name: "reason", kind: KindPeer, resourceID: "resource", descriptor: struct{}{}},
		{name: "descriptor", kind: KindPeer, resourceID: "resource", reason: ReasonPeerDelete, descriptor: make(chan int)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.kind, test.resourceID, nil, test.reason, test.descriptor, time.Time{}); err == nil {
				t.Fatal("New error = nil")
			}
		})
	}
}

func TestNewRejectsEmptyOwnerPublicKey(t *testing.T) {
	owner := " "
	if _, err := New(KindPeer, "peer-a", &owner, ReasonPeerDelete, struct{}{}, time.Time{}); err == nil {
		t.Fatal("New error = nil")
	}
}

func TestNewRejectsPetWithoutOwner(t *testing.T) {
	if _, err := New(KindPet, "pet-a", nil, ReasonResourceDelete, struct{}{}, time.Time{}); err == nil {
		t.Fatal("New error = nil")
	}
}

func TestNewCanonicalizesLocator(t *testing.T) {
	owner := " peer-a "
	record, err := New(KindPet, " pet-a ", &owner, ReasonResourceDelete, struct{}{}, time.Time{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if record.ResourceID != "pet-a" || record.OwnerPublicKey == nil || *record.OwnerPublicKey != "peer-a" {
		t.Fatalf("New locator = %q, %v", record.ResourceID, record.OwnerPublicKey)
	}
	if record.DeletionID != "pet/cGVlci1h/cGV0LWE" {
		t.Fatalf("New deletion ID = %q, want stable resource locator ID", record.DeletionID)
	}
	if owner != " peer-a " {
		t.Fatalf("New mutated owner input = %q", owner)
	}
}

func TestNewReusesResourceDeletionID(t *testing.T) {
	first, err := New(KindWorkspace, "workspace-a", nil, ReasonResourceDelete, struct{}{}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("New(first): %v", err)
	}
	second, err := New(KindWorkspace, "workspace-a", nil, ReasonAdminDelete, struct{}{}, time.Unix(2, 0))
	if err != nil {
		t.Fatalf("New(second): %v", err)
	}
	if first.DeletionID != "workspace/d29ya3NwYWNlLWE" || second.DeletionID != first.DeletionID {
		t.Fatalf("deletion IDs = %q and %q, want one stable resource ID", first.DeletionID, second.DeletionID)
	}
}

func TestNewScopesPetDeletionIDByOwner(t *testing.T) {
	firstOwner := "peer-a"
	secondOwner := "peer-b"
	first, err := New(KindPet, "shared-pet", &firstOwner, ReasonResourceDelete, struct{}{}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("New(first): %v", err)
	}
	second, err := New(KindPet, "shared-pet", &secondOwner, ReasonResourceDelete, struct{}{}, time.Unix(1, 0))
	if err != nil {
		t.Fatalf("New(second): %v", err)
	}
	if first.DeletionID == second.DeletionID {
		t.Fatalf("owner-scoped Pet deletion IDs both = %q", first.DeletionID)
	}
}

func TestNewUsesCurrentTimeWhenTimestampIsZero(t *testing.T) {
	before := time.Now().UTC()
	record, err := New(KindPeer, "peer-a", nil, ReasonPeerDelete, struct{}{}, time.Time{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if record.DeletedAt.Before(before) || record.DeletedAt.After(time.Now().UTC()) {
		t.Fatalf("DeletedAt = %s, want current time", record.DeletedAt)
	}
}
