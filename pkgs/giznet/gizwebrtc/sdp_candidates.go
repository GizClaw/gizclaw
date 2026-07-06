package gizwebrtc

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/pion/ice/v4"
	"github.com/pion/sdp/v3"
)

func rewriteSDPHostCandidates(rawSDP, publicUDPAddr, publicTCPAddr string) (string, error) {
	udpHost, udpPort, hasUDP, err := parsePublicICEAddr(publicUDPAddr)
	if err != nil {
		return "", err
	}
	tcpHost, tcpPort, hasTCP, err := parsePublicICEAddr(publicTCPAddr)
	if err != nil {
		return "", err
	}
	if !hasUDP && !hasTCP {
		return rawSDP, nil
	}

	var desc sdp.SessionDescription
	if err := desc.UnmarshalString(rawSDP); err != nil {
		return "", fmt.Errorf("gizwebrtc: parse answer sdp for public ICE endpoint: %w", err)
	}

	changed, err := rewriteCandidateAttributes(desc.Attributes, publicICECandidateAddrs{
		udpHost: udpHost,
		udpPort: udpPort,
		hasUDP:  hasUDP,
		tcpHost: tcpHost,
		tcpPort: tcpPort,
		hasTCP:  hasTCP,
	})
	if err != nil {
		return "", err
	}
	desc.Attributes = changed
	for _, media := range desc.MediaDescriptions {
		attrs, err := rewriteCandidateAttributes(media.Attributes, publicICECandidateAddrs{
			udpHost: udpHost,
			udpPort: udpPort,
			hasUDP:  hasUDP,
			tcpHost: tcpHost,
			tcpPort: tcpPort,
			hasTCP:  hasTCP,
		})
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

type publicICECandidateAddrs struct {
	udpHost string
	udpPort string
	hasUDP  bool
	tcpHost string
	tcpPort string
	hasTCP  bool
}

func rewriteSDPUDPHostCandidates(rawSDP, publicAddr string) (string, error) {
	return rewriteSDPHostCandidates(rawSDP, publicAddr, "")
}

func parsePublicICEAddr(addr string) (host string, port string, ok bool, err error) {
	if strings.TrimSpace(addr) == "" {
		return "", "", false, nil
	}
	host, port, err = net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return "", "", false, fmt.Errorf("gizwebrtc: invalid public ICE address: %w", err)
	}
	if strings.TrimSpace(host) == "" {
		return "", "", false, fmt.Errorf("gizwebrtc: public ICE address host is empty")
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsUnspecified() {
		return "", "", false, nil
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return "", "", false, fmt.Errorf("gizwebrtc: public ICE address port is invalid: %w", err)
	}
	return host, port, true, nil
}

func rewriteCandidateAttributes(attrs []sdp.Attribute, addrs publicICECandidateAddrs) ([]sdp.Attribute, error) {
	out := make([]sdp.Attribute, len(attrs))
	copy(out, attrs)
	for i := range out {
		if !out[i].IsICECandidate() {
			continue
		}
		value, changed, err := rewriteHostCandidate(out[i].Value, addrs)
		if err != nil {
			return nil, err
		}
		if changed {
			out[i].Value = value
		}
	}
	return out, nil
}

func rewriteHostCandidate(value string, addrs publicICECandidateAddrs) (string, bool, error) {
	candidate, err := ice.UnmarshalCandidate(value)
	if err != nil {
		return "", false, fmt.Errorf("gizwebrtc: parse answer ICE candidate: %w", err)
	}
	if candidate.Type() != ice.CandidateTypeHost {
		return value, false, nil
	}
	host := ""
	port := ""
	switch candidate.NetworkType() {
	case ice.NetworkTypeUDP4, ice.NetworkTypeUDP6:
		if !addrs.hasUDP {
			return value, false, nil
		}
		host, port = addrs.udpHost, addrs.udpPort
	case ice.NetworkTypeTCP4, ice.NetworkTypeTCP6:
		if !addrs.hasTCP {
			return value, false, nil
		}
		host, port = addrs.tcpHost, addrs.tcpPort
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
