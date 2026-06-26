package connectcmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
)

func TestFirmwareHelp(t *testing.T) {
	cmd := NewCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"firmware", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"list", "get", "download"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("firmware help missing %q: %s", want, out.String())
		}
	}
}

func TestFirmwareListHelp(t *testing.T) {
	cmd := NewCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"firmware", "list", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--context", "--cursor", "--limit", "--timeout"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("firmware list help missing %q: %s", want, out.String())
		}
	}
}

func TestFirmwareGetHelp(t *testing.T) {
	cmd := NewCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"firmware", "get", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--firmware-id", "--context", "--timeout"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("firmware get help missing %q: %s", want, out.String())
		}
	}
}

func TestFirmwareDownloadHelp(t *testing.T) {
	cmd := NewCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"firmware", "download", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--firmware-id", "--channel", "--path", "--output", "--context", "--timeout"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("firmware download help missing %q: %s", want, out.String())
		}
	}
}

func TestFirmwareChannelFlag(t *testing.T) {
	for _, value := range []string{"stable", " beta ", "develop", "pending"} {
		if _, err := firmwareChannelFlag(value); err != nil {
			t.Fatalf("firmwareChannelFlag(%q) error = %v", value, err)
		}
	}
	if _, err := firmwareChannelFlag("rollback"); err == nil {
		t.Fatal("firmwareChannelFlag should reject rollback")
	}
}

func TestFirmwareGetRejectsEmptyFirmwareID(t *testing.T) {
	cmd := NewCmd()
	cmd.SetArgs([]string{"firmware", "get", "--firmware-id", "   "})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "firmware-id must not be empty") {
		t.Fatalf("firmware get empty id err = %v", err)
	}
}

func TestFirmwareDownloadRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "bad channel",
			args: []string{"firmware", "download", "--firmware-id", "devkit", "--channel", "rollback", "--path", "firmware.bin", "--output", "app.bin"},
			want: "channel must be one of stable, beta, develop, pending",
		},
		{
			name: "stdout output",
			args: []string{"firmware", "download", "--firmware-id", "devkit", "--channel", "stable", "--path", "firmware.bin", "--output", "-"},
			want: "output must be a file path",
		},
		{
			name: "missing path",
			args: []string{"firmware", "download", "--firmware-id", "devkit", "--channel", "stable", "--output", "app.bin"},
			want: "path must not be empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewCmd()
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("firmware download err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestFirmwareCommandsPropagateConnectError(t *testing.T) {
	errConnect := errors.New("connect failed")
	original := connectFromContext
	connectFromContext = func(string) (*gizcli.Client, error) {
		return nil, errConnect
	}
	t.Cleanup(func() {
		connectFromContext = original
	})

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "ping", args: []string{"ping"}},
		{name: "server-info", args: []string{"server-info"}},
		{name: "test-speed", args: []string{"test-speed"}},
		{name: "list", args: []string{"firmware", "list"}},
		{name: "get", args: []string{"firmware", "get", "--firmware-id", "devkit"}},
		{name: "download", args: []string{"firmware", "download", "--firmware-id", "devkit", "--channel", "stable", "--path", "firmware.bin", "--output", "app.bin"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewCmd()
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if !errors.Is(err, errConnect) {
				t.Fatalf("%v err = %v, want %v", tc.args, err, errConnect)
			}
		})
	}
}
