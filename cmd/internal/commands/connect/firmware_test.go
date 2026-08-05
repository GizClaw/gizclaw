package connectcmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestFirmwareHelpOnlyExposesGet(t *testing.T) {
	cmd := NewCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"firmware", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "get") || strings.Contains(out.String(), "download") {
		t.Fatalf("firmware help = %s", out.String())
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
	for _, want := range []string{"--channel", "--context", "--timeout"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("firmware get help missing %q: %s", want, out.String())
		}
	}
}

func TestFirmwareChannelFlag(t *testing.T) {
	for _, value := range []string{"stable", " beta ", "develop", "pending"} {
		if _, err := firmwareChannelFlag(value); err != nil {
			t.Fatalf("firmwareChannelFlag(%q): %v", value, err)
		}
	}
	for _, value := range []string{"", "rollback"} {
		if _, err := firmwareChannelFlag(value); err == nil {
			t.Fatalf("firmwareChannelFlag(%q) should fail", value)
		}
	}
}

func TestFirmwareGetRejectsMissingOrInvalidChannel(t *testing.T) {
	for _, args := range [][]string{
		{"firmware", "get"},
		{"firmware", "get", "--channel", "rollback"},
	} {
		cmd := NewCmd()
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "channel must be one of") {
			t.Fatalf("%v error = %v", args, err)
		}
	}
}

func TestFirmwareGetPropagatesConnectError(t *testing.T) {
	errConnect := errors.New("connect failed")
	original := connectFromContext
	connectFromContext = func(string) (*gizcli.Client, error) { return nil, errConnect }
	t.Cleanup(func() { connectFromContext = original })

	cmd := NewCmd()
	cmd.SetArgs([]string{"firmware", "get", "--channel", "stable"})
	if err := cmd.Execute(); !errors.Is(err, errConnect) {
		t.Fatalf("error = %v, want %v", err, errConnect)
	}
}
