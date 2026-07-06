package gizwebrtc

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/pion/ice/v4"
	"github.com/pion/sdp/v3"
)

func rewriteSDPUDPHostCandidates(rawSDP, publicAddr string) (string, error) {
	if strings.TrimSpace(publicAddr) == "" {
		return rawSDP, nil
	}
	host, port, err := splitPublicICEAddr(publicAddr)
	if err != nil {
		return "", err
	}

	var desc sdp.SessionDescription
	if err := desc.UnmarshalString(rawSDP); err != nil {
		return "", fmt.Errorf("gizwebrtc: parse answer sdp for public ICE endpoint: %w", err)
	}

	changed, err := rewriteCandidateAttributes(desc.Attributes, host, port)
	if err != nil {
		return "", err
	}
	desc.Attributes = changed
	for _, media := range desc.MediaDescriptions {
		attrs, err := rewriteCandidateAttributes(media.Attributes, host, port)
		if err != nil {
			return "", err
		}
		media.Attributes = attrs
	}

	out, err := desc.Marshal()
	if err != nil {
		return "", fmt.Errorf("gizwebrtc: marshal answer sdp with public ICE endpoint: %w", err)
	}
	return string(out), nil
}

func splitPublicICEAddr(addr string) (host string, port string, err error) {
	host, port, err = net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return "", "", fmt.Errorf("gizwebrtc: invalid public ICE UDP address: %w", err)
	}
	if strings.TrimSpace(host) == "" {
		return "", "", fmt.Errorf("gizwebrtc: public ICE UDP address host is empty")
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return "", "", fmt.Errorf("gizwebrtc: public ICE UDP address port is invalid: %w", err)
	}
	return host, port, nil
}

func rewriteCandidateAttributes(attrs []sdp.Attribute, host, port string) ([]sdp.Attribute, error) {
	out := make([]sdp.Attribute, len(attrs))
	copy(out, attrs)
	for i := range out {
		if !out[i].IsICECandidate() {
			continue
		}
		value, changed, err := rewriteUDPHostCandidate(out[i].Value, host, port)
		if err != nil {
			return nil, err
		}
		if changed {
			out[i].Value = value
		}
	}
	return out, nil
}

func rewriteUDPHostCandidate(value, host, port string) (string, bool, error) {
	candidate, err := ice.UnmarshalCandidate(value)
	if err != nil {
		return "", false, fmt.Errorf("gizwebrtc: parse answer ICE candidate: %w", err)
	}
	if candidate.Type() != ice.CandidateTypeHost {
		return value, false, nil
	}
	switch candidate.NetworkType() {
	case ice.NetworkTypeUDP4, ice.NetworkTypeUDP6:
	default:
		return value, false, nil
	}

	fields := strings.Fields(value)
	if len(fields) < 6 {
		return "", false, fmt.Errorf("gizwebrtc: malformed answer ICE candidate %q", value)
	}
	fields[4] = host
	fields[5] = port
	return strings.Join(fields, " "), true, nil
}
