package gizwebrtc

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// MaxCredentialBytes bounds the opaque credential inside an encrypted offer.
const MaxCredentialBytes = 4096

const offerMagic = "GZOF"
const offerEnvelopeHeaderSize = 7 // magic, version, uint16 credential length

var errInvalidCredential = errors.New("gizwebrtc: invalid admission credential")

func encodeOfferEnvelope(sdp string, credential []byte) ([]byte, error) {
	if len(credential) > MaxCredentialBytes {
		return nil, errInvalidCredential
	}
	if len(credential) == 0 {
		return []byte(sdp), nil
	}
	out := make([]byte, offerEnvelopeHeaderSize+len(credential)+len(sdp))
	copy(out, offerMagic)
	out[4] = 1
	binary.BigEndian.PutUint16(out[5:7], uint16(len(credential)))
	copy(out[7:], credential)
	copy(out[7+len(credential):], sdp)
	return out, nil
}

func decodeOfferEnvelope(plaintext []byte) (sdp, credential []byte, err error) {
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
	return plaintext[7+n:], plaintext[7 : 7+n], nil
}
