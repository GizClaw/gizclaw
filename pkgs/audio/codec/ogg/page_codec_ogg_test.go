package ogg

import (
	"bytes"
	"errors"
	"io"
	"math"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
)

func deterministicBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte((i*131 + 17) % 251)
	}
	return b
}

func mustSinglePageRaw(t *testing.T) []byte {
	t.Helper()
	pages, err := BuildPacketPages(100, 1, []byte{1, 2, 3, 4}, 1234, true, true)
	if err != nil {
		t.Fatalf("BuildPacketPages: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("single page expected, got %d", len(pages))
	}
	raw, err := pages[0].MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	return raw
}

func sinePCM16kMono(frameSize int) []int16 {
	p := make([]int16, frameSize)
	for i := range p {
		x := math.Sin(2 * math.Pi * 440 * float64(i) / 16000)
		p[i] = int16(x * 10000)
	}
	return p
}

type failWriter struct{}

func (failWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write failed")
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

type failAfterNWriter struct {
	remain int
}

func (w *failAfterNWriter) Write(p []byte) (int, error) {
	if w.remain <= 0 {
		return 0, errors.New("injected write failure")
	}
	if len(p) <= w.remain {
		w.remain -= len(p)
		return len(p), nil
	}
	n := w.remain
	w.remain = 0
	return n, errors.New("injected write failure")
}

func TestPlatformMatrix(t *testing.T) {
	if !strings.Contains(supportedPlatformDescription, "darwin/arm64") {
		t.Fatalf("supportedPlatformDescription missing darwin/arm64: %q", supportedPlatformDescription)
	}

	cases := []struct {
		goos   string
		goarch string
		want   bool
	}{
		{goos: "linux", goarch: "amd64", want: true},
		{goos: "linux", goarch: "arm64", want: true},
		{goos: "darwin", goarch: "amd64", want: true},
		{goos: "darwin", goarch: "arm64", want: true},
		{goos: "windows", goarch: "amd64", want: false},
		{goos: "linux", goarch: "386", want: false},
	}

	for _, tc := range cases {
		if got := isSupportedPlatform(tc.goos, tc.goarch); got != tc.want {
			t.Fatalf("isSupportedPlatform(%q,%q)=%v, want %v", tc.goos, tc.goarch, got, tc.want)
		}
	}

	wantRuntime := nativeCGOEnabled && isSupportedPlatform(runtime.GOOS, runtime.GOARCH)
	if got := IsRuntimeSupported(); got != wantRuntime {
		t.Fatalf("IsRuntimeSupported()=%v, want %v", got, wantRuntime)
	}
}

func TestPageMarshalAndParseRoundTrip(t *testing.T) {
	page := &Page{
		Version:         0,
		HeaderType:      HeaderTypeBOS,
		GranulePosition: 99,
		BitstreamSerial: 7,
		PageSequence:    1,
		Segments:        []byte{5, 3},
		Payload:         []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}
	raw, err := page.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}

	parsed, err := ParsePage(raw)
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}

	if parsed.Version != page.Version || parsed.HeaderType != page.HeaderType {
		t.Fatalf("header mismatch: got version=%d type=0x%x", parsed.Version, parsed.HeaderType)
	}
	if parsed.GranulePosition != page.GranulePosition || parsed.BitstreamSerial != page.BitstreamSerial || parsed.PageSequence != page.PageSequence {
		t.Fatalf("metadata mismatch: got gp=%d serial=%d seq=%d", parsed.GranulePosition, parsed.BitstreamSerial, parsed.PageSequence)
	}
	if !bytes.Equal(parsed.Segments, page.Segments) {
		t.Fatalf("segment mismatch: got %v want %v", parsed.Segments, page.Segments)
	}
	if !bytes.Equal(parsed.Payload, page.Payload) {
		t.Fatalf("payload mismatch")
	}
}

func TestParsePageErrors(t *testing.T) {
	valid := mustSinglePageRaw(t)

	t.Run("short_header", func(t *testing.T) {
		_, err := ParsePage(valid[:10])
		if err == nil || !strings.Contains(err.Error(), "too short header") {
			t.Fatalf("expected short header error, got %v", err)
		}
	})

	t.Run("bad_capture", func(t *testing.T) {
		bad := append([]byte(nil), valid...)
		copy(bad[:4], []byte("Bad!"))
		_, err := ParsePage(bad)
		if err == nil || !strings.Contains(err.Error(), "invalid capture pattern") {
			t.Fatalf("expected invalid capture pattern error, got %v", err)
		}
	})

	t.Run("bad_version", func(t *testing.T) {
		bad := append([]byte(nil), valid...)
		bad[4] = 1
		_, err := ParsePage(bad)
		if err == nil || !strings.Contains(err.Error(), "unsupported version") {
			t.Fatalf("expected unsupported version error, got %v", err)
		}
	})

	t.Run("checksum_mismatch", func(t *testing.T) {
		bad := append([]byte(nil), valid...)
		bad[len(bad)-1] ^= 0xFF
		_, err := ParsePage(bad)
		if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
			t.Fatalf("expected checksum mismatch error, got %v", err)
		}
	})

	t.Run("truncated_segment_table", func(t *testing.T) {
		bad := append([]byte(nil), valid[:pageHeaderSize]...)
		bad[26] = 2
		_, err := ParsePage(bad)
		if err == nil || !strings.Contains(err.Error(), "truncated segment table") {
			t.Fatalf("expected truncated segment table error, got %v", err)
		}
	})

	t.Run("truncated_payload", func(t *testing.T) {
		_, err := ParsePage(valid[:len(valid)-1])
		if err == nil || !strings.Contains(err.Error(), "truncated payload") {
			t.Fatalf("expected truncated payload error, got %v", err)
		}
	})

	t.Run("trailing_data", func(t *testing.T) {
		withTrailing := append(append([]byte(nil), valid...), 0x00)
		_, err := ParsePage(withTrailing)
		if err == nil || !strings.Contains(err.Error(), "trailing data") {
			t.Fatalf("expected trailing data error, got %v", err)
		}
	})
}

func TestParsePagesRoundTrip(t *testing.T) {
	p1 := deterministicBytes(100)
	p2 := deterministicBytes(900)

	pages1, err := BuildPacketPages(9, 0, p1, 100, true, false)
	if err != nil {
		t.Fatalf("BuildPacketPages #1: %v", err)
	}
	pages2, err := BuildPacketPages(9, uint32(len(pages1)), p2, 200, false, true)
	if err != nil {
		t.Fatalf("BuildPacketPages #2: %v", err)
	}

	all := append(append([]*Page(nil), pages1...), pages2...)
	raw, err := MarshalPages(all)
	if err != nil {
		t.Fatalf("MarshalPages: %v", err)
	}

	parsed, err := ParsePages(raw)
	if err != nil {
		t.Fatalf("ParsePages: %v", err)
	}
	if len(parsed) != len(all) {
		t.Fatalf("page count mismatch: got %d want %d", len(parsed), len(all))
	}
}

func TestBuildPacketPagesBoundaries(t *testing.T) {
	lengths := []int{0, 1, 254, 255, 256, 510, 511, 65040}
	for _, n := range lengths {
		t.Run("len_"+strconv.Itoa(n), func(t *testing.T) {
			data := deterministicBytes(n)
			granule := uint64(1000 + n)
			pages, err := BuildPacketPages(77, 42, data, granule, true, true)
			if err != nil {
				t.Fatalf("BuildPacketPages(%d): %v", n, err)
			}
			if len(pages) == 0 {
				t.Fatal("expected at least one page")
			}
			if !pages[0].HasBOS() {
				t.Fatal("first page should have BOS")
			}
			if !pages[len(pages)-1].HasEOS() {
				t.Fatal("last page should have EOS")
			}
			if len(pages) > 1 && !pages[1].HasContinuation() {
				t.Fatal("second page should have continuation for multi-page packet")
			}

			raw, err := MarshalPages(pages)
			if err != nil {
				t.Fatalf("MarshalPages: %v", err)
			}
			parsed, err := ParsePages(raw)
			if err != nil {
				t.Fatalf("ParsePages: %v", err)
			}
			packets, err := ExtractPackets(parsed)
			if err != nil {
				t.Fatalf("ExtractPackets: %v", err)
			}
			if len(packets) != 1 {
				t.Fatalf("packet count=%d, want 1", len(packets))
			}
			if !bytes.Equal(packets[0].Data, data) {
				t.Fatalf("packet data mismatch, len=%d", n)
			}
			if packets[0].GranulePosition != granule {
				t.Fatalf("granule mismatch: got %d want %d", packets[0].GranulePosition, granule)
			}
			if !packets[0].BOS || !packets[0].EOS {
				t.Fatalf("flags mismatch: bos=%v eos=%v", packets[0].BOS, packets[0].EOS)
			}
		})
	}
}

func TestBuildPacketPagesSequenceOverflow(t *testing.T) {
	data := deterministicBytes(255 * 255)

	if _, err := BuildPacketPages(1, math.MaxUint32, data, 0, true, true); err == nil || !strings.Contains(err.Error(), "sequence overflow") {
		t.Fatalf("expected sequence overflow error, got %v", err)
	}

	if _, err := BuildPacketPages(1, math.MaxUint32-1, data, 0, true, true); err != nil {
		t.Fatalf("sequence max-1 should be valid, got %v", err)
	}
}

func TestExtractPacketsErrors(t *testing.T) {
	t.Run("unexpected_continuation", func(t *testing.T) {
		pages := []*Page{{
			Version:         0,
			HeaderType:      HeaderTypeContinued,
			GranulePosition: 0,
			BitstreamSerial: 1,
			PageSequence:    0,
			Segments:        []byte{1},
			Payload:         []byte{0x11},
		}}
		_, err := ExtractPackets(pages)
		if err == nil || !strings.Contains(err.Error(), "unexpected continuation") {
			t.Fatalf("expected unexpected continuation error, got %v", err)
		}
	})

	t.Run("missing_continuation", func(t *testing.T) {
		data := deterministicBytes(255 * 255)
		pages, err := BuildPacketPages(1, 0, data, 123, true, true)
		if err != nil {
			t.Fatalf("BuildPacketPages: %v", err)
		}
		if len(pages) < 2 {
			t.Fatalf("expected multi page packet, got %d", len(pages))
		}
		pages[1].HeaderType &^= HeaderTypeContinued
		_, err = ExtractPackets(pages)
		if err == nil || !strings.Contains(err.Error(), "missing continuation") {
			t.Fatalf("expected missing continuation error, got %v", err)
		}
	})

	t.Run("unterminated_packet", func(t *testing.T) {
		pages := []*Page{{
			Version:         0,
			HeaderType:      HeaderTypeBOS,
			GranulePosition: GranulePositionUnknown,
			BitstreamSerial: 1,
			PageSequence:    0,
			Segments:        []byte{255},
			Payload:         deterministicBytes(255),
		}}
		_, err := ExtractPackets(pages)
		if err == nil || !strings.Contains(err.Error(), "unterminated") {
			t.Fatalf("expected unterminated packet error, got %v", err)
		}
	})
}

func TestStreamWriterReaderRoundTrip(t *testing.T) {
	var out bytes.Buffer
	w, err := NewStreamWriter(&out, 888)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}

	p1 := deterministicBytes(64)
	p2 := deterministicBytes(66000)

	if _, err := w.WritePacket(p1, 320, false); err != nil {
		t.Fatalf("WritePacket #1: %v", err)
	}
	if _, err := w.WritePacket(p2, 640, true); err != nil {
		t.Fatalf("WritePacket #2: %v", err)
	}
	if _, err := w.WritePacket([]byte{1}, 960, false); err == nil || !strings.Contains(err.Error(), "already ended") {
		t.Fatalf("expected stream already ended error, got %v", err)
	}

	pages, err := ReadAllPages(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("ReadAllPages: %v", err)
	}
	if len(pages) < 2 {
		t.Fatalf("expected >=2 pages, got %d", len(pages))
	}
	for i, p := range pages {
		if p.PageSequence != uint32(i) {
			t.Fatalf("page sequence mismatch at %d: got %d", i, p.PageSequence)
		}
	}

	packets, err := ExtractPackets(pages)
	if err != nil {
		t.Fatalf("ExtractPackets: %v", err)
	}
	if len(packets) != 2 {
		t.Fatalf("packet count=%d, want 2", len(packets))
	}
	if !bytes.Equal(packets[0].Data, p1) || !bytes.Equal(packets[1].Data, p2) {
		t.Fatal("packet payload mismatch")
	}
	if !packets[0].BOS || packets[0].EOS {
		t.Fatalf("packet[0] flags mismatch: bos=%v eos=%v", packets[0].BOS, packets[0].EOS)
	}
	if packets[1].BOS || !packets[1].EOS {
		t.Fatalf("packet[1] flags mismatch: bos=%v eos=%v", packets[1].BOS, packets[1].EOS)
	}
}

func TestStreamWriterErrors(t *testing.T) {
	if _, err := NewStreamWriter(nil, 1); err == nil || !strings.Contains(err.Error(), "writer is nil") {
		t.Fatalf("expected writer is nil error, got %v", err)
	}

	{
		w, err := NewStreamWriter(failWriter{}, 1)
		if err != nil {
			t.Fatalf("NewStreamWriter: %v", err)
		}
		if _, err := w.WritePacket([]byte{1, 2, 3}, 1, false); err == nil || !strings.Contains(err.Error(), "write page") {
			t.Fatalf("expected write page error, got %v", err)
		}
	}

	{
		w, err := NewStreamWriter(shortWriter{}, 1)
		if err != nil {
			t.Fatalf("NewStreamWriter: %v", err)
		}
		if _, err := w.WritePacket([]byte{1, 2, 3}, 1, false); err == nil || !strings.Contains(err.Error(), "short write") {
			t.Fatalf("expected short write error, got %v", err)
		}
		if _, err := w.WritePacket([]byte{4, 5, 6}, 2, false); err == nil || !strings.Contains(err.Error(), "stream is broken") {
			t.Fatalf("expected broken stream error after short write, got %v", err)
		}
	}

	var nilWriter *StreamWriter
	if _, err := nilWriter.WritePacket([]byte{1}, 0, false); err == nil || !strings.Contains(err.Error(), "writer is nil") {
		t.Fatalf("expected nil writer receiver error, got %v", err)
	}
	if got := nilWriter.NextSequence(); got != 0 {
		t.Fatalf("nil writer NextSequence=%d, want 0", got)
	}
}

func TestStreamWriterPartialWriteFailureStateConsistency(t *testing.T) {
	packet := deterministicBytes(66000)
	pages, err := BuildPacketPages(777, 0, packet, 123, true, false)
	if err != nil {
		t.Fatalf("BuildPacketPages: %v", err)
	}
	if len(pages) < 2 {
		t.Fatalf("expected multi-page packet, got %d", len(pages))
	}

	firstRaw, err := pages[0].MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary first page: %v", err)
	}

	injected := &failAfterNWriter{remain: len(firstRaw) + 10}
	sw, err := NewStreamWriter(injected, 777)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}

	written, err := sw.WritePacket(packet, 123, false)
	if err == nil || !strings.Contains(err.Error(), "write page 1 failed") {
		t.Fatalf("expected write page 1 failed error, got %v", err)
	}
	if written != len(firstRaw)+10 {
		t.Fatalf("written=%d, want %d", written, len(firstRaw)+10)
	}
	if got := sw.NextSequence(); got != 1 {
		t.Fatalf("NextSequence()=%d, want 1 after first page persisted", got)
	}

	if _, err := sw.WritePacket([]byte{1, 2, 3}, 456, false); err == nil || !strings.Contains(err.Error(), "stream is broken") {
		t.Fatalf("expected broken stream error after partial write failure, got %v", err)
	}
}

func TestStreamReaderErrors(t *testing.T) {
	var nilReader *StreamReader
	if _, err := nilReader.NextPage(); err == nil || !strings.Contains(err.Error(), "reader is nil") {
		t.Fatalf("expected nil reader receiver error, got %v", err)
	}

	if _, err := NewStreamReader(nil).NextPage(); err == nil || !strings.Contains(err.Error(), "reader is nil") {
		t.Fatalf("expected nil reader error, got %v", err)
	}

	if _, err := NewStreamReader(bytes.NewReader([]byte("Ogg"))).NextPage(); err == nil || !strings.Contains(err.Error(), "truncated header") {
		t.Fatalf("expected truncated header error, got %v", err)
	}

	valid := mustSinglePageRaw(t)
	if _, err := NewStreamReader(bytes.NewReader(valid[:len(valid)-1])).NextPage(); err == nil || !strings.Contains(err.Error(), "truncated payload") {
		t.Fatalf("expected truncated payload error, got %v", err)
	}

	badChecksum := append([]byte(nil), valid...)
	badChecksum[len(badChecksum)-1] ^= 0xFF
	if _, err := NewStreamReader(bytes.NewReader(badChecksum)).NextPage(); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch error, got %v", err)
	}
}

func TestReadAllPackets(t *testing.T) {
	var out bytes.Buffer
	w, err := NewStreamWriter(&out, 66)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}
	if _, err := w.WritePacket(deterministicBytes(12), 1, false); err != nil {
		t.Fatalf("WritePacket #1: %v", err)
	}
	if _, err := w.WritePacket(deterministicBytes(400), 2, true); err != nil {
		t.Fatalf("WritePacket #2: %v", err)
	}

	packets, err := ReadAllPackets(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("ReadAllPackets: %v", err)
	}
	if len(packets) != 2 {
		t.Fatalf("packet count=%d, want 2", len(packets))
	}

	if _, err := ReadAllPages(io.LimitReader(bytes.NewReader(out.Bytes()), int64(out.Len()-1))); err == nil {
		t.Fatal("expected truncated read error")
	}
}

func TestPacketsSeq(t *testing.T) {
	var out bytes.Buffer
	w, err := NewStreamWriter(&out, 66)
	if err != nil {
		t.Fatalf("NewStreamWriter: %v", err)
	}
	p1 := deterministicBytes(12)
	p2 := deterministicBytes(400)
	if _, err := w.WritePacket(p1, 1, false); err != nil {
		t.Fatalf("WritePacket #1: %v", err)
	}
	if _, err := w.WritePacket(p2, 2, true); err != nil {
		t.Fatalf("WritePacket #2: %v", err)
	}

	var packets []Packet
	for packet, err := range Packets(bytes.NewReader(out.Bytes())) {
		if err != nil {
			t.Fatalf("Packets: %v", err)
		}
		packets = append(packets, packet)
	}

	if len(packets) != 2 {
		t.Fatalf("packet count=%d, want 2", len(packets))
	}
	if !bytes.Equal(packets[0].Data, p1) || !bytes.Equal(packets[1].Data, p2) {
		t.Fatal("packet payload mismatch")
	}
	if !packets[0].BOS || packets[0].EOS {
		t.Fatalf("first packet flags mismatch: bos=%v eos=%v", packets[0].BOS, packets[0].EOS)
	}
	if packets[1].BOS || !packets[1].EOS {
		t.Fatalf("second packet flags mismatch: bos=%v eos=%v", packets[1].BOS, packets[1].EOS)
	}
}

func TestPacketsSeqErrors(t *testing.T) {
	if yielded := 0; yielded != func() int {
		n := 0
		for _, err := range Packets(bytes.NewReader(nil)) {
			if err != nil {
				t.Fatalf("empty stream should not yield error, got %v", err)
			}
			n++
		}
		return n
	}() {
		t.Fatalf("empty stream yielded %d packets", yielded)
	}

	page := &Page{
		Version:         0,
		HeaderType:      HeaderTypeContinued,
		GranulePosition: 0,
		BitstreamSerial: 77,
		PageSequence:    0,
		Segments:        []byte{3},
		Payload:         []byte{1, 2, 3},
	}
	raw, err := page.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}

	gotErr := ""
	for _, err := range Packets(bytes.NewReader(raw)) {
		if err != nil {
			gotErr = err.Error()
			break
		}
	}
	if !strings.Contains(gotErr, "unexpected continuation") {
		t.Fatalf("expected unexpected continuation error, got %q", gotErr)
	}
}

func TestOpusInteropPacketPath(t *testing.T) {
	if !opus.IsRuntimeSupported() {
		t.Skip("requires native opus runtime")
	}

	enc, err := opus.NewEncoder(16000, 1, opus.ApplicationAudio)
	if err != nil {
		t.Fatalf("opus NewEncoder: %v", err)
	}
	defer func() {
		_ = enc.Close()
	}()

	dec, err := opus.NewDecoder(16000, 1)
	if err != nil {
		t.Fatalf("opus NewDecoder: %v", err)
	}
	defer func() {
		_ = dec.Close()
	}()

	pkt, err := enc.Encode(sinePCM16kMono(320), 320)
	if err != nil {
		t.Fatalf("opus Encode: %v", err)
	}
	if len(pkt) == 0 {
		t.Fatal("opus packet is empty")
	}

	pages, err := BuildPacketPages(2026, 0, pkt, 320, true, true)
	if err != nil {
		t.Fatalf("BuildPacketPages: %v", err)
	}
	raw, err := MarshalPages(pages)
	if err != nil {
		t.Fatalf("MarshalPages: %v", err)
	}

	packets, err := ReadAllPackets(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadAllPackets: %v", err)
	}
	if len(packets) != 1 {
		t.Fatalf("packet count=%d, want 1", len(packets))
	}
	if !bytes.Equal(packets[0].Data, pkt) {
		t.Fatal("ogg payload differs from opus packet")
	}
	if !packets[0].BOS || !packets[0].EOS {
		t.Fatalf("flags mismatch: bos=%v eos=%v", packets[0].BOS, packets[0].EOS)
	}

	pcm, err := dec.Decode(packets[0].Data, 320, false)
	if err != nil {
		t.Fatalf("opus Decode: %v", err)
	}
	if len(pcm) == 0 {
		t.Fatal("decoded pcm is empty")
	}
}
