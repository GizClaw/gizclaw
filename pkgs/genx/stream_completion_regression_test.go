package genx

import (
	"errors"
	"fmt"
	"io"
	"testing"
)

type completionFailure struct{ cause error }

func (e *completionFailure) Error() string { return "provider failed: " + e.cause.Error() }
func (e *completionFailure) Unwrap() error { return e.cause }
func (*completionFailure) PublicError() (string, string, bool) {
	panic("completion classification must not invoke public detail callbacks")
}

type completionFailureStream struct{ err error }

func (s completionFailureStream) Next() (*MessageChunk, error) { return nil, s.err }
func (completionFailureStream) Close() error                   { return nil }
func (completionFailureStream) CloseWithError(error) error     { return nil }

func TestStreamCompletionPreservesTypedFailures(t *testing.T) {
	for _, cause := range []error{io.EOF, ErrDone} {
		if !isDoneErr(fmt.Errorf("completed: %w", cause)) {
			t.Fatal("lost contextual completion")
		}
		for _, failure := range []error{&completionFailure{cause}, fmt.Errorf("stage: %w", &completionFailure{cause}), errors.Join(cause, &completionFailure{errors.New("provider unavailable")})} {
			if isDoneErr(failure) {
				t.Errorf("typed failure classified as completion: %v", failure)
			}
		}
	}
	if IsStreamDone(nil) {
		t.Fatal("nil classified as completion")
	}
	upstreamDone := errors.New("upstream iterator done")
	if !IsStreamDone(fmt.Errorf("upstream: %w", upstreamDone), upstreamDone) {
		t.Fatal("lost contextual upstream completion")
	}
	if IsStreamDone(&completionFailure{upstreamDone}, upstreamDone) {
		t.Fatal("upstream completion hid typed failure")
	}
	failure := &completionFailure{io.EOF}
	classified := ClassifyFailure(failure, FailureClassProvider)
	if class, ok := FailureClassOf(classified); !ok || class != FailureClassProvider || !errors.Is(classified, failure) {
		t.Fatalf("lost provider failure provenance: %v", classified)
	}
}

func TestStreamCompositionsPreserveTypedCompletionCauses(t *testing.T) {
	for _, cause := range []error{io.EOF, ErrDone} {
		failure := &completionFailure{cause}
		for _, tc := range []struct {
			name    string
			compose func(Stream) Stream
		}{
			{"sequence", func(s Stream) Stream { return CompositeSeq(s) }},
			{"merge", func(s Stream) Stream { return Merge(s) }},
			{"interleaved", func(s Stream) Stream { return MergeInterleaved(s) }},
			{"split", func(s Stream) Stream {
				a, b := Split(s, func(*MessageChunk) bool { return true })
				t.Cleanup(func() { _ = b.Close() })
				return a
			}},
		} {
			t.Run(tc.name+"/"+cause.Error(), func(t *testing.T) {
				stream := tc.compose(completionFailureStream{failure})
				defer stream.Close()
				for {
					_, err := stream.Next()
					if err != nil {
						if !errors.Is(err, failure) {
							t.Fatalf("lost typed failure: %v", err)
						}
						break
					}
				}
			})
		}
		t.Run("iterator/"+cause.Error(), func(t *testing.T) {
			_, err := Iter(completionFailureStream{failure}).FirstWhere(func(IterElement) bool { return false })
			if !errors.Is(err, failure) {
				t.Fatalf("iterator lost failure: %v", err)
			}
		})
	}
}
