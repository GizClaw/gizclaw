package peergenx

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestSayUsesVoiceTransformerAndConsumesOutput(t *testing.T) {
	ctx := context.Background()
	events := []string{}
	output := &recordingAudioOutput{}
	svc := New(Service{
		Peer:            newTestPeer(),
		Voices:          fakeVoices{events: &events},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
		AudioOutput:     output,
	})

	resp, err := svc.Say(ctx, SayRequest{Text: " hello ", VoiceID: "cancan"})
	if err != nil {
		t.Fatalf("Say() error = %v", err)
	}
	if !resp.Accepted {
		t.Fatalf("Say() = %+v, want accepted", resp)
	}
	wantEvents := []string{
		"get:voice:cancan",
		"get:tenant:volc:main",
		"get:credential:volc-token",
		"build:transformer:voice:cancan",
		"call:transformer:voice/cancan",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %#v, want %#v", events, wantEvents)
	}
	if !reflect.DeepEqual(output.texts, []string{"hello"}) {
		t.Fatalf("output texts = %#v, want hello", output.texts)
	}
}

func TestSayValidatesRequest(t *testing.T) {
	svc := New(Service{AudioOutput: &recordingAudioOutput{}})
	for _, tc := range []struct {
		name string
		req  SayRequest
		want error
	}{
		{name: "text", req: SayRequest{VoiceID: "v"}, want: ErrInvalid},
		{name: "target", req: SayRequest{Text: "hello"}, want: ErrInvalid},
		{name: "model", req: SayRequest{Text: "hello", ModelID: "tts"}, want: ErrUnsupported},
		{name: "credential override", req: SayRequest{Text: "hello", VoiceID: "v", CredentialName: "key"}, want: ErrUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Say(context.Background(), tc.req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Say() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestResolveTransformerAllowsTTSModel(t *testing.T) {
	events := []string{}
	svc := New(Service{
		Peer:            newTestPeer(),
		Models:          fakeModels{events: &events, modelKind: apitypes.ModelKindTts, providerKind: "volc-tenant"},
		Credentials:     fakeCredentials{events: &events},
		ProviderTenants: fakeTenants{events: &events},
		Builder:         fakeBuilder{events: &events},
	})

	if _, err := svc.Transformer().Transform(context.Background(), "model/tts", fakeStream{}); err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
}

type recordingAudioOutput struct {
	texts []string
}

func (o *recordingAudioOutput) ConsumeAgentOutput(_ context.Context, output genx.Stream) error {
	for {
		chunk, err := output.Next()
		if err != nil {
			if errors.Is(err, genx.ErrDone) {
				return nil
			}
			return err
		}
		if chunk == nil || chunk.IsEndOfStream() {
			continue
		}
		if text, ok := chunk.Part.(genx.Text); ok {
			o.texts = append(o.texts, string(text))
		}
	}
}
