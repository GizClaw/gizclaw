package gizclaw

import (
	"errors"
	"fmt"
	"io"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

const rpcBinaryFrameSize = 32 * 1024

func writeReaderBinaryFrames(stream *rpcStream, reader io.Reader) error {
	if reader == nil {
		return fmt.Errorf("rpc: nil binary reader")
	}
	buf := make([]byte, rpcBinaryFrameSize)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if err := stream.WriteFrame(rpcapi.Frame{Type: rpcapi.FrameTypeBinary, Payload: buf[:n]}); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return stream.WriteEOS()
		}
		if err != nil {
			return err
		}
	}
}
