package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestAudioDownloadReceipt(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		ok    bool
	}{
		{name: "verified", input: `{"received":true}`, ok: true},
		{name: "negative", input: `{"received":false}`},
		{name: "missing", input: `{}`},
		{name: "malformed", input: `{`},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := awaitAudioDownloadReceipt(t.Context(), io.NopCloser(strings.NewReader(test.input)))
			if (err == nil) != test.ok {
				t.Fatalf("receipt error = %v, want success %t", err, test.ok)
			}
		})
	}
	t.Run("premature EOF", func(t *testing.T) {
		err := awaitAudioDownloadReceipt(t.Context(), io.NopCloser(strings.NewReader("")))
		if !errors.Is(err, io.EOF) {
			t.Fatalf("receipt error = %v, want EOF", err)
		}
	})
}

func TestAudioDownloadReceiptCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "canceled", true: "deadline"}[deadline], func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			want := context.Canceled
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(t.Context(), 20*time.Millisecond)
				want = context.DeadlineExceeded
			}
			defer cancel()
			reader, writer := io.Pipe()
			defer writer.Close()
			done := make(chan error, 1)
			go func() { done <- awaitAudioDownloadReceipt(ctx, reader) }()
			if !deadline {
				cancel()
			}
			select {
			case err := <-done:
				if !errors.Is(err, want) {
					t.Fatalf("receipt error = %v, want %v", err, want)
				}
			case <-time.After(time.Second):
				t.Fatal("receipt cancellation left the input read blocked")
			}
		})
	}
}
