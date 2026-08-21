package giztest

import "testing"

func TestBoundedBufferRejectsOverflowWithoutPartialWrite(t *testing.T) {
	buffer := &boundedBuffer{max: 3}
	if _, err := buffer.Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	if n, err := buffer.Write([]byte("cd")); err == nil || n != 0 || buffer.String() != "ab" {
		t.Fatalf("write=(%d,%v) buffer=%q", n, err, buffer.String())
	}
}
