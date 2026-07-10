package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/internal/serverrpc"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	server, err := serverrpc.New()
	if err != nil {
		return err
	}
	defer server.Close()
	if err := writeJSON(map[string]string{
		"endpoint":   server.Endpoint,
		"public_key": server.PublicKey.String(),
	}); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := runCase(ctx, server, "ping", func(conn giznet.Conn) error {
		ping, err := serverrpc.Ping(conn, "js-server-ping")
		if err != nil {
			return fmt.Errorf("server-initiated ping: %w", err)
		}
		if ping.ServerTime <= 0 {
			return fmt.Errorf("server-initiated ping returned server_time=%d", ping.ServerTime)
		}
		return nil
	}); err != nil {
		return err
	}
	for _, test := range []struct {
		name string
		up   int64
		down int64
	}{
		{name: "zero"},
		{name: "upload-only", up: 32*1024 + 7},
		{name: "download-only", down: 32*1024 + 11},
		{name: "full-duplex", up: 64*1024 + 3, down: 64*1024 + 5},
	} {
		if err := runCase(ctx, server, test.name, func(conn giznet.Conn) error {
			uploaded, downloaded, err := serverrpc.SpeedTest(conn, "js-server-speed-"+test.name, test.up, test.down)
			if err != nil {
				return fmt.Errorf("server-initiated speed test %s: %w", test.name, err)
			}
			if uploaded != test.up || downloaded != test.down {
				return fmt.Errorf("server-initiated speed test %s transferred up=%d down=%d, want up=%d down=%d", test.name, uploaded, downloaded, test.up, test.down)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return writeJSON(map[string]bool{"ok": true})
}

func runCase(ctx context.Context, server *serverrpc.Server, name string, run func(giznet.Conn) error) error {
	if err := writeJSON(map[string]string{"case": name}); err != nil {
		return err
	}
	conn, err := server.Accept(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := run(conn); err != nil {
		return err
	}
	return writeJSON(map[string]any{"case": name, "ok": true})
}

func writeJSON(value any) error {
	return json.NewEncoder(os.Stdout).Encode(value)
}
