package gizwebrtc

import "testing"

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
