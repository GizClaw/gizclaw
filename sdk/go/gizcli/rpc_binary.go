package gizcli

import (
	"fmt"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func copyBinaryFrames(out io.Writer, stream *rpcStream) (int64, error) {
	var written int64
	for {
		frame, err := stream.ReadFrame()
		if err != nil {
			return written, err
		}
		if frame.Type == rpcapi.FrameTypeEOS {
			return written, nil
		}
		if frame.Type != rpcapi.FrameTypeBinary {
			return written, fmt.Errorf("rpc: expected binary frame, got type %d", frame.Type)
		}
		n, err := out.Write(frame.Payload)
		written += int64(n)
		if err != nil {
			return written, err
		}
		if n != len(frame.Payload) {
			return written, io.ErrShortWrite
		}
	}
}
