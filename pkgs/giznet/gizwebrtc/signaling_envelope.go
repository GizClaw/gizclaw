package gizwebrtc

import (
	"bytes"
	"encoding/binary"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet/giznetpb"
	"google.golang.org/protobuf/proto"
)

// MaxCredentialBytes bounds the encoded protobuf credential inside an encrypted offer.
const MaxCredentialBytes = 4096

const offerMagic = "GZOF"
const offerEnvelopeHeaderSize = 7 // magic, version, uint16 credential length

var errInvalidCredential = errors.New("gizwebrtc: invalid admission credential")

func encodeOfferEnvelope(sdp string, credential *giznetpb.AdmissionCredential) ([]byte, error) {
	if credential == nil {
		return []byte(sdp), nil
	}
	if len(credential.Type) > 128 || len(credential.Value) > MaxCredentialBytes || proto.Size(credential) > MaxCredentialBytes {
		return nil, errInvalidCredential
	}
	encoded, err := proto.Marshal(credential)
	if err != nil || len(encoded) == 0 {
		return nil, errInvalidCredential
	}
	out := make([]byte, offerEnvelopeHeaderSize+len(encoded)+len(sdp))
	copy(out, offerMagic)
	out[4] = 1
	binary.BigEndian.PutUint16(out[5:7], uint16(len(encoded)))
	copy(out[7:], encoded)
	copy(out[7+len(encoded):], sdp)
	return out, nil
}

func decodeOfferEnvelope(plaintext []byte) (sdp []byte, credential *giznetpb.AdmissionCredential, err error) {
	if !bytes.HasPrefix(plaintext, []byte(offerMagic)) {
		return plaintext, nil, nil
	}
	if len(plaintext) < offerEnvelopeHeaderSize || plaintext[4] != 1 {
		return nil, nil, errInvalidCredential
	}
	n := int(binary.BigEndian.Uint16(plaintext[5:7]))
	if n > MaxCredentialBytes || n > len(plaintext)-offerEnvelopeHeaderSize {
		return nil, nil, errInvalidCredential
	}
	if n == 0 {
		return plaintext[7:], nil, nil
	}
	credential = new(giznetpb.AdmissionCredential)
	if err := proto.Unmarshal(plaintext[7:7+n], credential); err != nil || len(credential.Type) > 128 || len(credential.Value) > MaxCredentialBytes {
		return nil, nil, errInvalidCredential
	}
	return plaintext[7+n:], credential, nil
}
