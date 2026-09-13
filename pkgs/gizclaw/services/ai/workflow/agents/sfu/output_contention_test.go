package sfu

import (
	"context"
	"slices"
	"testing"
	"time"
)

// TestRealtimeContinuousStreamStaysOneUtteranceAndKeepsDownlinkOrder covers
// the continuity the SFU Workspace promises in realtime mode. Uplink: voiced
// frames that keep arriving inside the hangover span several hangover windows
// as one utterance, with no spurious eos/bos pair splitting the stream in
// two. Downlink: once the floor is held, every packet of the holder's burst
// reaches the device once, in the order it was sent, on a single stream.
func TestRealtimeContinuousStreamStaysOneUtteranceAndKeepsDownlinkOrder(t *testing.T) {
	h := newHarness(t, Config{FloorIdle: 5 * time.Second})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, input := h.attach(ctx, testPeer)
	client := h.connector.client(0)

	input.push(inputBOS("live"))
	input.push(voicedChunk(0x01))
	waitFor(t, func() bool { return client.talkCount() == 1 }, "one bos for the whole stream")
	stop := time.Now().Add(4 * testTalkHangover)
	for time.Now().Before(stop) {
		input.push(voicedChunk(0x01))
		time.Sleep(testTalkHangover / 8)
		if got := client.talkCount(); got != 1 {
			t.Fatalf("continuous stream published %d talk messages mid-stream, want only the bos", got)
		}
	}
	if !h.talking(testPeer) {
		t.Fatal("utterance closed while voiced frames were still arriving")
	}
	waitFor(t, func() bool { return client.talkCount() == 2 }, "eos once the stream stops")
	talk := client.talk()
	if len(talk) != 2 || talk[0].Type != talkTypeBOS || talk[1].Type != talkTypeEOS || talk[0].Utterance != talk[1].Utterance {
		t.Fatalf("talk messages = %+v, want exactly one bos/eos pair for one utterance", talk)
	}

	reader := subscribeRemote(t, h, client, testRemote, "TR_b", "u-b", 1)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "floor granted")
	const burst = 40
	want := make([]byte, 0, burst)
	for i := range burst {
		marker := byte(i + 1)
		want = append(want, marker)
		reader.voiced(marker)
	}
	waitFor(t, func() bool { return h.queued(testPeer) == burst+1 }, "burst forwarded")
	cancel()
	chunks, _ := collect(t, output)
	streams := groupStreams(t, chunks)
	if len(streams) != 1 || streams[0].label != testRemote {
		t.Fatalf("streams = %+v, want one hold for the remote", streams)
	}
	if got := markers(streams[0].payload); !slices.Equal(got, want) {
		t.Fatalf("downlink markers = %x, want the burst in order", got)
	}
}

/*
TestUngatedRealtimeStreamHoldsTheFloorIndefinitely pins the documented
limitation of the frame-based uplink rule in consumeInput: voiced means "not
an Opus silence frame", so a device that streams raw, un-gated audio in
realtime mode never lets its utterance close. It therefore holds every
listener's downlink release open for as long as it streams and hears nothing
itself.

The fix is a sender-side energy VAD that emits DTX or stops sending while
nobody speaks, which is deliberately out of scope of the connector. Adding one
here, or any Server-side voice activity detection, changes this behaviour and
must change this test with it.
*/
func TestUngatedRealtimeStreamHoldsTheFloorIndefinitely(t *testing.T) {
	h := newHarness(t, Config{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, input := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	reader := subscribeRemote(t, h, client, testRemote, "TR_b", "u-b", 1)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "floor to the remote")

	input.push(inputBOS("live"))
	input.push(voicedChunk(0x01))
	waitFor(t, func() bool { return h.floorHolder(testPeer) == "" && h.talking(testPeer) }, "floor released by the local talk")
	queued := h.queued(testPeer)

	// Every frame is "voiced" because none of them is an Opus silence frame,
	// which is exactly what an un-gated device sends between words.
	stop := time.Now().Add(6 * testTalkHangover)
	for time.Now().Before(stop) {
		input.push(voicedChunk(0x02))
		reader.voiced(0xB1)
		time.Sleep(testTalkHangover / 8)
	}
	if got := client.talkCount(); got != 1 || !h.talking(testPeer) {
		t.Fatalf("un-gated stream closed its utterance after %d talk messages; a VAD change must update this test", got)
	}
	if holder := h.floorHolder(testPeer); holder != "" {
		t.Fatalf("floor holder = %q, want none while the local utterance stays open", holder)
	}
	if got := h.queued(testPeer); got != queued {
		t.Fatalf("queued downlink chunks = %d, want %d: remote audio reached a device that never stopped talking", got, queued)
	}
	if h.dropped(testPeer) == 0 {
		t.Fatal("remote packets were neither forwarded nor counted as dropped")
	}
}

// TestFloorHolderBurstSurvivesContention is the packet-level continuity rule:
// while one identity holds the floor, every other identity's voiced packets
// are dropped and counted, and the holder's own burst reaches the device
// complete and in order from its very first packet, including the packets
// that raced ahead of the holder's BOS on the data channel.
func TestFloorHolderBurstSurvivesContention(t *testing.T) {
	h := newHarness(t, Config{FloorIdle: 5 * time.Second, TalkHangover: 5 * time.Second})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output, _ := h.attach(ctx, testPeer)
	client := h.connector.client(0)
	readerB, readerC := newFakeReader(), newFakeReader()
	client.events.onTrackSubscribed(testRemote, "TR_b", readerB)
	client.events.onTrackSubscribed(testRemoteC, "TR_c", readerC)

	const preroll = 3
	want := make([]byte, 0, preroll)
	for i := range preroll {
		marker := byte(i + 1)
		want = append(want, marker)
		readerB.voiced(marker)
	}
	time.Sleep(20 * time.Millisecond)
	if got := h.queued(testPeer); got != 0 {
		t.Fatalf("queued = %d, want 0: packets were forwarded before any BOS", got)
	}
	client.remoteTalk(testRemote, talkTypeBOS, "u-b", 1)
	waitFor(t, func() bool { return h.floorHolder(testPeer) == testRemote }, "floor to B")
	client.remoteTalk(testRemoteC, talkTypeBOS, "u-c", 1)

	const burst = 24
	for i := range burst {
		marker := byte(preroll + i + 1)
		want = append(want, marker)
		readerB.voiced(marker)
		readerC.voiced(0xC0)
	}
	waitFor(t, func() bool { return h.queued(testPeer) == len(want)+1 }, "holder burst forwarded")
	waitFor(t, func() bool { return h.dropped(testPeer) >= burst-maxPrerollPackets }, "contending packets dropped")
	if holder := h.floorHolder(testPeer); holder != testRemote {
		t.Fatalf("floor holder = %q, want B for the whole burst", holder)
	}
	cancel()
	chunks, _ := collect(t, output)
	streams := groupStreams(t, chunks)
	if len(streams) != 1 || streams[0].label != testRemote {
		t.Fatalf("streams = %+v, want one hold for B", streams)
	}
	if got := markers(streams[0].payload); !slices.Equal(got, want) {
		t.Fatalf("forwarded markers = %x, want B's preroll and burst in order", got)
	}
}
