package giztestcmd

import (
	"testing"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func TestAudioFixturePlaylistMutationAndPlayback(t *testing.T) {
	fixture, err := newAudioFixture(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	item := &rpcpb.AudioPlayerItem{Url: "https://example.com/real-track.ogg", Title: new("Track"), SourceRef: new("catalog.track")}
	response, err := fixture.invoke(t.Context(), &rpcpb.ClientDeviceAudioPlayerPlaylistSetRequest{Items: []*rpcpb.AudioPlayerItem{item}})
	if err != nil {
		t.Fatal(err)
	}
	set := response.(*rpcpb.ClientDeviceAudioPlayerPlaylistSetResponse)
	if set.Value.State != "stopped" || set.Value.PlaylistLength != 1 || set.Value.PlaylistRevision != 2 {
		t.Fatalf("set state = %v", set.Value)
	}
	item.Url = "https://example.com/caller-mutation.ogg"
	response, err = fixture.invoke(t.Context(), &rpcpb.ClientDeviceAudioPlayerPlaylistGetRequest{})
	if err != nil {
		t.Fatal(err)
	}
	playlist := response.(*rpcpb.ClientDeviceAudioPlayerPlaylistGetResponse)
	if playlist.Items[0].Url != "https://example.com/real-track.ogg" {
		t.Fatal("fixture retained caller-owned item")
	}
	playlist.Items[0].Url = "https://example.com/response-mutation.ogg"
	response, err = fixture.invoke(t.Context(), &rpcpb.ClientDeviceAudioPlayerPlayRequest{Index: new(uint32(0))})
	if err != nil {
		t.Fatal(err)
	}
	if state := response.(*rpcpb.ClientDeviceAudioPlayerPlayResponse).Value; state.State != "playing" || state.GetCurrentIndex() != 0 {
		t.Fatalf("play state = %v", state)
	}
	if _, err := fixture.invoke(t.Context(), &rpcpb.ClientDeviceAudioPlayerPlaylistSetRequest{Items: []*rpcpb.AudioPlayerItem{{Url: "http://invalid.example/track"}}}); err == nil {
		t.Fatal("invalid playlist replaced playback")
	}
	response, err = fixture.invoke(t.Context(), &rpcpb.ClientDeviceAudioPlayerGetRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if state := response.(*rpcpb.ClientDeviceAudioPlayerGetResponse).Value; state.State != "playing" || state.PlaylistRevision != 2 {
		t.Fatalf("rejected set changed player: %v", state)
	}
	appended := &rpcpb.AudioPlayerItem{Url: "https://example.com/second.ogg"}
	if _, err := fixture.invoke(t.Context(), &rpcpb.ClientDeviceAudioPlayerPlaylistAppendRequest{Items: []*rpcpb.AudioPlayerItem{appended}}); err != nil {
		t.Fatal(err)
	}
	appended.Url = "https://example.com/changed.ogg"
	response, err = fixture.invoke(t.Context(), &rpcpb.ClientDeviceAudioPlayerPlaylistGetRequest{})
	if err != nil {
		t.Fatal(err)
	}
	playlist = response.(*rpcpb.ClientDeviceAudioPlayerPlaylistGetResponse)
	if len(playlist.Items) != 2 || playlist.Items[1].Url != "https://example.com/second.ogg" || playlist.PlaylistRevision != 3 {
		t.Fatalf("append = %v", playlist)
	}
	tooMany := make([]*rpcpb.AudioPlayerItem, 32)
	for index := range tooMany {
		tooMany[index] = &rpcpb.AudioPlayerItem{Url: "https://example.com/append.ogg"}
	}
	if _, err := fixture.invoke(t.Context(), &rpcpb.ClientDeviceAudioPlayerPlaylistAppendRequest{Items: tooMany}); err == nil {
		t.Fatal("append exceeded player capacity")
	}
	if len(fixture.items) != 2 || fixture.status.State != "playing" {
		t.Fatal("failed append changed playback")
	}
}
