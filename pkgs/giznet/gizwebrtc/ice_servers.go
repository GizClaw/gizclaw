package gizwebrtc

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pion/webrtc/v4"
)

type ICEServer struct {
	URLs           []string `json:"urls" yaml:"urls"`
	Username       string   `json:"username,omitempty" yaml:"username,omitempty"`
	Credential     string   `json:"credential,omitempty" yaml:"credential,omitempty"`
	CredentialMode string   `json:"credential_mode,omitempty" yaml:"credential-mode,omitempty"`
}

const (
	ICECredentialModeStatic   = "static"
	ICECredentialModeTURNREST = "turn-rest"

	turnCredentialTTL = 10 * time.Minute
)

func validateICEServers(servers []ICEServer) error {
	for i, server := range servers {
		if len(server.URLs) == 0 {
			return fmt.Errorf("gizwebrtc: ice_servers[%d].urls is required", i)
		}
		switch server.CredentialMode {
		case "", ICECredentialModeStatic:
		case ICECredentialModeTURNREST:
			if strings.TrimSpace(server.Credential) == "" {
				return fmt.Errorf("gizwebrtc: ice_servers[%d].credential is required for credential-mode %q", i, ICECredentialModeTURNREST)
			}
		default:
			return fmt.Errorf("gizwebrtc: ice_servers[%d].credential-mode has unsupported value %q", i, server.CredentialMode)
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
	return webrtcICEServersAt(servers, time.Now())
}

func webrtcICEServersAt(servers []ICEServer, now time.Time) []webrtc.ICEServer {
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
		username := server.Username
		credential := server.Credential
		if server.CredentialMode == ICECredentialModeTURNREST {
			username = turnRESTUsername(now.Add(turnCredentialTTL), server.Username)
			credential = turnRESTCredential(server.Credential, username)
		}
		out = append(out, webrtc.ICEServer{
			URLs:       urls,
			Username:   username,
			Credential: credential,
		})
	}
	return out
}

func turnRESTUsername(expiresAt time.Time, configuredUsername string) string {
	expires := strconv.FormatInt(expiresAt.Unix(), 10)
	if configuredUsername == "" {
		return expires
	}
	return expires + ":" + configuredUsername
}

func turnRESTCredential(secret, username string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write([]byte(username))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func HasTURNServer(servers []ICEServer) bool {
	for _, server := range servers {
		for _, rawURL := range server.URLs {
			url := strings.ToLower(strings.TrimSpace(rawURL))
			if strings.HasPrefix(url, "turn:") || strings.HasPrefix(url, "turns:") {
				return true
			}
		}
	}
	return false
}

func peerConnectionConfiguration(servers []ICEServer, policy webrtc.ICETransportPolicy) webrtc.Configuration {
	return webrtc.Configuration{
		ICEServers:         webrtcICEServers(servers),
		ICETransportPolicy: policy,
	}
}
