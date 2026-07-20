package gizcli

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

type SpeechSynthesisResult struct {
	Metadata rpcapi.SpeechSynthesizeResponse
	Bytes    int64
}

func (c *Client) TranscribeSpeech(ctx context.Context, id string, request rpcapi.SpeechTranscribeRequest, audio io.Reader) (*rpcapi.SpeechTranscribeResponse, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*rpcapi.SpeechTranscribeResponse, error) {
		return client.TranscribeSpeech(ctx, conn, id, request, audio)
	})
}

func (c *Client) SynthesizeSpeech(ctx context.Context, id string, request rpcapi.SpeechSynthesizeRequest, out io.Writer) (*SpeechSynthesisResult, error) {
	return callClientRPC(c, func(client *rpcClient, conn net.Conn) (*SpeechSynthesisResult, error) {
		result, err := client.SynthesizeSpeech(ctx, conn, id, request, out)
		return &result, err
	})
}

func (c *rpcClient) TranscribeSpeech(ctx context.Context, conn net.Conn, id string, request rpcapi.SpeechTranscribeRequest, audio io.Reader) (*rpcapi.SpeechTranscribeResponse, error) {
	if audio == nil {
		return nil, fmt.Errorf("speech transcription audio is required")
	}
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromSpeechTranscribeRequest)
	if err != nil {
		return nil, err
	}
	stream, err := newRPCStream(ctx, conn)
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	if err := stream.WriteRequestEnvelope(newRPCRequest(id, rpcapi.RPCMethodServerSpeechTranscribe, params)); err != nil {
		return nil, err
	}
	type responseResult struct {
		value *rpcapi.SpeechTranscribeResponse
		err   error
	}
	responses := make(chan responseResult, 1)
	go func() {
		value, err := readSpeechTranscriptionResponse(stream)
		responses <- responseResult{value: value, err: err}
	}()
	uploads := make(chan error, 1)
	uploadDone := make(chan struct{})
	go func() {
		defer close(uploadDone)
		uploads <- writeSpeechTranscriptionAudio(stream, audio)
	}()

	for {
		select {
		case response := <-responses:
			interruptSpeechUpload(stream, audio, uploadDone)
			return response.value, response.err
		case err := <-uploads:
			if err != nil {
				select {
				case response := <-responses:
					return response.value, response.err
				default:
					return nil, err
				}
			}
			select {
			case response := <-responses:
				return response.value, response.err
			case <-ctx.Done():
				interruptSpeechUpload(stream, audio, uploadDone)
				return nil, ctx.Err()
			}
		case <-ctx.Done():
			interruptSpeechUpload(stream, audio, uploadDone)
			return nil, ctx.Err()
		}
	}
}

func writeSpeechTranscriptionAudio(stream *rpcStream, audio io.Reader) error {
	buf := make([]byte, rpcapi.MaxFrameSize)
	for {
		n, readErr := audio.Read(buf)
		if n > 0 {
			if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: buf[:n]}); err != nil {
				return err
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				return readErr
			}
			break
		}
	}
	return stream.WriteEOS()
}

func interruptSpeechUpload(stream *rpcStream, audio io.Reader, uploadDone <-chan struct{}) {
	// Stop the context watcher before installing the terminal deadline so the
	// deferred Close cannot clear it while an upload goroutine is still exiting.
	_ = stream.Close()
	_ = stream.conn.SetDeadline(time.Now())
	select {
	case <-uploadDone:
		return
	default:
	}
	if closer, ok := audio.(io.Closer); ok {
		_ = closer.Close()
	}
}

func readSpeechTranscriptionResponse(stream *rpcStream) (*rpcapi.SpeechTranscribeResponse, error) {
	resp, responseEOS, err := stream.ReadResponseEnvelopeForMethod(rpcapi.RPCMethodServerSpeechTranscribe)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		if !responseEOS {
			_ = stream.ReadEOS()
		}
		return nil, fmt.Errorf("rpc: %w", rpcapi.Error{RequestID: resp.Id, Code: resp.Error.Code, Message: resp.Error.Message})
	}
	if resp.Result == nil {
		return nil, errRPCMissingResult
	}
	result, err := resp.Result.AsSpeechTranscribeResponse()
	if err != nil {
		return nil, wrapRPCResultError("speech transcribe", err)
	}
	if !responseEOS {
		if err := stream.ReadEOS(); err != nil {
			return nil, err
		}
	}
	return &result, nil
}

func (c *rpcClient) SynthesizeSpeech(ctx context.Context, conn net.Conn, id string, request rpcapi.SpeechSynthesizeRequest, out io.Writer) (SpeechSynthesisResult, error) {
	if out == nil {
		return SpeechSynthesisResult{}, fmt.Errorf("speech synthesis output is required")
	}
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromSpeechSynthesizeRequest)
	if err != nil {
		return SpeechSynthesisResult{}, err
	}
	stream, err := newRPCStream(ctx, conn)
	if err != nil {
		return SpeechSynthesisResult{}, err
	}
	defer stream.Close()
	if err := stream.WriteRequestEnvelope(newRPCRequest(id, rpcapi.RPCMethodServerSpeechSynthesize, params)); err != nil {
		return SpeechSynthesisResult{}, err
	}
	if err := stream.WriteEOS(); err != nil {
		return SpeechSynthesisResult{}, err
	}
	resp, responseEOS, err := stream.ReadResponseEnvelopeForMethod(rpcapi.RPCMethodServerSpeechSynthesize)
	if err != nil {
		return SpeechSynthesisResult{}, err
	}
	if resp.Error != nil {
		if !responseEOS {
			_ = stream.ReadEOS()
		}
		return SpeechSynthesisResult{}, fmt.Errorf("rpc: %w", rpcapi.Error{RequestID: resp.Id, Code: resp.Error.Code, Message: resp.Error.Message})
	}
	if resp.Result == nil {
		return SpeechSynthesisResult{}, errRPCMissingResult
	}
	metadata, err := resp.Result.AsSpeechSynthesizeResponse()
	if err != nil {
		return SpeechSynthesisResult{}, wrapRPCResultError("speech synthesize", err)
	}
	written, err := copyBinaryFrames(out, stream)
	if err != nil {
		return SpeechSynthesisResult{}, err
	}
	return SpeechSynthesisResult{Metadata: metadata, Bytes: written}, nil
}
