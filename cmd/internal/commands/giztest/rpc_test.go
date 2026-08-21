package giztest

import "testing"

func TestLookupMethodUsesProtoMetadata(t *testing.T) {
	info, err := lookupMethod("all.ping")
	if err != nil {
		t.Fatal(err)
	}
	if info.request != "PingRequest" || info.response != "PingResponse" {
		t.Fatalf("info = %#v", info)
	}
	if _, err := lookupMethod("missing.method"); err == nil {
		t.Fatal("unknown method accepted")
	}
}
