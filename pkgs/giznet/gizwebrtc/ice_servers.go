package gizwebrtc

import (
	"fmt"
	"strings"

	"github.com/pion/webrtc/v4"
)

type ICEServer struct {
	URLs       []string `json:"urls" yaml:"urls"`
	Username   string   `json:"username,omitempty" yaml:"username,omitempty"`
	Credential string   `json:"credential,omitempty" yaml:"credential,omitempty"`
}

func validateICEServers(servers []ICEServer) error {
	for i, server := range servers {
		if len(server.URLs) == 0 {
			return fmt.Errorf("gizwebrtc: ice_servers[%d].urls is required", i)
		}
		for j, rawURL := range server.URLs {
			url := strings.TrimSpace(rawURL)
			if url == "" {
				return fmt.Errorf("gizwebrtc: ice_servers[%d].urls[%d] is empty", i, j)
			}
			if !strings.HasPrefix(url, "stun:") && !strings.HasPrefix(url, "stuns:") &&
				!strings.HasPrefix(url, "turn:") && !strings.HasPrefix(url, "turns:") {
				return fmt.Errorf("gizwebrtc: ice_servers[%d].urls[%d] has unsupported scheme", i, j)
			}
		}
	}
	return nil
}

func webrtcICEServers(servers []ICEServer) []webrtc.ICEServer {
	if len(servers) == 0 {
		return nil
	}
	out := make([]webrtc.ICEServer, 0, len(servers))
	for _, server := range servers {
		urls := make([]string, 0, len(server.URLs))
		for _, rawURL := range server.URLs {
			if url := strings.TrimSpace(rawURL); url != "" {
				urls = append(urls, url)
			}
		}
		if len(urls) == 0 {
			continue
		}
		out = append(out, webrtc.ICEServer{
			URLs:       urls,
			Username:   server.Username,
			Credential: server.Credential,
		})
	}
	return out
}
