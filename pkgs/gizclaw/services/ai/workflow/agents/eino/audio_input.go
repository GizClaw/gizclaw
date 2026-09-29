package eino

import (
	"encoding/base64"
	"fmt"

	"github.com/cloudwego/eino/schema"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
)

// userAudioBlobs returns the inline audio parts of one Eino user message as
// GenX Blobs in order. Converting them to a format the model API accepts is
// the model Generator's job.
func userAudioBlobs(parts []schema.MessageInputPart) ([]*genx.Blob, error) {
	blobs := make([]*genx.Blob, 0, len(parts))
	for index, part := range parts {
		if part.Type != schema.ChatMessagePartTypeAudioURL {
			return nil, fmt.Errorf("eino: user message part %d has unsupported type %q", index, part.Type)
		}
		if part.Audio == nil || part.Audio.Base64Data == nil {
			return nil, fmt.Errorf("eino: user audio part %d has no inline data", index)
		}
		data, err := base64.StdEncoding.DecodeString(*part.Audio.Base64Data)
		if err != nil {
			return nil, fmt.Errorf("eino: decode user audio part %d: %w", index, err)
		}
		blobs = append(blobs, &genx.Blob{MIMEType: part.Audio.MIMEType, Data: data})
	}
	return blobs, nil
}
