package gizcli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestClientSpeechUsesSpeechSpecificStreamDeadlines(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		call    func(*Client) error
	}{
		{
			name: "transcription", timeout: defaultSpeechTranscriptionStreamTimeout,
			call: func(client *Client) error {
				_, err := client.TranscribeSpeech(context.Background(), "deadline", rpcapi.SpeechTranscribeRequest{
					ModelAlias: "asr", ContentType: "audio/L16;rate=16000;channels=1",
				}, bytes.NewReader([]byte{1}))
				return err
			},
		},
		{
			name: "synthesis", timeout: defaultSpeechSynthesisStreamTimeout,
			call: func(client *Client) error {
				_, err := client.SynthesizeSpeech(context.Background(), "deadline", rpcapi.SpeechSynthesizeRequest{
					VoiceAlias: "voice", Text: "hello",
				}, io.Discard)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream := &deadlineRecordingConn{}
			peer := &speechDeadlinePeerConn{stream: stream}
			client := &Client{conn: peer}
			start := time.Now()
			if err := test.call(client); err == nil {
				t.Fatal("speech call error = nil, want test stream failure")
			}
			deadline, ok := stream.firstNonZeroDeadline()
			if !ok {
				t.Fatal("speech stream did not receive a deadline")
			}
			if got := deadline.Sub(start); got < test.timeout-time.Second || got > test.timeout+time.Second {
				t.Fatalf("speech stream deadline = %s, want %s", got, test.timeout)
			}
		})
	}
}

type speechDeadlinePeerConn struct {
	stream net.Conn
}

func (c *speechDeadlinePeerConn) Dial(uint64) (net.Conn, error)             { return c.stream, nil }
func (*speechDeadlinePeerConn) ListenService(uint64) giznet.ServiceListener { return nil }
func (*speechDeadlinePeerConn) CloseService(uint64) error                   { return nil }
func (*speechDeadlinePeerConn) Read([]byte) (byte, int, error)              { return 0, 0, net.ErrClosed }
func (*speechDeadlinePeerConn) Write(byte, []byte) (int, error)             { return 0, net.ErrClosed }
func (*speechDeadlinePeerConn) PublicKey() giznet.PublicKey                 { return giznet.PublicKey{} }
func (*speechDeadlinePeerConn) PeerInfo() *giznet.PeerInfo                  { return nil }
func (*speechDeadlinePeerConn) Close() error                                { return nil }

type deadlineRecordingConn struct {
	mu        sync.Mutex
	deadlines []time.Time
}

func (*deadlineRecordingConn) Read([]byte) (int, error)         { return 0, net.ErrClosed }
func (*deadlineRecordingConn) Write([]byte) (int, error)        { return 0, net.ErrClosed }
func (*deadlineRecordingConn) Close() error                     { return nil }
func (*deadlineRecordingConn) LocalAddr() net.Addr              { return nil }
func (*deadlineRecordingConn) RemoteAddr() net.Addr             { return nil }
func (*deadlineRecordingConn) SetReadDeadline(time.Time) error  { return nil }
func (*deadlineRecordingConn) SetWriteDeadline(time.Time) error { return nil }
func (c *deadlineRecordingConn) SetDeadline(deadline time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deadlines = append(c.deadlines, deadline)
	return nil
}

func (c *deadlineRecordingConn) firstNonZeroDeadline() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, deadline := range c.deadlines {
		if !deadline.IsZero() {
			return deadline, true
		}
	}
	return time.Time{}, false
}

func TestTranscribeSpeechStreamsReaderBeforeEOF(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	release := make(chan struct{})
	serverDone := make(chan error, 1)
	go func() {
		stream, err := newRPCStream(context.Background(), serverSide)
		if err != nil {
			serverDone <- err
			return
		}
		defer stream.Close()
		request, requestEOS, err := stream.ReadRequestEnvelope()
		if err != nil {
			serverDone <- err
			return
		}
		if requestEOS {
			serverDone <- io.ErrUnexpectedEOF
			return
		}
		first, err := stream.ReadFrame()
		if err != nil {
			serverDone <- err
			return
		}
		if first.Type != rpcapi.FrameTypeBinary || !bytes.Equal(first.Payload, []byte{1, 2}) {
			serverDone <- io.ErrUnexpectedEOF
			return
		}
		close(release)
		second, err := stream.ReadFrame()
		if err != nil {
			serverDone <- err
			return
		}
		if second.Type != rpcapi.FrameTypeBinary || !bytes.Equal(second.Payload, []byte{3, 4}) {
			serverDone <- io.ErrUnexpectedEOF
			return
		}
		if err := stream.ReadEOS(); err != nil {
			serverDone <- err
			return
		}
		response, err := newRPCResultResponse(request.Id, rpcapi.SpeechTranscribeResponse{Transcript: "hello"}, (*rpcapi.RPCPayload).FromSpeechTranscribeResponse)
		if err == nil {
			_, err = stream.WriteResponseEnvelopeForMethod(request.Method, response)
		}
		if err == nil {
			err = stream.WriteEOS()
		}
		serverDone <- err
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := (&rpcClient{}).TranscribeSpeech(ctx, clientSide, "transcribe", rpcapi.SpeechTranscribeRequest{
		ModelAlias:  "asr-main",
		ContentType: "audio/L16;rate=16000;channels=1",
	}, &gatedSpeechReader{release: release})
	if err != nil {
		t.Fatalf("TranscribeSpeech() error = %v", err)
	}
	if result.Transcript != "hello" {
		t.Fatalf("Transcript = %q", result.Transcript)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("server error = %v", err)
	}
}

func TestTranscribeSpeechReturnsEarlyServerErrorBeforeAudioEOF(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	serverDone := make(chan error, 1)
	go func() {
		stream, err := newRPCStream(context.Background(), serverSide)
		if err != nil {
			serverDone <- err
			return
		}
		defer stream.Close()
		request, requestEOS, err := stream.ReadRequestEnvelope()
		if err == nil && requestEOS {
			err = io.ErrUnexpectedEOF
		}
		if err == nil {
			_, err = stream.WriteResponseEnvelopeForMethod(request.Method, rpcapi.Error{
				RequestID: request.Id,
				Code:      rpcapi.RPCErrorCodeInvalidParams,
				Message:   "model alias is invalid",
			}.RPCResponse())
		}
		if err == nil {
			err = stream.WriteEOS()
		}
		serverDone <- err
	}()

	audio, writer := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := (&rpcClient{}).TranscribeSpeech(ctx, clientSide, "invalid", rpcapi.SpeechTranscribeRequest{
		ModelAlias: "missing", ContentType: "audio/L16;rate=16000;channels=1",
	}, audio)
	if result != nil || err == nil || !strings.Contains(err.Error(), "model alias is invalid") {
		t.Fatalf("TranscribeSpeech() = (%+v, %v)", result, err)
	}
	if _, err := writer.Write([]byte{1}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("audio writer error = %v, want closed pipe", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("server error = %v", err)
	}
}

func TestSynthesizeSpeechWritesFirstFrameBeforeEOS(t *testing.T) {
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()
	firstWritten := make(chan struct{})
	serverDone := make(chan error, 1)
	go func() {
		stream, err := newRPCStream(context.Background(), serverSide)
		if err != nil {
			serverDone <- err
			return
		}
		defer stream.Close()
		request, requestEOS, err := stream.ReadRequestEnvelope()
		if err == nil && !requestEOS {
			err = stream.ReadEOS()
		}
		response, responseErr := newRPCResultResponse(request.Id, rpcapi.SpeechSynthesizeResponse{ContentType: "audio/pcm"}, (*rpcapi.RPCPayload).FromSpeechSynthesizeResponse)
		if err == nil {
			err = responseErr
		}
		if err == nil {
			_, err = stream.WriteResponseEnvelopeForMethod(request.Method, response)
		}
		if err == nil {
			err = stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{1, 2}})
		}
		if err == nil {
			select {
			case <-firstWritten:
			case <-time.After(time.Second):
				err = context.DeadlineExceeded
			}
		}
		if err == nil {
			err = stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: []byte{3, 4}})
		}
		if err == nil {
			err = stream.WriteEOS()
		}
		serverDone <- err
	}()

	writer := &signalingSpeechWriter{firstWritten: firstWritten}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := (&rpcClient{}).SynthesizeSpeech(ctx, clientSide, "synthesize", rpcapi.SpeechSynthesizeRequest{
		VoiceAlias:           "narrator",
		Text:                 "hello",
		AcceptedContentTypes: []string{"audio/pcm"},
	}, writer)
	if err != nil {
		t.Fatalf("SynthesizeSpeech() error = %v", err)
	}
	if result.Metadata.ContentType != "audio/pcm" || result.Bytes != 4 || !bytes.Equal(writer.Bytes(), []byte{1, 2, 3, 4}) {
		t.Fatalf("SynthesizeSpeech() = (%+v, %v)", result, writer.Bytes())
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("server error = %v", err)
	}
}

type gatedSpeechReader struct {
	index   int
	release <-chan struct{}
}

func (r *gatedSpeechReader) Read(buffer []byte) (int, error) {
	switch r.index {
	case 0:
		r.index++
		return copy(buffer, []byte{1, 2}), nil
	case 1:
		r.index++
		<-r.release
		return copy(buffer, []byte{3, 4}), io.EOF
	default:
		return 0, io.EOF
	}
}

type signalingSpeechWriter struct {
	bytes.Buffer
	firstWritten chan struct{}
	once         sync.Once
}

func (w *signalingSpeechWriter) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	if n > 0 {
		w.once.Do(func() { close(w.firstWritten) })
	}
	return n, err
}
