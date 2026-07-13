package gizwebrtc

import (
	"testing"
	"time"
)

func TestHasTURNServer(t *testing.T) {
	tests := []struct {
		name    string
		servers []ICEServer
		want    bool
	}{
		{
			name:    "empty",
			servers: nil,
			want:    false,
		},
		{
			name: "stun only",
			servers: []ICEServer{{
				URLs: []string{"stun:edge.example.com:3478", "stuns:edge.example.com:5349"},
			}},
			want: false,
		},
		{
			name: "turn url",
			servers: []ICEServer{{
				URLs: []string{" turn:edge.example.com:3478?transport=udp "},
			}},
			want: true,
		},
		{
			name: "turns url",
			servers: []ICEServer{{
				URLs: []string{"TURNS:edge.example.com:5349"},
			}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasTURNServer(tt.servers); got != tt.want {
				t.Fatalf("HasTURNServer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebRTCICEServersMintTURNRESTCredentials(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	servers := webrtcICEServersAt([]ICEServer{{
		URLs:           []string{"turn:edge.example.com:3478?transport=udp"},
		Username:       "edge",
		Credential:     "long-term-secret",
		CredentialMode: ICECredentialModeTURNREST,
	}}, now)
	if len(servers) != 1 {
		t.Fatalf("servers len = %d, want 1", len(servers))
	}
	got := servers[0]
	if got.Username != "1700000600:edge" {
		t.Fatalf("username = %q, want short-lived REST username", got.Username)
	}
	if got.Credential == "" || got.Credential == "long-term-secret" {
		t.Fatalf("credential = %q, want minted credential", got.Credential)
	}
	if want := turnRESTCredential("long-term-secret", got.Username); got.Credential != want {
		t.Fatalf("credential = %q, want %q", got.Credential, want)
	}
}
